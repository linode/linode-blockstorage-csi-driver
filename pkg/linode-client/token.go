package linodeclient

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// AccessTokenEnv is the environment variable holding a static Linode API token.
	AccessTokenEnv = "LINODE_TOKEN"
	// TokenFilePathEnv is the environment variable pointing at a mounted token file.
	TokenFilePathEnv = "LINODE_API_TOKEN_FILE"
	// TokenCacheTTLEnv optionally overrides the token file cache TTL in seconds.
	TokenCacheTTLEnv = "LINODE_API_TOKEN_CACHE_TTL_SECONDS"
	// DefaultTokenFilePath is the default path for a secret-mounted API token.
	DefaultTokenFilePath = "/var/run/secrets/linode/api-token"
	// DefaultTokenFileCacheTTL is how long a file-backed token is cached.
	DefaultTokenFileCacheTTL = time.Minute
)

// TokenProvider returns a Linode API token for the current request.
// Implementations may re-read a mounted secret so tokens can rotate without a restart.
type TokenProvider func(context.Context) (string, error)

// StaticTokenProvider returns a TokenProvider that always yields the given token.
func StaticTokenProvider(token string) TokenProvider {
	return staticTokenProvider{token: token}.GetToken
}

type staticTokenProvider struct {
	token string
}

func (t staticTokenProvider) GetToken(context.Context) (string, error) {
	if t.token == "" {
		return "", fmt.Errorf("%s must be set in the environment (use a k8s secret)", AccessTokenEnv)
	}
	return t.token, nil
}

// TokenFileProvider reads a token from a file with a short TTL cache so secret
// updates are picked up without restarting the process.
type TokenFileProvider struct {
	path     string
	now      func() time.Time
	cacheTTL time.Duration

	mu          sync.RWMutex
	cachedToken string
	expiresAt   time.Time
}

// NewTokenFileProvider constructs a file-backed token provider.
func NewTokenFileProvider(path string, cacheTTL time.Duration) *TokenFileProvider {
	return &TokenFileProvider{
		path:     path,
		cacheTTL: cacheTTL,
	}
}

func (t *TokenFileProvider) String() string {
	return t.path
}

func (t *TokenFileProvider) nowTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// GetToken returns a cached token when still valid, otherwise re-reads the file.
func (t *TokenFileProvider) GetToken(_ context.Context) (string, error) {
	now := t.nowTime()
	cacheTTL := t.cacheTTL
	if cacheTTL <= 0 {
		cacheTTL = DefaultTokenFileCacheTTL
	}

	t.mu.RLock()
	if t.cachedToken != "" && now.Before(t.expiresAt) {
		token := t.cachedToken
		t.mu.RUnlock()
		return token, nil
	}
	t.mu.RUnlock()

	rawToken, err := os.ReadFile(t.path)
	if err != nil {
		return "", fmt.Errorf("failed to read token file %q: %w", t.String(), err)
	}

	token := strings.TrimSpace(string(rawToken))
	if token == "" {
		return "", fmt.Errorf("token file %q is empty", t.String())
	}

	t.mu.Lock()
	t.cachedToken = token
	t.expiresAt = t.nowTime().Add(cacheTTL)
	t.mu.Unlock()

	return token, nil
}

// TokenFileCacheTTLFromEnv returns the configured cache TTL or the default.
func TokenFileCacheTTLFromEnv() time.Duration {
	tokenCacheTTL := DefaultTokenFileCacheTTL
	if raw, ok := os.LookupEnv(TokenCacheTTLEnv); ok {
		if ttlSeconds, err := strconv.Atoi(raw); err == nil && ttlSeconds > 0 {
			tokenCacheTTL = time.Duration(ttlSeconds) * time.Second
		}
	}
	return tokenCacheTTL
}

// TokenProviderFromFileOrEnv prefers a mounted token file (when readable), then
// falls back to LINODE_TOKEN. Returns an error when neither source yields a token.
// The second return value describes the chosen source for logging.
func TokenProviderFromFileOrEnv(ctx context.Context) (TokenProvider, string, error) {
	tokenFilePath := strings.TrimSpace(os.Getenv(TokenFilePathEnv))
	if tokenFilePath == "" {
		tokenFilePath = DefaultTokenFilePath
	}

	fileProvider := NewTokenFileProvider(tokenFilePath, TokenFileCacheTTLFromEnv())
	_, fileErr := fileProvider.GetToken(ctx)
	if fileErr == nil {
		return fileProvider.GetToken, fmt.Sprintf("file %q", fileProvider.String()), nil
	}

	if envToken := strings.TrimSpace(os.Getenv(AccessTokenEnv)); envToken != "" {
		return StaticTokenProvider(envToken), fmt.Sprintf("environment variable %q", AccessTokenEnv), nil
	}

	return nil, "", fmt.Errorf("failed to load linode api token from %s=%q: %w; fallback %s is not set",
		TokenFilePathEnv, tokenFilePath, fileErr, AccessTokenEnv)
}
