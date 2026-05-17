# Spec: Embedded Swarm-Intelligence Skill

## Overview

Embed a guidance system directly into swarminator that acts as a built-in skill, helping agents and orchestrators select appropriate personas, models, and execution strategies without requiring external skill installation.

## Problem Statement

External "swarm-intelligence" skills must be installed separately. New users or automated environments may not have the skill available, causing orchestrations to fail or produce suboptimal choices. Bundling core guidance ensures every swarminator instance has baseline intelligence.

## Requirements

### Functional

- `swarminator --tutorial swarm` activates embedded skill
- `swarminator --tutorial "which model for code review?"` Q&A mode
- Skill maintains a knowledge base of:
  - Agent capabilities (kilo multi-provider, gemini headless, claude ACP, codex explicit, command-code taste)
  - Model recommendations by task (review → claude-sonnet, code → gpt-4o, architecture → gemini-2.5-pro)
  - Protocol envelope usage (ROLE/INTENT/TARGET)
- Dynamic suggestions: query available agents for their model lists (via new `--list-models` internally) and recommend from what's installed
- Fallback to LLM query (`kilo/kilo-auto/free`) if CLI listing unavailable

### Non-Functional

- Fast response (<1s for static guidance, <5s for dynamic suggestions)
- No network required for static content
- Graceful degradation when `kilo` not available (use static knowledge)

## Implementation Design

### Tutorial Topic Extension

**File:** `internal/tutorial/content.go`

Extend `topics` map:

```go
var topics = map[string]string{
    // ... existing ...
    "swarm": `# Swarm-Intelligence Embedded Skill

This built-in skill helps you use swarminator effectively without external plugins.

## Available Agents

- **kilo** – Multi-provider gateway (OpenAI, Anthropic, Google, OpenRouter, GitHub Copilot)
- **gemini** – Google Gemini, headless mode, no ACP session management
- **claude** – Anthropic Claude with ACP session management, supports --agent-mode
- **codex** – OpenAI Codex, explicit-only harness
- **command-code** – CommandCode with taste learning

## Model Selection by Task

| Task           | Recommended Models           | Agent   |
|----------------|------------------------------|---------|
| Code review    | claude-sonnet-4, gemini-2.5-pro | claude, gemini |
| Architecture   | gemini-2.5-pro, gpt-4o      | gemini, kilo |
| Quick coding   | gpt-4o, claude-haiku        | kilo, claude |
| Testing        | claude-sonnet, codex-mini   | claude, codex |
| Bug hunting    | gemini-2.5-flash, o1        | gemini, kilo |

## Quick Reference

- List agents: swarminator --list-agents
- List models: swarminator --list-models (requires Request 1 implementation)
- Force agent: --agent=NAME
- Agent modes: --agent-mode=default|plan|yolo|autoEdit (ACP agents only)

## Envelope Protocol

ROLE: <role like "reviewer", "architect">
INTENT: <intent like "challenge", "review", "decompose">
TARGET: <target like "security", "api-design">

Use intent to auto-select phase.
`,
}
```

### Q&A Logic

**File:** `cmd/swarminator/tutorial.go` (extend `answerTutorial`)

Current behavior: if topic not in `topics`, calls `kilo` agent for answer.

New behavior for "swarm" topic:

```go
func answerTutorial(ctx context.Context, query string, model string) string {
    // Check if query is a question (contains ?, "what", "which", "how")
    isQuestion := strings.HasSuffix(query, "?") || strings.Contains(strings.ToLower(query), "what ") || ...

    if query == "swarm" || isQuestion && looksLikeSwarmQuestion(query) {
        // Handle as embedded skill
        return handleSwarmSkill(ctx, query)
    }

    // Existing: try embedded topics, else ask kilo
    if text := tutorial.TopicText(query); text != "" {
        return text
    }
    // fallback to kilo...
}
```

**`handleSwarmSkill` function:**

```go
func handleSwarmSkill(ctx context.Context, query string) string {
    // Classify query type:
    // - "which model" → suggestModels(query)
    // - "what agents" → listAgentsSummary()
    // - "how to" → protocol guidance
    // - default: LLM answer via kilo

    if strings.Contains(query, "model") {
        return suggestModelsForTask(ctx, query)
    }
    if strings.Contains(query, "agent") {
        return agentsSummary()
    }
    // ... other canned responses

    // Dynamic: ask LLM using kilo auto
    return queryKiloForAnswer(ctx, query)
}
```

### Dynamic Model Suggestion Engine

**New file:** `internal/skill/suggester.go`

```go
package skill

// SuggestModelsForTask analyzes a task description and recommends models.
func SuggestModelsForTask(ctx context.Context, taskDescription string) (string, error) {
    // 1. Detect task category
    category := categorizeTask(taskDescription) // "review", "code", "arch", "test", "bug"

    // 2. Get available agents via registry
    registry := llm.NewAgentRegistry()
    registry.Detect()

    // 3. For each agent, try ListModels (requires Request 1 implementation)
    //    Collect models that match category
    recommendations := collectModelMatches(registry, category)

    // 4. If no listings available, fallback to static knowledge base:
    staticMap := map[string][]staticModelRec{
        "review": {
            {Agent: "claude", Model: "anthropic/claude-sonnet-4", Reason: "strong reasoning"},
            {Agent: "gemini", Model: "google/gemini-2.5-pro", Reason: "large context"},
        },
        // ...
    }

    // 5. Format response:
    // "For code review, consider:
    //  - claude --agent=claude -m claude-sonnet-4 (excellent reasoning)
    //  - gemini --agent=gemini -m gemini-2.5-pro (1M context)
    //
    // Use swarminator --list-models to see all available."
}
```

### LLM Fallback via Kilo

If agent listing fails (Request 1 not yet implemented):

```go
func queryKiloForAnswer(ctx context.Context, question string) string {
    // Directly instantiate KiloProvider (bypass UnifiedProvider)
    provider := llm.NewKiloProvider("kilo")
    resp, err := provider.Complete(ctx, llm.CompletionRequest{
        Model: "kilo/kilo-auto/free",
        Input: fmt.Sprintf("Question about swarminator usage: %s", question),
    })
    if err != nil {
        return "Error: could not fetch answer; please check kilo installation."
    }
    return resp
}
```

## Testing

- Unit tests for `categorizeTask` mapping
- Tests for `agentsSummary()` output format
- Tests for fallback path when no agents detected
- Test question detection heuristics

## Dependencies

- **Depends on Request 1** for dynamic model suggestions (optional; can fallback to static)
- Can be implemented independently using static knowledge base

## Documentation

- Update README: "Use `--tutorial swarm` for embedded skill"
- List example questions:
  - `--tutorial "which model for code review?"`
  - `--tutorial "what agents are available?"`
  - `--tutorial "how do I list models?"`

---

## Change: Group Models by Engine

**Requirement:** The skill should group models by the engine (kilo, claude, codex, command-code) rather than mixing them.

**Implementation:** In suggestion output:

```
Available models by engine:

[KILO]
  openai/gpt-4o          (context: 128K)
  openai/gpt-4.1         (context: 128K)
  anthropic/claude-sonnet-4 (200K)

[CLAUDE]
  claude-sonnet-4-5-20250929
  claude-opus-4-7-20250514

[CODEx]
  codex-mini
  codex-2

[COMMAND-CODE]
  (uses last interactive model; see `cmd /model`)
```

Code: group results by `agent.Name` before formatting.

---

*End of spec*
