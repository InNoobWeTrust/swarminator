package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"swarminator/internal/docs"
	"swarminator/internal/tutorial"
	"swarminator/pkg/llm"
)

const (
	tutorialTimeout = 120 * time.Second
)

type tutorialRequest struct {
	Query     string
	Agent     string
	Model     string
	AgentMode string
}

// tutorialCompleter is the minimal interface needed for the tutorial assistant so
// it can be swapped out in tests.
type tutorialCompleter interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (string, error)
}

var errTutorialDefaultModelUnavailable = errors.New("could not infer a default cheap model for tutorial Q&A")

// answerTutorial answers either a built-in tutorial topic directly or an
// explicit-agent Q&A request through the selected agent/model.
func answerTutorial(ctx context.Context, req tutorialRequest) (string, error) {
	return answerTutorialWith(ctx, req, nil)
}

// answerTutorialWith is the injectable version used by tests.
func answerTutorialWith(ctx context.Context, req tutorialRequest, provider tutorialCompleter) (string, error) {
	query := strings.TrimSpace(req.Query)
	if isSwarmTutorialQuery(query) {
		return renderSwarmTutorial(ctx, query, req.Agent), nil
	}
	if tutorial.IsBuiltInQuery(query) {
		return tutorial.Lookup(query), nil
	}

	model, err := resolveTutorialModel(ctx, req.Agent, req.Model, query)
	if err != nil {
		return "", err
	}
	if provider == nil {
		provider = llm.NewUnifiedProvider("", req.Agent)
	}

	tctx, cancel := context.WithTimeout(ctx, tutorialTimeout)
	defer cancel()

	input := buildTutorialInput(ctx, req.Agent, model, query)

	wasInferred := strings.TrimSpace(req.Model) == "" && model != ""

	result, err := provider.Complete(tctx, llm.CompletionRequest{
		Model:     model,
		Persona:   tutorialPersona(req.Agent, query),
		Input:     input,
		AgentMode: req.AgentMode,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("tutorial Q&A returned an empty response for --agent=%s -m %s", req.Agent, model)
	}
	out := strings.TrimSpace(result)
	if wasInferred {
		out = fmt.Sprintf("[inferred model %q as cheap default for --agent=%s]\n%s", model, req.Agent, out)
	}
	return out, nil
}

func isSwarmTutorialQuery(query string) bool {
	q := strings.TrimSpace(strings.ToLower(query))
	// Match exact aliases or strongly swarm-specific phrases.
	// Keep generic "model" and "agent" questions on the kilo-backed path unless
	// they clearly ask for the built-in swarm guide.
	if isSwarmTutorialAlias(q) {
		return true
	}
	exactPhrases := []string{
		"list providers", "list models", "choose engine",
		"engine choice", "which engine",
		"swarm guidance", "swarm agent", "swarm model",
		"which persona", "persona choice",
		"quorum mode", "quorum modes", "consensus mode",
		"phase 1", "phase 2", "phase 3",
		"swarm workflow", "maker-fix", "persona group",
	}
	for _, phrase := range exactPhrases {
		if strings.Contains(q, phrase) {
			return true
		}
	}
	return false
}

func isSwarmTutorialAlias(q string) bool {
	switch q {
	case "swarm", "swarm-intelligence", "skill=swarm-intelligence", "skill=swarm", "agent-guide", "agent guide":
		return true
	default:
		return false
	}
}

func renderSwarmTutorial(ctx context.Context, query string, filterAgent string) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(tutorial.Lookup("swarm")))
	b.WriteString("\n\n## Query Focus\n\n")
	b.WriteString(classifySwarmQuery(query))

	groups := llm.DiscoverListings(ctx, filterAgent)
	if suggestions := renderSwarmSuggestions(groups, filterAgent); suggestions != "" {
		b.WriteString("\n\n")
		b.WriteString(suggestions)
	}
	return strings.TrimSpace(b.String())
}

