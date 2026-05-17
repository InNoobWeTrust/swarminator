package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentRegistryDetect(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir)

	// kilo: --version exits 0 → Available=true, Authenticated=true
	writeTestExecutable(t, tempDir, "kilo", `#!/bin/sh
if [ "$1" = "--version" ]; then
	echo "1.0.0"
	exit 0
fi
exit 1
`)

	// gemini: --version exits 0 → Available=true, Authenticated=true
	// (auth status is NOT probed; it can hang indefinitely on real gemini)
	writeTestExecutable(t, tempDir, "gemini", `#!/bin/sh
if [ "$1" = "--version" ]; then
	echo "0.38.2"
	exit 0
fi
exit 1
`)

	// codex: --version exits 1 → not usable → Available=false
	writeTestExecutable(t, tempDir, "codex", `#!/bin/sh
exit 1
`)

	registry := NewAgentRegistry()
	if err := registry.Detect(); err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if got := len(registry.agents); got != 5 {
		t.Fatalf("len(registry.agents) = %d, want 5", got)
	}

	kilo := mustFindAgent(t, registry.agents, "kilo")
	if got := kilo.ACPArgs; len(got) != 1 || got[0] != "acp" {
		t.Fatalf("kilo.ACPArgs = %v, want [acp]", got)
	}
	if !kilo.Available {
		t.Fatalf("kilo.Available = false, want true")
	}
	if !kilo.Authenticated {
		t.Fatalf("kilo.Authenticated = false, want true")
	}

	gemini := mustFindAgent(t, registry.agents, "gemini")
	if got := gemini.ACPArgs; len(got) != 0 {
		t.Fatalf("gemini.ACPArgs = %v, want []", got)
	}
	if !gemini.Available {
		t.Fatalf("gemini.Available = false, want true")
	}
	// --version exits 0 → Authenticated=true (auth status is not probed)
	if !gemini.Authenticated {
		t.Fatalf("gemini.Authenticated = false, want true")
	}

	codex := mustFindAgent(t, registry.agents, "codex")
	if got := codex.ACPArgs; len(got) != 0 {
		t.Fatalf("codex.ACPArgs = %v, want []", got)
	}
	if codex.Available {
		t.Fatalf("codex.Available = true, want false")
	}
	if codex.Authenticated {
		t.Fatalf("codex.Authenticated = true, want false")
	}

	commandCode := mustFindAgent(t, registry.agents, "command-code")
	if got := commandCode.ACPArgs; len(got) != 0 {
		t.Fatalf("command-code.ACPArgs = %v, want []", got)
	}
	if commandCode.Available {
		t.Fatalf("command-code.Available = true, want false")
	}
	if commandCode.Authenticated {
		t.Fatalf("command-code.Authenticated = true, want false")
	}

	claude := mustFindAgent(t, registry.agents, "claude")
	if got := claude.ACPArgs; len(got) != 1 || got[0] != "--acp" {
		t.Fatalf("claude.ACPArgs = %v, want [--acp]", got)
	}
	if claude.Available {
		t.Fatalf("claude.Available = true, want false")
	}
	if claude.Authenticated {
		t.Fatalf("claude.Authenticated = true, want false")
	}
}

