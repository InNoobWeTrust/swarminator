package main

import (
	"strings"
	"testing"
)

func TestGenerateDeterministic(t *testing.T) {
	out1 := generate()
	out2 := generate()
	if out1 != out2 {
		t.Fatal("generate() is not deterministic: two calls produced different output")
	}
}

func TestGenerateKeyContent(t *testing.T) {
	out := generate()
	for _, want := range []string{
		"# Swarminator CLI Reference",
		"## Usage",
		"--agent NAME -m MODEL -p PERSONA -t SECONDS",
		"## Starting Points",
		"## Flags",
		"## Tutorial Mode",
		"There is no global default kilo fallback",
		"suggest a cheap model for code review",
		"## Agents and Model Routing",
		"--list-models",
		"swarm-intelligence",
		"## Embedded Agent Guide",
		"One swarminator call = one node",
		"Live Agent+Model Suggestions",
		"Minimal Swarm Example",
		"## Rules and Exit Codes",
		"ExitSuccess",
		"ExitRetryable",
		"ExitRuleViolation",
		"## Protocol Envelope",
		"## Phase Map",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generate() output missing expected content: %q", want)
		}
	}
}

func TestGenerateNoTimestamps(t *testing.T) {
	out := generate()
	// A naive but effective check: generated docs must not contain date-like patterns
	// such as "2026" or "2025" which would indicate a runtime timestamp.
	for _, ts := range []string{"2025", "2026", "Generated at", "Date:"} {
		if strings.Contains(out, ts) {
			t.Errorf("generate() output contains potential timestamp string: %q", ts)
		}
	}
}