func renderSwarmSuggestions(groups []llm.EngineListing, filterAgent string) string {
	if len(groups) == 0 {
		return ""
	}

	var b strings.Builder
	if strings.TrimSpace(filterAgent) == "" {
		b.WriteString("## Available Agents\n\n")
		b.WriteString("Model suggestions are agent-scoped. Pass `--agent=NAME` to narrow the guide to one agent, or ask tutorial Q&A such as `swarminator --tutorial \"suggest a cheap model for code review\" --agent=NAME [-m MODEL]`.\n")
	} else {
		b.WriteString("## Agent-Scoped Model Hints\n\n")
		b.WriteString("These hints are limited to the explicit `--agent` you selected. Prefer free or cheap models first; premium models are usually chosen explicitly by the caller.\n")
	}
	b.WriteString("If automation relies on shell startup files to expose PATH entries for agent CLIs, invoke nodes with a login shell, for example: `$SHELL -l -c 'printf \"%s\" \"$TASK\" | swarminator --agent=AGENT -m MODEL -p \"FULL PERSONA PROMPT TEXT\" -t 120'`.\n")

	hasRendered := false
	for _, group := range groups {
		if len(group.Providers) == 0 {
			continue
		}
		provider := group.Providers[0]
		b.WriteString(fmt.Sprintf("\n### %s\n", group.Engine))
		b.WriteString(fmt.Sprintf("Use: `--agent=%s`\n", group.Engine))
		if provider.Notes != "" {
			b.WriteString(fmt.Sprintf("Notes: %s\n", provider.Notes))
		}
		if !provider.Available {
			b.WriteString("Status: not currently available on PATH/authenticated in this environment.\n")
			continue
		}
		hasRendered = true
		if strings.TrimSpace(filterAgent) == "" {
			b.WriteString("To get model suggestions for this agent, rerun with `--agent=" + group.Engine + "` or ask an agent-scoped tutorial Q&A question.\n")
			continue
		}
		if provider.Name == "command-code" {
			b.WriteString("Status: available, but no live model list is exposed. For current compatibility guidance, ask tutorial Q&A explicitly, for example: `swarminator --tutorial \"what model should I use for code review\" --agent=command-code -m MODEL`.\n")
			continue
		}
		if len(provider.Models) == 0 {
			if provider.ModelSource == "cli-unavailable" {
				b.WriteString(fmt.Sprintf("Status: engine is available, but the live CLI model list could not be fetched. Ask tutorial Q&A such as `swarminator --tutorial \"suggest a cheap model for code review\" --agent=%s`, or retry `swarminator --list-models --agent %s`.\n", group.Engine, group.Engine))
			} else {
				b.WriteString(fmt.Sprintf("Status: no live models were listed for this engine. Ask tutorial Q&A such as `swarminator --tutorial \"suggest a cheap model for code review\" --agent=%s` for current compatibility guidance.\n", group.Engine))
			}
			continue
		}
		if provider.ModelSource != "cli" {
			b.WriteString(fmt.Sprintf("Status: only embedded model hints are available here. For current compatibility guidance, ask tutorial Q&A such as `swarminator --tutorial \"suggest a cheap model for code review\" --agent=%s`.\n", group.Engine))
			continue
		}
		b.WriteString("Current live candidates:\n")
		for _, model := range pickSuggestedModels(provider.Models) {
			b.WriteString(fmt.Sprintf("- `--agent=%s -m %s`\n", group.Engine, model))
		}
		b.WriteString(fmt.Sprintf("Inspect the full list: `swarminator --list-models --agent %s`\n", group.Engine))
	}
	if !hasRendered {
		b.WriteString("\nNo currently available agents exposed model guidance. Start with `swarminator --list-agents`, then retry after configuring an agent.\n")
	}
	return strings.TrimSpace(b.String())
}

func resolveTutorialModel(ctx context.Context, agent string, requestedModel string, query string) (string, error) {
	if strings.TrimSpace(requestedModel) != "" {
		return strings.TrimSpace(requestedModel), nil
	}
	if !tutorial.IsModelSuggestionQuery(query) {
		return "", errors.New("tutorial Q&A requires -m MODEL; built-in topics work without it, and model-suggestion questions may omit it only when swarminator can infer a cheap default for the chosen agent")
	}
	model := inferCheapTutorialModel(ctx, agent)
	if model == "" {
		return "", fmt.Errorf("%w for --agent=%s; pass -m MODEL explicitly", errTutorialDefaultModelUnavailable, agent)
	}
	return model, nil
}

func inferCheapTutorialModel(ctx context.Context, agent string) string {
	groups := llm.DiscoverListings(ctx, agent)
	for _, group := range groups {
		for _, provider := range group.Providers {
			if len(provider.Models) == 0 {
				continue
			}
			models := pickSuggestedModels(provider.Models)
			if len(models) > 0 {
				return models[0]
			}
		}
	}
	return fallbackTutorialModelForAgent(agent)
}

func fallbackTutorialModelForAgent(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "kilo":
		return "kilo/kilo-auto/free"
	case "gemini":
		return "google/gemini-2.5-flash"
	case "claude":
		return "claude/sonnet"
	case "codex":
		return "codex-mini"
	default:
		return ""
	}
}

