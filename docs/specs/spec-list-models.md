# Spec: Model/Provider Listing Mode

## Overview

Add `--list-models` and `--list-providers` commands to swarminator to programmatically discover available models and providers for each configured agent.

## Problem Statement

Currently `--list-agents` shows which agent binaries are available, but there's no way to list which models each agent can use. Orchestrators need this to make informed routing decisions and validate model strings before execution.

## Requirements

### Functional

- `swarminator --list-models` - List all available models across all agents
- `swarminator --list-models --agent=kilo` - Filter to specific agent
- `swarminator --list-providers` - List configured providers per agent
- Optional `--json` flag for machine-readable output
- Output includes: agent name, provider (if applicable), model ID, context window size (when available)

### Non-Functional

- Fast (<2s for all agents)
- Parallel agent querying
- Graceful degradation when agent CLI lacks listing support
- Cache results per invocation (no repeated queries)

## Implementation Design

### CLI Changes

**File:** `internal/cli/args.go`

```go
type Args struct {
    // ... existing fields ...
    ListModels    bool   // --list-models
    ListProviders bool   // --list-providers
    ListAgent     string // --agent when listing (reuses Agent field)
    JSONOutput    bool   // --json
}
```

Add flag parsing:
- `--list-models` (optional: `--list-models` or `--list-models=AGENT`)
- `--list-providers` (optional: `--list-providers` or `--list-providers=AGENT`)
- These are mutually exclusive with node execution flags

**File:** `cmd/swarminator/main.go`

Add handlers:
- `runListModels(args cli.Args)`
- `runListProviders(args cli.Args)`

Called before stdin read, after `--list-agents`.

### Agent Listing Interface

**New file:** `pkg/llm/lister.go`

```go
package llm

// ModelInfo represents a discoverable model.
type ModelInfo struct {
    ID          string `json:"id"`           // e.g. "openai/gpt-4o"
    DisplayName string `json:"display_name"` // e.g. "GPT-4o"
    Context     int    `json:"context"`      // context window in tokens
    Provider    string `json:"provider"`     // e.g. "openai"
}

// ProviderInfo represents a discoverable provider.
type ProviderInfo struct {
    ID            string   `json:"id"`
    Name          string   `json:"name"`
    Models        []string `json:"models"`        // model IDs
    Authenticated bool     `json:"authenticated"`
}

// AgentLister extends Provider with listing capabilities.
type AgentLister interface {
    Provider
    ListModels(ctx context.Context, provider string) ([]ModelInfo, error)
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
}
```

### Agent-Specific Implementations

#### Kilo Agent (priority: high)

**File:** `pkg/llm/kilo.go`

Add methods to `KiloProvider`:

```go
func (p *KiloProvider) ListModels(ctx context.Context, provider string) ([]ModelInfo, error) {
    args := []string{"models"}
    if provider != "" {
        args = append(args, provider)
    }
    args = append(args, "--json")

    cmd := exec.CommandContext(ctx, p.binary, args...)
    stdout, err := cmd.StdoutPipe()
    // ... error handling ...

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("failed to start %s: %w", p.binary, err)
    }

    var models []ModelInfo
    decoder := json.NewDecoder(stdout)
    // Parse kilo's JSON output format (array or newline-delimited)
    // Document expected format from kilo v0.21.0+

    if err := cmd.Wait(); err != nil {
        return nil, fmt.Errorf("kilo models failed: %w", err)
    }
    return models, nil
}

func (p *KiloProvider) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
    // Try: `kilo config get providers --json` or parse from models list
    // Group models by provider
}
```

**Expected kilo output format:** `kilo models --json` returns JSON array of objects with fields: `id`, `provider`, `display_name`, `context_window`, etc.

#### Gemini Agent (priority: medium)

**File:** `pkg/llm/gemini.go`

Add methods:

```go
func (p *GeminiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
    // Strategy 1: Try API if GOOGLE_API_KEY set
    //   GET https://generativelanguage.googleapis.com/v1beta/models
    //   Filter models with generateContent capability
    //   Return: ID like "google/gemini-2.5-flash", strip "models/" prefix

    // Strategy 2: Static fallback list (gemini-2.5-pro, gemini-2.5-flash, etc.)
    staticModels := []ModelInfo{
        {ID: "google/gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", Context: 1000000, Provider: "google"},
        {ID: "google/gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", Context: 1000000, Provider: "google"},
        // ... from official docs
    }
    return staticModels, nil
}

func (p *GeminiProvider) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
    return []ProviderInfo{
        {ID: "google", Name: "Google", Models: []string{"gemini-2.5-pro", "gemini-2.5-flash"}, Authenticated: true},
    }, nil
}
```

