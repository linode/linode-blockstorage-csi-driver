package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

var tracer trace.Tracer

// InitTracer initializes the global tracer.
func InitTracer(serviceName string) {
	tracer = otel.Tracer(serviceName)
}

// RecordError starts a new span for error tracking, logs the error, and records attributes.
func RecordError(ctx context.Context, operationName string, err error, params map[string]string) {
	// Starting a span for the operation
	_, span := tracer.Start(ctx, operationName)
	defer span.End()

	// Add error information to span
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)

	// Set custom attributes
	for key, value := range params {
		span.SetAttributes(attribute.String(key, value))
	}

	// Log the error
	klog.Errorf("Error in operation %s: %v. Params: %v", operationName, err, params)
}

// RecordSuccess starts a span for successful operations and records custom attributes.
func RecordSuccess(ctx context.Context, operationName string, params map[string]string) {
	// Starting a span for the operation
	_, span := tracer.Start(ctx, operationName)
	defer span.End()

	// Set custom attributes
	for key, value := range params {
		span.SetAttributes(attribute.String(key, value))
	}

	// Mark the span as successful
	span.SetStatus(codes.Ok, "operation successful")

	// Log success for debugging purpose
	klog.Infof("Operation %s succeeded. Params: %v", operationName, params)
}
