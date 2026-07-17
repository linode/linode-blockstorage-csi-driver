package linodeclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTokenFile(t *testing.T, dir, token string) string {
	t.Helper()
	path := filepath.Join(dir, "api-token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestTokenFileProviderCache(t *testing.T) {
	dir := t.TempDir()
	path := writeTokenFile(t, dir, "token-v1")

	now := time.Now()
	provider := NewTokenFileProvider(path, DefaultTokenFileCacheTTL)
	provider.now = func() time.Time { return now }

	first, err := provider.GetToken(t.Context())
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if first != "token-v1" {
		t.Fatalf("first token = %q, want token-v1", first)
	}

	if err := os.WriteFile(path, []byte("token-v2"), 0o600); err != nil {
		t.Fatalf("update token file: %v", err)
	}

	cached, err := provider.GetToken(t.Context())
	if err != nil {
		t.Fatalf("GetToken cached: %v", err)
	}
	if cached != "token-v1" {
		t.Fatalf("cached token = %q, want token-v1 (within TTL)", cached)
	}

	now = now.Add(DefaultTokenFileCacheTTL + time.Second)
	refreshed, err := provider.GetToken(t.Context())
	if err != nil {
		t.Fatalf("GetToken after TTL: %v", err)
	}
	if refreshed != "token-v2" {
		t.Fatalf("refreshed token = %q, want token-v2", refreshed)
	}
}

func TestTokenFileProviderEmptyFile(t *testing.T) {
	path := writeTokenFile(t, t.TempDir(), "   \n")
	provider := NewTokenFileProvider(path, DefaultTokenFileCacheTTL)
	_, err := provider.GetToken(t.Context())
	if err == nil {
		t.Fatal("expected error for empty token file")
	}
}

func TestTokenFileProviderMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-token")
	provider := NewTokenFileProvider(path, DefaultTokenFileCacheTTL)
	_, err := provider.GetToken(t.Context())
	if err == nil {
		t.Fatal("expected error for missing token file")
	}
}

func TestTokenFileCacheTTLFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(TokenCacheTTLEnv, "")
		if got := TokenFileCacheTTLFromEnv(); got != DefaultTokenFileCacheTTL {
			t.Fatalf("got %v, want default %v", got, DefaultTokenFileCacheTTL)
		}
	})
	t.Run("configured", func(t *testing.T) {
		t.Setenv(TokenCacheTTLEnv, "7")
		if got := TokenFileCacheTTLFromEnv(); got != 7*time.Second {
			t.Fatalf("got %v, want 7s", got)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		t.Setenv(TokenCacheTTLEnv, "invalid")
		if got := TokenFileCacheTTLFromEnv(); got != DefaultTokenFileCacheTTL {
			t.Fatalf("got %v, want default", got)
		}
	})
	t.Run("non-positive", func(t *testing.T) {
		t.Setenv(TokenCacheTTLEnv, "0")
		if got := TokenFileCacheTTLFromEnv(); got != DefaultTokenFileCacheTTL {
			t.Fatalf("got %v, want default", got)
		}
	})
}

func TestTokenProviderFromFileOrEnv(t *testing.T) {
	t.Run("uses file when available", func(t *testing.T) {
		t.Setenv(AccessTokenEnv, "env-token")
		path := writeTokenFile(t, t.TempDir(), "file-token")
		t.Setenv(TokenFilePathEnv, path)

		provider, source, err := TokenProviderFromFileOrEnv(t.Context(), "static-token")
		if err != nil {
			t.Fatalf("TokenProviderFromFileOrEnv: %v", err)
		}
		wantSource := `file "` + path + `"`
		if source != wantSource {
			t.Fatalf("source = %q, want %q", source, wantSource)
		}
		token, err := provider(t.Context())
		if err != nil {
			t.Fatalf("provider: %v", err)
		}
		if token != "file-token" {
			t.Fatalf("token = %q, want file-token", token)
		}
	})

	t.Run("falls back to static token when file missing", func(t *testing.T) {
		t.Setenv(AccessTokenEnv, "")
		t.Setenv(TokenFilePathEnv, filepath.Join(t.TempDir(), "missing-token-file"))

		provider, source, err := TokenProviderFromFileOrEnv(t.Context(), "static-token")
		if err != nil {
			t.Fatalf("TokenProviderFromFileOrEnv: %v", err)
		}
		wantSource := `flag or environment variable "LINODE_TOKEN"`
		if source != wantSource {
			t.Fatalf("source = %q, want %q", source, wantSource)
		}
		token, err := provider(t.Context())
		if err != nil {
			t.Fatalf("provider: %v", err)
		}
		if token != "static-token" {
			t.Fatalf("token = %q, want static-token", token)
		}
	})

	t.Run("falls back to env when file and static missing", func(t *testing.T) {
		t.Setenv(AccessTokenEnv, "env-token")
		t.Setenv(TokenFilePathEnv, filepath.Join(t.TempDir(), "missing-token-file"))

		provider, source, err := TokenProviderFromFileOrEnv(t.Context(), "")
		if err != nil {
			t.Fatalf("TokenProviderFromFileOrEnv: %v", err)
		}
		wantSource := `environment variable "LINODE_TOKEN"`
		if source != wantSource {
			t.Fatalf("source = %q, want %q", source, wantSource)
		}
		token, err := provider(t.Context())
		if err != nil {
			t.Fatalf("provider: %v", err)
		}
		if token != "env-token" {
			t.Fatalf("token = %q, want env-token", token)
		}
	})

	t.Run("errors when all unavailable", func(t *testing.T) {
		t.Setenv(AccessTokenEnv, "")
		t.Setenv(TokenFilePathEnv, filepath.Join(t.TempDir(), "missing-token-file"))

		_, _, err := TokenProviderFromFileOrEnv(t.Context(), "")
		if err == nil {
			t.Fatal("expected error when file, static, and env unavailable")
		}
	})
}

func TestStaticTokenProvider(t *testing.T) {
	token, err := StaticTokenProvider("static")(t.Context())
	if err != nil {
		t.Fatalf("StaticTokenProvider: %v", err)
	}
	if token != "static" {
		t.Fatalf("token = %q, want static", token)
	}
	_, err = StaticTokenProvider("")(t.Context())
	if err == nil {
		t.Fatal("expected error for empty static token")
	}
}
