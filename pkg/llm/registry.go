package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// AgentInfo represents information about an AI agent
type AgentInfo struct {
	Name          string
	Binary        string
	ACPArgs       []string
	Available     bool
	Authenticated bool
	ModelPrefixes []string
}

// AgentRegistry manages detection and retrieval of AI agents
type AgentRegistry struct {
	agents []AgentInfo
	mutex  sync.RWMutex
}

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make([]AgentInfo, 0),
	}
}

// probeTimeout is the maximum time allowed for each agent availability probe.
// "auth status" on some agents (e.g. gemini) can hang indefinitely waiting for
// a network service; a strict timeout prevents Detect() from blocking the
// caller.
const probeTimeout = 5 * time.Second

// Detect finds all available agents and updates the registry.
// It probes each agent with "--version" only (fast, local, no network) rather
// than "auth status" which can hang for some agents (e.g. gemini ≥0.38).
func (r *AgentRegistry) Detect() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	agents := KnownAgents()

	for i := range agents {
		// First fast check: is the binary on PATH at all?
		if _, err := exec.LookPath(agents[i].Binary); err != nil {
			continue
		}

		// Probe with --version under a strict timeout to avoid hanging on
		// agents whose startup or auth flow requires network I/O.
		probeCtx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		versionCmd := exec.CommandContext(probeCtx, agents[i].Binary, "--version")
		err := versionCmd.Run()
		cancel()

		if err != nil {
			// Timed out or non-zero exit — binary is on PATH but not usable.
			continue
		}

		agents[i].Available = true
		// We treat a working binary as "authenticated" because:
		//   (a) auth checks for some agents (gemini) can hang indefinitely, and
		//   (b) the agent itself will surface auth errors at call time where we
		//       can surface them cleanly to the caller.
		agents[i].Authenticated = true
	}

	r.agents = agents
	return nil
}

// GetForModel returns the best agent for a given model
func (r *AgentRegistry) GetForModel(model string) *AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// First try to match by model prefix
	for _, agent := range r.agents {
		if !agent.Available {
			continue
		}

		for _, prefix := range agent.ModelPrefixes {
			if strings.HasPrefix(model, prefix) {
				return &agent
			}
		}
	}

	// If no prefix match, return first available authenticated agent
	for _, agent := range r.agents {
		if agent.Available && agent.Authenticated {
			return &agent
		}
	}

	// If no authenticated agent, return first available agent
	for _, agent := range r.agents {
		if agent.Available {
			return &agent
		}
	}

	return nil
}

// GetByName returns an available agent for a given registry name.
func (r *AgentRegistry) GetByName(name string) *AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for i := range r.agents {
		agent := r.agents[i]
		if agent.Available && strings.EqualFold(agent.Name, name) {
			return &agent
		}
	}

	return nil
}

// GetAllAvailable returns all available agents
func (r *AgentRegistry) GetAllAvailable() []AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	available := make([]AgentInfo, 0)
	for _, agent := range r.agents {
		if agent.Available {
			available = append(available, agent)
		}
	}
	return available
}

// GetAll returns all known agents after detection, including unavailable ones,
// in KnownAgents() order.
func (r *AgentRegistry) GetAll() []AgentInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	out := make([]AgentInfo, len(r.agents))
	copy(out, r.agents)
	return out
}

// RouteResult describes the routing decision made for a model string.
type RouteResult struct {
	AgentName     string // selected agent name (empty on error)
	MatchedPrefix string // prefix that matched, or "" for unqualified fallback
	RouteReason   string // human-readable explanation
}

