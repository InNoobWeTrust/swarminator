package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"swarminator/internal/protocol/acp"
)

type stubProvider struct {
	response string
	err      error
	calls    int
	requests []CompletionRequest
}

func (p *stubProvider) Complete(_ context.Context, req CompletionRequest) (string, error) {
	p.calls++
	p.requests = append(p.requests, req)
	return p.response, p.err
}

type stubRegistry struct {
	agents           []AgentInfo
	detected         bool
	detectErr        error
	detectCalls      int
	getForModelCalls int
	getByNameCalls   int
	getAllAvailCalls int
}

func (r *stubRegistry) Detect() error {
	r.detectCalls++
	if r.detectErr != nil {
		return r.detectErr
	}
	r.detected = true
	return nil
}

func (r *stubRegistry) GetForModel(model string) *AgentInfo {
	r.getForModelCalls++
	if !r.detected {
		return nil
	}
	return (&AgentRegistry{agents: r.agents}).GetForModel(model)
}

func (r *stubRegistry) GetByName(name string) *AgentInfo {
	r.getByNameCalls++
	if !r.detected {
		return nil
	}
	return (&AgentRegistry{agents: r.agents}).GetByName(name)
}

func (r *stubRegistry) GetAll() []AgentInfo {
	if !r.detected {
		return nil
	}
	out := make([]AgentInfo, len(r.agents))
	copy(out, r.agents)
	return out
}

func (r *stubRegistry) ResolveRoute(model string) (RouteResult, error) {
	if !r.detected {
		return RouteResult{}, fmt.Errorf("not detected")
	}
	return (&AgentRegistry{agents: r.agents}).ResolveRoute(model)
}

func (r *stubRegistry) GetAllAvailable() []AgentInfo {
	r.getAllAvailCalls++
	if !r.detected {
		return nil
	}
	return (&AgentRegistry{agents: r.agents}).GetAllAvailable()
}

func testAgents() []AgentInfo {
	return []AgentInfo{
		{Name: "kilo", Binary: "kilo", ACPArgs: []string{"acp"}, Available: true, Authenticated: true, ModelPrefixes: []string{"kilo/", "minimax/", "openai/", "github-copilot/", "openrouter/", "o1-", "o3-", "gpt-", "codex-"}},
		{Name: "gemini", Binary: "gemini", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{"google/", "gemini/", "gemini-"}},
		{Name: "codex", Binary: "codex", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{}},
		{Name: "claude", Binary: "claude", ACPArgs: []string{"--acp"}, Available: true, Authenticated: true, ModelPrefixes: []string{"claude/", "anthropic/", "sonnet-"}},
	}
}

// testProviderFactories holds the per-agent factory functions for test double injection.
// Using a named struct prevents silent mismatches from positional overloading.
type testProviderFactories struct {
	acp    func(binary string, args ...string) Provider
	kilo   func(binary string) Provider
	codex  func(binary string) Provider
	gemini func(binary string) Provider
}

func newTestUnifiedProvider(reg unifiedRegistry, adk Provider, f testProviderFactories) *UnifiedProvider {
	return &UnifiedProvider{
		registry:          reg,
		adkFallback:       adk,
		agentProviders:    make(map[string]Provider),
		newACPProvider:    f.acp,
		newKiloProvider:   f.kilo,
		newCodexProvider:  f.codex,
		newGeminiProvider: f.gemini,
	}
}

