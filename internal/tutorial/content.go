package tutorial

import "strings"

var topics = map[string]string{
	"full": `# Swarminator Tutorial

Swarminator is a Go-based swarm node runner with:
- deterministic safety rules enforced in code
- Gemini headless execution
- Claude ACP support
- kilo multi-provider routing/gateway
- Codex explicit-only harness
- tutorial mode for self-documentation
- lightweight role/intent envelopes instead of heavy schemas

Core workflow:
1. Orchestrator selects model + persona.
2. Swarminator validates hard rules.
3. Swarminator runs a node request.
4. Orchestrator merges outputs and manages consensus.

Use:
- --tutorial quickstart
- --tutorial rules
- --phases
- --protocol`,
	"quickstart": `# Quick Start

Run a Gemini headless node:
  cat input.txt | swarminator -m google/gemini-2.5-flash -p "You are a reviewer" -t 60

Run a kilo-routed GPT/OpenAI-family node:
  cat input.txt | swarminator -m github-copilot/gpt-5-mini -p "You are a reviewer" -t 60

Preflight check:
  swarminator --list-agents
  printf 'hello' | swarminator -m google/gemini-2.5-flash -p 'You are a reviewer.' -t 60 --dry-run

Read tutorial:
  swarminator --tutorial rules

Read protocol:
  swarminator --protocol

Read phase map:
  swarminator --phases`,
	"rules": `# Embedded Rules

Hard rules:
- no inline credentials in persona or input
- timeout must fail closed if unsupported
- stdin must be non-empty for node execution
- selected agent must be installed and authenticated
- unknown provider-style prefixes fail during routing
- node execution still requires non-empty stdin, model, persona, timeout > 0

Rule violations exit with code 3.`,
	"protocol": `# KS Envelope

Optional headers followed by free-form body:

ROLE: reviewer
INTENT: challenge
TARGET: skill-review

<free-form body>

This is human-readable and easy for the orchestrator to merge.`,
	"quorum": `# Quorum

Phase 1 commonly uses two models and proceeds on either consensus or explicit conflict.
Disagreement is not fatal by default; the orchestrator may elevate it to review/synthesis.`,
	"safety": `# Safety

- Never inline credentials in persona or input.
- Prefer environment variables for model access.
- Treat stderr feedback as advisory unless exit code is 3.
- Exit 2 means retryable failure.
- Exit 3 means prompt/policy violation.`,
}

const phaseMap = `# Intent To Phase Map

INIT      -> bootstrap and validation
EXTRACT   -> gather or audit
CHALLENGE -> adversarial review
FORWARD   -> synthesis and prioritization
REVIEW    -> approve or reject
DECOMPOSE -> split into tasks
MAKE      -> produce candidate output
BREAK     -> QA and challenge output
MERGE     -> combine or preserve disagreement
FINALIZE  -> emit final node response`

const protocol = `# Lightweight Protocol

Recommended envelope:

ROLE: <role>
INTENT: <intent>
TARGET: <target>

<free-form body>

Headers are optional. Plain text body is the primary payload.`

// TopicEntry holds the key and text of a single tutorial topic.
type TopicEntry struct {
	Key  string
	Text string
}

// Topics returns all topic keys in a stable order.
func Topics() []string {
	return []string{"full", "quickstart", "rules", "protocol", "quorum", "safety"}
}

// TopicText returns the text for the given topic key, or an empty string if not found.
func TopicText(key string) string {
	return topics[key]
}

// ReferenceTopics returns all tutorial topics in stable order as TopicEntry values.
func ReferenceTopics() []TopicEntry {
	keys := Topics()
	out := make([]TopicEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, TopicEntry{Key: k, Text: topics[k]})
	}
	return out
}

func Lookup(query string) string {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		q = "full"
	}
	if text, ok := topics[q]; ok {
		return text
	}
	if strings.HasPrefix(q, "phase=") {
		return phaseMap
	}
	if strings.Contains(q, "phase") {
		return phaseMap
	}
	if strings.Contains(q, "protocol") || strings.Contains(q, "envelope") {
		return protocol
	}
	if strings.Contains(q, "rule") || strings.Contains(q, "safety") {
		return topics["rules"]
	}
	return `# Swarminator Tutorial

Topics:
- full
- quickstart
- rules
- protocol
- quorum
- safety

Examples:
  swarminator --tutorial quickstart
  swarminator --tutorial rules`
}

func Phases() string {
	return phaseMap
}

func Protocol() string {
	return protocol
}
