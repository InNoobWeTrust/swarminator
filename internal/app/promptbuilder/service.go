package promptbuilder

import (
	"context"
	"strings"

	"swarminator/internal/docs"
	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/tutorial"
)

type Service struct {
	discovery agent.DiscoveryProvider
}

func NewServiceWithRegistry(discovery agent.DiscoveryProvider) *Service {
	return &Service{discovery: discovery}
}

func (s *Service) BuildTutorialInput(ctx context.Context, agentName, model, query string) string {
	var b strings.Builder
	b.WriteString(docs.EmbeddedReference())
	b.WriteString("\n\n---\n\n")
	b.WriteString("Tutorial assistant context:\n")
	b.WriteString("selected agent: ")
	b.WriteString(agentName)
	b.WriteString("\nselected model: ")
	b.WriteString(model)
	b.WriteString("\n")
	groups := s.discovery.DiscoverListings(ctx, agentName)
	discovery := s.discovery.FormatDiscoveryText(groups, true, true)
	if strings.TrimSpace(discovery) != "" {
		b.WriteString("\nlive agent discovery:\n")
		b.WriteString(discovery)
		b.WriteString("\n")
	}
	b.WriteString("\nQuestion: ")
	b.WriteString(query)
	return b.String()
}

func (s *Service) GetPersona(agentName, query string) string {
	return tutorial.TutorialPersona(agentName, query)
}

type tutorialCompleter interface {
	Complete(ctx context.Context, req agent.CompletionRequest) (string, error)
}
