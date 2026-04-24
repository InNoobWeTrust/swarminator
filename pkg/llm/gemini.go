package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"swarminator/internal/agents"
)

type GeminiProvider struct{}

func NewGeminiProvider() Provider {
	return &GeminiProvider{}
}

func (p *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
	if strings.TrimSpace(req.Model) == "" {
		return "", errors.New("model is required")
	}
	if strings.TrimSpace(req.Persona) == "" {
		return "", errors.New("persona is required")
	}
	if strings.TrimSpace(req.Input) == "" {
		return "", errors.New("stdin input is empty")
	}
	if strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")) == "" {
		return "", errors.New("GOOGLE_API_KEY is required for live Gemini execution")
	}

	assistant, err := agents.NewGeminiAssistant(ctx, req.Model, "swarminator_node", "A swarm node executor", req.Persona)
	if err != nil {
		return "", fmt.Errorf("failed to initialize Gemini assistant: %w", err)
	}

	output, err := assistant.Advise(ctx, req.Input)
	if err != nil {
		return "", err
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return "", errors.New("model returned an empty response")
	}
	return output, nil
}

var _ Provider = (*GeminiProvider)(nil)
