package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"swarminator/internal/domain/agent"
)

var (
	newUnifiedRegistry = func() agent.AgentRegistry {
		return NewAgentRegistry()
	}
	newUnifiedADKProvider = func(model, projectID string) LLMAdapter {
		return NewADKProvider(model, projectID)
	}
	newUnifiedACPProvider = func(binary string, args ...string) LLMAdapter {
		return NewACPProvider(binary, args...)
	}
	newUnifiedKiloProvider = func(binary string) LLMAdapter {
		return NewKiloProvider(binary)
	}
	newUnifiedCodexProvider = func(binary string) LLMAdapter {
		return NewCodexProvider(binary)
	}
	newUnifiedCommandCodeProvider = func(binary string) LLMAdapter {
		return NewCommandCodeProvider(binary)
	}
	newUnifiedGeminiProvider = func(binary string) LLMAdapter {
		return NewGeminiProvider(binary)
	}
)

type UnifiedProvider struct {
	registry               agent.AgentRegistry
	adkFallback            LLMAdapter
	agentProviders         map[string]LLMAdapter
	newACPProvider         func(binary string, args ...string) LLMAdapter
	newKiloProvider        func(binary string) LLMAdapter
	newCodexProvider       func(binary string) LLMAdapter
	newCommandCodeProvider func(binary string) LLMAdapter
	newGeminiProvider      func(binary string) LLMAdapter
	agentOverride          string
	mutex                  sync.RWMutex
}

func NewUnifiedProvider(projectID string, agentOverride ...string) *UnifiedProvider {
	override := ""
	if len(agentOverride) > 0 {
		override = strings.TrimSpace(agentOverride[0])
	}

	return &UnifiedProvider{
		registry:               newUnifiedRegistry(),
		adkFallback:            newUnifiedADKProvider("", projectID),
		agentProviders:         make(map[string]LLMAdapter),
		newACPProvider:         newUnifiedACPProvider,
		newKiloProvider:        newUnifiedKiloProvider,
		newCodexProvider:       newUnifiedCodexProvider,
		newCommandCodeProvider: newUnifiedCommandCodeProvider,
		newGeminiProvider:      newUnifiedGeminiProvider,
		agentOverride:          override,
	}
}

func (u *UnifiedProvider) DetectAgents() error {
	return u.registry.Detect()
}

func normalizeModelForAgent(agentName, model string) string {
	switch agentName {
	case "gemini":
		return normalizeGeminiModel(model)
	default:
		return model
	}
}

func (u *UnifiedProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	u.mutex.RLock()
	agentsDetected := len(u.registry.GetAllAvailable()) > 0
	u.mutex.RUnlock()

	if !agentsDetected {
		_ = u.DetectAgents()
	}

	if u.agentOverride == "" {
		return "", fmt.Errorf("no agent specified; use --agent=NAME (kilo, gemini, claude, codex, command-code)")
	}
	a := u.registry.GetByName(u.agentOverride)
	if a == nil {
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
	agent := *a

	normalizedModel := normalizeModelForAgent(agent.Name, req.ModelID)
	normalizedReq := req
	normalizedReq.ModelID = normalizedModel

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

	response, err := provider.Complete(ctx, normalizedReq)
	if err != nil {
		return "", fmt.Errorf("agent-%s failed: %w", agent.Name, err)
	}

	return response, nil
}