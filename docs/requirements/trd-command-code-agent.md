# TRD: Command-Code Agent Integration

> **Status**: draft
> **Owner**: user-requested
> **Created**: 2026-05-07
> **Swarm Inputs**: `ses_2019d529fffe1u1tS4xvWnKM6w`

## Parent PRD

Direct user request — add `command-code` as a supported execution engine with headless invocation semantics, while acknowledging that model selection remains owned by the agent’s interactive session.

## Technical Overview

Swarminator already supports multiple execution transports: kilo one-shot, Gemini headless, Claude ACP, and Codex JSONL execution. Command Code fits the same architectural slot as Codex: a non-ACP, one-shot CLI adapter with engine-specific behavior.

The key behavioral difference is model control. Swarminator should accept the normal `-m` flag for interface consistency, but the command-code adapter should not treat it as an authoritative runtime override unless the upstream CLI gains reliable support. That constraint must be explicit in docs, error handling, and the embedded guidance layer.

## Architecture Decisions

### ADR-1: Add command-code as an explicit-only engine

- **Context**: Command Code does not map cleanly onto static provider prefixes and the broader repo is moving toward explicit engine choice.
- **Decision**: Register `command-code` with binary `cmd` and require `--agent=command-code`.
- **Rationale**: Matches current explicit-engine execution model and avoids misleading routing behavior.
- **Alternatives Considered**:
  - Auto-route `cmd/` or `commandcode/` model prefixes — rejected because model selection is not actually controlled by swarminator.

### ADR-2: Treat `-m` as advisory, not authoritative

- **Context**: The user explicitly states command-code uses the last interactive model and cannot be configured here.
- **Decision**: Accept `-m` at the swarminator layer but ignore it in the provider implementation, optionally emitting advisory feedback.
- **Rationale**: Keeps CLI shape consistent without pretending runtime control exists.
- **Alternatives Considered**:
  - Reject `-m` when `--agent=command-code` — rejected because `-m` is globally required today and changing that would complicate parser semantics.

## System Components

- **Registry entry**: add command-code to `pkg/llm/registry.go:246`.
- **Provider implementation**: new `pkg/llm/commandcode.go` alongside `kilo.go`, `gemini.go`, and `codex.go`.
- **Unified factory wiring**: add factory + switch case in `pkg/llm/unified.go:127`.
- **CLI/help/docs**: update `cmd/swarminator/main.go:215`, README agent tables, and generated CLI docs.
- **Project-local hint**: `.commandcode/taste/taste.md:1` confirms the repo already carries Command Code taste data.

## API Contracts / Interfaces

### Provider execution contract

```text
Binary: cmd
Invocation shape: cmd --print "<persona+input>"

Input:
  - persona+input: formatted as existing providers do
  - model: accepted by swarminator but ignored by provider

Output:
  - stdout: plain text response
  - stderr/non-zero exit: wrapped as provider failure
```

### Unsupported options

```text
--agent-mode
  -> reject for command-code with same style used for kilo/codex non-ACP agents
```

## Data Models

### Agent registry entry

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `Name` | string | `command-code` | engine identifier used by `--agent` |
| `Binary` | string | `cmd` | executable name |
| `ACPArgs` | []string | empty | no ACP transport |
| `ModelPrefixes` | []string | empty | explicit-only engine |

## Security Assessment

### Authentication & Authorization

- Detection should remain a `--version` probe only, mirroring other agents, to avoid auth prompts.
- Runtime auth remains delegated to Command Code.

### Data Protection

- Do not log interactive session state or taste artifacts.
- Advisory messages must not expose local config internals.

### Input Validation & Injection Prevention

- Invoke `cmd` with argument arrays only.
- Do not splice persona/input into shell strings.

### Infrastructure & Configuration

- Provider must respect context cancellation and use existing process wait timeout patterns.
- No background sessions or daemon assumptions.

### Supply Chain & Dependencies

- No new dependency beyond the installed `cmd` binary.

### Failure Modes

- Empty output + non-zero exit is a hard failure.
- Unsupported `--agent-mode` fails closed with a clear message.

## Non-Functional Requirements

- **Performance**: parity with other one-shot providers.
- **Scalability**: adding the provider must not require protocol changes to the rest of the engine stack.
- **Observability**: errors should clearly identify `cmd` as the failing binary.
- **Reliability**: provider must terminate cleanly on context cancellation.

## Acceptance Criteria

- `--list-agents` includes `command-code` with explicit-only notes.
- `--agent=command-code` invokes a dedicated provider implementation.
- `--agent-mode` is rejected for command-code.
- `-m` remains accepted globally but is not forwarded as a controlling runtime flag for command-code.
- Help/docs clearly state that command-code uses the last model from interactive state.

## Implementation Plan

1. Add `command-code` entry to `pkg/llm/registry.go`.
2. Create `pkg/llm/commandcode.go` using the Codex/Kilo one-shot execution pattern.
3. Wire factory and switch support into `pkg/llm/unified.go`.
4. Update CLI help, README, and generated docs.
5. Add tests for startup failure, cancellation, unsupported `--agent-mode`, and ignored model semantics.
6. Follow up with listing support only if request 2 lands.

## Child BDD Specs

- Deferred until command-code CLI behavior is verified against a real installed binary.

## ⚔ Challenge Gate

> **Status**: pending
> **Challenger**: swarm synthesis + self-challenge
> **Date**: 2026-05-07

### Debate Record

| # | Vector | Challenge | Response | Verdict |
|---|--------|-----------|----------|---------|
| 1 | evidence | Are we certain `cmd --print` is the stable headless contract? | Swarm recommendation is aligned with public docs, but real-binary verification remains required. | escalated |
| 2 | usability | Will users be confused that `-m` is accepted but ignored? | Docs and feedback must state this explicitly; keeping CLI shape consistent is still preferable. | author-won |

### Challenge Summary

- **Challenges raised**: 2
- **Author victories**: 1
- **Challenger victories**: 0
- **Escalated**: 1
- **Overall verdict**: ACCEPTED pending binary verification
