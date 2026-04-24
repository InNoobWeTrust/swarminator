package llm

import "context"

type CompletionRequest struct {
	Model   string
	Persona string
	Input   string
}

type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (string, error)
}