func TestUnifiedProviderAgentSelection(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		expectedAgent string
	}{
		{name: "kilo prefix", model: "minimax/m1", expectedAgent: "kilo"},
		{name: "gemini prefix", model: "gemini-2.5-pro", expectedAgent: "gemini"},
		{name: "gpt prefix routes to kilo", model: "gpt-4.1", expectedAgent: "kilo"},
		{name: "o3 prefix routes to kilo", model: "o3-mini", expectedAgent: "kilo"},
		{name: "claude prefix", model: "anthropic/claude-3-7-sonnet", expectedAgent: "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var selectedBinary string
			var selectedArgs []string
			var kiloSelected bool
			var codexSelected bool
			var geminiSelected bool
			adk := &stubProvider{response: "adk"}
			provider := newTestUnifiedProvider(
				&AgentRegistry{agents: testAgents()},
				adk,
				testProviderFactories{
					acp: func(binary string, args ...string) Provider {
						selectedBinary = binary
						selectedArgs = append([]string(nil), args...)
						return &stubProvider{response: binary}
					},
					kilo: func(binary string) Provider {
						selectedBinary = binary
						kiloSelected = true
						return &stubProvider{response: binary}
					},
					codex: func(binary string) Provider {
						selectedBinary = binary
						codexSelected = true
						return &stubProvider{response: binary}
					},
					gemini: func(binary string) Provider {
						selectedBinary = binary
						geminiSelected = true
						return &stubProvider{response: binary}
					},
				},
			)

			resp, err := provider.Complete(context.Background(), CompletionRequest{Model: tt.model, Input: "prompt"})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if resp != tt.expectedAgent {
				t.Fatalf("Complete() response = %q, want %q", resp, tt.expectedAgent)
			}
			if selectedBinary != tt.expectedAgent {
				t.Fatalf("selected binary = %q, want %q", selectedBinary, tt.expectedAgent)
			}
			switch tt.expectedAgent {
			case "kilo":
				if !kiloSelected {
					t.Fatal("expected KiloProvider factory to be used for kilo agent")
				}
			case "codex":
				if !codexSelected {
					t.Fatal("expected CodexProvider factory to be used for codex agent")
				}
			case "gemini":
				if !geminiSelected {
					t.Fatal("expected GeminiProvider factory to be used for gemini agent")
				}
			default:
				// ACP-based agents: verify --acp args are passed
				wantArgs := []string{"--acp"}
				if len(selectedArgs) != len(wantArgs) || selectedArgs[0] != wantArgs[0] {
					t.Fatalf("selected args = %v, want %v", selectedArgs, wantArgs)
				}
			}
			if adk.calls != 0 {
				t.Fatalf("ADK fallback calls = %d, want 0", adk.calls)
			}
		})
	}
}

func TestUnifiedProviderAgentOverride(t *testing.T) {
	origRegistry := newUnifiedRegistry
	origADKProvider := newUnifiedADKProvider
	origACPProvider := newUnifiedACPProvider
	origKiloProvider := newUnifiedKiloProvider
	origCodexProvider := newUnifiedCodexProvider
	t.Cleanup(func() {
		newUnifiedRegistry = origRegistry
		newUnifiedADKProvider = origADKProvider
		newUnifiedACPProvider = origACPProvider
		newUnifiedKiloProvider = origKiloProvider
		newUnifiedCodexProvider = origCodexProvider
	})

	registry := &stubRegistry{agents: testAgents(), detected: true}
	adk := &stubProvider{response: "adk"}
	var selectedBinary string
	var selectedArgs []string

	newUnifiedRegistry = func() unifiedRegistry { return registry }
	newUnifiedADKProvider = func(model, projectID string) Provider { return adk }
	newUnifiedACPProvider = func(binary string, args ...string) Provider {
		selectedBinary = binary
		selectedArgs = append([]string(nil), args...)
		return &stubProvider{response: binary}
	}

	provider := NewUnifiedProvider("project-id", "claude")
	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gpt-4.1", Input: "prompt"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp != "claude" {
		t.Fatalf("Complete() response = %q, want %q", resp, "claude")
	}
	if selectedBinary != "claude" {
		t.Fatalf("selected binary = %q, want %q", selectedBinary, "claude")
	}
	if len(selectedArgs) != 1 || selectedArgs[0] != "--acp" {
		t.Fatalf("selected args = %v, want [--acp]", selectedArgs)
	}
	if registry.getByNameCalls == 0 {
		t.Fatal("expected GetByName to be used for agent override")
	}
	if registry.getForModelCalls != 0 {
		t.Fatalf("GetForModel calls = %d, want 0", registry.getForModelCalls)
	}
	if adk.calls != 0 {
		t.Fatalf("ADK fallback calls = %d, want 0", adk.calls)
	}
}

func TestUnifiedProviderCodexExplicitOverride(t *testing.T) {
	// Verify that --agent=codex with a GPT model still routes through CodexProvider.
	var codexBinaryUsed string
	var codexSelected bool

	registry := &AgentRegistry{agents: testAgents()}
	adk := &stubProvider{response: "adk"}

	provider := newTestUnifiedProvider(registry, adk, testProviderFactories{
		acp: func(binary string, args ...string) Provider {
			return &stubProvider{response: binary}
		},
		kilo: func(binary string) Provider {
			return &stubProvider{response: binary}
		},
		codex: func(binary string) Provider {
			codexBinaryUsed = binary
			codexSelected = true
			return &stubProvider{response: "codex-explicit"}
		},
	})
	provider.agentOverride = "codex"

	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gpt-4.1", Input: "prompt"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp != "codex-explicit" {
		t.Fatalf("Complete() response = %q, want %q", resp, "codex-explicit")
	}
	if !codexSelected {
		t.Fatal("expected CodexProvider factory to be invoked for explicit --agent=codex override")
	}
	if codexBinaryUsed != "codex" {
		t.Fatalf("codex binary = %q, want %q", codexBinaryUsed, "codex")
	}
	if adk.calls != 0 {
		t.Fatalf("ADK fallback calls = %d, want 0", adk.calls)
	}
}

