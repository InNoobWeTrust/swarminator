package tutorial_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	apptutorial "swarminator/internal/app/tutorial"
	"swarminator/internal/domain/agent"
	domaintutorial "swarminator/internal/domain/tutorial"
)

type stubRegistry struct{}

func (stubRegistry) Detect() error { return nil }

func (stubRegistry) GetForModel(string) *agent.AgentInfo { return nil }

func (stubRegistry) GetByName(string) *agent.AgentInfo { return nil }

func (stubRegistry) GetAllAvailable() []agent.AgentInfo { return nil }

func (stubRegistry) GetAll() []agent.AgentInfo { return nil }

func (stubRegistry) ResolveRoute(string) (agent.RouteResult, error) {
	return agent.RouteResult{}, nil
}

type stubDiscovery struct {
	groups []agent.EngineListing
}

func (s stubDiscovery) DiscoverListings(context.Context, string) []agent.EngineListing {
	return s.groups
}

func (s stubDiscovery) FormatDiscoveryText(groups []agent.EngineListing, showModels, showProviders bool) string {
	var b strings.Builder
	for _, group := range groups {
		b.WriteString("ENGINE ")
		b.WriteString(group.Engine)
		b.WriteByte('\n')
		for _, provider := range group.Providers {
			b.WriteString("  PROVIDER ")
			b.WriteString(provider.Name)
			b.WriteByte('\n')
			if showProviders {
				b.WriteString("    available: ")
				if provider.Available {
					b.WriteString("true\n")
				} else {
					b.WriteString("false\n")
				}
			}
			if showModels {
				for _, model := range provider.Models {
					b.WriteString("    - ")
					b.WriteString(model.ID)
					b.WriteByte('\n')
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}

type stubLLMProvider struct {
	err error
}

func (s stubLLMProvider) Complete(context.Context, agent.CompletionRequest) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return "unexpected LLM call", nil
}

func (stubLLMProvider) DetectAgents() error { return nil }

func newTestService() *apptutorial.Service {
	groups := []agent.EngineListing{
		{
			Engine: "gemini",
			Providers: []agent.ProviderListing{{
				Name:        "gemini",
				Available:   true,
				Notes:       "native headless CLI execution",
				ModelSource: "embedded",
				Models: []agent.ModelInfo{{
					ID:     "google/gemini-2.5-flash",
					Source: "embedded",
				}},
			}},
		},
		{
			Engine: "command-code",
			Providers: []agent.ProviderListing{{
				Name:        "command-code",
				Available:   true,
				Notes:       "explicit-only (--agent=command-code)",
				ModelSource: "none",
			}},
		},
	}

	return apptutorial.NewServiceWithRegistry(
		stubRegistry{},
		stubDiscovery{groups: groups},
		stubLLMProvider{err: errors.New("unexpected LLM call")},
	)
}

func TestAnswerTutorialBuiltInTopicBypassesProvider(t *testing.T) {
	svc := newTestService()
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
		svc := newTestService()
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
		svc := newTestService()
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
	svc := newTestService()
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
	svc := newTestService()
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
	svc := newTestService()
	result, err := svc.Answer(context.Background(), apptutorial.TutorialRequest{Query: "swarm-intelligence"})
	if err != nil {
		t.Fatalf("Answer() error = %v", err)
	}
	if !strings.Contains(result, "# Swarm-Intelligence Guide") {
		t.Errorf("swarm-intelligence alias did not return swarm guide, got %q", result)
	}
}
