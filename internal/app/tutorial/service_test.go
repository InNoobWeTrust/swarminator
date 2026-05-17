package tutorial_test

import (
	"context"
	"strings"
	"testing"

	apptutorial "swarminator/internal/app/tutorial"
	domaintutorial "swarminator/internal/domain/tutorial"
)

// stub completer wired into Service via the unexported provider field.
// We test through the public Service.Answer method; model inference / built-in
// routing happens inside the service and does not reach the provider at all for
// built-in topics.

func TestAnswerTutorialBuiltInTopicBypassesProvider(t *testing.T) {
	svc := apptutorial.NewService()
	result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: "quickstart"})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	want := domaintutorial.Lookup("quickstart")
	if result != want {
		t.Fatalf("expected built-in lookup result, got %q", result)
	}
}

func TestIsSwarmTutorialQueryNarrowing(t *testing.T) {
	// These contain "model" or "agent" in a how-to context and must NOT be
	// intercepted by swarm routing — they should reach tutorial Q&A.
	notSwarm := []string{
		"how do I pass --agent-mode to the model call",
		"what model should I use",
		"what agent runs by default",
		"how do I configure my provider",
		"what are the agent flags",
		"provider configuration",
	}
	for _, q := range notSwarm {
		svc := apptutorial.NewService()
		// A non-swarm query with no agent/model set should fail with a model
		// error, not silently return the swarm guide.
		result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: q})
		if err == nil && strings.Contains(result, "# Swarm-Intelligence Guide") {
			t.Errorf("query %q was incorrectly routed to swarm guide", q)
		}
	}

	// Swarm alias queries should return the swarm guide without calling any LLM.
	swarmAliases := []string{
		"swarm",
		"swarm-intelligence",
		"skill=swarm-intelligence",
	}
	for _, q := range swarmAliases {
		svc := apptutorial.NewService()
		result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: q})
		if err != nil {
			t.Fatalf("Answer(%q) error = %v", q, err)
		}
		if !strings.Contains(result, "# Swarm-Intelligence Guide") {
			t.Errorf("swarm alias %q did not return swarm guide", q)
		}
	}
}

func TestAnswerTutorialSwarmUsesDiscoveryGuidance(t *testing.T) {
	svc := apptutorial.NewService()
	result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: "swarm"})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	for _, want := range []string{
		"# Swarm-Intelligence Guide",
		"One swarminator call = one node",
		"$SHELL -l -c",
		"## Three-Phase Workflow",
		"## Persona Groups",
		"## Persona Prompt Source",
		"## Starter Persona Prompt Patterns",
		"## Minimal Swarm Example",
		"## Available Agents",
		"--agent=NAME",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("swarm tutorial missing %q", want)
		}
	}
}

func TestAnswerTutorialSwarmWithExplicitAgentShowsAgentScopedHints(t *testing.T) {
	svc := apptutorial.NewService()
	result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: "swarm", Agent: "gemini"})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	for _, want := range []string{"## Agent-Scoped Model Hints", "### gemini", "--agent=gemini", "Status:"} {
		if !strings.Contains(result, want) {
			t.Errorf("swarm tutorial missing %q", want)
		}
	}
}

func TestAnswerTutorialSwarmAliasUsesDiscoveryGuidance(t *testing.T) {
	svc := apptutorial.NewService()
	result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: "swarm-intelligence"})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if !strings.Contains(result, "# Swarm-Intelligence Guide") {
		t.Errorf("swarm-intelligence alias did not return swarm guide, got %q", result)
	}
}