func tutorialPersona(agent string, query string) string {
	base := "You are a knowledgeable assistant for the swarminator CLI. " +
		"The embedded reference is your primary source; use it for exact flags, exit codes, and examples. " +
		"Answer for the explicit agent selected by the caller. " +
		"Show exact swarminator commands when useful. " +
		"If a question is genuinely outside the scope of swarminator, say so briefly and point to `swarminator --help`."
	if tutorial.IsModelSuggestionQuery(query) {
		return base + " This is an agent-scoped model suggestion question for `--agent=" + agent + "`. Prefer free or low-cost compatible models first. Treat premium models as an explicit caller choice unless no cheaper compatible option exists. Use current compatibility information from live discovery context first; if your runtime supports web search or browsing, use it to verify up-to-date compatibility rather than relying only on stale memory."
	}
	return base + " Use the explicit agent/model chosen by the caller; do not silently reroute to another agent or assume a default kilo fallback."
}

func buildTutorialInput(ctx context.Context, agent string, model string, query string) string {
	var b strings.Builder
	b.WriteString(docs.EmbeddedReference())
	b.WriteString("\n\n---\n\n")
	b.WriteString("Tutorial assistant context:\n")
	b.WriteString("selected agent: ")
	b.WriteString(agent)
	b.WriteString("\nselected model: ")
	b.WriteString(model)
	b.WriteString("\n")
	discovery := llm.FormatDiscoveryText(llm.DiscoverListings(ctx, agent), true, true)
	if strings.TrimSpace(discovery) != "" {
		b.WriteString("\nlive agent discovery:\n")
		b.WriteString(discovery)
		b.WriteString("\n")
	}
	b.WriteString("\nQuestion: ")
	b.WriteString(query)
	return b.String()
}

func pickSuggestedModels(models []llm.ModelListing) []string {
	if len(models) == 0 {
		return nil
	}
	selected := make([]string, 0, 3)
	seenKeys := make(map[string]struct{})
	usedFamilies := make(map[string]struct{})
	addModel := func(name string) bool {
		key := canonicalModelKey(name)
		if _, ok := seenKeys[key]; ok {
			return false
		}
		selected = append(selected, name)
		seenKeys[key] = struct{}{}
		usedFamilies[modelFamily(name)] = struct{}{}
		return true
	}
	addFirstMatch := func(markers ...string) {
		for preferUnusedFamily := 0; preferUnusedFamily < 2; preferUnusedFamily++ {
			for _, model := range models {
				name := model.Name
				key := canonicalModelKey(name)
				if _, ok := seenKeys[key]; ok {
					continue
				}
				if preferUnusedFamily == 0 {
					if _, ok := usedFamilies[modelFamily(name)]; ok {
						continue
					}
				}
				lower := strings.ToLower(name)
				for _, marker := range markers {
					if strings.Contains(lower, marker) {
						addModel(name)
						return
					}
				}
			}
		}
	}

	addFirstMatch("auto", "free", "default")
	addFirstMatch("flash", "mini", "lite", "fast", "haiku", "nano")
	addFirstMatch("pro", "opus", "sonnet", "reasoning", "thinking", "o1", "o3", "r1")

	for preferUnusedFamily := 0; preferUnusedFamily < 2; preferUnusedFamily++ {
		for _, model := range models {
			name := model.Name
			if len(selected) >= 3 {
				break
			}
			key := canonicalModelKey(name)
			if _, ok := seenKeys[key]; ok {
				continue
			}
			if preferUnusedFamily == 0 {
				if _, ok := usedFamilies[modelFamily(name)]; ok {
					continue
				}
			}
			addModel(name)
		}
	}

	return selected
}

func canonicalModelKey(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

func modelFamily(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 {
		return parts[0]
	}
	if parts[0] == "kilo" && len(parts) >= 3 {
		return strings.TrimPrefix(parts[1], "~")
	}
	return strings.TrimPrefix(parts[0], "~")
}

func classifySwarmQuery(query string) string {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(q, "phase") || strings.Contains(q, "workflow") || strings.Contains(q, "maker") || strings.Contains(q, "breaker"):
		return "Focus: phase workflow and retry logic"
	case strings.Contains(q, "quorum") || strings.Contains(q, "consensus"):
		return "Focus: quorum, retry, and stop conditions"
	case strings.Contains(q, "persona"):
		return "Focus: persona groups and prompt construction"
	case strings.Contains(q, "protocol") || strings.Contains(q, "usage"):
		return "Focus: protocol and usage help"
	case strings.Contains(q, "engine") || strings.Contains(q, "agent") || strings.Contains(q, "provider"):
		return "Focus: engine choice and routing"
	case strings.Contains(q, "model"):
		return "Focus: dynamic agent+model suggestions"
	default:
		return "Focus: first-time swarm orchestration guidance"
	}
}
