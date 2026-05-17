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
  cat input.txt | swarminator --agent=gemini -m google/gemini-2.5-flash -p "You are a reviewer" -t 60

Run a kilo default free node:
  cat input.txt | swarminator --agent=kilo -m kilo/kilo-auto/free -p "You are a reviewer" -t 60

Run a kilo-routed GPT/OpenAI-family node:
  cat input.txt | swarminator --agent=kilo -m github-copilot/gpt-5-mini -p "You are a reviewer" -t 60

Preflight check:
  swarminator --list-agents
  swarminator --list-models --agent kilo
  printf 'hello' | swarminator --agent=gemini -m google/gemini-2.5-flash -p 'You are a reviewer.' -t 60 --dry-run

Read tutorial:
  swarminator --tutorial rules

Read protocol:
  swarminator --protocol

Read phase map:
  swarminator --phases`,
	"rules": "# Embedded Rules\n\n" +
		"Hard rules:\n" +
		"- no inline credentials in persona or input\n" +
		"- timeout must fail closed if unsupported\n" +
		"- stdin must be non-empty for node execution\n" +
		"- explicit `--agent=NAME` is required for node execution\n" +
		"- selected agent must be installed and authenticated\n" +
		"- node execution still requires non-empty stdin, model, persona, timeout > 0\n\n" +
		"Rule violations exit with code 3.",
	"protocol": `# KS Envelope

Optional headers followed by free-form body:

ROLE: reviewer
INTENT: challenge
TARGET: skill-review

<free-form body>

This is human-readable and easy for the orchestrator to merge.`,
	"quorum": `# Quorum

For swarm orchestration, treat quorum as a per-persona policy rather than a single global vote.

- Run each required persona on 2-3 models from different provider families when possible.
- Use a stronger model for Synthesis, Review, and complex Maker passes.
- Retry a failed persona with a different-family model before degrading or stopping.
- Stop the swarm if you cannot assemble a valid quorum for a required phase.`,
	"safety": `# Safety

- Never inline credentials in persona or input.
- Prefer environment variables for model access.
- Treat stderr feedback as advisory unless exit code is 3.
- Exit 2 means retryable failure.
- Exit 3 means prompt/policy violation.`,
	"swarm": `# Swarm-Intelligence Guide

Use this topic when an orchestrating agent or caller is told to coordinate work with swarminator.
For a single one-off node, prefer swarminator --tutorial quickstart.

Canonical topic: swarminator --tutorial swarm.
Alias: swarminator --tutorial swarm-intelligence.

## Core Rules

1. One swarminator call = one node = one model + one persona prompt.
2. Nodes are read-only. They inspect context and return text; they do not edit files directly.
3. For automation where PATH or shell startup files matter, invoke swarminator from a login shell: $SHELL -l -c '... swarminator ...'. Direct interactive execution is fine when PATH is already correct.
4. For code-making swarms, Maker outputs should be diff blocks that the orchestrator reviews before applying.
5. Run preflight first. Abort on hard-rule violations such as missing required flags, empty stdin for node runs, or inline credentials in persona/input. See swarminator --tutorial rules.

## Orchestrator Preflight

1. Confirm the user actually wants multi-agent swarm orchestration.
2. Clarify the primary domain and the final deliverable shape.
3. Verify swarminator is available. For automation, prefer: $SHELL -l -c 'command -v swarminator'.
4. Inspect the CLI surface: $SHELL -l -c 'swarminator --help'.
5. List engines: $SHELL -l -c 'swarminator --list-agents'.
6. Inspect models: $SHELL -l -c 'swarminator --list-models [--agent NAME]'.
7. Decide the agent+model set and the full persona prompt text before the first node run.

## Node Interface

- Required flags: -m MODEL_ID, -p "FULL PERSONA PROMPT TEXT", -t SECONDS, --agent=NAME.
- -p must contain the full literal persona prompt text, not just an ID or short label.
- Pipe the task or document on stdin.
- Use --dry-run before the real node call when validating routing and the envelope.
- ACP-only agent modes exist for gemini and claude; inspect them with swarminator --tutorial "agent modes".