func TestAgentRegistryGetForModel(t *testing.T) {
	registry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Binary: "kilo", ACPArgs: []string{"acp"}, Available: true, Authenticated: true, ModelPrefixes: []string{"kilo/", "minimax/", "openai/", "o1-", "o3-", "gpt-", "codex-"}},
			{Name: "gemini", Binary: "gemini", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{"google/", "gemini/", "gemini-"}},
			{Name: "codex", Binary: "codex", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{}},
			{Name: "claude", Binary: "claude", ACPArgs: []string{"--acp"}, Available: true, Authenticated: true, ModelPrefixes: []string{"claude/", "anthropic/", "sonnet-"}},
		},
	}

	tests := []struct {
		name      string
		model     string
		wantAgent string
	}{
		{name: "google prefix uses gemini", model: "google/gemini-2.5-pro", wantAgent: "gemini"},
		{name: "gemini prefix uses gemini", model: "gemini/flash", wantAgent: "gemini"},
		{name: "kilo prefix uses kilo", model: "kilo/default", wantAgent: "kilo"},
		{name: "openai prefix uses kilo", model: "openai/gpt-5", wantAgent: "kilo"},
		{name: "o3 prefix uses kilo", model: "o3-mini", wantAgent: "kilo"},
		{name: "codex- prefix uses kilo", model: "codex-mini", wantAgent: "kilo"},
		{name: "gpt prefix uses kilo", model: "gpt-4.1", wantAgent: "kilo"},
		{name: "claude prefix uses claude", model: "claude/sonnet", wantAgent: "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := registry.GetForModel(tt.model)
			if agent == nil {
				t.Fatalf("GetForModel(%q) = nil, want %q", tt.model, tt.wantAgent)
			}
			if agent.Name != tt.wantAgent {
				t.Fatalf("GetForModel(%q) agent = %q, want %q", tt.model, agent.Name, tt.wantAgent)
			}
		})
	}

	fallbackRegistry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Binary: "kilo", ACPArgs: []string{"acp"}, Available: true, Authenticated: false, ModelPrefixes: []string{"kilo/", "openai/"}},
			{Name: "gemini", Binary: "gemini", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{"google/", "gemini/"}},
			{Name: "codex", Binary: "codex", ACPArgs: []string{}, Available: true, Authenticated: true, ModelPrefixes: []string{}},
		},
	}

	fallback := fallbackRegistry.GetForModel("unknown/provider")
	if fallback == nil {
		t.Fatalf("GetForModel(%q) = nil, want authenticated fallback", "unknown/provider")
	}
	if fallback.Name != "gemini" {
		t.Fatalf("GetForModel(%q) fallback agent = %q, want %q", "unknown/provider", fallback.Name, "gemini")
	}
}

func TestCodexExplicitOnlyRouting(t *testing.T) {
	// Codex has no ModelPrefixes in KnownAgents; it must not be selected by prefix.
	agents := KnownAgents()
	var codexInfo AgentInfo
	for _, a := range agents {
		if a.Name == "codex" {
			codexInfo = a
		}
	}
	if len(codexInfo.ModelPrefixes) != 0 {
		t.Fatalf("KnownAgents() codex.ModelPrefixes = %v, want empty (explicit-only)", codexInfo.ModelPrefixes)
	}

	// GetByName still resolves codex when the binary is marked available.
	registry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Binary: "kilo", Available: true, Authenticated: true, ModelPrefixes: []string{"kilo/", "openai/", "gpt-"}},
			{Name: "codex", Binary: "codex", Available: true, Authenticated: true, ModelPrefixes: []string{}},
		},
	}
	if got := registry.GetByName("codex"); got == nil || got.Name != "codex" {
		t.Fatalf("GetByName(\"codex\") = %v, want codex agent", got)
	}

	// GetForModel with a GPT prefix must NOT select codex.
	if got := registry.GetForModel("gpt-4.1"); got == nil || got.Name == "codex" {
		t.Fatalf("GetForModel(\"gpt-4.1\") = %v, want kilo (not codex)", got)
	}
}

func TestAgentRegistryGetAllAvailable(t *testing.T) {
	registry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Available: true},
			{Name: "gemini", Available: false},
			{Name: "codex", Available: true},
			{Name: "claude", Available: false},
		},
	}

	available := registry.GetAllAvailable()
	if got := len(available); got != 2 {
		t.Fatalf("len(GetAllAvailable()) = %d, want 2", got)
	}

	if available[0].Name != "kilo" {
		t.Fatalf("GetAllAvailable()[0].Name = %q, want %q", available[0].Name, "kilo")
	}
	if available[1].Name != "codex" {
		t.Fatalf("GetAllAvailable()[1].Name = %q, want %q", available[1].Name, "codex")
	}
}

