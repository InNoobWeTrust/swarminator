package modelsdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"swarminator/internal/domain/swarmrun"
)

const defaultURL = "https://models.dev/api.json"

type Options struct {
	URL        string
	CacheDir   string
	TTL        time.Duration
	HTTPClient *http.Client
	Now        func() time.Time
}

type Resolver struct {
	url        string
	cacheDir   string
	ttl        time.Duration
	httpClient *http.Client
	now        func() time.Time
}

type cacheMeta struct {
	FetchedAt time.Time `json:"fetched_at"`
}

type providerBlob struct {
	Models map[string]modelBlob `json:"models"`
}

type modelBlob struct {
	ID    string    `json:"id"`
	Limit limitBlob `json:"limit"`
}

type limitBlob struct {
	Context int `json:"context"`
	Input   int `json:"input"`
	Output  int `json:"output"`
}

func NewResolver(options Options) *Resolver {
	url := strings.TrimSpace(options.URL)
	if url == "" {
		url = defaultURL
	}
	cacheDir := strings.TrimSpace(options.CacheDir)
	if cacheDir == "" {
		cacheDir = defaultCacheDir()
	}
	ttl := options.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Resolver{url: url, cacheDir: cacheDir, ttl: ttl, httpClient: httpClient, now: now}
}

func (r *Resolver) Resolve(ctx context.Context, budgetRef string) (swarmrun.TokenBudget, error) {
	namespace, modelID, err := parseBudgetRef(budgetRef)
	if err != nil {
		return swarmrun.TokenBudget{}, err
	}

	blob, err := r.loadFreshOrFetch(ctx)
	if err != nil {
		cachedBlob, cacheErr := r.readCacheBlob()
		if cacheErr != nil {
			return swarmrun.TokenBudget{}, err
		}
		return lookupBudget(cachedBlob, namespace, modelID)
	}
	return lookupBudget(blob, namespace, modelID)
}

func parseBudgetRef(budgetRef string) (string, string, error) {
	trimmed := strings.TrimSpace(budgetRef)
	idx := strings.IndexByte(trimmed, '/')
	if idx <= 0 || idx == len(trimmed)-1 {
		return "", "", fmt.Errorf("budget_ref %q must contain namespace/model", budgetRef)
	}
	return trimmed[:idx], trimmed[idx+1:], nil
}

func lookupBudget(blob map[string]providerBlob, namespace, modelID string) (swarmrun.TokenBudget, error) {
	provider, ok := blob[namespace]
	if !ok {
		return swarmrun.TokenBudget{}, fmt.Errorf("budget namespace %q was not found in models.dev metadata", namespace)
	}
	model, ok := provider.Models[modelID]
	if !ok {
		return swarmrun.TokenBudget{}, fmt.Errorf("budget model %q/%q was not found in models.dev metadata", namespace, modelID)
	}
	if model.Limit.Context <= 0 || model.Limit.Output <= 0 {
		return swarmrun.TokenBudget{}, fmt.Errorf("budget model %q/%q is missing required limit fields", namespace, modelID)
	}
	maxInput := model.Limit.Input
	if maxInput <= 0 {
		maxInput = model.Limit.Context
	}
	return swarmrun.TokenBudget{ContextWindow: model.Limit.Context, MaxInputTokens: maxInput, MaxOutputTokens: model.Limit.Output}, nil
}

func (r *Resolver) loadFreshOrFetch(ctx context.Context) (map[string]providerBlob, error) {
	meta, metaErr := r.readCacheMeta()
	if metaErr == nil && r.now().UTC().Sub(meta.FetchedAt) <= r.ttl {
		return r.readCacheBlob()
	}
	return r.fetchAndCache(ctx)
}

func (r *Resolver) fetchAndCache(ctx context.Context) (map[string]providerBlob, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create models.dev request: %w", err)
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch models.dev metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models.dev metadata: status %d", response.StatusCode)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read models.dev metadata: %w", err)
	}
	blob, err := parseBlob(payload)
	if err != nil {
		return nil, err
	}
	if err := r.writeCache(payload); err != nil {
		return nil, err
	}
	return blob, nil
}

func (r *Resolver) readCacheBlob() (map[string]providerBlob, error) {
	payload, err := os.ReadFile(r.cacheBlobPath())
	if err != nil {
		return nil, fmt.Errorf("read cached models.dev metadata: %w", err)
	}
	return parseBlob(payload)
}

func (r *Resolver) readCacheMeta() (cacheMeta, error) {
	data, err := os.ReadFile(r.cacheMetaPath())
	if err != nil {
		return cacheMeta{}, err
	}
	var meta cacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return cacheMeta{}, err
	}
	if meta.FetchedAt.IsZero() {
		return cacheMeta{}, errors.New("cache metadata is missing fetched_at")
	}
	return meta, nil
}

func (r *Resolver) writeCache(payload []byte) error {
	if err := os.MkdirAll(r.cacheDir, 0o700); err != nil {
		return fmt.Errorf("create models.dev cache dir %q: %w", r.cacheDir, err)
	}
	if err := os.WriteFile(r.cacheBlobPath(), payload, 0o600); err != nil {
		return fmt.Errorf("write models.dev cache blob: %w", err)
	}
	meta, err := json.Marshal(cacheMeta{FetchedAt: r.now().UTC()})
	if err != nil {
		return fmt.Errorf("marshal models.dev cache metadata: %w", err)
	}
	if err := os.WriteFile(r.cacheMetaPath(), meta, 0o600); err != nil {
		return fmt.Errorf("write models.dev cache metadata: %w", err)
	}
	return nil
}

func parseBlob(payload []byte) (map[string]providerBlob, error) {
	var blob map[string]providerBlob
	if err := json.Unmarshal(payload, &blob); err != nil {
		return nil, fmt.Errorf("parse models.dev metadata: %w", err)
	}
	return blob, nil
}

func (r *Resolver) cacheBlobPath() string {
	return filepath.Join(r.cacheDir, "models.dev-api.json")
}

func (r *Resolver) cacheMetaPath() string {
	return filepath.Join(r.cacheDir, "models.dev-meta.json")
}

func defaultCacheDir() string {
	cacheHome := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if cacheHome == "" {
		if userCacheDir, err := os.UserCacheDir(); err == nil {
			cacheHome = userCacheDir
		}
	}
	if cacheHome == "" {
		cacheHome = os.TempDir()
	}
	return filepath.Join(cacheHome, "swarminator")
}
