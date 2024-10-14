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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
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
}

func NewNonBlockingGRPCServer() NonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	wg            sync.WaitGroup
	server        *grpc.Server
	metricsServer *http.Server
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	s.wg.Add(1)
	go s.serve(endpoint, ids, cs, ns)
	go s.startMetricsServer(":8081")
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
	err := s.metricsServer.Shutdown(context.Background())
	if err != nil {
		klog.Errorf("failed to stop metrics server: %v", err)
	}
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
	if err := s.metricsServer.Close(); err != nil {
		klog.Errorf("failed to force stop metrics server: %v", err)
	}
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logger.LogGRPC),
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

	s.metricsServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	klog.V(4).Infof("Starting metrics server at %s", addr)
	if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		klog.Fatalf("Failed to serve metrics: %v", err)
	}
}
