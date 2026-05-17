# Swarminator Enhancement Specs & Implementation Plans

**Date:** 2026-05-07
**Author:** Swarminator Architect
**Status:** Draft for review

---

## Overview

This document contains detailed specifications and implementation plans for four feature requests:

1. **Agent Model Listing Mode** - `--list-models` and `--list-providers` flags to enumerate configured providers and models for each agent
2. **CommandCode Agent Support** - Add `command-code` (cmd) agent with headless mode, model-agnostic routing
3. **Embedded Swarm-Intelligence Skill** - Built-in guidance for agents to use swarminator without external skill installation
4. **Skill Installation from Repo** - Tools to install skills directly from GitHub or local repositories

---

## Request 1: Agent Model Listing Mode

### Problem

Orchestrators and users need a way to programmatically discover which models are available for each configured agent. Currently swarminator only lists agents with `--list-agents` but not the models each agent can use.

### Goals

- Add `--list-models` flag to list available models for a specific agent or all agents
- Add `--list-providers` flag to list configured providers (for agents that support provider selection)
- Output parseable (JSON and human-readable table)
- Support filtering by agent name
- Fallback gracefully when agent CLI doesn't support listing

### Non-Goals

- Real-time model availability checking (cached/detected once)
- Model metadata (context window, cost) unless agent provides it
- Automatic refresh or periodic updates (explicit refresh only)

---

### Technical Design

#### CLI Changes

**New flags:**
```go
type Args struct {
    // ... existing fields ...
    ListModels     bool   // --list-models
    ListProviders  bool   // --list-providers
    ListAgent      string // optional: --list-agent=NAME to filter
    JSONOutput     bool   // --json for machine-readable output
}
```

Parser additions in `internal/cli/args.go`:
- `--list-models` (boolean flag, optionally takes `=AGENT` to filter)
- `--list-providers` (boolean flag, optionally takes `=AGENT`)
- `--json` output format modifier

#### Agent Query Interface

Define a new `AgentLister` interface that agents may optionally implement:

```go
// AgentLister allows an agent to list available models/providers.
type AgentLister interface {
    ListModels(ctx context.Context, provider string) ([]ModelInfo, error)
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
}

type ModelInfo struct {
    ID          string `json:"id"`
    DisplayName string `json:"display_name,omitempty"`
    Context     int    `json:"context,omitempty"`
    Provider    string `json:"provider"`
}

type ProviderInfo struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Models      []string `json:"models"`
    Authenticated bool   `json:"authenticated"`
}
```

**Detection:** Each agent provider (`kilo.go`, `gemini.go`, `codex.go`, `acp.go`) will optionally implement these listing methods.

#### Agent-Specific Listing Strategies

##### 1. Kilo Agent

**Binary command:** `kilo models [provider] --json`

**Implementation:** `pkg/llm/kilo.go:ListModels()`
```go
func (p *KiloProvider) ListModels(ctx context.Context, provider string) ([]ModelInfo, error) {
    args := []string{"models"}
    if provider != "" {
        args = append(args, provider)
    }
    args = append(args, "--json")
    cmd := exec.CommandContext(ctx, p.binary, args...)
    // parse JSON output
}
```

**Output format:** Newline-delimited JSON objects (one per model) or a JSON array.

**Provider listing:** `kilo config get providers` or parse from `kilo models` without args.

##### 2. Gemini Agent

