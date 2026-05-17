# Requirements-Driven Spec Index

**Created:** 2026-05-07
**Method:** swarm synthesis + requirements-driven technical design

## Artifacts

1. `trd-explicit-agent-selection.md`
   - Status: **verified-current**
   - Meaning: core behavior is already implemented; follow-up work is docs/test consistency.

2. `trd-engine-grouped-model-listing.md`
   - Status: **draft**
   - Adds engine-grouped model/provider discovery.

3. `trd-command-code-agent.md`
   - Status: **draft**
   - Adds `command-code` / `cmd` as an explicit engine.

4. `trd-embedded-swarm-guidance.md`
   - Status: **draft**
   - Embeds generic swarm guidance into tutorial mode.

5. `trd-skill-installation-delegation.md`
   - Status: **draft**
   - Adds delegated skill installation entrypoint.

## Swarm Provenance

These specs were crowdsourced with parallel nodes and then synthesized:

- model/provider listing: `ses_2019d52a3ffeMBtr0KlPAgVgQx`
- command-code agent: `ses_2019d529fffe1u1tS4xvWnKM6w`
- embedded swarm guidance: `ses_2019d5294ffePOlR7qlNpEAIyJ`
- skill installation: `ses_1fa8fe4cdffemtzs3M3WQR02kT`
- cross-cutting alignment: `ses_2019d5273ffeQ5bAnVGv3tTMVo`
- explicit-agent verification: `ses_1fa898194ffeES6kzT0g2X9IXP` + direct repo verification

## Recommended Order

1. `trd-explicit-agent-selection.md` follow-up cleanup
2. `trd-command-code-agent.md`
3. `trd-engine-grouped-model-listing.md`
4. `trd-embedded-swarm-guidance.md`
5. `trd-skill-installation-delegation.md`

## Cross-Cutting Notes

- Engine grouping is the consistent top-level UX across listing and guidance.
- `command-code` should remain explicit-only.
- Existing generated docs appear stale relative to current explicit-agent behavior and should be reconciled before new docs work lands.
- The older ad-hoc docs in `docs/spec-*.md` and `docs/enhancement-specs.md` should be treated as superseded by this directory.