#### Claude Agent (priority: medium)

**File:** `pkg/llm/acp.go` (extend ACPProvider)

```go
func (p *ACPProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
    // Strategy 1: Try `ant beta:models list --format json` if ant CLI installed
    // Strategy 2: Direct API call: GET https://api.anthropic.com/v1/models with ANTHROPIC_API_KEY
    // Strategy 3: Static list (claude-opus-4, claude-sonnet-4, claude-haiku-3)
}
```

#### Codex Agent (priority: low)

**File:** `pkg/llm/codex.go`

```go
func (p *CodexProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
    // Strategy 1: OpenAI API GET /v1/models with OPENAI_API_KEY, filter codex-* and gpt-* models
    // Strategy 2: Static list from OpenAI docs
}
```

#### CommandCode Agent (future)

Will implement when Request 2 is done.

### Fallback: LLM-Powered Model Discovery

If all agent listing fails, swarminator can query `kilo/kilo-auto/free` to fetch the latest model lists from official documentation websites (Google AI Studio, Anthropic, OpenAI, etc.).

**Helper:** `queryModelListViaLLM(ctx context.Context, agentName string) ([]ModelInfo, error)`

### Output Format

**Human-readable table (default):**

```
AGENT     PROVIDER      MODEL                        CONTEXT
kilo      openai        gpt-4o                      128K
kilo      anthropic    claude-sonnet-4              200K
gemini    google        gemini-2.5-flash            1M
claude    anthropic    claude-opus-4                200K
```

Models grouped by agent (kilo, gemini, claude, codex, command-code), then by provider within kilo.

**JSON output** (`--json`):

```json
{
  "kilo": [
    {"id":"openai/gpt-4o","display_name":"GPT-4o","context":128000,"provider":"openai"},
    {"id":"anthropic/claude-sonnet-4","display_name":"Claude Sonnet 4","context":200000,"provider":"anthropic"}
  ],
  "gemini": [
    {"id":"google/gemini-2.5-flash","display_name":"Gemini 2.5 Flash","context":1000000,"provider":"google"}
  ]
}
```

Keyed by agent name to support grouping.
AGENT     MODEL                              CONTEXT    PROVIDER
kilo      openai/gpt-4o                      128K       openai
kilo      anthropic/claude-sonnet-4          200K       anthropic
gemini    google/gemini-2.5-flash            1M         google
claude    anthropic/claude-opus-4            200K       anthropic
```

**JSON output** (`--json`):

```json
{
  "kilo": [
    {"id":"openai/gpt-4o","display_name":"GPT-4o","context":128000,"provider":"openai"},
    {"id":"anthropic/claude-sonnet-4","display_name":"Claude Sonnet 4","context":200000,"provider":"anthropic"}
  ],
  "gemini": [
    {"id":"google/gemini-2.5-flash","display_name":"Gemini 2.5 Flash","context":1000000,"provider":"google"}
  ]
}
```

### Error Handling

- Agent binary not found → skip with warning to stderr
- Agent listing command fails → capture error, continue
- Timeout → treat as unavailable, continue
- If all agents fail → exit with code 3 and message "no model listings available"

### Testing Strategy

- Mock agent binaries that output sample JSON
- Test parsing of kilo, gemini static responses
- Test fallback path when binaries missing
- Test JSON output format
- Test agent filtering

### Documentation Updates

- Update `cmd/gen-docs/main.go` to include `--list-models` and `--list-providers`
- Update `internal/docs/cli_reference.md`
- Update README.md with examples

### Open Questions

1. Should `--list-models` also show models from unauthenticated agents? **Decision:** Show all models the agent *could* use if authenticated; mark auth status in JSON.
2. What's the official kilo models JSON schema? Need to check latest release.
3. Should we support `--refresh` to bypass cache? **Decision:** No caching needed across invocations; each call is fresh.

---

*End of spec*
