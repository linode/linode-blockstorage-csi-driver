package driver

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

func NewLogger(ctx context.Context) *Logger {
	return &Logger{
		klogr: klog.NewKlogr(),
	}
}

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

func (l *Logger) V(level int) logr.Logger {
	return l.klogr.V(level)
}

func (l *Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.klogr.Error(err, msg, keysAndValues...)
}

func GetLogger(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerKey{}).(*Logger); ok {
		return logger
	}
	return NewLogger(ctx)
}
