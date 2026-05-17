package llm

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"swarminator/internal/domain/agent"
)

const listingTimeout = 5 * time.Second

func DiscoverListings(ctx context.Context, filterAgent string, registry agent.AgentRegistry) []agent.EngineListing {
	all := registry.GetAll()
	if len(all) == 0 {
		all = agent.KnownAgents()
	}

	groups := make([]agent.EngineListing, 0, len(all))
	for _, a := range all {
		if filterAgent != "" && !strings.EqualFold(a.Name, filterAgent) {
			continue
		}
	models, modelSource := discoverModels(ctx, a)
	modelsWithName := make([]agent.ModelInfo, len(models))
	for i, m := range models {
		modelsWithName[i] = agent.ModelInfo{ID: m.Name, Source: m.Source}
	}
	groups = append(groups, agent.EngineListing{
		Engine: a.Name,
		Providers: []agent.ProviderListing{{
			Name:        a.Name,
			Binary:      a.Binary,
			Available:   a.Available,
			Source:      "embedded",
			Notes:       providerNotes(a.Name),
			ModelSource: modelSource,
			Models:      modelsWithName,
		}},
	})
	}

	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].Engine < groups[j].Engine
	})
	return groups
}

func providerNotes(agentName string) string {
	switch agentName {
	case "kilo":
		return "gateway for provider-qualified models"
	case "gemini":
		return "native headless CLI execution"
	case "claude":
		return "ACP agent"
	case "codex":
		return "explicit-only (--agent=codex)"
	case "command-code":
		return "explicit-only (--agent=command-code)"
	default:
		return ""
	}
}

func discoverModels(ctx context.Context, a agent.AgentInfo) ([]agent.ModelListing, string) {
	switch a.Name {
	case "kilo":
		if models := listKiloModels(ctx, a.Binary); len(models) > 0 {
			return models, "cli"
		}
		return nil, "cli-unavailable"
	case "gemini":
		return staticModelListings([]string{"google/gemini-2.5-flash", "google/gemini-2.5-pro", "gemini-2.5-flash"}, "embedded"), "embedded"
	case "claude":
		return staticModelListings([]string{"claude/sonnet", "anthropic/claude-3.7-sonnet", "sonnet-4"}, "embedded"), "embedded"
	case "codex":
		return staticModelListings([]string{"codex-mini", "openai/gpt-5.1-codex"}, "embedded"), "embedded"
	default:
		return nil, "none"
	}
}

func staticModelListings(models []string, source string) []agent.ModelListing {
	out := make([]agent.ModelListing, 0, len(models))
	for _, m := range models {
		out = append(out, agent.ModelListing{Name: m, Source: source})
	}
	return out
}

func listKiloModels(ctx context.Context, binary string) []agent.ModelListing {
	if strings.TrimSpace(binary) == "" {
		return nil
	}
	listCtx, cancel := context.WithTimeout(ctx, listingTimeout)
	defer cancel()

	cmd := exec.CommandContext(listCtx, binary, "models")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	models := make([]agent.ModelListing, 0, 32)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		models = append(models, agent.ModelListing{Name: line, Source: "cli"})
	}
	if scanner.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return nil
	}
	_ = cmd.Wait()
	return models
}

func FormatDiscoveryText(groups []agent.EngineListing, includeModels, includeProviders bool) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("ENGINE %s\n", g.Engine))
		for _, p := range g.Providers {
			b.WriteString(fmt.Sprintf("  PROVIDER %s\n", p.Name))
			if includeProviders {
				b.WriteString(fmt.Sprintf("    binary: %s\n", p.Binary))
				b.WriteString(fmt.Sprintf("    available: %t\n", p.Available))
				b.WriteString(fmt.Sprintf("    source: %s\n", p.Source))
				if p.Notes != "" {
					b.WriteString(fmt.Sprintf("    notes: %s\n", p.Notes))
				}
			}
			if includeModels {
				b.WriteString(fmt.Sprintf("    model-source: %s\n", p.ModelSource))
				if len(p.Models) == 0 {
					b.WriteString("    models: none\n")
					continue
				}
				for _, m := range p.Models {
					b.WriteString(fmt.Sprintf("    - %s\n", m.ID))
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}