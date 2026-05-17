# Spec: CommandCode Agent Integration

## Overview

Add support for CommandCode (`cmd`) as a first-class agent in swarminator, enabling headless mode execution with taste-aware responses.

## Problem Statement

CommandCode is a widely-used AI coding agent with taste learning. It operates in headless mode via `cmd --print "query"` and uses the last interactive model selection. Currently swarminator doesn't support it, limiting orchestrator coverage.

## Requirements

### Functional

- Register `command-code` agent with binary name `cmd`
- Implement `CommandCodeProvider` using `cmd --print` for one-shot execution
- Support `--yolo` equivalent via provider option (maps to `--yolo` flag)
- Model passthrough: `-m` flag is accepted but may be ignored (CommandCode uses session model)
- Works in pipelines: stdin → swarminator → cmd → stdout
- Exit code propagation: map CommandCode exit codes to swarminator semantics

### Non-Functional

- Detection via `cmd --version` on PATH
- Timeout handling: respect `-t` flag
- No interactive session hanging (headless only)
- Clear error when `cmd` not installed

## Implementation Design

### Agent Registration

**File:** `pkg/llm/registry.go`

Add to `KnownAgents()`:

```go
{
    Name:          "command-code",
    Binary:        "cmd",
    ACPArgs:       []string{}, // no ACP, headless only
    ModelPrefixes: []string{"commandcode/", "cmd/"}, // optional auto-routing
},
```

**Routing note:** Like `codex`, CommandCode has no meaningful model prefix without explicit user intent. Recommend explicit `--agent=command-code` usage.

### Provider Implementation

**New file:** `pkg/llm/commandcode.go`

```go
package llm

import (
    "bufio"
    "context"
    "fmt"
    "os/exec"
    "strings"
    "time"
)

// CommandCodeProvider executes CommandCode in headless mode.
type CommandCodeProvider struct {
    binary string
    args   []string // additional args for testing
    yolo   bool     // --yolo bypass permissions
}

func NewCommandCodeProvider(binary string) Provider {
    return &CommandCodeProvider{binary: binary, args: []string{}}
}

func (p *CommandCodeProvider) WithYOLO(enabled bool) *CommandCodeProvider {
    cp := *p
    cp.yolo = enabled
    return &cp
}

func (p *CommandCodeProvider) Complete(ctx context.Context, req CompletionRequest) (string, error) {
    // Build message with persona+input
    message := req.Input
    if req.Persona != "" {
        message = fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)
    }

    // Build args: cmd --print "message" [--yolo]
    allArgs := []string{"--print", message}
    if p.yolo {
        allArgs = append(allArgs, "--yolo")
    }

    // CommandCode does not support --model in headless mode; it uses
    // the last model from interactive session or default.
    // We accept -m in swarminator args but ignore it here.
    if req.Model != "" {
        // Optionally log advisory via feedback sink (not available here)
    }

    cmd := exec.CommandContext(ctx, p.binary, append(p.args, allArgs...)...)

    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return "", fmt.Errorf("failed to open stdout for %s: %w", p.binary, err)
    }

    if err := cmd.Start(); err != nil {
        return "", fmt.Errorf("failed to start %s: %w", p.binary, err)
    }

    var response strings.Builder
    scanner := bufio.NewScanner(stdout)
    scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            waitForProcessExit(cmd, acpProcessWaitTimeout)
            return response.String(), ctx.Err()
        default:
        }
        response.WriteString(scanner.Text())
    }

    if err := scanner.Err(); err != nil {
        waitForProcessExit(cmd, acpProcessWaitTimeout)
        return "", fmt.Errorf("reading %s output: %w", p.binary, err)
    }

    if ctx.Err() != nil {
        waitForProcessExit(cmd, acpProcessWaitTimeout)
        return response.String(), ctx.Err()
    }

    exitErr := cmd.Wait()
    output := strings.TrimSpace(response.String())

    if output == "" && exitErr != nil {
        return "", fmt.Errorf("%s failed: %w", p.binary, exitErr)
    }

    return output, nil
}
```

### Unified Provider Integration

**File:** `pkg/llm/unified.go`

Add factory method:

```go
type UnifiedProvider struct {
    // ...
    newCommandCodeProvider func(binary string) Provider
    // ...
}

func NewUnifiedProvider(projectID string, agentOverride ...string) *UnifiedProvider {
    return &UnifiedProvider{
        // ...
        newCommandCodeProvider: func(binary string) Provider {
            return NewCommandCodeProvider(binary)
        },
        // ...
    }
}
```

Add case in `Complete()` switch:

```go
case "command-code":
    if req.AgentMode != "" {
        return "", fmt.Errorf("--agent-mode is not supported for agent %q", agent.Name)
    }
    provider = u.newCommandCodeProvider(agent.Binary)
```

### Model Handling

Since CommandCode headless doesn't accept `--model`:

- If `req.Model != ""`, emit advisory via feedback if enabled
- Do not fail; just ignore

**Option:** Future: if `--model` is set, try `--model` flag if supported by newer `cmd` versions.

### Detection

Works same as other agents: `exec.LookPath("cmd")` and `cmd --version` probe.

### Listing Support

CommandCode currently lacks programmatic model listing. Implement `ListModels` as:

1. Check if `cmd models list --json` exists (future-proof)
2. If not, return error `ErrListingNotSupported`
3. Orchestrator can fallback to LLM query

## Testing

### Unit Tests

- Mock `cmd` binary that echoes input
- Verify `Complete()` constructs args correctly
- Verify timeout/cancellation
- Verify yolo flag passthrough
- Verify error on startup failure

### Integration Test

- Requires `cmd` installed; run with sample prompt
- Check output is non-empty

### Manual Test

```bash
echo "What is Go?" | swarminator --agent=command-code -p "You are a helpful assistant" -t 30
```

## Documentation

- Add to README.md agent table
- Add to `cmd/gen-docs/main.go` agents list
- Note model behavior: "uses last interactive model"
- Show example: `--agent=command-code` required or `-m commandcode/...` if prefix routing

## Open Questions

1. Does CommandCode support `--model` override in headless mode? **Research:** Docs indicate no; uses session model. Needs confirmation.
2. Should we add a `--model` override for CommandCode anyway to future-proof? **Decision:** accept but ignore with advisory.
3. Exit code mapping: CommandCode uses codes 0,1,3,4,5,6,7,130. We'll wrap non-zero as failure.

---

*End of spec*
