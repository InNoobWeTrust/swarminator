package tutorial_test

import (
	"strings"
	"testing"

	"swarminator/internal/tutorial"
)

func TestTopicsReturnKnownKeys(t *testing.T) {
	topics := tutorial.Topics()
	expected := map[string]bool{
		"full": true, "quickstart": true, "rules": true,
		"protocol": true, "quorum": true, "safety": true, "swarm": true,
	}
	for _, k := range topics {
		if !expected[k] {
			t.Errorf("unexpected topic key: %q", k)
		}
		delete(expected, k)
	}
	for k := range expected {
		t.Errorf("missing expected topic key: %q", k)
	}
}

func TestTopicTextNonEmpty(t *testing.T) {
	for _, k := range tutorial.Topics() {
		if tutorial.TopicText(k) == "" {
			t.Errorf("TopicText(%q) returned empty string", k)
		}
	}
}

func TestTopicTextUnknownEmpty(t *testing.T) {
	if tutorial.TopicText("nonexistent_xyz") != "" {
		t.Error("TopicText() for unknown key should return empty string")
	}
}

func TestReferenceTopicsStable(t *testing.T) {
	entries1 := tutorial.ReferenceTopics()
	entries2 := tutorial.ReferenceTopics()
	if len(entries1) != len(entries2) {
		t.Fatal("ReferenceTopics() returns different lengths on repeated calls")
	}
	for i := range entries1 {
		if entries1[i].Key != entries2[i].Key {
			t.Errorf("ReferenceTopics() key at index %d differs: %q vs %q", i, entries1[i].Key, entries2[i].Key)
		}
	}
}

func TestFullTopicContainsCurrentBehavior(t *testing.T) {
	text := tutorial.TopicText("full")
	for _, want := range []string{"Gemini headless", "Claude ACP", "kilo multi-provider", "Codex explicit-only"} {
		if !strings.Contains(text, want) {
			t.Errorf("full topic missing expected content: %q", want)
		}
	}
}

func TestQuickstartTopicContainsCurrentCommands(t *testing.T) {
	text := tutorial.TopicText("quickstart")
	for _, want := range []string{"github-copilot/", "--dry-run", "--list-agents", "--agent=gemini", "--agent=kilo"} {
		if !strings.Contains(text, want) {
			t.Errorf("quickstart topic missing expected command: %q", want)
		}
	}
}

func TestRulesTopicNoLongerContainsOutdatedAuth(t *testing.T) {
	text := tutorial.TopicText("rules")
	if strings.Contains(text, "GOOGLE_API_KEY") {
		t.Error("rules topic should no longer contain GOOGLE_API_KEY")
	}
}

func TestSwarmTopicContainsAgentGuideContent(t *testing.T) {
	text := tutorial.TopicText("swarm")
	for _, want := range []string{"Swarm-Intelligence Guide", "One swarminator call = one node", "Three-Phase Workflow", "Persona Groups", "Persona Prompt Source", "Starter Persona Prompt Patterns", "Minimal Swarm Example", "swarm-intelligence"} {
		if !strings.Contains(text, want) {
			t.Errorf("swarm topic missing expected content: %q", want)
		}
	}
}

func TestLookupSupportsSwarmAliases(t *testing.T) {
	want := tutorial.TopicText("swarm")
	for _, alias := range []string{"swarm-intelligence", "skill=swarm-intelligence", "agent-guide"} {
		if got := tutorial.Lookup(alias); got != want {
			t.Errorf("Lookup(%q) mismatch", alias)
		}
	}
}

func TestIsBuiltInQuery(t *testing.T) {
	for _, query := range []string{"quickstart", "rules", "protocol", "phase=extract", "what are the rules", "swarm-intelligence"} {
		if !tutorial.IsBuiltInQuery(query) {
			t.Errorf("IsBuiltInQuery(%q) = false, want true", query)
		}
	}
	for _, query := range []string{"how do I pass a timeout", "which model should I use", "can claude answer this"} {
		if tutorial.IsBuiltInQuery(query) {
			t.Errorf("IsBuiltInQuery(%q) = true, want false", query)
		}
	}
}

func TestIsModelSuggestionQuery(t *testing.T) {
	for _, query := range []string{"which model should I use", "suggest a cheap model", "recommend model for review", "compatible model for gemini"} {
		if !tutorial.IsModelSuggestionQuery(query) {
			t.Errorf("IsModelSuggestionQuery(%q) = false, want true", query)
		}
	}
	for _, query := range []string{"how do I pass a timeout", "what are the rules", "phase 1"} {
		if tutorial.IsModelSuggestionQuery(query) {
			t.Errorf("IsModelSuggestionQuery(%q) = true, want false", query)
		}
	}
}
