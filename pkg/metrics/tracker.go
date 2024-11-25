package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

var Tracer trace.Tracer

// InitTracer initializes the global tracer.
func InitTracer(serviceName string) {
	Tracer = otel.Tracer(serviceName)
}

// RecordError logs the error and records attributes in the existing span.
func RecordError(ctx context.Context, operationName string, err error, params map[string]string) {
	// Retrieve the current span from the context
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		// If no valid span exists, start a new one
		_, span = Tracer.Start(ctx, operationName)
		defer span.End()
	}

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

// RecordSuccess records custom attributes in the existing span.
func RecordSuccess(ctx context.Context, operationName string, params map[string]string) {
	// Retrieve the current span from the context
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		// If no valid span exists, start a new one
		_, span = Tracer.Start(ctx, operationName)
		defer span.End()
	}

	// Set custom attributes
	for key, value := range params {
		span.SetAttributes(attribute.String(key, value))
	}

	// Mark the span as successful
	span.SetStatus(codes.Ok, "operation successful")

	// Log success for debugging purposes
	klog.Infof("Operation %s succeeded. Params: %v", operationName, params)
}
