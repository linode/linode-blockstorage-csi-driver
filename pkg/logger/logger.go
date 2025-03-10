package logger

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

// NewLogger creates a new Logger instance with a klogr logger.
func NewLogger(ctx context.Context) (logr.Logger, context.Context) {
	log := klog.NewKlogr()
	return log, context.WithValue(ctx, logr.Logger{}, log)
}

// WithMethod returns a new Logger with method and traceID values,
// a context containing the new Logger, and a function to log method completion.
func WithMethod(log logr.Logger, method string) (logger logr.Logger, completionFunc func()) {
	traceID := uuid.New().String()

	logger = log.WithValues("method", method, "traceID", traceID)
	completionFunc = func() {
		logger.V(4).Info("Method completed")
	}
	return
}

// GetLogger retrieves the Logger from the context, or creates a new one if not present.
func GetLogger(ctx context.Context) (logr.Logger, context.Context) {
	if logger, ok := ctx.Value(logr.Logger{}).(logr.Logger); ok {
		return logger, ctx
	}
	return NewLogger(ctx)
}

func LogGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	logger, ctx := GetLogger(ctx)
	logger.V(3).Info("GRPC call", "method", info.FullMethod)
	logger.V(5).Info("GRPC request", "request", req)
	resp, err := handler(ctx, req)
	if err != nil {
		logger.Error(err, "GRPC error")
	} else {
		logger.V(5).Info("GRPC response", "response", resp)
	}
	return resp, err
}