func TestAgentInfoString(t *testing.T) {
	tests := []struct {
		name  string
		agent AgentInfo
		want  string
	}{
		{
			name:  "unavailable",
			agent: AgentInfo{Name: "kilo", Binary: "kilo", Available: false, Authenticated: false},
			want:  "kilo (kilo): unavailable",
		},
		{
			name:  "available but unauthenticated",
			agent: AgentInfo{Name: "gemini", Binary: "gemini", Available: true, Authenticated: false},
			want:  "gemini (gemini): available",
		},
		{
			name:  "authenticated",
			agent: AgentInfo{Name: "claude", Binary: "claude", Available: true, Authenticated: true},
			want:  "claude (claude): authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.String(); got != tt.want {
				t.Fatalf("AgentInfo.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeTestExecutable(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustFindAgent(t *testing.T, agents []AgentInfo, name string) AgentInfo {
	t.Helper()

	for _, agent := range agents {
		if agent.Name == name {
			return agent
		}
	}

	t.Fatalf("agent %q not found", name)
	return AgentInfo{}
}

func TestAgentRegistryGetAll(t *testing.T) {
	registry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Available: true},
			{Name: "gemini", Available: false},
			{Name: "codex", Available: true},
			{Name: "claude", Available: false},
		},
	}

	all := registry.GetAll()
	if got := len(all); got != 4 {
		t.Fatalf("len(GetAll()) = %d, want 4", got)
	}
	// Should include unavailable agents.
	if all[1].Name != "gemini" {
		t.Fatalf("GetAll()[1].Name = %q, want %q", all[1].Name, "gemini")
	}
}

func TestAgentRegistryResolveRoute(t *testing.T) {
	registry := &AgentRegistry{
		agents: []AgentInfo{
			{Name: "kilo", Binary: "kilo", Available: true, Authenticated: true, ModelPrefixes: []string{"kilo/", "openai/", "github-copilot/", "openrouter/", "gpt-", "o1-", "o3-", "codex-"}},
			{Name: "gemini", Binary: "gemini", Available: true, Authenticated: true, ModelPrefixes: []string{"google/", "gemini/", "gemini-"}},
			{Name: "codex", Binary: "codex", Available: true, Authenticated: true, ModelPrefixes: []string{}},
			{Name: "claude", Binary: "claude", Available: true, Authenticated: true, ModelPrefixes: []string{"claude/", "anthropic/", "sonnet-"}},
		},
	}

	tests := []struct {
		name        string
		model       string
		wantAgent   string
		wantErrFrag string
	}{
		{name: "github-copilot prefix", model: "github-copilot/gpt-5-mini", wantAgent: "kilo"},
		{name: "openrouter prefix", model: "openrouter/meta-llama-3", wantAgent: "kilo"},
		{name: "google prefix", model: "google/gemini-2.5-flash", wantAgent: "gemini"},
		{name: "gemini dash prefix", model: "gemini-2.5-pro", wantAgent: "gemini"},
		{name: "claude prefix", model: "claude/sonnet-3-7", wantAgent: "claude"},
		{name: "unknown provider prefix fails", model: "badprovider/model", wantErrFrag: "badprovider/model"},
		{name: "unqualified model falls back", model: "somemodel", wantAgent: "kilo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ResolveRoute(tt.model)
			if tt.wantErrFrag != "" {
				if err == nil {
					t.Fatalf("ResolveRoute(%q) error = nil, want error containing %q", tt.model, tt.wantErrFrag)
				}
				if !strings.Contains(err.Error(), tt.wantErrFrag) {
					t.Fatalf("ResolveRoute(%q) error = %q, want substring %q", tt.model, err.Error(), tt.wantErrFrag)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRoute(%q) unexpected error = %v", tt.model, err)
			}
			if result.AgentName != tt.wantAgent {
				t.Fatalf("ResolveRoute(%q).AgentName = %q, want %q", tt.model, result.AgentName, tt.wantAgent)
			}
		})
	}
}

func TestKnownAgentsIncludesGitHubCopilotPrefix(t *testing.T) {
	agents := KnownAgents()
	for _, a := range agents {
		if a.Name == "kilo" {
			for _, p := range a.ModelPrefixes {
				if p == "github-copilot/" {
					return
				}
			}
			t.Fatalf("kilo ModelPrefixes = %v, want to include %q", a.ModelPrefixes, "github-copilot/")
		}
	}
	t.Fatal("kilo agent not found in KnownAgents()")
}
