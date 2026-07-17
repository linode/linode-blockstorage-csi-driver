package linodeclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
		wantErr bool
	}{
		{
			name: "Valid input without custom API URL",
			args: args{
				token:  "test-token",
				ua:     "test-user-agent",
				apiURL: "",
			},
			wantErr: false,
		},
		{
			name: "Valid input with custom API URL",
			args: args{
				token:  "test-token",
				ua:     "test-user-agent",
				apiURL: "https://api.linode.com/v4",
			},
			wantErr: false,
		},
		{
			name: "Invalid API URL",
			args: args{
				token:  "test-token",
				ua:     "test-user-agent",
				apiURL: "://invalid-url",
			},
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
			}
		})
	}
}

func TestNewLinodeClientWithTokenProviderNil(t *testing.T) {
	_, err := NewLinodeClientWithTokenProvider("ua", "", nil)
	if err == nil {
		t.Fatal("expected error for nil token provider")
	}
}

// TestTokenTransportAuthorization builds the real shipped client with a
// TokenProvider, issues requests through linodego against a local server, and
// asserts Authorization is set per-request from the provider (including after
// the provider returns a different token).
func TestTokenTransportAuthorization(t *testing.T) {
	var gotAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[],"page":1,"pages":1,"results":0}`)
	}))
	t.Cleanup(server.Close)

	var token atomic.Value
	token.Store("token-one")
	provider := TokenProvider(func(context.Context) (string, error) {
		v := token.Load()
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("unexpected token type %T", v)
		}
		return s, nil
	})

	client, err := NewLinodeClientWithTokenProvider("test-ua", server.URL+"/v4", provider)
	if err != nil {
		t.Fatalf("NewLinodeClientWithTokenProvider: %v", err)
	}
	// linodego uses `for range retryCount` attempts; keep a small positive count.
	client.SetRetryCount(1)

	if _, err := client.ListVolumes(t.Context(), nil); err != nil {
		t.Fatalf("ListVolumes with token-one: %v", err)
	}

	token.Store("token-two")
	if _, err := client.ListVolumes(t.Context(), nil); err != nil {
		t.Fatalf("ListVolumes with token-two: %v", err)
	}

	if len(gotAuth) < 2 {
		t.Fatalf("expected at least 2 requests with Authorization, got %v", gotAuth)
	}
	if gotAuth[0] != "Bearer token-one" {
		t.Fatalf("first Authorization = %q, want %q", gotAuth[0], "Bearer token-one")
	}
	last := gotAuth[len(gotAuth)-1]
	if last != "Bearer token-two" {
		t.Fatalf("last Authorization = %q, want %q", last, "Bearer token-two")
	}
}

func TestTokenTransportUsesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be reached when token provider fails")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	provider := TokenProvider(func(context.Context) (string, error) {
		return "", errors.New("token missing")
	})
	client, err := NewLinodeClientWithTokenProvider("test-ua", server.URL+"/v4", provider)
	if err != nil {
		t.Fatalf("NewLinodeClientWithTokenProvider: %v", err)
	}
	client.SetRetryCount(1)

	_, err = client.ListVolumes(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error when token provider fails")
	}
}

func TestTokenTransportOmitsEmptyAuthorization(t *testing.T) {
	var gotAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Header.Get returns "" when unset; distinguish from "Bearer ".
		if _, ok := r.Header["Authorization"]; ok {
			gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		} else {
			gotAuth = append(gotAuth, "<missing>")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[],"page":1,"pages":1,"results":0}`)
	}))
	t.Cleanup(server.Close)

	provider := TokenProvider(func(context.Context) (string, error) {
		return "", nil
	})
	client, err := NewLinodeClientWithTokenProvider("test-ua", server.URL+"/v4", provider)
	if err != nil {
		t.Fatalf("NewLinodeClientWithTokenProvider: %v", err)
	}
	client.SetRetryCount(1)

	// Call may fail auth-wise on a real API; against our stub it should succeed.
	if _, err := client.ListVolumes(t.Context(), nil); err != nil {
		t.Fatalf("ListVolumes with empty token: %v", err)
	}
	if len(gotAuth) == 0 {
		t.Fatal("expected at least one request")
	}
	if gotAuth[0] != "<missing>" {
		t.Fatalf("Authorization = %q, want header omitted", gotAuth[0])
	}
}
