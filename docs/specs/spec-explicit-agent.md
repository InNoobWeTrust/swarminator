# Spec: Explicit Agent Selection (`--agent` Required)

## Overview

Change swarminator's model routing from automatic prefix-based to explicit agent selection, requiring `--agent=NAME` for all node executions. This prevents breakage when users configure custom provider names in kilo.

## Problem Statement

Current automatic routing by model prefix conflicts with kilo's custom provider configuration. When a user adds a custom provider in kilo (e.g., `mycorp/gpt`), swarminator's hardcoded prefix list won't recognize it, causing routing failures unless they manually specify `--agent`. The automatic routing creates a false sense of convenience that breaks with customization.

Additionally, automatic routing is implicit and surprising; explicit selection is clearer and more deterministic.

## Requirements

### Functional

- **`--agent=NAME` becomes required for all node executions** (unless model has no provider prefix and there's exactly one authenticated agent, but still recommended)
- Remove or deprecate automatic prefix-based routing
- Provide clear error message when `--agent` omitted, suggesting available agents
- Maintain backwards compatibility via opt-in flag? **Decision:** Breaking change; major version bump.

### Non-Functional

- Zero runtime overhead for routing (direct lookup)
- Clear error messages with actionable remediation (`--agent=kilo` etc.)
- Documentation updated to reflect required flag

## Implementation Design

### CLI Argument Validation

**File:** `internal/cli/args.go`

Modify validation at end of `Parse()`:

```go
// After parsing all flags
if shouldRunNode(args) { // not --list-*, --tutorial, etc.
    if args.Agent == "" {
        return Args{}, errors.New("--agent is required; available agents: kilo, gemini, claude, codex, command-code")
    }
    // ... rest of validation
}
```

Error message should call `registry.GetAll()` to list detected agents.

### Remove Automatic Routing

**File:** `pkg/llm/registry.go`

Deprecate `ResolveRoute()` and `GetForModel()`. Keep them for internal backwards compatibility but they won't be called by `UnifiedProvider`.

**File:** `pkg/llm/unified.go`

Modify `Complete()`:

```go
func (u *UnifiedProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
    // ... detection ...

    // Agent is now mandatory
    if u.agentOverride == "" {
        return "", errors.New("internal error: agent override required but not set") // should never happen after cli validation
    }

    agent := u.registry.GetByName(u.agentOverride)
    if agent == nil {
        all := u.registry.GetAll()
        names := make([]string, 0, len(all))
        for _, a := range all {
            if a.Available {
                names = append(names, a.Name)
            }
        }
        return "", fmt.Errorf("--agent=%q is not available; available agents: %s", u.agentOverride, strings.Join(names, ", "))
    }

    // ... rest unchanged ...
}
```

### Model Prefix Handling

Since `--agent` is explicit, the model string no longer needs to encode provider information. However, to maintain compatibility:

- If the model string contains a provider prefix (e.g., `anthropic/claude-sonnet`), extract the model name portion **only** if the agent doesn't already provide it
- For `kilo` agent: pass full `-m` value unchanged (kilo needs provider-qualified model IDs)
- For `gemini`/`claude`/`codex`: strip any provider prefix, use bare model name (their CLI expects native model IDs)
- For `command-code`: ignore `-m` (last interactive model used)

**Mapping table:**

| Agent       | `-m` format passed to binary           |
|-------------|----------------------------------------|
| kilo        | full (e.g. `openai/gpt-4o`)            |
| gemini      | stripped (e.g. `gemini-2.5-flash`)    |
| claude      | stripped (e.g. `claude-sonnet-4`)     |
| codex       | stripped (e.g. `codex-mini`)          |
| command-code| ignored (no flag)                      |

Implementation: In each provider's `Complete()`, add normalization:

**GeminiProvider already has** `normalizeGeminiModel()`. Ensure it handles both `google/` and `gemini/` prefixes.

**ACPProvider (claude):** Add `normalizeClaudeModel()` that strips `anthropic/` or `claude/` prefix if present.

**CodexProvider:** Add `normalizeCodexModel()` that strips `openai/` or `codex-`? Codex expects `-m codex-mini` so just pass through.

### Error Messages

When user forgets `--agent`:

```
Error: --agent is required.
Available agents: kilo (authenticated), gemini (available), claude (unavailable), codex (unavailable)
Use: swarminator --agent=kilo -m openai/gpt-4o -p "..." -t 60
```

When user specifies unavailable agent:

```
Error: --agent=foo is not available.
Known agents: kilo, gemini, claude, codex, command-code
Detected: kilo (authenticated), gemini (available)
```

### Compatibility Migration Path

**Deprecation period:** Optionally keep automatic routing behind a flag `--auto-route` for one release, but default to requiring `--agent`.

**Documentation:** Update README immediately with `--agent` required in all examples.

### Skill Impact

The embedded skill (Request 3) should now:

- Always recommend specifying `--agent` explicitly
- Show examples grouped by agent:
  ```
  KILO (multi-provider):
    swarminator --agent=kilo -m openai/gpt-4o -p "..." -t 60

  GEMINI (headless):
    swarminator --agent=gemini -m gemini-2.5-flash -p "..." -t 60

  CLAUDE (ACP with modes):
    swarminator --agent=claude -m claude-sonnet-4 -p "..." -t 90 --agent-mode=yolo
  ```

## Testing

- Test that omitting `--agent` exits with error code 3
- Test that `--agent=unknown` produces helpful error
- Test that model with provider prefix works with each agent (gemini strips, kilo keeps)
- Test that bare model names work (gemini accepts `gemini-2.5-flash`, claude accepts `claude-sonnet`)

## Open Questions

1. Should we allow `--agent` to be set via environment variable `SWARMINATOR_AGENT`? Not in scope.
2. Should we keep `--list-agents` or deprecate? Keep for discovery.

---

*End of spec*
