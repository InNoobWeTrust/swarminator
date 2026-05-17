# TRD: Embedded Swarm Guidance

> **Status**: draft
> **Owner**: user-requested
> **Created**: 2026-05-07
> **Swarm Inputs**: `ses_2019d5294ffePOlR7qlNpEAIyJ`

## Parent PRD

Direct user request — embed a generic `swarm-intelligence` capability directly into swarminator so agents can learn how to use swarminator from the terminal without depending on an external skill installation.

## Technical Overview

Swarminator already has an embedded tutorial system backed by static topics and an optional kilo-powered answer path. That makes tutorial mode the natural host for a built-in swarm guidance experience. The embedded guidance should help the orchestrator choose personas and engines/models, while staying non-interactive and terminal-friendly.

The guidance needs two knowledge layers: a static baseline that always works, and a dynamic enrichment layer that can use engine model listings or, when unavailable, ask a trusted helper model (`kilo/kilo-auto/free`) to summarize official docs. Recommendations must be grouped by engine because execution is engine-explicit.

## Architecture Decisions

### ADR-1: Extend tutorial mode instead of creating a separate skill loader

- **Context**: `answerTutorial()` already combines embedded reference content with a kilo-backed answer path in `cmd/swarminator/tutorial.go:38`.
- **Decision**: Implement the embedded swarm guidance as an extension of tutorial mode (`--tutorial swarm` and related questions).
- **Rationale**: Reuses existing UX, avoids introducing a second guidance subsystem, and keeps the feature available even when external skill installation is absent.
- **Alternatives Considered**:
  - New top-level `--swarm-skill` command — rejected because it duplicates tutorial infrastructure.

### ADR-2: Use engine-grouped recommendations

- **Context**: The user explicitly wants model suggestions grouped by engine (`kilo`, `claude`, `codex`, `command-code`, plus Gemini where relevant).
- **Decision**: All recommendation output is grouped by engine headings first.
- **Rationale**: Aligns guidance with actual execution choices (`--agent=NAME`).
- **Alternatives Considered**:
  - Present a flat “best model” list — rejected because it hides transport differences.

### ADR-3: Make dynamic enrichment optional, not mandatory

- **Context**: Current tutorial mode degrades gracefully to static content when kilo is unavailable in `cmd/swarminator/tutorial.go:48`.
- **Decision**: Keep a static baseline and layer dynamic discovery on top when listing or helper LLMs are available.
- **Rationale**: Guarantees the feature works in offline or minimally configured environments.
- **Alternatives Considered**:
  - Require live model discovery — rejected because it makes the embedded guidance brittle.

## System Components

- **Static topic registry**: `internal/tutorial/content.go:5`.
- **Tutorial execution path**: `cmd/swarminator/tutorial.go:42`.
- **Embedded docs source**: `internal/docs/cli_reference.md` exposed through `docs.EmbeddedReference()` in `cmd/swarminator/tutorial.go:63`.
- **Kilo helper path**: `llm.NewKiloProvider()` selected in `cmd/swarminator/tutorial.go:57`.
- **Future listing integration**: depends on a new lister capability from the model-listing TRD.

## API Contracts / Interfaces

### CLI surface

```text
swarminator --tutorial swarm
swarminator --tutorial "which model for code review?"
swarminator --tutorial "which persona should I use for decomposition?"
```

### Output contract

```text
[ENGINE]
  <model> — <why it fits>
Suggested persona:
  <persona text or label>
Example:
  swarminator --agent=<engine> -m <model> -p "<persona>" -t <seconds>
```

## Data Models

### Static recommendation entry

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `TaskCategory` | string | normalized task bucket | e.g. review, architecture, decomposition |
| `Engine` | string | known engine | top-level group |
| `Model` | string | displayable model identifier | recommended model |
| `PersonaHint` | string | non-empty | suggested persona or role |
| `Reason` | string | non-empty | short rationale |

## Security Assessment

### Authentication & Authorization

- Guidance must not assume engine login state beyond passive detection.
- If using helper LLM fallback, keep it read-only and documentation-oriented.

### Data Protection

- Do not send local secrets or unrelated repo content when asking helper models for model guidance.
- Restrict helper prompts to docs/reference context and the user’s question.

### Input Validation & Injection Prevention

- Tutorial questions are free text but must only feed existing provider APIs via argument-safe execution paths.
- Never pass user questions into shell-parsed commands.

### Infrastructure & Configuration

- Helper model timeout should remain bounded by tutorial timeout.
- Dynamic listing/API enrichment must degrade to static guidance on failure.

### Supply Chain & Dependencies

- Reuse existing kilo provider path instead of adding web-scraping dependencies.

### Failure Modes

- If dynamic enrichment fails, return static guidance rather than an empty answer.
- If no engine data exists, still explain how to use `--list-agents` and future listing commands.

## Non-Functional Requirements

- **Performance**: static answers <1s; enriched answers bounded by tutorial timeout.
- **Scalability**: adding a new engine should only require new static recommendations and optional lister integration.
- **Observability**: recommendation output should indicate when it is using static vs dynamic knowledge.
- **Reliability**: tutorial mode must remain functional without kilo.

## Acceptance Criteria

- `--tutorial swarm` returns an embedded overview explaining engines, personas, and command structure.
- Question-based tutorial responses can recommend models/personas grouped by engine.
- Dynamic enrichment uses live engine data when available and static fallback otherwise.
- Output includes runnable swarminator examples with explicit `--agent`.
- Command-code appears as its own engine group, even if it can only describe model behavior qualitatively.

## Implementation Plan

1. Add a new `swarm` topic and static recommendation tables to `internal/tutorial/content.go`.
2. Extend `answerTutorialWith()` to detect swarm-guidance questions before generic kilo answering.
3. Add a helper that classifies request intent (model choice, persona choice, protocol help).
4. Integrate engine-grouped model suggestions from the future listing capability when available.
5. Add kilo-based docs helper fallback for unresolved model questions.
6. Update tutorial docs and tests.

## Child BDD Specs

- Deferred until the wording and output contract are approved.

## ⚔ Challenge Gate

> **Status**: pending
> **Challenger**: swarm synthesis + security lens
> **Date**: 2026-05-07

### Debate Record

| # | Vector | Challenge | Response | Verdict |
|---|--------|-----------|----------|---------|
| 1 | scope | Is this really a “skill” if it is non-interactive and tutorial-backed? | Yes; the embedded version is a guidance artifact, not a plugin runtime. | author-won |
| 2 | longevity | Will static recommendations become stale? | Dynamic enrichment plus explicit fallback provenance mitigates drift. | author-won |
| 3 | usability | “Suggest and ask user to choose” implies interactivity; can tutorial mode do that? | In current CLI shape it can suggest options and instruct the caller to choose, but cannot run a true prompt loop without a new UI contract. | challenger-won |

### Challenge Summary

- **Challenges raised**: 3
- **Author victories**: 2
- **Challenger victories**: 1
- **Escalated**: 0
- **Overall verdict**: REVISE wording from interactive choice to suggestion-based guidance unless CLI interaction is expanded
