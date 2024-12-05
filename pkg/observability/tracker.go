package observability

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	tracer "go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

// Constants for tracing status types
const (
	TracingError       = "error"
	TracingSuccess     = "success"
	TracingSubfunction = "subfunction"
)

// Global tracing variables
var (
	Tracer            tracer.Tracer
	TracerProvider    *trace.TracerProvider
	SkipObservability bool
)

// InitOtelTracing initializes the OpenTelemetry tracing and returns an exporter.
func InitOtelTracing(ctx context.Context, serviceName, serviceVersion, tracingPort string) {
	oltpEndpoint := fmt.Sprintf("otel-collector:%s", tracingPort)

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(oltpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		klog.Errorf("Failed to create OTLP HTTP exporter: %v", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
		),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		klog.Errorf("Failed to create resource: %v", err)
	}

	TracerProvider = trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(TracerProvider)

	klog.Infof("OpenTelemetry tracing initialized for service: %s, version: %s, port: %s", serviceName, serviceVersion, tracingPort)
}

// InitTracer initializes the global tracer.
func InitTracer(ctx context.Context, serviceName, serviceVersion, tracingPort string) {
	// Initialize the OTLP exporter and TracerProvider
	InitOtelTracing(ctx, serviceName, serviceVersion, tracingPort)

	// Set the global tracer
	Tracer = otel.Tracer(serviceName)
	klog.Infof("Tracing initialized successfully for service: %s, version: %s, port: %s", serviceName, serviceVersion, tracingPort)
}

//nolint:spancheck // Intentional: span.End() is called outside this function.
func CreateSpan(ctx context.Context, operationName string) (context.Context, tracer.Span) {
	ctx, span := Tracer.Start(ctx, operationName)
	return ctx, span
}

// TraceFunctionData handles tracing for success, error, or subfunction calls.
func TraceFunctionData(span tracer.Span, operationName string, params map[string]string, status string, err error) {
	// Add attributes to the span
	for key, value := range params {
		span.SetAttributes(attribute.String(key, value))
	}

	// Record error, success, or subfunction call based on the status
	switch status {
	case TracingError:
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			klog.Errorf("Error in operation %s: %v. Params: %v", operationName, err, params)
		}
	case TracingSuccess:
		span.SetStatus(codes.Ok, "operation successful")
		klog.Infof("Operation %s succeeded. Params: %v", operationName, params)
	case TracingSubfunction:
		span.SetStatus(codes.Ok, "Sub-function call successful")
		klog.Infof("Sub-function call in operation %s succeeded. Params: %v", operationName, params)
	default:
		klog.Warningf("Unknown status: %s for operation %s", status, operationName)
	}
	span.End()
}

// SerializeRequest serializes an object to a JSON string for logging or processing.
func SerializeRequest(req interface{}) string {
	objBody, err := json.Marshal(req)
	if err != nil {
		klog.ErrorS(err, "Failed to serialize struct to a string")
		return fmt.Sprintf("serialization error: %v", err)
	}
	return string(objBody)
}
