package llm

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	"swarminator/internal/domain/agent"
)

var (
	ErrNoAgentsAvailable       = errors.New("no agents available; install and authenticate at least one of: kilo, gemini, claude, codex")
	ErrNoModelRouting        = errors.New("no agent supports model prefix")
	ErrNoModelRoutingMessage = "use --agent=NAME to override"
)

const probeTimeout = 5 * time.Second

type AgentRegistry struct {
	agents []agent.AgentInfo
	mutex  sync.RWMutex
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make([]agent.AgentInfo, 0),
	}
}

func (r *AgentRegistry) Detect() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	agents := agent.KnownAgents()

	for i := range agents {
		if _, err := exec.LookPath(agents[i].Binary); err != nil {
			continue
		}

		probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		versionCmd := exec.CommandContext(probeCtx, agents[i].Binary, "--version")
		err := versionCmd.Run()
		cancel()

		if err != nil {
			continue
		}

		agents[i].Available = true
		agents[i].Authenticated = true
	}

	r.agents = agents
	return nil
}

func (r *AgentRegistry) GetForModel(model string) *agent.AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, a := range r.agents {
		if !a.Available {
			continue
		}

		for _, prefix := range a.ModelPrefixes {
			if strings.HasPrefix(model, prefix) {
				return &a
			}
		}
	}

	for _, a := range r.agents {
		if a.Available && a.Authenticated {
			return &a
		}
	}

	for _, a := range r.agents {
		if a.Available {
			return &a
		}
	}

	return nil
}

func (r *AgentRegistry) GetByName(name string) *agent.AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for i := range r.agents {
		a := r.agents[i]
		if a.Available && strings.EqualFold(a.Name, name) {
			return &a
		}
	}

	return nil
}

func (r *AgentRegistry) GetAllAvailable() []agent.AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	available := make([]agent.AgentInfo, 0)
	for _, a := range r.agents {
		if a.Available {
			available = append(available, a)
		}
	}
	return available
}

func (r *AgentRegistry) GetAll() []agent.AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	out := make([]agent.AgentInfo, len(r.agents))
	copy(out, r.agents)
	return out
}

func (r *AgentRegistry) ResolveRoute(model string) (agent.RouteResult, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, a := range r.agents {
		if !a.Available {
			continue
		}
		for _, prefix := range a.ModelPrefixes {
			if strings.HasPrefix(model, prefix) {
				return agent.RouteResult{
					AgentName:     a.Name,
					MatchedPrefix: prefix,
					RouteReason:   "model prefix \"" + prefix + "\" matched agent \"" + a.Name + "\"",
				}, nil
			}
		}
	}

	if isProviderStylePrefix(model) {
		known := make([]string, 0, len(r.agents))
		for _, a := range r.agents {
			if len(a.ModelPrefixes) > 0 {
				known = append(known, a.Name+" ("+strings.Join(a.ModelPrefixes, ", ")+")")
			}
		}
		return agent.RouteResult{}, errors.New("no agent supports model prefix for \"" + model + "\"; known routing: " + strings.Join(known, "; ") + "; use --agent=NAME to override")
	}

	for _, a := range r.agents {
		if a.Available && a.Authenticated {
			return agent.RouteResult{
				AgentName:     a.Name,
				MatchedPrefix: "",
				RouteReason:   "unqualified model \"" + model + "\": selected first available authenticated agent \"" + a.Name + "\"",
			}, nil
		}
	}

	for _, a := range r.agents {
		if a.Available {
			return agent.RouteResult{
				AgentName:     a.Name,
				MatchedPrefix: "",
				RouteReason:   "unqualified model \"" + model + "\": selected first available agent \"" + a.Name + "\"",
			}, nil
		}
	}

	return agent.RouteResult{}, ErrNoAgentsAvailable
}

func isProviderStylePrefix(model string) bool {
	if idx := strings.Index(model, "/"); idx > 0 {
		return true
	}
	knownDashPrefixes := []string{"gpt-", "o1-", "o3-", "codex-", "gemini-", "sonnet-"}
	for _, p := range knownDashPrefixes {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}