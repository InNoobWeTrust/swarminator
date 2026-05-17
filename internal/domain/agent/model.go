package agent

import "context"

type AgentInfo struct {
	Name              string
	Binary            string
	ACPArgs           []string
	Available         bool
	Authenticated     bool
	ModelPrefixes     []string
	SupportsModelFlag bool
}

// ModelListing is an infra-facing DTO for agent model discovery.
type ModelListing struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ModelInfo is the canonical domain representation of a single model.
type ModelInfo struct {
	ID     string
	Source string
}

type ProviderListing struct {
	Name        string        `json:"name"`
	Binary      string        `json:"binary"`
	Available   bool          `json:"available"`
	Source      string        `json:"source"`
	Notes       string        `json:"notes,omitempty"`
	ModelSource string        `json:"model_source"`
	Models      []ModelInfo   `json:"models,omitempty"`
}

type EngineListing struct {
	Engine    string            `json:"engine"`
	Providers []ProviderListing `json:"providers"`
}

type Intent string

type AgentMode string

type CompletionRequest struct {
	AgentName string
	ModelID   string
	Persona   string
	Input     string
	AgentMode AgentMode
}

type CompletionResult struct {
	Output string
}

type AgentRegistry interface {
	Detect() error
	GetForModel(model string) *AgentInfo
	GetByName(name string) *AgentInfo
	GetAllAvailable() []AgentInfo
	GetAll() []AgentInfo
	ResolveRoute(model string) (RouteResult, error)
}

// LLMProvider is the execution port: run a completion request against an LLM agent.
type LLMProvider interface {
	Complete(ctx context.Context, req CompletionRequest) (string, error)
	DetectAgents() error
}

// DiscoveryProvider is the discovery port: enumerate engine/provider/model listings.
type DiscoveryProvider interface {
	DiscoverListings(ctx context.Context, filterAgent string) []EngineListing
	FormatDiscoveryText(groups []EngineListing, showModels, showProviders bool) string
}

type RouteResult struct {
	AgentName     string
	MatchedPrefix string
	RouteReason   string
}

func AgentRequiresModelFlag(name string) bool {
	for _, a := range KnownAgents() {
		if a.Name == name {
			return a.SupportsModelFlag
		}
	}
	return true
}

func KnownAgents() []AgentInfo {
	return []AgentInfo{
		{
			Name:              "kilo",
			Binary:            "kilo",
			ACPArgs:           []string{"acp"},
			ModelPrefixes:     []string{"kilo/", "minimax/", "openai/", "github-copilot/", "openrouter/", "o1-", "o3-", "gpt-", "codex-"},
			SupportsModelFlag: true,
		},
		{
			Name:              "gemini",
			Binary:            "gemini",
			ACPArgs:           []string{},
			ModelPrefixes:     []string{"google/", "gemini/", "gemini-"},
			SupportsModelFlag: true,
		},
		{
			Name:              "codex",
			Binary:            "codex",
			ACPArgs:           []string{},
			ModelPrefixes:     []string{},
			SupportsModelFlag: true,
		},
		{
			Name:              "command-code",
			Binary:            "cmd",
			ACPArgs:           []string{},
			ModelPrefixes:     []string{},
			SupportsModelFlag: false,
		},
		{
			Name:              "claude",
			Binary:            "claude",
			ACPArgs:           []string{"--acp"},
			ModelPrefixes:     []string{"claude/", "anthropic/", "sonnet-"},
			SupportsModelFlag: false,
		},
	}
}