func TestUnifiedProviderADKFallback(t *testing.T) {
	registry := &stubRegistry{agents: testAgents(), detected: true}
	geminiProvider := &stubProvider{err: errors.New("gemini failed")}
	adkProvider := &stubProvider{response: "adk fallback"}
	provider := newTestUnifiedProvider(registry, adkProvider, testProviderFactories{
		acp: func(binary string, _ ...string) Provider { return &stubProvider{} },
		gemini: func(binary string) Provider { return geminiProvider },
	})

	// Use a gemini model so the Gemini factory is invoked.
	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gemini-2.5-flash", Input: "prompt"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp != "adk fallback" {
		t.Fatalf("Complete() response = %q, want %q", resp, "adk fallback")
	}
	if geminiProvider.calls != 1 {
		t.Fatalf("Gemini calls = %d, want 1", geminiProvider.calls)
	}
	if adkProvider.calls != 1 {
		t.Fatalf("ADK fallback calls = %d, want 1", adkProvider.calls)
	}
}

func TestUnifiedProviderRateLimitFallback(t *testing.T) {
	registry := &stubRegistry{agents: testAgents(), detected: true}
	geminiProvider := &stubProvider{err: &acp.Error{Code: 429, Message: "rate limited"}}
	adkProvider := &stubProvider{response: "adk retry"}
	provider := newTestUnifiedProvider(registry, adkProvider, testProviderFactories{
		acp: func(binary string, _ ...string) Provider { return &stubProvider{} },
		gemini: func(binary string) Provider { return geminiProvider },
	})

	// Use a gemini model so the Gemini factory is invoked.
	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gemini-2.5-flash", Input: "prompt"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp != "adk retry" {
		t.Fatalf("Complete() response = %q, want %q", resp, "adk retry")
	}
	if geminiProvider.calls != 1 {
		t.Fatalf("Gemini calls = %d, want 1", geminiProvider.calls)
	}
	if adkProvider.calls != 1 {
		t.Fatalf("ADK fallback calls = %d, want 1", adkProvider.calls)
	}
}

func TestUnifiedProviderLazyDetection(t *testing.T) {
	registry := &stubRegistry{agents: testAgents()}
	geminiProvider := &stubProvider{response: "gemini"}
	adkProvider := &stubProvider{response: "adk"}
	provider := newTestUnifiedProvider(registry, adkProvider, testProviderFactories{
		acp: func(binary string, _ ...string) Provider { return &stubProvider{} },
		gemini: func(binary string) Provider { return geminiProvider },
	})

	if registry.detectCalls != 0 {
		t.Fatalf("detect calls before Complete() = %d, want 0", registry.detectCalls)
	}
	if len(provider.agentProviders) != 0 {
		t.Fatalf("provider cache size before Complete() = %d, want 0", len(provider.agentProviders))
	}

	// Use a gemini model so the Gemini factory (injected above) is used for lazy detection.
	resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gemini-2.5-flash", Input: "prompt"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if resp != "gemini" {
		t.Fatalf("Complete() response = %q, want %q", resp, "gemini")
	}
	if registry.detectCalls != 1 {
		t.Fatalf("detect calls after Complete() = %d, want 1", registry.detectCalls)
	}
	if geminiProvider.calls != 1 {
		t.Fatalf("Gemini calls = %d, want 1", geminiProvider.calls)
	}
	if adkProvider.calls != 0 {
		t.Fatalf("ADK fallback calls = %d, want 0", adkProvider.calls)
	}
	if len(provider.agentProviders) != 1 {
		t.Fatalf("provider cache size after Complete() = %d, want 1", len(provider.agentProviders))
	}
}

