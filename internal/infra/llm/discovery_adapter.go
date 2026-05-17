package llm

import (
	"context"

	"swarminator/internal/domain/agent"
)

// discoveryAdapter implements agent.DiscoveryProvider using the infra listing functions.
type discoveryAdapter struct {
	registry agent.AgentRegistry
}

// NewDiscoveryProvider returns an agent.DiscoveryProvider backed by the infra llm listing logic.
func NewDiscoveryProvider(registry agent.AgentRegistry) agent.DiscoveryProvider {
	return &discoveryAdapter{registry: registry}
}

func (d *discoveryAdapter) DiscoverListings(ctx context.Context, filterAgent string) []agent.EngineListing {
	return DiscoverListings(ctx, filterAgent, d.registry)
}

func (d *discoveryAdapter) FormatDiscoveryText(groups []agent.EngineListing, showModels, showProviders bool) string {
	return FormatDiscoveryText(groups, showModels, showProviders)
}
