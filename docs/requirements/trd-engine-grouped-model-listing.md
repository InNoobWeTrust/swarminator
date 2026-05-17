# TRD: Engine-Grouped Model And Provider Listing

> **Status**: draft
> **Owner**: user-requested
> **Created**: 2026-05-07
> **Swarm Inputs**: `ses_2019d52a3ffeMBtr0KlPAgVgQx`

## Parent PRD

Direct user request — expose configured providers and models per supported engine, grouped by engine, with graceful fallback when an agent CLI cannot list them directly.

## Technical Overview

Swarminator currently detects installed agents and prints status/prefix metadata via `--list-agents`, but it does not expose actual configured providers or available models. That blocks orchestrators from selecting models based on what the user has really configured.

This feature adds a read-only discovery mode that enumerates providers/models by engine. The design should prefer agent-native listing commands where available, fall back to trusted APIs or embedded reference data where necessary, and preserve a stable machine-readable output contract.

## Architecture Decisions

### ADR-1: Introduce a separate listing capability instead of overloading `Provider`

- **Context**: Current `Provider` only supports text completion in `pkg/llm/provider.go:12`.
- **Decision**: Add a parallel lister capability (`AgentLister`) rather than changing `Provider` semantics directly.
- **Rationale**: Keeps execution and discovery concerns separate while remaining extensible.
- **Alternatives Considered**:
  - Extend `Provider` with listing methods — rejected because it forces all providers to implement non-execution concerns immediately.
  - Keep all listing logic in `main.go` — rejected because agent-specific knowledge belongs beside providers.

### ADR-2: Group output by engine first, provider second

- **Context**: The user wants model presentation anchored to execution engines (`kilo`, `claude`, `codex`, `command-code`, `gemini`).
- **Decision**: Use engine as the top-level grouping key in both text and JSON outputs.
- **Rationale**: Aligns with explicit `--agent` execution and avoids blending provider identity with transport identity.
- **Alternatives Considered**:
  - Group by provider globally — rejected because a provider may be reachable through different engines or not at all.

### ADR-3: Use tiered fallback instead of pretending every engine can self-enumerate

- **Context**: Kilo can likely enumerate rich model catalogs; Gemini/Claude/Codex CLIs may not.
- **Decision**: Use a tiered source order: native CLI → trusted API/docs helper → embedded reference → explicit “unavailable”.
- **Rationale**: Preserves correctness without inventing unsupported agent behaviors.
- **Alternatives Considered**:
  - Require native listing support everywhere — rejected because it would block the feature for most engines.

## System Components

- **CLI parser**: add listing flags in `internal/cli/args.go`.
- **Main dispatcher**: add early-exit handlers alongside `runListAgents()` in `cmd/swarminator/main.go:45`.
- **Agent registry**: still supplies availability and grouping metadata in `pkg/llm/registry.go:13` and `pkg/llm/registry.go:246`.
- **Lister abstraction**: new package-level interface paired with provider implementations.
- **Embedded reference layer**: new static fallback data for engines that cannot self-enumerate.

## API Contracts / Interfaces

### CLI surface

```text
swarminator --list-models [--agent=NAME] [--json]
swarminator --list-providers [--agent=NAME] [--json]

Input:
  - agent: optional filter for one engine
  - json: optional machine-readable output mode

Output:
  - text: grouped by engine
  - json: object keyed by engine name
```

### Agent lister interface

```go
type ModelInfo struct {
    ID          string
    DisplayName string
    Provider    string
    Context     int
    Source      string
}

type ProviderInfo struct {
    ID            string
    DisplayName   string
    Authenticated bool
    Source        string
}

type AgentLister interface {
    ListModels(ctx context.Context) ([]ModelInfo, error)
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
}
```

## Data Models

### Engine-grouped JSON response

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `engine` | string | known agent name | top-level grouping key |
| `providers[]` | array | optional | providers visible through that engine |
| `models[]` | array | optional | models visible through that engine |
| `source` | string | `cli`/`api`/`embedded`/`none` | provenance of listing data |

## Security Assessment

### Authentication & Authorization

- Listing must not trigger interactive login flows.
- Authentication state should be reported passively from detection/agent-native read-only commands.

### Data Protection

- Do not print tokens, local config paths with secrets, or provider credentials.
- When using APIs, keep auth material in environment variables only.

### Input Validation & Injection Prevention

- `--agent` filter must be validated against known agents.
- Any external command invocation must use argument arrays, never shell interpolation.

### Infrastructure & Configuration

- Hard timeouts are required for external listing commands and API probes.
- Listing failures must degrade safely to embedded data or explicit “unsupported”.

### Supply Chain & Dependencies

- Prefer existing standard library HTTP/JSON support; avoid large new dependencies.
- If external APIs are queried, document provider-specific auth requirements.

### Failure Modes

- Fail soft per engine, not globally, unless the CLI contract itself is invalid.
- JSON schema should remain stable even when some engines have no data.

## Non-Functional Requirements

- **Performance**: complete within 10 seconds worst case for all engines.
- **Scalability**: adding `command-code` should only require registry + lister wiring.
- **Observability**: each engine entry should indicate data source and degraded state.
- **Reliability**: partial results are acceptable; hangs are not.

## Acceptance Criteria

- `--list-models` and `--list-providers` exist and are mutually compatible with `--json`.
- Default output is grouped by engine, not by provider.
- `--agent=NAME` filters to one engine when listing.
- Kilo path uses a richer source when available; non-kilo engines degrade gracefully.
- JSON output contains explicit provenance per engine.
- No listing path blocks on auth prompts or unbounded network calls.

## Implementation Plan

1. Add `ListModels`, `ListProviders`, and `JSONOutput` flags to `internal/cli/args.go`.
2. Add `runListModels()` and `runListProviders()` dispatch paths in `cmd/swarminator/main.go`.
3. Introduce `pkg/llm/lister.go` with `AgentLister`, `ModelInfo`, and `ProviderInfo`.
4. Implement kilo lister first using agent-native capabilities.
5. Add fallback implementations for Gemini, Claude, and Codex using trusted API/docs or embedded data.
6. Add stable text + JSON formatters keyed by engine.
7. Update generated docs and README examples.
8. Add tests for filtering, grouping, fallback provenance, and partial failure behavior.

## Child BDD Specs

- Deferred until CLI contract shape is approved.

## ⚔ Challenge Gate

> **Status**: pending
> **Challenger**: swarm synthesis + security lens
> **Date**: 2026-05-07

### Debate Record

| # | Vector | Challenge | Response | Verdict |
|---|--------|-----------|----------|---------|
| 1 | evidence | Can every engine truly self-enumerate models? | No; the design explicitly uses tiered fallback and provenance markers. | author-won |
| 2 | longevity | Will provider-grouped output drift from explicit-engine execution semantics? | Grouping by engine avoids that drift and matches `--agent` execution. | author-won |
| 3 | scope | Is `--list-providers` enough, or is `--list-models` required separately? | Keep both commands for clarity even if they share an implementation core. | escalated |

### Challenge Summary

- **Challenges raised**: 3
- **Author victories**: 2
- **Challenger victories**: 0
- **Escalated**: 1
- **Overall verdict**: REVISE naming before backlog entry