func TestUnifiedProviderProviderReuse(t *testing.T) {
	registry := &stubRegistry{agents: testAgents(), detected: true}
	adkProvider := &stubProvider{response: "adk"}
	geminiProvider := &stubProvider{response: "gemini"}
	createdProviders := 0
	provider := newTestUnifiedProvider(registry, adkProvider, testProviderFactories{
		acp: func(binary string, _ ...string) Provider { return &stubProvider{} },
		gemini: func(binary string) Provider {
			createdProviders++
			return geminiProvider
		},
	})

	// Use a gemini model so the Gemini factory (injected above) is used.
	for i := 0; i < 2; i++ {
		resp, err := provider.Complete(context.Background(), CompletionRequest{Model: "gemini-2.5-flash", Input: "prompt"})
		if err != nil {
			t.Fatalf("Complete() call %d error = %v", i+1, err)
		}
		if resp != "gemini" {
			t.Fatalf("Complete() call %d response = %q, want %q", i+1, resp, "gemini")
		}
	}

	if createdProviders != 1 {
		t.Fatalf("created Gemini providers = %d, want 1", createdProviders)
	}
	if geminiProvider.calls != 2 {
		t.Fatalf("Gemini provider calls = %d, want 2", geminiProvider.calls)
	}
	if len(provider.agentProviders) != 1 {
		t.Fatalf("provider cache size = %d, want 1", len(provider.agentProviders))
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "acp error 429", err: &acp.Error{Code: 429, Message: "rate limited"}, want: true},
		{name: "rpc error message", err: errors.New("RPC error (429)"), want: true},
		{name: "adk rate limit error", err: &RateLimitError{Err: errors.New("too many requests")}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRateLimitError(tt.err); got != tt.want {
				t.Fatalf("isRateLimitError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestUnifiedProviderExplicitAgentNotFound(t *testing.T) {
	// --agent=unknown should fail with a clear error mentioning known agents.
	registry := &stubRegistry{agents: testAgents(), detected: true}
	adk := &stubProvider{response: "adk"}
	provider := newTestUnifiedProvider(registry, adk, testProviderFactories{
		acp:   func(binary string, _ ...string) Provider { return &stubProvider{} },
		kilo:  func(binary string) Provider { return &stubProvider{} },
		codex: func(binary string) Provider { return &stubProvider{} },
	})
	provider.agentOverride = "unknown-agent"

	_, err := provider.Complete(context.Background(), CompletionRequest{Model: "gpt-4.1", Input: "prompt"})
	if err == nil {
		t.Fatal("Complete() error = nil, want error for unknown --agent")
	}
	if !strings.Contains(err.Error(), "unknown-agent") {
		t.Fatalf("Complete() error = %q, want it to mention the agent name", err.Error())
	}
	if !strings.Contains(err.Error(), "kilo") {
		t.Fatalf("Complete() error = %q, want it to list known agents", err.Error())
	}
	if adk.calls != 0 {
		t.Fatalf("ADK fallback calls = %d, want 0 (no silent fallback)", adk.calls)
	}
}

func TestUnifiedProviderAgentModeUnsupportedForNonACP(t *testing.T) {
	// --agent-mode with kilo or codex should return a clear error.
	tests := []struct {
		name  string
		agent string
		model string
	}{
		{name: "kilo rejects agent-mode", agent: "kilo", model: "kilo/my-model"},
		{name: "codex rejects agent-mode", agent: "codex", model: "gpt-4.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := &AgentRegistry{agents: testAgents()}
			adk := &stubProvider{response: "adk"}
			provider := newTestUnifiedProvider(registry, adk, testProviderFactories{
				acp:   func(binary string, _ ...string) Provider { return &stubProvider{response: "acp"} },
				kilo:  func(binary string) Provider { return &stubProvider{response: "kilo"} },
				codex: func(binary string) Provider { return &stubProvider{response: "codex"} },
			})
			provider.agentOverride = tt.agent

			_, err := provider.Complete(context.Background(), CompletionRequest{
				Model:     tt.model,
				Input:     "prompt",
				AgentMode: "yolo",
			})
			if err == nil {
				t.Fatal("Complete() error = nil, want error for --agent-mode with non-ACP agent")
			}
			if !strings.Contains(err.Error(), "--agent-mode") {
				t.Fatalf("Complete() error = %q, want it to mention --agent-mode", err.Error())
			}
			if !strings.Contains(err.Error(), tt.agent) {
				t.Fatalf("Complete() error = %q, want it to mention the agent name", err.Error())
			}
			if adk.calls != 0 {
				t.Fatalf("ADK fallback calls = %d, want 0", adk.calls)
			}
		})
	}
}

func TestUnifiedProviderUnknownProviderPrefixFails(t *testing.T) {
	// A model with a provider-style prefix that no agent handles should fail
	// with an actionable error instead of silently falling back to ADK.
	registry := &stubRegistry{agents: testAgents(), detected: true}
	adk := &stubProvider{response: "adk"}
	provider := newTestUnifiedProvider(registry, adk, testProviderFactories{
		acp:   func(binary string, _ ...string) Provider { return &stubProvider{} },
		kilo:  func(binary string) Provider { return &stubProvider{} },
		codex: func(binary string) Provider { return &stubProvider{} },
	})

	_, err := provider.Complete(context.Background(), CompletionRequest{Model: "unknown-provider/model-x", Input: "prompt"})
	if err == nil {
		t.Fatal("Complete() error = nil, want error for unknown provider prefix")
	}
	if !strings.Contains(err.Error(), "unknown-provider/model-x") {
		t.Fatalf("Complete() error = %q, want it to include the model name", err.Error())
	}
	if adk.calls != 0 {
		t.Fatalf("ADK fallback calls = %d, want 0 (no silent fallback for unknown provider prefix)", adk.calls)
	}
}
