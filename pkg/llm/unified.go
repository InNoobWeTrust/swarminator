package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type unifiedRegistry interface {
	Detect() error
	GetForModel(model string) *AgentInfo
	GetByName(name string) *AgentInfo
	GetAllAvailable() []AgentInfo
	GetAll() []AgentInfo
	ResolveRoute(model string) (RouteResult, error)
}

var (
	newUnifiedRegistry = func() unifiedRegistry {
		return NewAgentRegistry()
	}
	newUnifiedADKProvider = func(model, projectID string) Provider {
		return NewADKProvider(model, projectID)
	}
	newUnifiedACPProvider = func(binary string, args ...string) Provider {
		return NewACPProvider(binary, args...)
	}
	newUnifiedKiloProvider = func(binary string) Provider {
		return NewKiloProvider(binary)
	}
	newUnifiedCodexProvider = func(binary string) Provider {
		return NewCodexProvider(binary)
	}
	newUnifiedCommandCodeProvider = func(binary string) Provider {
		return NewCommandCodeProvider(binary)
	}
	newUnifiedGeminiProvider = func(binary string) Provider {
		return NewGeminiProvider(binary)
	}
)

// UnifiedProvider combines CLI agents with ADK fallback
type UnifiedProvider struct {
	registry               unifiedRegistry
	adkFallback            Provider
	agentProviders         map[string]Provider // agent name -> provider instances
	newACPProvider         func(binary string, args ...string) Provider
	newKiloProvider        func(binary string) Provider
	newCodexProvider       func(binary string) Provider
	newCommandCodeProvider func(binary string) Provider
	newGeminiProvider      func(binary string) Provider
	agentOverride          string
	mutex                  sync.RWMutex
}

// NewUnifiedProvider creates a new unified provider
func NewUnifiedProvider(projectID string, agentOverride ...string) *UnifiedProvider {
	override := ""
	if len(agentOverride) > 0 {
		override = strings.TrimSpace(agentOverride[0])
	}

	return &UnifiedProvider{
		registry:               newUnifiedRegistry(),
		adkFallback:            newUnifiedADKProvider("", projectID), // Empty model string, will be set per request
		agentProviders:         make(map[string]Provider),
		newACPProvider:         newUnifiedACPProvider,
		newKiloProvider:        newUnifiedKiloProvider,
		newCodexProvider:       newUnifiedCodexProvider,
		newCommandCodeProvider: newUnifiedCommandCodeProvider,
		newGeminiProvider:      newUnifiedGeminiProvider,
		agentOverride:          override,
	}
}

// DetectAgents manually triggers agent detection
func (u *UnifiedProvider) DetectAgents() error {
	return u.registry.Detect()
}

// normalizeModelForAgent normalizes the model string according to the agent's expectations.
// For gemini, strips google/ and gemini/ prefixes. Other agents receive the model as-is.
func normalizeModelForAgent(agentName, model string) string {
	switch agentName {
	case "gemini":
		return normalizeGeminiModel(model)
	default:
		// kilo, claude, codex: pass model as-is.
		// kilo expects provider-qualified IDs (kilo/..., openai/..., etc.)
		// claude does not use model string in ACP protocol
		// codex passes model through to binary
		return model
	}
}

// Complete implements the Provider interface
func (u *UnifiedProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	// Lazy detection - detect agents on first use if not already detected
	u.mutex.RLock()
	agentsDetected := len(u.registry.GetAllAvailable()) > 0
	u.mutex.RUnlock()

	if !agentsDetected {
		// Best-effort detection. If it errors, GetByName will return nil below
		// and we return a clear error — no silent ADK fallback.
		_ = u.DetectAgents()
	}

	// Explicit agent is required; do not perform automatic prefix routing.
	if u.agentOverride == "" {
		return "", fmt.Errorf("no agent specified; use --agent=NAME (kilo, gemini, claude, codex, command-code)")
	}
	a := u.registry.GetByName(u.agentOverride)
	if a == nil {
		// Collect known agent names for a helpful error message.
		all := u.registry.GetAll()
		names := make([]string, 0, len(all))
		for _, info := range all {
			names = append(names, info.Name)
		}
		return "", fmt.Errorf(
			"explicit --agent=%q is unknown or unavailable; known agents: %s",
			u.agentOverride, strings.Join(names, ", "),
		)
	}
	agent := a

	// Normalize model string for the selected agent.
	normalizedModel := normalizeModelForAgent(agent.Name, req.Model)
	normalizedReq := req
	normalizedReq.Model = normalizedModel

	u.mutex.Lock()
	provider, exists := u.agentProviders[agent.Name]
	if !exists {
		switch agent.Name {
		case "kilo":
			if normalizedReq.AgentMode != "" {
				u.mutex.Unlock()
				return "", fmt.Errorf("--agent-mode is not supported for agent %q; it is only supported for ACP agents (gemini, claude)", agent.Name)
			}
			kf := u.newKiloProvider
			if kf == nil {
				kf = NewKiloProvider
			}
			provider = kf(agent.Binary)
		case "codex":
			if normalizedReq.AgentMode != "" {
				u.mutex.Unlock()
				return "", fmt.Errorf("--agent-mode is not supported for agent %q; it is only supported for ACP agents (gemini, claude)", agent.Name)
			}
			cf := u.newCodexProvider
			if cf == nil {
				cf = NewCodexProvider
			}
			provider = cf(agent.Binary)
		case "command-code":
			if normalizedReq.AgentMode != "" {
				u.mutex.Unlock()
				return "", fmt.Errorf("--agent-mode is not supported for agent %q; it is only supported for ACP agents (gemini, claude)", agent.Name)
			}
			cf := u.newCommandCodeProvider
			if cf == nil {
				cf = NewCommandCodeProvider
			}
			provider = cf(agent.Binary)
		case "gemini":
			gf := u.newGeminiProvider
			if gf == nil {
				gf = NewGeminiProvider
			}
			provider = gf(agent.Binary)
		default:
			providerFactory := u.newACPProvider
			if providerFactory == nil {
				providerFactory = NewACPProvider
			}
			provider = providerFactory(agent.Binary, agent.ACPArgs...)
		}
		u.agentProviders[agent.Name] = provider
	}
	u.mutex.Unlock()

	// Call the explicit agent. With --agent=NAME required, a failure here is
	// returned directly — no silent ADK fallback. Silently rerouting would
	// violate the fail-closed contract of explicit agent selection.
	response, err := provider.Complete(ctx, normalizedReq)
	if err != nil {
		return "", fmt.Errorf("agent-%s failed: %w", agent.Name, err)
	}

	return response, nil
}

