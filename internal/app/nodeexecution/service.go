package nodeexecution

import (
	"context"

	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/protocol"
)

type Service struct {
	provider agent.LLMProvider
}

func NewServiceWithProvider(provider agent.LLMProvider) *Service {
	return &Service{provider: provider}
}

func (s *Service) Execute(ctx context.Context, req agent.CompletionRequest) (string, error) {
	return s.provider.Complete(ctx, req)
}

func (s *Service) DiscoverAgents(ctx context.Context) error {
	return s.provider.DetectAgents()
}

func (s *Service) BuildEnvelope(input, persona string) protocol.Envelope {
	return protocol.NewEnvelope("node", protocol.InferIntent(persona), "orchestrator", input)
}