package agentdiscovery

import (
	"context"

	"swarminator/internal/domain/agent"
)

type Service struct {
	registry  agent.AgentRegistry
	discovery agent.DiscoveryProvider
}

func NewServiceWithRegistry(registry agent.AgentRegistry, discovery agent.DiscoveryProvider) *Service {
	return &Service{registry: registry, discovery: discovery}
}

func (s *Service) Discover(ctx context.Context) error {
	return s.registry.Detect()
}

func (s *Service) GetRegistry() agent.AgentRegistry {
	return s.registry
}

func (s *Service) ListAgents(ctx context.Context) ([]agent.AgentInfo, error) {
	_ = s.registry.Detect()
	return s.registry.GetAll(), nil
}

func (s *Service) ListModels(ctx context.Context, agentName string) ([]agent.ModelInfo, error) {
	groups := s.discovery.DiscoverListings(ctx, agentName)
	if len(groups) == 0 {
		return nil, nil
	}

	var models []agent.ModelInfo
	for _, g := range groups {
		for _, p := range g.Providers {
			for _, m := range p.Models {
				models = append(models, agent.ModelInfo{
					ID:     m.ID,
					Source: m.Source,
				})
			}
		}
	}
	return models, nil
}

func (s *Service) ListProviders(ctx context.Context, agentName string) ([]agent.ProviderListing, error) {
	groups := s.discovery.DiscoverListings(ctx, agentName)
	var providers []agent.ProviderListing
	for _, g := range groups {
		for _, p := range g.Providers {
			providers = append(providers, agent.ProviderListing{
				Name:        p.Name,
				Binary:      p.Binary,
				Available:   p.Available,
				Source:      p.Source,
				Notes:       p.Notes,
				ModelSource: p.ModelSource,
				Models:      convertModels(p.Models),
			})
		}
	}
	return providers, nil
}

func convertModels(listings []agent.ModelInfo) []agent.ModelInfo {
	if listings == nil {
		return nil
	}
	models := make([]agent.ModelInfo, len(listings))
	for i, m := range listings {
		models[i] = agent.ModelInfo{ID: m.ID, Source: m.Source}
	}
	return models
}
