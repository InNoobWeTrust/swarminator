# Swarminator CLI Reference

> Generated from source. Do not edit manually.

## Usage

```
cat input.txt | swarminator -m MODEL -p PERSONA -t SECONDS [OPTIONS]
swarminator --tutorial TOPIC_OR_QUESTION [-m MODEL]
swarminator --phases
swarminator --protocol
swarminator --help
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `-m MODEL` | Yes (node) | Model identifier, e.g. `kilo/kilo-auto/free`, `gemini-2.5-flash` |
| `-p PERSONA` | Yes (node) | System persona for the node |
| `-t SECONDS` | Yes (node) | Timeout in seconds (must be > 0) |
| `--agent=NAME` | No | Force a specific agent binary (e.g. `kilo`, `gemini`) |
| `--feedback=stderr` | No | Emit advisory feedback to stderr |
| `--dry-run` | No | Print the protocol envelope and exit without calling an LLM |
| `--tutorial TOPIC` | No | Print tutorial text or ask kilo assistant (see Tutorial Mode); optionally override model with `-m` |
| `--phases` | No | Print the intent-to-phase map and exit |
| `--protocol` | No | Print the lightweight envelope protocol and exit |
| `--help` | No | Print this help and exit |

## Tutorial Mode

When `--tutorial` is given, swarminator first attempts an LLM-assisted answer:

- Agent: `kilo`
- Model: `kilo/kilo-auto/free` (default; override with `-m MODEL`)
- Context: embedded generated CLI reference (this document)
- Timeout: 120 seconds

If `kilo` is not installed, not detected, times out, or returns an error,
tutorial mode falls back to the static topic lookup.

### Static Tutorial Topics

#### `full`

# Swarminator Tutorial

Swarminator is a Go-based swarm node runner with:
- deterministic safety rules enforced in code
- ADK-Go-backed Gemini execution
- tutorial mode for self-documentation
- lightweight role/intention envelopes instead of heavy schemas

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

Run a node:
  cat input.txt | swarminator -m gemini-2.5-flash -p "You are a reviewer" -t 60

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
- gemini execution requires GOOGLE_API_KEY

Rule violations exit with code 3.

#### `protocol`

# KS Envelope

Optional headers followed by free-form body:

ROLE: reviewer
INTENT: challenge
TARGET: skill-review

<free-form body>

This is human-readable and easy for the orchestrator to merge.

#### `quorum`

# Quorum

Phase 1 commonly uses two models and proceeds on either consensus or explicit conflict.
Disagreement is not fatal by default; the orchestrator may elevate it to review/synthesis.

#### `safety`

# Safety

- Never inline credentials in persona or input.
- Prefer environment variables for model access.
- Treat stderr feedback as advisory unless exit code is 3.
- Exit 2 means retryable failure.
- Exit 3 means prompt/policy violation.

### Examples

```
swarminator --tutorial quickstart
swarminator --tutorial rules
swarminator --tutorial "how do I pass a timeout?"
swarminator --tutorial quickstart -m kilo/kilo-auto/free
```

## Agents and Model Routing

UnifiedProvider selects an agent based on model prefix. Known agents:

| Agent | Binary | Model Prefixes |
|-------|--------|----------------|
| `kilo` | `kilo` | kilo/, minimax/ |
| `gemini` | `gemini` | google/, gemini/, gemini- |
| `codex` | `codex` | openai/, o1-, o3-, gpt-, codex- |
| `claude` | `claude` | claude/, anthropic/, sonnet- |

If no prefix matches, UnifiedProvider selects the first available authenticated agent,
then falls back to ADK (Gemini) on rate-limit errors.

### kilo model routing

The `kilo` agent is a gateway that internally routes to many model providers beyond
the `kilo/` and `minimax/` prefixes listed above. Any model identifier accepted by
the kilo CLI can be used by passing it under the `kilo/` namespace. Examples:

```
swarminator -m kilo/grok-3         -p "..." -t 60   # xAI Grok via kilo
swarminator -m kilo/kilo-auto/free -p "..." -t 60   # kilo default free model
```

Run `kilo models` (if available) to list all model identifiers your kilo installation supports.

**Tutorial mode** bypasses UnifiedProvider and calls the `kilo` agent directly
with model `kilo/kilo-auto/free` by default; override with `-m MODEL`.

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
