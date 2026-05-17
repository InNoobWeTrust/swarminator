package main

import (
	"context"
	"strings"
	"testing"

	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
)

type mockProvider struct {
	result          string
	err             error
	calls           int
	capturedModel   string
	capturedPersona string
	capturedInput   string
}

func (m *mockProvider) Complete(_ context.Context, req llm.CompletionRequest) (string, error) {
	m.calls++
	m.capturedModel = req.Model
	m.capturedPersona = req.Persona
	m.capturedInput = req.Input
	return m.result, m.err
}

func TestAnswerTutorialBuiltInTopicBypassesProvider(t *testing.T) {
	mock := &mockProvider{result: "should be ignored"}
	result, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "quickstart"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("built-in tutorial should bypass provider, got %d calls", mock.calls)
	}
	if result != tutorial.Lookup("quickstart") {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestAnswerTutorialIncludesDocsInInput(t *testing.T) {
	mock := &mockProvider{result: "answer"}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "myquestion", Agent: "gemini", Model: "google/gemini-2.5-flash"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if !strings.Contains(mock.capturedInput, "# Swarminator CLI Reference") {
		t.Error("input to provider does not contain embedded docs header")
	}
	if !strings.Contains(mock.capturedInput, "myquestion") {
		t.Error("input to provider does not contain user query")
	}
	if !strings.Contains(mock.capturedInput, "selected agent: gemini") {
		t.Error("input to provider does not contain selected agent")
	}
	if !strings.Contains(mock.capturedInput, "ENGINE gemini") {
		t.Error("input to provider does not contain agent-scoped discovery")
	}
}

func TestAnswerTutorialReturnsProviderError(t *testing.T) {
	mock := &mockProvider{err: context.DeadlineExceeded}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "myquestion", Agent: "gemini", Model: "google/gemini-2.5-flash"}, mock)
	if err == nil {
		t.Fatal("answerTutorialWith() error = nil, want non-nil")
	}
}

func TestAnswerTutorialReturnsErrorOnEmptyResponse(t *testing.T) {
	mock := &mockProvider{result: "   "}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "myquestion", Agent: "gemini", Model: "google/gemini-2.5-flash"}, mock)
	if err == nil {
		t.Fatal("answerTutorialWith() error = nil, want non-nil")
	}
}

func TestAnswerTutorialPersonaSet(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "test", Agent: "gemini", Model: "google/gemini-2.5-flash"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if !strings.Contains(mock.capturedPersona, "explicit agent/model chosen by the caller") {
		t.Errorf("persona mismatch: got %q", mock.capturedPersona)
	}
}

func TestAnswerTutorialModelSuggestionUsesCheapDefaultForAgent(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "which model should I use", Agent: "gemini"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.capturedModel != "google/gemini-2.5-flash" {
		t.Errorf("cheap default model mismatch: got %q", mock.capturedModel)
	}
}

func TestAnswerTutorialCustomModelPassedThrough(t *testing.T) {
	mock := &mockProvider{result: "ok"}
	_, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "test", Agent: "kilo", Model: "kilo/custom/model"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.capturedModel != "kilo/custom/model" {
		t.Errorf("expected custom model %q, got %q", "kilo/custom/model", mock.capturedModel)
	}
}

func TestIsSwarmTutorialQueryNarrowing(t *testing.T) {
	// These queries contain "model" or "agent" in a how-to context and should
	// NOT be intercepted by the swarm routing — they should reach tutorial Q&A.
	notSwarm := []string{
		"how do I pass --agent-mode to the model call",
		"what model should I use",
		"what agent runs by default",
		"how do I configure my provider",
		"what are the agent flags",
		"provider configuration",
	}
	for _, q := range notSwarm {
		if isSwarmTutorialQuery(q) {
			t.Errorf("isSwarmTutorialQuery(%q) = true, want false (should reach kilo assistant)", q)
		}
	}

	// These should route to the swarm listing.
	isSwarm := []string{
		"swarm",
		"swarm-intelligence",
		"skill=swarm-intelligence",
		"list providers",
		"list models",
		"choose engine",
		"engine choice",
		"which engine should I use",
		"swarm guidance",
		"swarm agent selection",
		"which persona should I use",
		"show quorum modes",
		"phase 1",
		"maker-fix",
	}
	for _, q := range isSwarm {
		if !isSwarmTutorialQuery(q) {
			t.Errorf("isSwarmTutorialQuery(%q) = false, want true", q)
		}
	}
}

func TestAnswerTutorialSwarmUsesDiscoveryGuidance(t *testing.T) {
	mock := &mockProvider{result: "provider answer should be ignored"}
	result, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "swarm"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("swarm tutorial should bypass provider, got %d calls", mock.calls)
	}
	for _, want := range []string{"# Swarm-Intelligence Guide", "One swarminator call = one node", "$SHELL -l -c", "## Three-Phase Workflow", "## Persona Groups", "## Persona Prompt Source", "## Starter Persona Prompt Patterns", "## Minimal Swarm Example", "## Available Agents", "--agent=NAME"} {
		if !strings.Contains(result, want) {
			t.Fatalf("swarm tutorial missing %q in %q", want, result)
		}
	}
}

func TestAnswerTutorialSwarmWithExplicitAgentShowsAgentScopedHints(t *testing.T) {
	mock := &mockProvider{result: "provider answer should be ignored"}
	result, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "swarm", Agent: "gemini"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("swarm tutorial should bypass provider, got %d calls", mock.calls)
	}
	for _, want := range []string{"## Agent-Scoped Model Hints", "### gemini", "embedded model hints", "--agent=gemini"} {
		if !strings.Contains(result, want) {
			t.Fatalf("swarm tutorial missing %q in %q", want, result)
		}
	}
}

func TestAnswerTutorialSwarmAliasUsesDiscoveryGuidance(t *testing.T) {
	mock := &mockProvider{result: "provider answer should be ignored"}
	result, err := answerTutorialWith(context.Background(), tutorialRequest{Query: "swarm-intelligence"}, mock)
	if err != nil {
		t.Fatalf("answerTutorialWith() error = %v", err)
	}
	if mock.calls != 0 {
		t.Fatalf("swarm-intelligence tutorial should bypass provider, got %d calls", mock.calls)
	}
	if !strings.Contains(result, "# Swarm-Intelligence Guide") {
		t.Fatalf("swarm-intelligence tutorial missing embedded guide header in %q", result)
	}
}
