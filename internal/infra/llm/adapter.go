package llm

import (
	"context"
	"swarminator/internal/domain/agent"
)

type LLMAdapter interface {
	Complete(ctx context.Context, req agent.CompletionRequest) (string, error)
}

type AgentLister interface {
	ListAgents(ctx context.Context) ([]agent.AgentInfo, error)
}

type ModelLister interface {
	ListModels(ctx context.Context, agentName string) ([]agent.ModelInfo, error)
}

type DiscoveryService interface {
	DiscoverListings(ctx context.Context, filterAgent string, registry agent.AgentRegistry) []agent.EngineListing
	FormatDiscoveryText(groups []agent.EngineListing, includeModels, includeProviders bool) string
}