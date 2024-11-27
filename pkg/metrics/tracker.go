package metrics

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
)

// Constants for tracing status types
const (
	TracingError       = "error"
	TracingSuccess     = "success"
	TracingSubfunction = "subfunction"
)

var Tracer trace.Tracer

// InitTracer initializes the global tracer.
func InitTracer(serviceName string) {
	Tracer = otel.Tracer(serviceName)
}

// TraceFunctionData handles tracing for success, error, or subfunction calls.
func TraceFunctionData(ctx context.Context, operationName string, params map[string]string, status string, err error) {
	// Create a child span for the operation
	_, span := Tracer.Start(ctx, operationName)
	defer span.End()

	// Set attributes
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
		klog.Warningf("Unknown status type: %s. Operation: %s", status, operationName)
	}
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
