package logger_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/linode/linode-blockstorage-csi-driver/pkg/logger"
)

func TestLogGRPC(t *testing.T) {
	type args struct {
		req     interface{}
		info    *grpc.UnaryServerInfo
		handler grpc.UnaryHandler
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			name: "Successful GRPC call",
			args: args{
				req: "test request",
				info: &grpc.UnaryServerInfo{
					FullMethod: "/test.Service/TestMethod",
				},
				handler: func(ctx context.Context, req interface{}) (interface{}, error) {
					return "test response", nil
				},
			},
			want:    "test response",
			wantErr: false,
		},
		{
			name: "GRPC call with error",
			args: args{
				req: "test request",
				info: &grpc.UnaryServerInfo{
					FullMethod: "/test.Service/TestMethod",
				},
				handler: func(ctx context.Context, req interface{}) (interface{}, error) {
					return nil, status.Errorf(codes.Internal, "test error")
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := logger.LogGRPC(context.Background(), tt.args.req, tt.args.info, tt.args.handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("LogGRPC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LogGRPC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLogger_WithMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{
			name:   "WithMethod with valid input",
			method: "TestMethod",
		},
		{
			name:   "WithMethod with empty method",
			method: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, ctx := logger.NewLogger(context.Background())
			_, done := logger.WithMethod(l, tt.method)

			if ctx == nil {
				t.Error("Logger.WithMethod() returned nil context")
			}
			if done == nil {
				t.Error("Logger.WithMethod() returned nil function")
			}

			// Check if the context contains the logger
			if ctx != nil {
				contextLogger, ok := ctx.Value(logr.Logger{}).(logr.Logger)
				if !ok || contextLogger != l {
					t.Error("Logger.WithMethod() context does not contain the correct logger")
				}
			}

			// Call the returned function and check if it doesn't panic
			if done != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("Logger.WithMethod() returned function panicked: %v", r)
						}
					}()
					done()
				}()
			}
		})
	}
}
