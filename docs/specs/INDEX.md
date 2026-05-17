# Swarminator Enhancement Specifications

**Version:** 0.2 — May 2026
**Status:** Draft — awaiting implementation

This directory contains individual, independent specifications for planned swarminator enhancements. Each spec is self-contained and can be implemented in parallel.

---

## Spec Index

| # | Spec | File | Priority | Est. Effort |
|---|------|------|----------|-------------|
| 1 | Explicit Agent Selection (`--agent` required) | [spec-explicit-agent.md](spec-explicit-agent.md) | **High** – prevents routing breakage | 4 hrs |
| 2 | Agent Model/Provider Listing Mode | [spec-list-models.md](spec-list-models.md) | **High** – orchestrator visibility | 22 hrs |
| 3 | CommandCode Agent Support | [spec-commandcode-agent.md](spec-commandcode-agent.md) | **Medium** – popular agent coverage | 9 hrs |
| 4 | Embedded Swarm-Intelligence Skill | [spec-embedded-skill.md](spec-embedded-skill.md) | **Medium** – built-in guidance | 13 hrs |

**Total estimated effort:** ~48 hours (~6 working days with single engineer).

---

## Design Principles

- **Explicit over implicit:** Agent selection must be explicit (`--agent` required)
- **Agent grouping:** Models are always grouped by agent/engine (kilo, claude, gemini, codex, command-code)
- **Graceful degradation:** When agent CLI lacks listing, fall back to static knowledge or LLM query
- **Built-in guidance:** Agent-facing swarminator usage help ships inside the binary
- **Zero-skills baseline:** Swarminator works out-of-the-box without any external skill installation

---

## Dependencies

```
spec-explicit-agent.md  (no deps)  → can implement immediately
spec-list-models.md     (depends on agent lister interface)
spec-commandcode-agent.md  (depends on spec-explicit-agent.md for --agent requirement)
spec-embedded-skill.md  (depends on: spec-list-models.md for dynamic suggestions)
```

---

## Implementation Order Recommendation

1. **First:** spec-explicit-agent.md (breaking change; foundation)
2. **Second:** spec-commandcode-agent.md (independent, adds agent)
3. **Third:** spec-list-models.md (depends on agent interface; enables skill)
4. **Fourth:** spec-embedded-skill.md (uses listing from step 3)

Or parallelize: 1, 2, and 3 together; 4 after 3.

---

## Change Log

- **2026-05-07** — Initial spec set created; separated from combined spec per user request
- **2026-05-17** — Removed external skill-installation track; swarminator guidance is embedded in the binary

---

*End of index*
