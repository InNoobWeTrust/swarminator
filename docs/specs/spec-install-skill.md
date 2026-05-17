# Superseded: External Skill Installation

## Status

Rejected. This document described the wrong product direction.

## Why This Spec Was Wrong

- The goal is a zero-setup experience for agents that need to use swarminator.
- Agents should learn how to use swarminator from the swarminator CLI itself.
- Swarminator is not intended to become a cross-agent plugin manager or skill installer.

## Correct Direction

- Embed the `swarm-intelligence` guidance directly inside swarminator.
- Expose it through tutorial mode, especially `swarminator --tutorial swarm` and `swarminator --tutorial swarm-intelligence`.
- Bundle the reference material that an agent needs: engine selection, model discovery commands, persona suggestions, quorum modes, phases, protocol, and runnable examples.
- Use live discovery commands such as `--list-agents`, `--list-models`, and `--list-providers` to enrich the embedded guide when the local environment supports them.

## Out Of Scope

- `--install-skill` or any similar installer surface
- copying files into `~/.agents/skills/` or other agent-owned skill directories
- delegated installation into third-party agent runtimes

## Canonical References

- `docs/spec-embedded-skill.md`
- `docs/requirements/trd-embedded-swarm-guidance.md`

---

*End of note*
