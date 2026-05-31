# Swarminator CLI Reference

> Generated from source. Do not edit manually.

## Usage

```
cat input.txt | swarminator swarm exec --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]
cat input.txt | swarminator swarm start --swarm-root PATH --orchestrator NAME --run-dir PATH [--event-sink file:///PATH]
swarminator runs final --run-dir PATH
swarminator runs inspect --run-dir PATH
swarminator runs tail --run-dir PATH
swarminator runs wait --run-dir PATH
cat input.txt | swarminator --agent NAME -m MODEL -p PERSONA -t SECONDS [OPTIONS]
swarminator --list-agents
swarminator --list-models [--agent NAME] [--json]
swarminator --list-providers [--agent NAME] [--json]
swarminator --tutorial TOPIC
swarminator --tutorial QUESTION --agent NAME -m MODEL
swarminator --tutorial "suggest a cheap model for TASK" --agent NAME
swarminator --phases
swarminator --protocol
swarminator --help
```

## Starting Points

- Private swarm run: `swarminator swarm exec --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123`
- Async private run: `swarminator swarm start --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123`
- Inspect a run: `swarminator runs inspect --run-dir /tmp/run-123`
- Single node execution: `swarminator --tutorial quickstart`
- Multi-node swarm guidance: `swarminator --tutorial swarm`

## Private Swarm Protocol

- The private orchestrator uses an OpenAI-compatible chat-completions transport configured from the XDG orchestrator profile.
- Kilo gateway profiles typically use `backend: openai-compatible`, `message_api_format: openai.chat.completions`, `base_url_ref: KILO_BASE_URL`, `auth.credential_ref: KILO_API_KEY`, and optional `env_file` plus `timeout_seconds`.
- The private orchestrator uses transport-native tools for external actions such as worker-node execution, and those tool schemas are generated from the discovered worker model/persona catalog.
- Final answers are plain-text Markdown and are printed directly to stdout by `swarm exec`.
- Worker results are returned to the orchestrator as readable Markdown tool results with artifact references.
- Worker results are written to `nodes/` and also fed back into the next orchestrator turn as readable Markdown plus artifact references.

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--swarm-root PATH` | Yes (swarm exec/start) | Swarm configuration root; expects `models/` and `personas/` directories for private runtime lookup. |
| `--orchestrator NAME` | Yes (swarm exec/start) | Orchestrator model ID to load from `--swarm-root/models`. |
| `--run-dir PATH` | Yes (swarm exec/start, runs *) | Private run directory; created with `0700` permissions and used for status, events, transcript, memory, nodes, and final output. |
| `--event-sink file:///PATH` | No (swarm exec/start) | Optional events JSONL file sink. Must stay within `--run-dir`. |
| `-m MODEL` | Yes (node) | Model identifier, e.g. `google/gemini-2.5-flash`, `github-copilot/gpt-5-mini` |
| `-p PERSONA` | Yes (node) | Full system persona prompt text for the node; controls behavior and expected response style |
| `-t SECONDS` | Yes (node) | Timeout in seconds (must be > 0); use larger values for reasoning-heavy nodes |
| `--agent=NAME` | Yes (node) | Required for node execution. Select the agent binary (`kilo`, `gemini`, `claude`, `codex`, `command-code`). Fails if unknown or unavailable. |
| `--agent-mode=MODE` | No | Set the underlying agent session mode (ACP agents only: gemini, claude). Gemini values: default, autoEdit, yolo, plan. For gemini, autoEdit selects the gemini CLI auto_edit mode. Not supported for kilo, codex, or command-code. |
| `--feedback=stderr` | No | Emit advisory feedback to stderr |
| `--dry-run` | No | Preflight: validate input, validate explicit agent, print envelope; no LLM call |
| `--list-agents` | No | Print all known agents with status and model prefixes; does not require `-m`, `-p`, `-t`, or stdin |
| `--list-models` | No | Print models grouped by engine/provider; optionally filter with `--agent`; supports `--json` |
| `--list-providers` | No | Print providers grouped by engine/provider; optionally filter with `--agent`; supports `--json` |
| `--json` | No | Emit JSON output for `--list-models` or `--list-providers` |
| `--tutorial TOPIC` | No | Print embedded tutorial content or run explicit-agent tutorial Q&A (see Tutorial Mode) |
| `--phases` | No | Print the intent-to-phase map and exit |
| `--protocol` | No | Print the lightweight envelope protocol and exit |
| `--help` | No | Print this help and exit |

## Tutorial Mode

Tutorial mode has two paths:

- Built-in topics: `quickstart`, `rules`, `protocol`, `quorum`, `safety`, `swarm`, plus phase/protocol/rules heuristics. These print embedded guidance directly.
- Freeform Q&A: requires explicit `--agent=NAME` and `-m MODEL`. There is no global default kilo fallback.
- Agent-scoped model suggestion Q&A: requires `--agent=NAME`; this is the only tutorial Q&A case where `-m MODEL` is optional, and when omitted swarminator tries to infer a cheap default for that agent.
- Context: embedded generated CLI reference plus agent-scoped discovery data when available.
- Timeout: 120 seconds.

The `swarm` topic is different: `--tutorial swarm` is the canonical built-in agent guide, and `--tutorial swarm-intelligence` is an alias. The swarm guide bypasses tutorial Q&A and prints embedded guidance plus agent-scoped hints when available.

### Static Tutorial Topics

#### `full`

# Swarminator Tutorial

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
- --protocol

#### `quickstart`

# Quick Start

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
  swarminator --phases

#### `rules`

# Embedded Rules

Hard rules:
- no inline credentials in persona or input
- timeout must fail closed if unsupported
- stdin must be non-empty for node execution
- explicit `--agent=NAME` is required for node execution
- selected agent must be installed and authenticated
- node execution still requires non-empty stdin, model, persona, timeout > 0

