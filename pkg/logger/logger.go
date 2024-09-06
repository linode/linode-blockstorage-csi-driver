package logger

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

type LoggerKey struct{}

type Logger struct {
	klogr logr.Logger
}

// NewLogger creates a new Logger instance with a klogr logger.
func NewLogger(ctx context.Context) *Logger {
	return &Logger{
		klogr: klog.NewKlogr(),
	}
}

// WithMethod returns a new Logger with method and traceID values,
// a context containing the new Logger, and a function to log method completion.
func (l *Logger) WithMethod(method string) (*Logger, context.Context, func()) {
	traceID := uuid.New().String()
	newLogger := &Logger{
		klogr: klog.NewKlogr().WithValues("method", method, "traceID", traceID),
	}
	ctx := context.WithValue(context.Background(), LoggerKey{}, newLogger)

	newLogger.V(4).Info("Starting method")

	return newLogger, ctx, func() {
		newLogger.V(4).Info("Method completed")
	}
}

// V returns a logr.Logger with the specified verbosity level.
func (l *Logger) V(level int) logr.Logger {
	return l.klogr.V(level)
}

// Error logs an error message with the specified keys and values.
func (l *Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.klogr.Error(err, msg, keysAndValues...)
}

// GetLogger retrieves the Logger from the context, or creates a new one if not present.
func GetLogger(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerKey{}).(*Logger); ok {
		return logger
	}
	return NewLogger(ctx)
}
