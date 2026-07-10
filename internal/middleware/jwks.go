package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
)

// JWKSFetcher fetches and caches JWKS from an Authentik endpoint.
type JWKSFetcher struct {
	cache    *jwk.Cache
	cacheTTL time.Duration
	url      string
	initOnce sync.Once
	initErr  error
}

// NewJWKSFetcher creates a JWKS fetcher that caches keys with the given TTL.
// The cache starts background refresh on first use.
func NewJWKSFetcher(jwksURL string, cacheTTL time.Duration) *JWKSFetcher {
	return &JWKSFetcher{
		url:      jwksURL,
		cacheTTL: cacheTTL,
	}
}

// Init starts the background JWKS cache. Safe to call multiple times.
func (f *JWKSFetcher) Init(ctx context.Context) error {
	f.initOnce.Do(func() {
		cache, err := jwk.NewCache(ctx, httprc.NewClient())
		if err != nil {
			f.initErr = fmt.Errorf("jwks: create cache: %w", err)
			return
		}
		if err := cache.Register(ctx, f.url, jwk.WithMinInterval(f.cacheTTL)); err != nil {
			f.initErr = fmt.Errorf("jwks: register %s: %w", f.url, err)
			return
		}
		// Pre-fetch to fail fast if the endpoint is unreachable.
		if _, err := cache.Lookup(ctx, f.url); err != nil {
			slog.Warn("jwks: initial fetch failed, will retry in background", "url", f.url, "error", err)
		}
		f.cache = cache
	})
	return f.initErr
}

// KeySet returns the current JWK set from cache.
func (f *JWKSFetcher) KeySet(ctx context.Context) (jwk.Set, error) {
	if f.cache == nil {
		return nil, fmt.Errorf("jwks: not initialized")
	}
	return f.cache.Lookup(ctx, f.url)
}