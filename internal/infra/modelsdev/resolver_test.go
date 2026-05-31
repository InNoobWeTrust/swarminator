package modelsdev

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolverResolveSupportsExplicitAndDerivedInputLimits(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"openai": {"models": {"gpt-5-mini": {"id": "gpt-5-mini", "limit": {"context": 1050000, "input": 922000, "output": 128000}}}},
			"google": {"models": {"gemini-2.5-flash": {"id": "gemini-2.5-flash", "limit": {"context": 1000000, "output": 64000}}}}
		}`)
	}))
	defer server.Close()

	resolver := NewResolver(Options{
		URL:        server.URL,
		CacheDir:   t.TempDir(),
		TTL:        time.Hour,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Unix(1716552000, 0).UTC() },
	})

	openaiBudget, err := resolver.Resolve(context.Background(), "openai/gpt-5-mini")
	if err != nil {
		t.Fatalf("Resolve(openai) error = %v", err)
	}
	if openaiBudget.ContextWindow != 1050000 || openaiBudget.MaxInputTokens != 922000 || openaiBudget.MaxOutputTokens != 128000 {
		t.Fatalf("openai budget = %#v, want explicit input/output limits", openaiBudget)
	}

	googleBudget, err := resolver.Resolve(context.Background(), "google/gemini-2.5-flash")
	if err != nil {
		t.Fatalf("Resolve(google) error = %v", err)
	}
	if googleBudget.MaxInputTokens != 1000000 {
		t.Fatalf("google MaxInputTokens = %d, want fallback to context window", googleBudget.MaxInputTokens)
	}
	if googleBudget.MaxOutputTokens != 64000 {
		t.Fatalf("google MaxOutputTokens = %d, want 64000", googleBudget.MaxOutputTokens)
	}
}

func TestResolverResolveUsesStaleCacheWhenRefreshFails(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"anthropic": {"models": {"claude-sonnet-4.6": {"id": "claude-sonnet-4.6", "limit": {"context": 200000, "output": 32000}}}}}`)
	}))

	resolver := NewResolver(Options{
		URL:        live.URL,
		CacheDir:   cacheDir,
		TTL:        time.Hour,
		HTTPClient: live.Client(),
		Now:        func() time.Time { return time.Unix(1716552000, 0).UTC() },
	})
	if _, err := resolver.Resolve(context.Background(), "anthropic/claude-sonnet-4.6"); err != nil {
		t.Fatalf("Resolve() warm cache error = %v", err)
	}
	live.Close()

	staleResolver := NewResolver(Options{
		URL:      live.URL,
		CacheDir: cacheDir,
		TTL:      time.Nanosecond,
		Now:      func() time.Time { return time.Unix(1716555600, 0).UTC() },
	})
	got, err := staleResolver.Resolve(context.Background(), "anthropic/claude-sonnet-4.6")
	if err != nil {
		t.Fatalf("Resolve() stale cache error = %v", err)
	}
	if got.ContextWindow != 200000 || got.MaxOutputTokens != 32000 {
		t.Fatalf("stale cache budget = %#v, want anthropic limits", got)
	}
}

func TestResolverResolveFailsClosedWhenModelMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"openai": {"models": {}}}`)
	}))
	defer server.Close()

	resolver := NewResolver(Options{URL: server.URL, CacheDir: t.TempDir(), TTL: time.Hour, HTTPClient: server.Client()})
	if _, err := resolver.Resolve(context.Background(), "openai/gpt-4o"); err == nil {
		t.Fatal("Resolve() error = nil, want missing model failure")
	}
}