// isProviderStylePrefix returns true when the model string starts with a
// provider-style prefix (e.g. "foo/" or known multi-char prefixes like "gpt-").
// This is used to detect explicit provider intent so unknown prefixes can be
// rejected rather than silently falling back.
func isProviderStylePrefix(model string) bool {
	// Slash after the first segment indicates provider/model notation.
	if idx := strings.Index(model, "/"); idx > 0 {
		return true
	}
	// Dash-delimited prefixes used by well-known providers.
	knownDashPrefixes := []string{"gpt-", "o1-", "o3-", "codex-", "gemini-", "sonnet-"}
	for _, p := range knownDashPrefixes {
		if strings.HasPrefix(model, p) {
			return true
		}
	}
	return false
}

// ResolveRoute selects an agent for the given model string and returns a
// RouteResult.  It returns an error when the model carries a provider-style
// prefix that is not recognised by any known agent.
func (r *AgentRegistry) ResolveRoute(model string) (RouteResult, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// First try prefix match against available agents.
	for _, agent := range r.agents {
		if !agent.Available {
			continue
		}
		for _, prefix := range agent.ModelPrefixes {
			if strings.HasPrefix(model, prefix) {
				return RouteResult{
					AgentName:     agent.Name,
					MatchedPrefix: prefix,
					RouteReason:   fmt.Sprintf("model prefix %q matched agent %q", prefix, agent.Name),
				}, nil
			}
		}
	}

	// If the model has a provider-style prefix that no agent recognises, fail
	// with an actionable error instead of silently falling back.
	if isProviderStylePrefix(model) {
		known := make([]string, 0, len(r.agents))
		for _, a := range r.agents {
			if len(a.ModelPrefixes) > 0 {
				known = append(known, fmt.Sprintf("%s (%s)", a.Name, strings.Join(a.ModelPrefixes, ", ")))
			}
		}
		return RouteResult{}, fmt.Errorf(
			"no agent supports model prefix for %q; known routing: %s; use --agent=NAME to override",
			model, strings.Join(known, "; "),
		)
	}

	// Unqualified model name: best-effort fallback to first available authenticated agent.
	for _, agent := range r.agents {
		if agent.Available && agent.Authenticated {
			return RouteResult{
				AgentName:     agent.Name,
				MatchedPrefix: "",
				RouteReason:   fmt.Sprintf("unqualified model %q: selected first available authenticated agent %q", model, agent.Name),
			}, nil
		}
	}

	// Final fallback: first available agent regardless of authentication.
	for _, agent := range r.agents {
		if agent.Available {
			return RouteResult{
				AgentName:     agent.Name,
				MatchedPrefix: "",
				RouteReason:   fmt.Sprintf("unqualified model %q: selected first available agent %q", model, agent.Name),
			}, nil
		}
	}

	return RouteResult{}, fmt.Errorf("no agents available; install and authenticate at least one of: kilo, gemini, claude, codex")
}

// KnownAgents returns the full static list of agent definitions (Available/Authenticated
// fields are always false; call Detect() to populate runtime availability).
func KnownAgents() []AgentInfo {
	return []AgentInfo{
		{
			Name:          "kilo",
			Binary:        "kilo",
			ACPArgs:       []string{"acp"},
			ModelPrefixes: []string{"kilo/", "minimax/", "openai/", "github-copilot/", "openrouter/", "o1-", "o3-", "gpt-", "codex-"},
		},
		{
			Name:          "gemini",
			Binary:        "gemini",
			ACPArgs:       []string{},
			ModelPrefixes: []string{"google/", "gemini/", "gemini-"},
		},
		{
			Name:          "codex",
			Binary:        "codex",
			ACPArgs:       []string{},
			ModelPrefixes: []string{},
		},
		{
			Name:          "claude",
			Binary:        "claude",
			ACPArgs:       []string{"--acp"},
			ModelPrefixes: []string{"claude/", "anthropic/", "sonnet-"},
		},
	}
}

// String returns a string representation of the agent info
func (a AgentInfo) String() string {
	status := "unavailable"
	if a.Available {
		if a.Authenticated {
			status = "authenticated"
		} else {
			status = "available"
		}
	}
	return fmt.Sprintf("%s (%s): %s", a.Name, a.Binary, status)
}
