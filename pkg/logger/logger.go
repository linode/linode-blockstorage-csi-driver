package logger

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

type LoggerKey struct{}

type Logger struct {
	Klogr logr.Logger
}

// NewLogger creates a new Logger instance with a klogr logger.
func NewLogger(ctx context.Context) *Logger {
	return &Logger{
		Klogr: klog.NewKlogr(),
	}
}

// WithMethod returns a new Logger with method and traceID values,
// a context containing the new Logger, and a function to log method completion.
func (l *Logger) WithMethod(method string) (*Logger, context.Context, func()) {
	traceID := uuid.New().String()
	newLogger := &Logger{
		Klogr: klog.NewKlogr().WithValues("method", method, "traceID", traceID),
	}
	ctx := context.WithValue(context.Background(), LoggerKey{}, newLogger)

	newLogger.V(4).Info("Starting method")

	return newLogger, ctx, func() {
		newLogger.V(4).Info("Method completed")
	}
}

// V returns a logr.Logger with the specified verbosity level.
func (l *Logger) V(level int) logr.Logger {
	return l.Klogr.V(level)
}

// Error logs an error message with the specified keys and values.
func (l *Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.Klogr.Error(err, msg, keysAndValues...)
}

// GetLogger retrieves the Logger from the context, or creates a new one if not present.
func GetLogger(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerKey{}).(*Logger); ok {
		return logger
	}
	return NewLogger(ctx)
}

func LogGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	logger := GetLogger(ctx)
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