**Current state:** Gemini CLI lacks `--list-models` feature (see GitHub issue #7512). 

**Approach Tiered:**
1. **Primary:** Query Gemini REST API directly: `GET https://generativelanguage.googleapis.com/v1beta/models`
   - Requires `GOOGLE_API_KEY` or `gcloud auth print-access-token`
   - Filter for `generateContent` supported models
2. **Fallback:** Use static model list (hardcoded from Gemini documentation)
3. **LLM-powered:** Ask `kilo/kilo-auto/free` to fetch latest model list from web if API unavailable

**Implementation:** `pkg/llm/gemini.go:ListModels()`
```go
func (p *GeminiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
    // Try API first
    if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
        return fetchFromGeminiAPI(ctx, apiKey)
    }
    // fallback static list or LLM query
}
```

##### 3. Claude Agent

**Current state:** Claude Code CLI has no `model list` command; model selection via interactive `/model`. Anthropic's `ant` CLI exists but not the same binary.

**Approach:**
1. **Anthropic API:** `GET https://api.anthropic.com/v1/models` with `ANTHROPIC_API_KEY`
2. **Static list:** Common Claude models (opus, sonnet, haiku)
3. **LLM-powered fallback:** Ask `kilo/kilo-auto/free` for latest model list from docs

**Implementation:** `pkg/llm/acp.go:ListModels()` method (ACP agent wrapper).

##### 4. Codex Agent

**Current state:** Codex CLI has no model listing. Models are configured in Codex settings or inferred from OpenAI API.

**Approach:**
1. **OpenAI API:** `GET https://api.openai.com/v1/models` with `OPENAI_API_KEY`
2. Filter for `codex-*` models
3. **LLM-powered fallback** if API unavailable

##### 5. CommandCode Agent (see Request 2)

### Command Structure

```
swarminator --list-models [--agent=NAME] [--json]
swarminator --list-providers [--agent=NAME] [--json]
```

When `--agent` omitted, lists for all available agents.

**Output formats:**

**Human-readable table:**
```
AGENT     PROVIDER      MODEL                        CONTEXT
kilo      openai        gpt-4o                      128K
kilo      anthropic    claude-sonnet-4              200K
gemini    google        gemini-2.5-flash            1M
```

**JSON output** (`--json`):
```json
{
  "kilo": [
    {"id":"openai/gpt-4o","provider":"openai","display_name":"GPT-4o","context":128000},
    ...
  ],
  "gemini": [...]
}
```

**Providers-only output** (`--list-providers`):
```
AGENT     PROVIDER      MODELS     AUTH
kilo      openai        12         yes
kilo      anthropic     8          yes
gemini    google        6          yes
```

### Error Handling

- If agent unavailable: print warning, skip
- If agent listing fails: capture stderr, continue with other agents
- If all agents fail: exit with code 3, actionable message

### Implementation Steps

1. **Extend `Args` struct** with list flags (1 hr)
2. **Add model listing interface** in `pkg/llm/` (2 hrs)
3. **Implement `ListModels` for KiloProvider** using `kilo models --json` (2 hrs)
4. **Implement `ListModels` for GeminiProvider** with API + fallback + LLM query (3 hrs)
5. **Implement `ListModels` for ACPProvider** (claude) via Anthropic API + fallback (3 hrs)
6. **Implement `ListModels` for CodexProvider** via OpenAI API + fallback (3 hrs)
7. **Add `runListModels()` and `runListProviders()` in `main.go`** (2 hrs)
8. **Update help text and documentation** (1 hr)
9. **Write tests** - mock agents, verify output parsing (4 hrs)
10. **Update `cmd/gen-docs`** to include new commands (1 hr)

**Total estimate:** 22 hours (~3 days)

---

## Request 2: CommandCode Agent Support

### Problem

CommandCode (`cmd`) is a popular coding agent with headless mode (via `--print` or `-p`) and taste learning. It uses the "last used model" from interactive sessions and cannot be configured via CLI flag. Currently swarminator doesn't support it.

### Goals

- Add `command-code` agent to known agents (binary: `cmd`)
- Implement `CommandCodeProvider` that runs `cmd --print ...` for headless execution
- Support model passthrough when available (but not required)
- Support `--yolo` equivalent for permissions (`--dangerously-skip-permissions`)
- Support `--agent-mode` mapping if applicable (probably not)
- Handle exit codes appropriately

### Non-Goals

- Interactive "taste" learning integration - just pass through
- Model management within swarminator (CommandCode manages its own model list)
- ACP protocol support (CommandCode is headless-only)

---

### Technical Design

#### Agent Registration

Add to `pkg/llm/registry.go:KnownAgents()`:

```go
{
    Name:          "command-code",
    Binary:        "cmd",
    ACPArgs:       []string{}, // headless only, no ACP
    ModelPrefixes: []string{"commandcode/", "cmd/"}, // default prefixes (may be none - use explicit agent)
},
```

**Routing behavior:**
- No model prefix routing by default (like codex) - requires `--agent=command-code`
- Or optionally prefix `commandcode/` to route automatically

#### Provider Implementation

Create new file `pkg/llm/commandcode.go`:

```go
package llm

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"
)

// CommandCodeProvider runs CommandCode in headless mode via `cmd --print`.
type CommandCodeProvider struct {
    binary       string
    args         []string
    yoloMode     bool // --yolo: bypass permissions
}

func NewCommandCodeProvider(binary string) Provider {
    return &CommandCodeProvider{
        binary: binary,
        args:   []string{},
    }
}

func (p *CommandCodeProvider) WithYOLO(yolo bool) *CommandCodeProvider {
    cp := *p
    cp.yoloMode = yolo
    return &cp
}

// commandCodeOutput is the stdout response (plain text)
func (p *CommandCodeProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
    message := req.Input
    if req.Persona != "" {
        message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
    }

    allArgs := []string{"--print", message}
    if p.yoloMode {
        allArgs = append(allArgs, "--yolo")
    }
    // Note: CommandCode does not accept --model flag for headless; it uses
    // the last interactive model. Model is ignored.
    // (We could pass via --model if it becomes supported)

    cmd := exec.CommandContext(ctx, p.binary, append(p.args, allArgs...)...)
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return "", fmt.Errorf("failed to open stdout for %s: %w", p.binary, err)
    }

    if err := cmd.Start(); err != nil {
        return "", fmt.Errorf("failed to start %s: %w", p.binary, err)
    }

    var responseText strings.Builder
    scanner := bufio.NewScanner(stdout)
    scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            waitForProcessExit(cmd, acpProcessWaitTimeout)
            return responseText.String(), ctx.Err()
        default:
        }
        responseText.WriteString(scanner.Text())
    }

    if err := scanner.Err(); err != nil {
        waitForProcessExit(cmd, acpProcessWaitTimeout)
        return "", fmt.Errorf("reading %s output: %w", p.binary, err)
    }

    if ctx.Err() != nil {
        waitForProcessExit(cmd, acpProcessWaitTimeout)
        return responseText.String(), ctx.Err()
    }

    exitErr := cmd.Wait()
    output := strings.TrimSpace(responseText.String())

    // CommandCode exit codes: 0=success, non-zero=error
    if output == "" && exitErr != nil {
        return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
    }

    return output, nil
}
```

#### Unified Provider Integration

In `pkg/llm/unified.go`, add a factory for commandcode:

```go
newCommandCodeProvider = func(binary string) Provider {
    return NewCommandCodeProvider(binary)
},
```

And in the switch (lines 128-161):
```go
case "command-code":
    if req.AgentMode != "" {
        return "", fmt.Errorf("--agent-mode is not supported for agent %q", agent.Name)
    }
    ccProvider := u.newCommandCodeProvider(agent.Binary)
    provider = ccProvider
```

Add the field to `UnifiedProvider` struct.

#### Model Handling

CommandCode headless mode doesn't accept a `--model` flag. It uses the last model from interactive session. Therefore:
- If user passes `-m model-name`, ignore it with advisory feedback if `--feedback=stderr`
- Document that `-m` is ignored for `--agent=command-code`

Alternative: if user passes `--model`, it's an error? Better to ignore with warning.

**Decision:** Ignore `-m` for command-code agent with advisory message: `"swarminator: --agent=command-code ignores -m; it uses last interactive model"`

#### Agent Mode

CommandCode has permission modes: `standard`, `plan`, `auto-accept`. They map to flags:
- `--agent-mode=autoEdit` or `default`? Actually they use `--permission-mode`
- Not directly compatible; reject `--agent-mode` with clear error.

#### Listing Capability

CommandCode does not have a non-interactive model listing command. Implement listing via:

1. Try `cmd models list --json` (if it exists in future)
2. Fallback: LLM query via `kilo/kilo-auto/free` to fetch latest CommandCode supported models from docs
3. Return placeholder: "Model list unavailable; use `cmd /model` interactively"

#### Exit Code Handling

CommandCode exit codes (from docs):
- 0: success
- 1: general error
- 3: not authenticated
- 4: permission denied
- 5: rate limit
- 6: network failure
- 7: server error
- 130: interrupted

We map all non-zero to generic error with wrapped exit status; swarminator will convert to exit code 2 (retryable) or let the error bubble up.

---

### Implementation Steps

1. **Add agent to KnownAgents** in `registry.go` (15 min)
2. **Create `commandcode.go`** provider (2 hrs)
3. **Add factory to `unified.go`** and case logic (1 hr)
4. **Add model ignore warning** in `main.go` or provider (30 min)
5. **Implement `ListModels` fallback via LLM** (2 hrs)
6. **Tests:** mock commandcode binary, test headless invocation (2 hrs)
7. **Documentation** updates (1 hr)

**Total estimate:** 9 hours (~1 day)

---

## Request 3: Embedded Swarm-Intelligence Skill

### Problem

Agents need to understand how to use swarminator without installing external skills. The "swarm-intelligence" skill should be bundled directly, providing guidance on persona selection, model choices, and protocol usage.

### Goals

- Embed a skill-like guidance system accessible via `--tutorial swarm` or `--tutorial skill=swarm-intelligence`
- The embedded content explains swarminator's envelope protocol, agent selection, and node execution
- Include interactive Q&A mode: user asks a question, skill suggests persona and model based on query content
- Suggestion engine uses available agent CLIs to fetch live model/providers when possible
- Falls back to LLM query (`kilo/kilo-auto/free`) to scrape official docs for agent capabilities

### Non-Goals

- Full interactive TUI skill (keep embedded as text+logic)
- Skill installation/uninstallation (embedded only)
- Dynamic skill updates (static embedded content)

---

### Technical Design

#### New Tutorial Topic

Add to `internal/tutorial/content.go`:

```go
var topics = map[string]string{
    // ... existing topics ...
    "swarm-intelligence": `# Swarm-Intelligence Skill

This embedded skill helps you use swarminator effectively.

## Quick Reference

- Role selection: reviewer, architect, challenger, extractor, maker
- Intent mapping: review → PHASE_REVIEW, challenge → PHASE_CHALLENGE, etc.
- Model routing: automatic by prefix, or use --agent to force

## Agent Model Selection

Available agents:
- kilo (multi-provider gateway)
- gemini (Google Gemini)
- claude (Anthropic Claude)
- codex (OpenAI Codex)
- command-code (CommandCode)

For latest model listings, use --list-models.
`,
}
```

#### Interactive Question-Answering

Enhance `--tutorial` to support skill-like Q&A when topic is "swarm" or "skill":

**Current flow:** `answerTutorial(ctx, query, model)` in `tutorial.go` uses `kilo` agent.

**New behavior for `swarm-intelligence` topic:**

If query matches question patterns, provide canned answers; otherwise, delegate to `kilo/kilo-auto/free` to answer intelligently.

**Examples:**
- Q: "Which model should I use for code review?" → suggests `gemini-2.5-pro` or `claude-sonnet`
- Q: "What agents are available?" → outputs table from `--list-agents`
- Q: "How do I list models?" → directs to `--list-models`

Rather than full interactive dialogue, keep embedded static guidance and use LLM for dynamic Q&A.

#### Dynamic Suggestion Engine

When user asks `--tutorial "which model for X?"`, the system:

1. Detects task type from query (code, review, architecture, testing)
2. Queries live agent model lists via `--list-models --json` for available agents
3. Recommends top 2-3 models based on task fit
4. Provides example command

Agent capability fetching uses internal package (call provider `ListModels` directly).

**Fallback if CLI listing fails:**
- Query `kilo/kilo-auto/free` with prompt:
  ```
  Given the following task: <user query>, what are the best models to use via swarminator?
  List models with full IDs (e.g. google/gemini-2.5-flash, anthropic/claude-sonnet-4)
  Cite authoritative sources.
  ```

#### Implementation Approach

1. Extend `internal/tutorial/content.go` with new `swarm` topic containing:
   - Agent capabilities table
   - Model routing rules
   - Common command patterns
2. Extend `answerTutorial()` in `cmd/swarminator/tutorial.go`:
   - When topic is "swarm" or "intelligence", treat as embedded skill
   - Parse query to detect question type
   - If model recommendation: call helper `suggestModels(query string)`
3. `suggestModels()` function:
   a. Create `AgentRegistry`, detect agents
   b. For each agent, call `ListModels()` (new interface)
   c. If all fail, call LLM via `KiloProvider` directly
   d. Format recommendation as text response
4. Cache model list per process lifetime (avoid repeated CLI calls)

---

### Implementation Steps

1. **Add swarm-intelligence content** to `tutorial/content.go` (2 hrs)
2. **Create `suggestModels` helper** in a new `internal/skill/` package or within `tutorial.go` (3 hrs)
3. **Integrate with `answerTutorial`** for skill mode (1 hr)
4. **Implement model listing calls** to agents with timeout and error handling (2 hrs)
5. **Add LLM fallback** query using kilo direct call (2 hrs)
6. **Write tests** for suggestion logic and content (2 hrs)
7. **Documentation** (1 hr)

**Total estimate:** 13 hours (~1.5 days)

---

## Request 4: External Skill Installation

### Status

Rejected. This was a misunderstanding of the product goal.

### Correct Goal

Swarminator should be usable by agents without any external skill installation. The right UX is:

- the binary explains itself through `--help`
- `--tutorial swarm` and `--tutorial swarm-intelligence` print the built-in agent guide
- personas, quorum modes, phases, protocol, and runnable examples ship inside the binary
- live discovery commands enrich the guide instead of requiring separate setup

### Why The Installer Direction Was Wrong

- It added setup steps instead of removing them.
- It pushed swarminator toward cross-agent plugin management.
- It made the first-run agent experience worse on a clean machine.

### Replacement

Treat the embedded swarm guidance as the canonical way for agents to learn swarminator usage.

---

## Summary Timeline

| Feature                     | Est. Hours | Est. Days |
|-----------------------------|-----------:|----------:|
| 1. Agent Model Listing      | 22         | 3         |
| 2. CommandCode Agent        | 9          | 1         |
| 3. Embedded Swarm-Skill     | 13         | 2         |
| **Total**                   | **44**     | **6**     |

These can be implemented sequentially or in parallel with multiple agents.

---

## Dependencies & Risks

### External Dependencies

- Agent CLI versions supporting listing commands:
  - Kilo: v0.21.0+ has `kilo models --json`
  - Gemini: awaiting `--list-models` feature; use API or static list
  - Claude: `ant models list` (separate binary) or API call
  - Codex: `codex debug models` (dev command?) or OpenAI API
- API keys may be required for listing via API: `GOOGLE_API_KEY`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`

### Risks

- Agent CLI output format changes: need robust parsing
- Rate limits on API calls: add caching and backoff
- Permission denied errors: handle gracefully


---

## Appendix: Agent CLI Reference

### Kilo
```
kilo models [provider] [--json] [--verbose] [--refresh]
kilo config get providers
```

### Gemini
- Proposed: `gemini --list-models` (not yet implemented)
- API: `GET https://generativelanguage.googleapis.com/v1beta/models`
- Interactive: `/model` command

### Claude
- Interactive: `/model` command (in-session)
- API: `ant beta:models list` (requires `ant` CLI)
- API direct: `curl https://api.anthropic.com/v1/models`

### Codex
- API: `GET https://api.openai.com/v1/models` (filter codex-*)
- Interactive: model selection in settings

### CommandCode
- Interactive: `/model` command
- No programmatic listing documented; possibly `cmd info --json` includes current model

---

*End of document.*
