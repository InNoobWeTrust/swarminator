package llm

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const listingTimeout = 5 * time.Second

// ModelListing describes one model entry for discovery output.
type ModelListing struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// ProviderListing describes one provider entry grouped under an engine.
type ProviderListing struct {
	Name        string         `json:"name"`
	Binary      string         `json:"binary"`
	Available   bool           `json:"available"`
	Source      string         `json:"source"`
	Notes       string         `json:"notes,omitempty"`
	ModelSource string         `json:"model_source"`
	Models      []ModelListing `json:"models,omitempty"`
}

// EngineListing groups providers by engine first.
type EngineListing struct {
	Engine    string            `json:"engine"`
	Providers []ProviderListing `json:"providers"`
}

// DiscoverListings returns engine-grouped provider/model discovery data.
func DiscoverListings(ctx context.Context, filterAgent string) []EngineListing {
	registry := NewAgentRegistry()
	_ = registry.Detect()
	all := registry.GetAll()
	if len(all) == 0 {
		all = KnownAgents()
	}

	groups := make([]EngineListing, 0, len(all))
	for _, agent := range all {
		if filterAgent != "" && !strings.EqualFold(agent.Name, filterAgent) {
			continue
		}
		models, modelSource := discoverModels(ctx, agent)
		groups = append(groups, EngineListing{
			Engine: agent.Name,
			Providers: []ProviderListing{{
				Name:        agent.Name,
				Binary:      agent.Binary,
				Available:   agent.Available,
				Source:      "embedded",
				Notes:       providerNotes(agent.Name),
				ModelSource: modelSource,
				Models:      models,
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

func discoverModels(ctx context.Context, agent AgentInfo) ([]ModelListing, string) {
	switch agent.Name {
	case "kilo":
		if models := listKiloModels(ctx, agent.Binary); len(models) > 0 {
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

func staticModelListings(models []string, source string) []ModelListing {
	out := make([]ModelListing, 0, len(models))
	for _, model := range models {
		out = append(out, ModelListing{Name: model, Source: source})
	}
	return out
}

func listKiloModels(ctx context.Context, binary string) []ModelListing {
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
	models := make([]ModelListing, 0, 32)
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
		models = append(models, ModelListing{Name: line, Source: "cli"})
	}
	if scanner.Err() != nil {
		waitForProcessExit(cmd, acpProcessWaitTimeout)
		return nil
	}
	// Ignore non-zero exit status: a partial model list is more useful than
	// nothing (e.g. kilo may exit 1 but still print models to stdout).
	_ = cmd.Wait()
	return models
}

func FormatDiscoveryText(groups []EngineListing, includeModels bool, includeProviders bool) string {
	var b strings.Builder
	for i, group := range groups {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("ENGINE %s\n", group.Engine))
		for _, provider := range group.Providers {
			b.WriteString(fmt.Sprintf("  PROVIDER %s\n", provider.Name))
			if includeProviders {
				b.WriteString(fmt.Sprintf("    binary: %s\n", provider.Binary))
				b.WriteString(fmt.Sprintf("    available: %t\n", provider.Available))
				b.WriteString(fmt.Sprintf("    source: %s\n", provider.Source))
				if provider.Notes != "" {
					b.WriteString(fmt.Sprintf("    notes: %s\n", provider.Notes))
				}
			}
			if includeModels {
				b.WriteString(fmt.Sprintf("    model-source: %s\n", provider.ModelSource))
				if len(provider.Models) == 0 {
					b.WriteString("    models: none\n")
					continue
				}
				for _, model := range provider.Models {
					b.WriteString(fmt.Sprintf("    - %s\n", model.Name))
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}