## Repeated Command Pattern

The orchestrator repeats one swarminator node call per persona+model pair, then merges outputs outside swarminator.

Example automation-safe node call:
$SHELL -l -c 'printf "%s" "$TASK" | swarminator --agent=AGENT -m MODEL -p "FULL PERSONA PROMPT TEXT" -t 120'

Use the login-shell wrapper when the environment depends on shell startup files to expose agent CLIs. For a multi-model pass, repeat that command for each chosen model, collect the outputs, then synthesize or review them before the next phase.

## Three-Phase Workflow

### Phase 1 - Gather Inputs
Run one Ingest persona and one Analysis persona across 2-3 models each. Merge the outputs outside swarminator into a phase summary with goals, constraints, findings, and open questions.

### Phase 2 - Synthesize and Challenge
Run Synthesis personas, then Review personas. If reviewers find critical gaps, run Synthesis-Revise and repeat the review loop.

### Phase 3 - Decompose and Produce
Run Decompose, then Maker per task, then Breaker per task. If a task fails review, run Maker-Fix and retry.

## Persona Groups

- Ingest: business-analyst, technical-documentation-auditor, audience-analyst, communications-strategist, ux-researcher, product-researcher.
- Analysis: adversarial-reviewer, technical-analyst, business-analyst-pm, content-analyst, design-systems-analyst.
- Synthesis: solution-architect, content-strategist, senior-ux-designer, review-plan-synthesizer, senior-product-manager.
- Review: qa-engineer, plan-challenger, critical-stakeholder-reviewer, senior-editor, usability-and-accessibility-specialist.
- Decompose: technical-lead, review-lead, content-lead, design-lead, product-lead.
- Maker: code-maker, technical-writer-finding, feature-writer, slide-content-writer, component-designer.
- Breaker: code-breaker, qa-reviewer-finding, design-quality-reviewer, product-spec-reviewer, slide-quality-reviewer, copy-editor.
- Maker-Fix: code-maker-fix, technical-writer-finding-fix, writer-fix, component-designer-fix.
- Synthesis-Revise: solution-architect-reviser, review-plan-reviser, outline-reviser, strategist-reviser.
- Escalation: senior-reviewer after repeated review rejection.

## Persona Prompt Source

- The -p flag is always full caller-supplied prompt text.
- Swarminator does not resolve persona IDs like business-analyst or code-maker automatically.
- For a first run, write the full prompt from the group purpose and required output headings.
- Treat the persona group names below as starter roles, not magic built-in IDs.

## Starter Persona Prompt Patterns

- Ingest starter: "You are an Ingest persona for a swarminator Phase 1 pass. Extract raw requirements, constraints, and context from the input. Return Markdown with: ## Goals And Constraints, ## Key Inputs, ## Extracted Findings, ## Open Questions."
- Analysis starter: "You are an Analysis persona for a swarminator Phase 1 pass. Normalize the inputs, identify assumptions, and surface gaps or contradictions. Return Markdown with: ## Extracted Findings, ## Risks, ## Open Questions."
- Synthesis starter: "You are a Synthesis persona for a swarminator Phase 2 pass. Merge prior outputs into one coherent specification. Return Markdown with: ## Specification Summary, ## Design Decisions, ## Acceptance Criteria, ## Task Decomposition Hints, ## Open Questions."
- Review starter: "You are a Review persona for a swarminator Phase 2 pass. Critique the specification for gaps, contradictions, weak assumptions, and missing acceptance criteria. Return Markdown with clear findings and an approval or rejection recommendation."
- Decompose starter: "You are a Decompose persona for a swarminator Phase 3 pass. Break the approved specification into atomic executable tasks. Return Markdown with a numbered task list, dependencies, and risk notes."
- Maker starter: "You are a Maker persona for a swarminator Phase 3 pass. Produce the requested implementation as Markdown. For code changes, emit diff blocks and explain assumptions briefly."
- Breaker starter: "You are a Breaker persona for a swarminator Phase 3 pass. Validate the Maker output. Return Markdown beginning with: ### Verdict: PASS | NEEDS REVISION | FAIL, then list critical flaws."
- Maker-Fix starter: "You are a Maker-Fix persona for a swarminator Phase 3 repair pass. Revise the prior Maker output to address Breaker findings while preserving the accepted parts."