Rule violations exit with code 3.

#### `protocol`

# KS Envelope

Optional headers followed by free-form body:

ROLE: <role>
INTENT: <intent>
TARGET: <target>

<free-form body>

This is human-readable and easy for the orchestrator to merge.

#### `quorum`

# Quorum

For swarm orchestration, treat quorum as a per-persona policy rather than a single global vote.

- Run each required persona on 2-3 models from different provider families when possible.
- Use a stronger model for Synthesis, Review, and complex Maker passes.
- Retry a failed persona with a different-family model before degrading or stopping.
- Stop the swarm if you cannot assemble a valid quorum for a required phase.

#### `safety`

# Safety

- Never inline credentials in persona or input.
- Prefer environment variables for model access.
- Treat stderr feedback as advisory unless exit code is 3.
- Exit 2 means retryable failure.
- Exit 3 means prompt/policy violation.

## Embedded Agent Guide

`--tutorial swarm` and `--tutorial swarm-intelligence` print the built-in agent guide.
Model hints are agent-scoped: pass `--agent=NAME` to narrow the guide to one agent before asking for model suggestions.

# Swarm-Intelligence Guide

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

### Examples

```
printf 'hello' | swarminator swarm exec --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123
printf 'hello' | swarminator swarm start --swarm-root ./swarm --orchestrator main --run-dir /tmp/run-123
swarminator runs wait --run-dir /tmp/run-123
swarminator runs inspect --run-dir /tmp/run-123
swarminator --tutorial quickstart
swarminator --tutorial rules
swarminator --tutorial swarm
swarminator --tutorial swarm-intelligence
swarminator --tutorial "how do I pass a timeout?" --agent=gemini -m google/gemini-2.5-flash
swarminator --tutorial "suggest a cheap model for code review" --agent=gemini
printf 'hello' | swarminator --agent=kilo -m kilo/kilo-auto/free -p 'You are a reviewer.' -t 60
```

## Agents and Model Routing

swarminator requires an explicit agent for node execution. Automatic model-prefix routing
is not used to select the execution binary. Known agents:

| Agent | Binary | Model Prefixes | Notes |
|-------|--------|----------------|-------|
| `kilo` | `kilo` | kilo/, minimax/, openai/, github-copilot/, openrouter/, o1-, o3-, gpt-, codex- |  |
| `gemini` | `gemini` | google/, gemini/, gemini- | headless one-shot execution (no ACP) |
| `codex` | `codex` |  | explicit-only (`--agent=codex`); no automatic prefix routing |
| `command-code` | `cmd` |  | explicit-only (`--agent=command-code`); one-shot CLI execution |
| `claude` | `claude` | claude/, anthropic/, sonnet- | ACP agent |

Execution model: Gemini uses headless one-shot CLI execution. Claude uses ACP.
The `kilo` agent is a gateway that handles GPT/OpenAI-family prefixes.
The `command-code` agent is explicit-only and does one-shot CLI execution.

Use `--list-agents` to inspect availability and `--agent=NAME` to choose the execution binary.
Explicit `--agent=NAME` fails with an error listing known agents if NAME is unknown or unavailable.

### kilo model routing

The `kilo` agent is a gateway that internally routes to many model providers beyond
the prefixes listed above. Any model identifier accepted by the kilo CLI can be used.

```
printf 'hello' | swarminator --agent=kilo -m kilo/grok-3         -p "..." -t 60   # xAI Grok via kilo
printf 'hello' | swarminator --agent=kilo -m kilo/kilo-auto/free -p "..." -t 60   # kilo default free model
printf 'hello' | swarminator --agent=command-code -m some-model -p "..." -t 60
```

Run `kilo models` (if available) to list all model identifiers your kilo installation supports.

**Tutorial Q&A mode** uses the explicit `--agent` selected by the caller.
Freeform Q&A requires `-m MODEL`, except model-suggestion questions where swarminator may infer a cheap default for the chosen agent.
There is no global default kilo fallback for tutorial Q&A.

### Preflight and introspection

```
swarminator --list-agents
swarminator --list-models --agent kilo --json
swarminator --list-providers
printf 'hello' | swarminator --agent=gemini -m google/gemini-2.5-flash -p 'You are a researcher.' -t 60 --dry-run
printf 'hello' | swarminator --agent=kilo -m github-copilot/gpt-5-mini -p 'You are a spec writer.' -t 60 --dry-run
```

## Rules and Exit Codes

Hard rules are enforced before any LLM call:

- Model, persona, and non-empty stdin are required.
- Timeout must be > 0.
- Inline credentials (`api_key`, `secret`, `password`, `token`) are rejected in persona and input.
- Node personas containing `run shell` or `modify files` trigger an advisory (not a violation).

| Exit Code | Constant | Meaning |
|-----------|----------|---------|
| `0` | `ExitSuccess` | Success |
| `2` | `ExitRetryable` | Retryable failure (network, timeout, rate limit) |
| `3` | `ExitRuleViolation` | Rule or policy violation; do not retry unchanged |

## Protocol Envelope

# Lightweight Protocol

Recommended envelope:

ROLE: <role>
INTENT: <intent>
TARGET: <target>

<free-form body>

Headers are optional. Plain text body is the primary payload.

## Phase Map

# Intent To Phase Map

INIT      -> bootstrap and validation
EXTRACT   -> gather or audit
CHALLENGE -> adversarial review
FORWARD   -> synthesis and prioritization
REVIEW    -> approve or reject
DECOMPOSE -> split into tasks
MAKE      -> produce candidate output
BREAK     -> QA and challenge output
MERGE     -> combine or preserve disagreement
FINALIZE  -> emit final node response
