package main

import (
	"context"
	"strings"
	"time"

	"swarminator/internal/docs"
	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
)

const (
	tutorialTimeout = 120 * time.Second
	tutorialModel   = "kilo/kilo-auto/free"
	tutorialPersona = "You are a knowledgeable assistant for the swarminator CLI. " +
		"The embedded reference is your primary source; use it for exact flags, exit codes, and examples. " +
		"For topics not fully covered by the reference, draw on your broader knowledge of the kilo ecosystem, " +
		"supported models, and swarm patterns to give a complete, actionable answer. " +
		"Show exact commands when useful. " +
		"If a question is genuinely outside the scope of swarminator, say so briefly and point to `swarminator --help`."
)

// kiloCompleter is the minimal interface needed for the tutorial assistant so
// it can be swapped out in tests.
type kiloCompleter interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (string, error)
}

// resolveTutorialModel returns model if non-empty, otherwise defaultTutorialModel.
func resolveTutorialModel(model string) string {
	if strings.TrimSpace(model) != "" {
		return strings.TrimSpace(model)
	}
	return tutorialModel
}

// answerTutorial attempts to answer the query via the kilo assistant backed by
// the embedded CLI reference. If kilo is unavailable or fails it falls back to
// the static tutorial.Lookup result. An optional model override may be supplied;
// if empty, tutorialModel (kilo/kilo-auto/free) is used.
func answerTutorial(ctx context.Context, query string, model string) string {
	return answerTutorialWith(ctx, query, model, nil)
}

// answerTutorialWith is the injectable version used by tests.
func answerTutorialWith(ctx context.Context, query string, model string, provider kiloCompleter) string {
	if provider == nil {
		registry := llm.NewAgentRegistry()
		if err := registry.Detect(); err != nil {
			return tutorial.Lookup(query)
		}
		agent := registry.GetByName("kilo")
		if agent == nil {
			return tutorial.Lookup(query)
		}
		provider = llm.NewKiloProvider(agent.Binary)
	}

	tctx, cancel := context.WithTimeout(ctx, tutorialTimeout)
	defer cancel()

	input := docs.EmbeddedReference() + "\n\n---\n\nQuestion: " + query

	result, err := provider.Complete(tctx, llm.CompletionRequest{
		Model:   resolveTutorialModel(model),
		Persona: tutorialPersona,
		Input:   input,
	})
	if err != nil || strings.TrimSpace(result) == "" {
		return tutorial.Lookup(query)
	}
	return strings.TrimSpace(result)
}
