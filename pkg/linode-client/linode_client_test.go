package linodeclient

import (
	"testing"

	"github.com/linode/linodego"
)

func TestNewLinodeClient(t *testing.T) {
	type args struct {
		token  string
		ua     string
		apiURL string
	}
	tests := []struct {
		name    string
		args    args
		want    *linodego.Client
		wantErr bool
	}{
		{
			name: "Valid input without custom API URL",
			args: args{
				token: "test-token",
				ua:    "test-user-agent",
				apiURL: "",
			},
			want: &linodego.Client{},
			wantErr: false,
		},
		{
			name: "Valid input with custom API URL",
			args: args{
				token: "test-token",
				ua:    "test-user-agent",
				apiURL: "https://api.linode.com/v4",
			},
			want: &linodego.Client{},
			wantErr: false,
		},
		{
			name: "Invalid API URL",
			args: args{
				token: "test-token",
				ua:    "test-user-agent",
				apiURL: "://invalid-url",
			},
			want: nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLinodeClient(tt.args.token, tt.args.ua, tt.args.apiURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLinodeClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got == nil {
				t.Errorf("NewLinodeClient() returned nil, expected non-nil")
				return
			}
		})
	}
}
