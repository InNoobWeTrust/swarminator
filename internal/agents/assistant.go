package agents

import (
	"context"
	"errors"
	"os"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

type Assistant struct {
	runner *runner.Runner
	userID string
}

func NewGeminiAssistant(ctx context.Context, modelName, agentName, description, instruction string) (*Assistant, error) {
	normalizedModel := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(modelName, "google/"), "gemini/"))
	llm, err := gemini.NewModel(ctx, normalizedModel, &genai.ClientConfig{APIKey: strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))})
	if err != nil {
		return nil, err
	}
	root, err := NewNodeAgent(llm, NodeConfig{
		Name:        agentName,
		Description: description,
		Persona:     instruction,
		Role:        "assistant",
		Intent:      "advise",
		Target:      "orchestrator",
	})
	if err != nil {
		return nil, err
	}
	r, err := runner.New(runner.Config{
		AppName:           "swarminator",
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, err
	}
	return &Assistant{runner: r, userID: "swarminator"}, nil
}

func (a *Assistant) Advise(ctx context.Context, prompt string) (string, error) {
	if a == nil || a.runner == nil {
		return "", errors.New("assistant is not initialized")
	}
	content := genai.NewContentFromText(prompt, "user")
	var cfg agent.RunConfig
	seq := a.runner.Run(ctx, a.userID, "assistant-session", content, cfg)
	var parts []string
	for evt, err := range seq {
		if err != nil {
			return "", err
		}
		if evt == nil || !evt.IsFinalResponse() || evt.Content == nil {
			continue
		}
		text := contentText(evt.Content)
		if text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "", errors.New("assistant returned no final response")
	}
	return strings.Join(parts, "\n\n"), nil
}

func contentText(content *genai.Content) string {
	if content == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil || part.Text == "" {
			continue
		}
		b.WriteString(part.Text)
	}
	return strings.TrimSpace(b.String())
}
