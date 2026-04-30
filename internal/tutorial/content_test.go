package tutorial_test

import (
	"testing"

	"swarminator/internal/tutorial"
)

func TestTopicsReturnKnownKeys(t *testing.T) {
	topics := tutorial.Topics()
	expected := map[string]bool{
		"full": true, "quickstart": true, "rules": true,
		"protocol": true, "quorum": true, "safety": true,
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
