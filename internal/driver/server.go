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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
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
	// Setter to set the http server config
	SetMetricsConfig(enableMetrics, metricsPort string)
	SetTracingConfig(enableTracing string, tracingPort string)
}

// Variables for open telemetry setup
var tracerProvider *trace.TracerProvider

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

	// fields to set up tracingServer
	enableTracing string
	tracingPort   string
}

// SetMetricsConfig sets the enableMetrics and metricsPort fields from environment variables
func (s *nonBlockingGRPCServer) SetMetricsConfig(enableMetrics, metricsPort string) {
	s.enableMetrics = enableMetrics
	s.metricsPort = metricsPort
}

// SetTracingConfig sets the enableTracing and tracingPort fields from environment variables
func (s *nonBlockingGRPCServer) SetTracingConfig(enableTracing, tracingPort string) {
	s.enableTracing = enableTracing
	s.tracingPort = tracingPort
}

func InitOtelTracing() (*otlptrace.Exporter, error) {
	// Setup OTLP exporter
	ctx := context.Background()
	oltpEndpoint := "otel-collector:4318"
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(oltpEndpoint),
		otlptracehttp.WithInsecure(), // Use WithInsecure() if the endpoint does not use TLS
	)
	if err != nil {
		klog.ErrorS(err, "Failed to create the exported resource")
	}

	// Resource will autopopulate spans with common attributes
	res, err := resource.New(ctx,
		resource.WithFromEnv(), // pull attributes from OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME environment variables
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		klog.ErrorS(err, "Failed to create the OTLP resource, spans will lack some metadata")
	}

	// Create a trace provider with the exporter.
	// Use propagator and sampler defined in environment variables.
	traceProvider := trace.NewTracerProvider(trace.WithBatcher(exporter), trace.WithResource(res))

	// Register the trace provider as global.
	otel.SetTracerProvider(traceProvider)

	return exporter, nil
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
	klog.Infof("Enable metrics: %v", enableMetrics)

	// Start metrics server if enableMetrics is true
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
		klog.Errorf("Failed to stop metrics server: %v", err)
	}

	if tracerProvider != nil {
		err := tracerProvider.Shutdown(context.Background())
		if err != nil {
			klog.Errorf("Failed to shut down tracer provider: %v", err)
		}
	}
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
	if err := s.metricsServer.Close(); err != nil {
		klog.Errorf("Failed to force stop metrics server: %v", err)
	}
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	// Create otel gRPC ServerHandler
	serverHandler := otelgrpc.NewServerHandler()

	opts := []grpc.ServerOption{
		grpc.StatsHandler(serverHandler), // Stats handler for otel
		grpc.ChainUnaryInterceptor(
			logger.LogGRPC, // Existing logging interceptor
		),
	}

	enableTracing, err := strconv.ParseBool(s.enableTracing)
	if err != nil {
		klog.Errorf("Error parsing enableTracing: %v", err)
	}
	if enableTracing {
		exporter, exporterError := InitOtelTracing()
		if exporterError != nil {
			klog.Fatalf("Failed to initialize otel tracing: %v", err)
		}

		// Exporter will flush traces on shutdown
		defer func() {
			if exporterError = exporter.Shutdown(context.Background()); exporterError != nil {
				klog.Errorf("Could not shutdown otel exporter: %v", exporterError)
			}
		}()
		opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))
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

	klog.Infof("Port %v", addr)

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
