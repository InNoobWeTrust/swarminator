package tutorial

import (
	"context"
	"fmt"
	"strings"
	"time"

	"swarminator/internal/app/promptbuilder"
	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/tutorial"
	"swarminator/internal/infra/llm"
)

const tutorialTimeout = 120 * time.Second

type Service struct {
	registry      agent.AgentRegistry
	discovery     agent.DiscoveryProvider
	llmProvider   agent.LLMProvider
	promptBuilder *promptbuilder.Service
}

func NewService() *Service {
	registry := llm.NewAgentRegistry()
	discovery := llm.NewDiscoveryProvider(registry)
	return NewServiceWithRegistry(registry, discovery, llm.NewUnifiedProvider(""))
}

func NewServiceWithRegistry(registry agent.AgentRegistry, discovery agent.DiscoveryProvider, llmProvider agent.LLMProvider) *Service {
	return &Service{
		registry:      registry,
		discovery:     discovery,
		llmProvider:   llmProvider,
		promptBuilder: promptbuilder.NewServiceWithRegistry(discovery),
	}
}

func (s *Service) Answer(ctx context.Context, req TutorialRequest) (string, error) {
	query := strings.TrimSpace(req.Query)
	if isSwarmTutorialQuery(query) {
		return renderSwarmTutorial(ctx, query, req.Agent, s.discovery), nil
	}
	if tutorial.IsBuiltInQuery(query) {
		return tutorial.Lookup(query), nil
	}

	model, err := resolveTutorialModel(ctx, req.Agent, req.Model, query, s.discovery)
	if err != nil {
		return "", err
	}

	llmProvider := s.llmProvider

	tctx, cancel := context.WithTimeout(ctx, tutorialTimeout)
	defer cancel()

	input := s.promptBuilder.BuildTutorialInput(ctx, req.Agent, model, query)

	wasInferred := strings.TrimSpace(req.Model) == "" && model != ""

	result, err := llmProvider.Complete(tctx, agent.CompletionRequest{
		ModelID:   model,
		Persona:   tutorial.TutorialPersona(req.Agent, query),
		Input:     input,
		AgentMode: agent.AgentMode(req.AgentMode),
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

type TutorialRequest struct {
	Query     string
	Agent     string
	Model     string
	AgentMode string
}

func isSwarmTutorialQuery(query string) bool {
	q := strings.TrimSpace(strings.ToLower(query))
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

func renderSwarmTutorial(ctx context.Context, query, filterAgent string, discovery agent.DiscoveryProvider) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(tutorial.Lookup("swarm")))
	b.WriteString("\n\n## Query Focus\n\n")
	b.WriteString(classifySwarmQuery(query))

	groups := discovery.DiscoverListings(ctx, filterAgent)
	if suggestions := renderSwarmSuggestions(groups, filterAgent); suggestions != "" {
		b.WriteString("\n\n")
		b.WriteString(suggestions)
	}
	return strings.TrimSpace(b.String())
}

func renderSwarmSuggestions(groups []agent.EngineListing, filterAgent string) string {
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

func resolveTutorialModel(ctx context.Context, agentName, requestedModel, query string, discovery agent.DiscoveryProvider) (string, error) {
	if strings.TrimSpace(requestedModel) != "" {
		return strings.TrimSpace(requestedModel), nil
	}
	if !tutorial.IsModelSuggestionQuery(query) {
		return "", fmt.Errorf("tutorial Q&A requires -m MODEL; built-in topics work without it, and model-suggestion questions may omit it only when swarminator can infer a cheap default for the chosen agent")
	}
	model := inferCheapTutorialModel(ctx, agentName, discovery)
	if model == "" {
		return "", fmt.Errorf("could not infer a default cheap model for tutorial Q&A for --agent=%s; pass -m MODEL explicitly", agentName)
	}
	return model, nil
}

func inferCheapTutorialModel(ctx context.Context, agentName string, discovery agent.DiscoveryProvider) string {
	groups := discovery.DiscoverListings(ctx, agentName)
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
	return fallbackTutorialModelForAgent(agentName)
}

func fallbackTutorialModelForAgent(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
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

func pickSuggestedModels(models []agent.ModelInfo) []string {
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
				name := model.ID
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
			name := model.ID
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
