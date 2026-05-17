# TRD Superseded: External Skill Installation

> **Status**: superseded
> **Owner**: user-requested correction
> **Updated**: 2026-05-17

## Decision

This TRD is no longer active. The underlying assumption was wrong: swarminator should not become a delegated installer for third-party agent skills.

## Replacement Direction

Swarminator should be self-describing for agent use:

- the `swarm-intelligence` guidance is embedded in the binary
- agents discover how to use swarminator through `--help`, `--tutorial swarm`, `--tutorial swarm-intelligence`, `--phases`, and `--protocol`
- live commands such as `--list-agents`, `--list-models`, and `--list-providers` enrich that built-in guide

## Why The Previous Direction Was Rejected

- It added setup work instead of removing setup work.
- It blurred product boundaries by turning swarminator into a cross-agent plugin manager.
- It weakened usability for the primary case: telling an agent to use swarminator immediately on a fresh machine.

## Canonical References

- `docs/spec-embedded-skill.md`
- `docs/requirements/trd-embedded-swarm-guidance.md`

---

*End of note*
