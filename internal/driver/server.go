/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
	"github.com/linode/linode-blockstorage-csi-driver/pkg/observability"
)

// Defines Non blocking GRPC server interfaces
type NonBlockingGRPCServer interface {
	// Start services at the endpoint
	Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer)
	// Waits for the service to stop
	Wait()
	// Stops the service gracefully
	Stop()
	// Stops the service forcefully
	ForceStop()
	// Setter to set the observability http server config
	SetMetricsConfig(enableMetrics, metricsPort string)
}

func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	wg            sync.WaitGroup
	server        *grpc.Server
	metricsServer *http.Server

	// fields to set up metricsServer
	enableMetrics string
	metricsPort   string
}

// SetMetricsConfig sets the enableMetrics and metricsPort fields from environment variables
func (s *nonBlockingGRPCServer) SetMetricsConfig(enableMetrics, metricsPort string) {
	s.enableMetrics = enableMetrics
	s.metricsPort = metricsPort
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	s.wg.Add(1)
	go s.serve(endpoint, ids, cs, ns)

	// Parse the enableMetrics string into a boolean
	enableMetrics, err := strconv.ParseBool(s.enableMetrics)
	if err != nil {
		klog.Errorf("Error parsing enableMetrics: %v", err)
		return
	}
	klog.Infof("Enable observability: %v", enableMetrics)

	// Start observability server if enableMetrics is true
	if enableMetrics {
		port := ":" + s.metricsPort
		go s.startMetricsServer(port)
	}
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
	err := s.metricsServer.Shutdown(context.Background())
	if err != nil {
		klog.Errorf("Failed to stop observability server: %v", err)
	}

	if observability.TracerProvider != nil {
		traceErr := observability.TracerProvider.Shutdown(context.Background())
		if traceErr != nil {
			klog.Errorf("Failed to shut down tracer provider: %v", traceErr)
		}
	}
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
	if err := s.metricsServer.Close(); err != nil {
		klog.Errorf("Failed to force stop observability server: %v", err)
	}
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	// Create otel gRPC ServerHandler
	serverHandler := otelgrpc.NewServerHandler()

	opts := []grpc.ServerOption{
		grpc.StatsHandler(serverHandler), // Stats handler for otel
		grpc.ChainUnaryInterceptor(
			logger.LogGRPC, // Existing logging interceptor
			observability.UnaryServerInterceptorWithParams(), // This gets params being passed into a grpc func
		),
	}

	urlObj, err := url.Parse(endpoint)
	if err != nil {
		klog.Fatal(err.Error())
	}

	var addr string
	switch scheme := urlObj.Scheme; scheme {
	case "unix":
		addr = urlObj.Path
		if err = os.Remove(addr); err != nil && !os.IsNotExist(err) {
			klog.Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}
	case "tcp":
		addr = urlObj.Host
	default:
		klog.Fatalf("%v endpoint scheme not supported", urlObj.Scheme)
	}

	klog.V(4).Infof("Start listening with scheme %v, addr %v", urlObj.Scheme, addr)
	// nolint: noctx // We don't need to use context here
	listener, err := net.Listen(urlObj.Scheme, addr)
	if err != nil {
		klog.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
	}

	klog.V(4).Infof("Listening for connections on address: %#v", listener.Addr())

	if err := server.Serve(listener); err != nil {
		klog.Fatalf("Failed to serve: %v", err)
	}
}

func (s *nonBlockingGRPCServer) startMetricsServer(addr string) {
	defer s.wg.Done()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	klog.Infof("Port %v", addr)

	s.metricsServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	klog.V(4).Infof("Starting observability server at %s", addr)
	if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		klog.Fatalf("Failed to serve observability: %v", err)
	}
}
