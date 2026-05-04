package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"swarminator/internal/protocol/acp"
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
	newUnifiedGeminiProvider = func(binary string) Provider {
		return NewGeminiProvider(binary)
	}
)

// UnifiedProvider combines CLI agents with ADK fallback
type UnifiedProvider struct {
	registry             unifiedRegistry
	adkFallback          Provider
	agentProviders       map[string]Provider // agent name -> provider instances
	newACPProvider       func(binary string, args ...string) Provider
	newKiloProvider      func(binary string) Provider
	newCodexProvider     func(binary string) Provider
	newGeminiProvider    func(binary string) Provider
	agentOverride        string
	mutex                sync.RWMutex
}

// NewUnifiedProvider creates a new unified provider

func NewUnifiedProvider(projectID string, agentOverride ...string) *UnifiedProvider {
	override := ""
	if len(agentOverride) > 0 {
		override = strings.TrimSpace(agentOverride[0])
	}

	return &UnifiedProvider{
		registry:          newUnifiedRegistry(),
		adkFallback:       newUnifiedADKProvider("", projectID), // Empty model string, will be set per request
		agentProviders:    make(map[string]Provider),
		newACPProvider:    newUnifiedACPProvider,
		newKiloProvider:   newUnifiedKiloProvider,
		newCodexProvider:  newUnifiedCodexProvider,
		newGeminiProvider: newUnifiedGeminiProvider,
		agentOverride:     override,
	}
}

// DetectAgents manually triggers agent detection
func (u *UnifiedProvider) DetectAgents() error {
	return u.registry.Detect()
}

// Complete implements the Provider interface
func (u *UnifiedProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	// Lazy detection - detect agents on first use if not already detected
	u.mutex.RLock()
	agentsDetected := len(u.registry.GetAllAvailable()) > 0
	u.mutex.RUnlock()

	if !agentsDetected {
		if err := u.DetectAgents(); err != nil {
			// If detection fails, fall back to ADK directly
			return u.adkFallback.Complete(ctx, req)
		}
	}

	// Use an explicit CLI override when requested; otherwise resolve by model.
	var agent *AgentInfo
	if u.agentOverride != "" {
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
		agent = a
	} else {
		route, err := u.registry.ResolveRoute(req.Model)
		if err != nil {
			// Unknown provider-style prefix: fail with actionable error.
			return "", err
		}
		agent = u.registry.GetByName(route.AgentName)
		if agent == nil {
			// Resolved agent name is no longer available (race or detection gap).
			return u.adkFallback.Complete(ctx, req)
		}
	}

		// Get or create provider for this agent
		u.mutex.Lock()
		provider, exists := u.agentProviders[agent.Name]
		if !exists {
			switch agent.Name {
			case "kilo":
				if req.AgentMode != "" {
					u.mutex.Unlock()
					return "", fmt.Errorf("--agent-mode is not supported for agent %q; it is only supported for ACP agents (gemini, claude)", agent.Name)
				}
				kf := u.newKiloProvider
				if kf == nil {
					kf = NewKiloProvider
				}
				provider = kf(agent.Binary)
			case "codex":
				if req.AgentMode != "" {
					u.mutex.Unlock()
					return "", fmt.Errorf("--agent-mode is not supported for agent %q; it is only supported for ACP agents (gemini, claude)", agent.Name)
				}
				cf := u.newCodexProvider
				if cf == nil {
					cf = NewCodexProvider
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

	// Try ACP call via the CLI agent first
	response, err := provider.Complete(ctx, req)
	if err != nil {
		// Check if it's a 429 rate limit error
		if isRateLimitError(err) {
			// Fall back to ADK
			fallbackResp, fallbackErr := u.adkFallback.Complete(ctx, req)
			if fallbackErr != nil {
				return "", fmt.Errorf("agent-%s: attempt 1 failed (rate limited): %v; fallback failed: %w", agent.Name, err, fallbackErr)
			}
			return fallbackResp, nil
		}

		// For other errors, fall back to ADK
		fallbackResp, fallbackErr := u.adkFallback.Complete(ctx, req)
		if fallbackErr != nil {
			return "", fmt.Errorf("agent-%s: attempt 1 failed: %v; fallback failed: %w", agent.Name, err, fallbackErr)
		}
		return fallbackResp, nil
	}

	return response, nil
}

// isRateLimitError checks if the error is a 429 rate limit error from ACP or ADK
func isRateLimitError(err error) bool {
	// Check if it's an ACP RPC error with code 429
	rpcErr, ok := err.(*acp.Error)
	if ok && rpcErr.Code == 429 {
		return true
	}

	// Also check if the error message contains "429" (for wrapped errors)
	if err != nil && err.Error() == "RPC error (429)" {
		return true
	}

	// Check if it's an ADK RateLimitError
	_, ok = err.(*RateLimitError)
	if ok {
		return true
	}

	return false
}
