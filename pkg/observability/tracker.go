package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	tracer "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// Global tracing variables
var (
	Tracer            tracer.Tracer
	TracerProvider    *trace.TracerProvider
	SkipObservability = true
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

// TraceFunctionData handles tracing for success, error, or subfunction calls.
func TraceFunctionData(span tracer.Span, operationName string, params map[string]string, err error) {
	// Add attributes to the span
	for key, value := range params {
		span.SetAttributes(attribute.String(key, value))
	}

	// Determine the trace type based on error
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		span.End()
		klog.Errorf("Error in operation %s: %v. Params: %v", operationName, err, params)
	} else {
		span.SetStatus(codes.Ok, "Sub-function call successful")
		span.End()
		klog.Infof("Sub-function call in operation %s succeeded. Params: %v", operationName, params)
	}
}

// SerializeObject serializes an object to a JSON string for logging or processing.
func SerializeObject(obj interface{}) string {
	objBody, err := json.Marshal(obj)
	if err != nil {
		klog.ErrorS(err, "Failed to serialize struct to a string")
		return fmt.Sprintf("serialization error: %v", err)
	}
	return string(objBody)
}

// UnaryServerInterceptorWithParams function tries to get the parameters being input into the grpc function
func UnaryServerInterceptorWithParams() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Retrieve the existing span from the context
		span := tracer.SpanFromContext(ctx)
		if span == nil {
			return handler(ctx, req) // No span, proceed normally
		}

		// Log the request parameters as attributes on the existing span
		reqData, err := json.Marshal(req)
		if err == nil {
			span.SetAttributes(attribute.String("grpc.request", string(reqData)))
		} else {
			span.SetAttributes(attribute.String("grpc.request.error", err.Error()))
		}

		// Call the actual handler to process the request
		resp, err := handler(ctx, req)

		// Log the response parameters as attributes on the existing span
		if resp != nil {
			respData, er := json.Marshal(resp)
			if er == nil {
				span.SetAttributes(attribute.String("grpc.response", string(respData)))
			} else {
				span.SetAttributes(attribute.String("grpc.response.error", er.Error()))
			}
		}

		// Log errors, if any
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "Success")
		}

		return resp, err
	}
}

// StartFunctionSpan creates a tracing span using the calling function's name
func StartFunctionSpan(ctx context.Context) (context.Context, tracer.Span) {
	// Get the name of the current function
	pc, file, line, ok := runtime.Caller(1) // Retrieve all outputs

	if !ok {
		klog.Warning("Failed to retrieve function name from runtime.Caller")
		functionName := "unknown_function"
		return Tracer.Start(ctx, functionName)
	}

	// Log the file and line number for debugging purposes
	klog.V(4).Infof("Tracing function from %s:%d", file, line)

	// Extract the function name
	functionName := runtime.FuncForPC(pc).Name()

	// Extract only the function name (removing package path)
	if idx := strings.LastIndex(functionName, "."); idx != -1 {
		functionName = functionName[idx+1:]
	}

	// Start and return the span
	return Tracer.Start(ctx, functionName)
}
