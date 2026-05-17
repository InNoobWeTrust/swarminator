# TRD: Explicit Agent Selection

> **Status**: verified-current
> **Owner**: user-requested
> **Created**: 2026-05-07
> **Swarm Inputs**: `ses_1fa898194ffeES6kzT0g2X9IXP` + direct repo verification

## Parent PRD

Direct user request — make engine choice explicit and stop depending on model-prefix routing because custom kilo providers break implicit routing.

## Technical Overview

Swarminator should treat the underlying engine as an explicit execution choice, not an inferred routing side effect. This avoids breakage when kilo users define custom provider prefixes that are invisible to swarminator’s static prefix table.

The current branch already implements the core behavior: CLI parsing requires `--agent`, runtime execution rejects missing overrides, and the README/help text describes explicit engine selection. The remaining work is not primary implementation; it is consistency cleanup, regression coverage, and documentation alignment in generated docs.

## Architecture Decisions

### ADR-1: Require explicit `--agent` for node execution

- **Context**: Static prefix routing is brittle when agent ecosystems, especially kilo, allow user-defined providers or broader model namespaces than swarminator can safely hardcode.
- **Decision**: Make `--agent=NAME` mandatory for node execution.
- **Rationale**: Explicit engine choice is deterministic, easier to validate, and decouples model naming from transport selection.
- **Alternatives Considered**:
  - Keep automatic prefix routing — rejected because custom kilo providers break correctness.
  - Add configurable local routing maps — rejected because it recreates agent config inside swarminator.

### ADR-2: Normalize model strings per selected engine

- **Context**: Once engine choice is explicit, model strings no longer need to carry transport semantics uniformly.
- **Decision**: Normalize model strings at the provider boundary per engine.
- **Rationale**: Keeps CLI usage simple while preserving native expectations of each agent binary.
- **Alternatives Considered**:
  - Pass raw model strings unchanged to every engine — rejected because Gemini already needs prefix stripping and other agents may have native-only IDs.
  - Normalize in CLI parsing — rejected because normalization belongs with provider execution semantics.

## System Components

- **CLI parser**: requires `Args.Agent` for node execution in `internal/cli/args.go:126`.
- **Main command help**: documents explicit agent requirement in `cmd/swarminator/main.go:227`.
- **Unified provider**: rejects missing `agentOverride` and normalizes model strings per engine in `pkg/llm/unified.go:108` and `pkg/llm/unified.go:127`.
- **Provider-specific normalization**: Gemini strips `google/` and `gemini/` prefixes in `pkg/llm/gemini.go:31`.
- **Registry**: still contains legacy route logic in `pkg/llm/registry.go:184`, but main execution no longer depends on it.

## API Contracts / Interfaces

### Node execution contract

```text
swarminator --agent=NAME -m MODEL -p PERSONA -t SECONDS

Input:
  - agent: string, required for node execution
  - model: string, required
  - persona: string, required
  - timeout: integer > 0, required

Output:
  - stdout: trimmed agent response on success
  - stderr: actionable validation/runtime errors on failure

Errors:
  - missing agent: CLI parse failure
  - unknown/unavailable agent: unified provider failure
```

### Model normalization contract

```text
kilo         -> keep provider-qualified model IDs unchanged
gemini       -> strip google/ and gemini/ prefixes before invoking CLI
claude       -> currently pass through; review if native-only IDs should be enforced later
codex        -> currently pass through
command-code -> planned future behavior: ignore model override, use agent session state
```

## Data Models

### CLI args

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `Agent` | string | non-empty for node execution | Explicit engine selector |
| `Model` | string | non-empty | Model identifier interpreted by selected engine |

### Execution context

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `agentOverride` | string | required at runtime | Unified provider’s selected engine |
| `normalizedModel` | string | derived | Model string rewritten for provider compatibility |

## Security Assessment

### Authentication & Authorization

- No auth model change. Explicit engine selection reduces accidental calls to unintended providers.
- Authorization remains delegated to installed agent CLIs.

### Data Protection

- No new data storage.
- Error text should not leak local provider credentials or config internals.

### Input Validation & Injection Prevention

- `--agent` must be validated against detected agent names, not interpolated into shell strings.
- Model normalization must be pure string transformation with no shell evaluation.

### Infrastructure & Configuration

- Avoid reintroducing implicit routing through environment-driven fallbacks.
- Keep agent lookup tied to detected binaries only.

### Supply Chain & Dependencies

- No new dependencies required.

### Failure Modes

- Fail closed when `--agent` is missing or unknown.
- Dry-run/help output must not imply routing still occurs automatically.

## Non-Functional Requirements

- **Performance**: no extra runtime overhead beyond current detection and normalization.
- **Scalability**: explicit engine model scales with new providers without hardcoded routing tables.
- **Observability**: error messages must clearly name required agent choices.
- **Reliability**: CLI validation and runtime checks must agree on requirement semantics.

## Acceptance Criteria

- `internal/cli/args.go:132` continues requiring `--agent` for node execution.
- `pkg/llm/unified.go:108` continues rejecting missing agent override with no routing fallback.
- Help/README/generated docs all consistently state that automatic routing is not supported.
- Dry-run output no longer relies on legacy route reasoning for mainline guidance.
- Regression tests cover missing agent, unknown agent, and Gemini normalization.

## Implementation Plan

1. Mark this request as **already implemented in core execution path**.
2. Update stale generated docs in `internal/docs/cli_reference.md:151` to remove automatic-routing language.
3. Review `runDryRun()` in `cmd/swarminator/main.go:153` to ensure it reinforces explicit-engine behavior rather than legacy route reasoning.
4. Add/refresh tests for CLI validation and `normalizeModelForAgent` behavior.
5. When command-code is added, extend all explicit-agent error/help strings to include it.

## Child BDD Specs

- Deferred; existing work mainly needs verification-focused scenarios rather than new behavior design.

## ⚔ Challenge Gate

> **Status**: passed with follow-up docs work
> **Challenger**: swarm synthesis + self-challenge
> **Date**: 2026-05-07

### Debate Record

| # | Vector | Challenge | Response | Verdict |
|---|--------|-----------|----------|---------|
| 1 | scope | Is this still a future spec if the behavior already exists? | Reframed as verification + consistency cleanup, not net-new implementation. | author-won |
| 2 | longevity | Could legacy route helpers accidentally reintroduce implicit behavior in docs or dry-run? | Yes; implementation plan explicitly calls out stale route references. | challenger-won |

### Challenge Summary

- **Challenges raised**: 2
- **Author victories**: 1
- **Challenger victories**: 1
- **Escalated**: 0
- **Overall verdict**: ACCEPTED with follow-up cleanup