## Quorum Policy

- Run each required persona on 2-3 models from different provider families when possible.
- Include at least one stronger model for Synthesis, Review, and complex Maker passes.
- Retry a failed persona with a different-family model before degrading or stopping.
- Stop the swarm if you cannot assemble a valid quorum for a required phase.

## Minimal Swarm Example

Below is a copy-pasteable two-node shell example that runs an Ingest node and an Analysis node, then prints their outputs:

  #!/bin/sh
  INGEST_OUT=$(printf '%s' "..." | swarminator --agent=kilo -m kilo/kilo-auto/free -p "You are an Ingest persona. Output: ## Findings, ## Open Questions." -t 60)
  ANALYSIS_OUT=$(printf '%s' "..." | swarminator --agent=kilo -m kilo/kilo-auto/free -p "You are an Analysis persona. Output: ## Risks, ## Gaps." -t 60)
  echo "=== Ingest ==="
  echo "$INGEST_OUT"
  echo "=== Analysis ==="
  echo "$ANALYSIS_OUT"

For a real swarm, repeat that pattern through the three phases, selecting appropriate persona prompts and model+agent combinations for each node.

## Live Agent+Model Suggestions

At runtime, swarminator appends agent-scoped hints. Pass --agent=NAME to narrow guidance to one agent before asking for model suggestions.

## Built-in References

- swarminator --tutorial quickstart
- swarminator --tutorial rules
- swarminator --tutorial quorum
- swarminator --protocol
- swarminator --phases

Do not rely on a hard-coded kilo model list because live engine offerings change over time.
`,
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
	return []string{"full", "quickstart", "rules", "protocol", "quorum", "safety", "swarm"}
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
	q := normalizeQuery(query)
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
- swarm

Examples:
  swarminator --tutorial quickstart
  swarminator --tutorial rules
  swarminator --tutorial swarm`
}

func Phases() string {
	return phaseMap
}

func Protocol() string {
	return protocol
}

func normalizeQuery(query string) string {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return "full"
	}
	switch q {
	case "swarm-intelligence", "skill=swarm-intelligence", "skill=swarm", "agent-guide", "agent guide":
		return "swarm"
	default:
		return q
	}
}

// IsBuiltInQuery reports whether tutorial input should be answered directly from
// embedded content instead of routing through agent-backed Q&A.
func IsBuiltInQuery(query string) bool {
	q := normalizeQuery(query)
	if _, ok := topics[q]; ok {
		return true
	}
	if strings.HasPrefix(q, "phase=") {
		return true
	}
	if strings.Contains(q, "phase") {
		return true
	}
	if strings.Contains(q, "protocol") || strings.Contains(q, "envelope") {
		return true
	}
	if strings.Contains(q, "rule") || strings.Contains(q, "safety") {
		return true
	}
	return false
}

// IsModelSuggestionQuery reports whether tutorial input is asking for model
// selection guidance rather than a concrete how-to answer.
func IsModelSuggestionQuery(query string) bool {
	q := normalizeQuery(query)
	if strings.Contains(q, "which model") || strings.Contains(q, "what model") {
		return true
	}
	if strings.Contains(q, "suggest") && strings.Contains(q, "model") {
		return true
	}
	if strings.Contains(q, "recommend") && strings.Contains(q, "model") {
		return true
	}
	if strings.Contains(q, "compatible model") || strings.Contains(q, "current model") {
		return true
	}
	if strings.Contains(q, "cheap model") || strings.Contains(q, "free model") || strings.Contains(q, "affordable model") || strings.Contains(q, "lowest cost") {
		return true
	}
	return false
}
