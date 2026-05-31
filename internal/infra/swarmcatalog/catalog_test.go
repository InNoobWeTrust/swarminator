package swarmcatalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrchestratorModel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "main.json"), []byte(`{
		"id": "main",
		"invoke": "google/gemini-2.5-flash",
		"budget_ref": "google/gemini-2.5-flash",
		"max_output_tokens": 2048,
		"runtime_profile": "main"
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadOrchestratorModel(root, "main")
	if err != nil {
		t.Fatalf("LoadOrchestratorModel() error = %v", err)
	}
	if got.ID != "main" {
		t.Fatalf("ID = %q, want %q", got.ID, "main")
	}
	if got.Invoke != "google/gemini-2.5-flash" {
		t.Fatalf("Invoke = %q, want %q", got.Invoke, "google/gemini-2.5-flash")
	}
	if got.RuntimeProfile != "main" {
		t.Fatalf("RuntimeProfile = %q, want %q", got.RuntimeProfile, "main")
	}
}

func TestLoadOrchestratorModelRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, name := range []string{"a.json", "b.json"} {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte(`{
			"id": "main",
			"invoke": "google/gemini-2.5-flash",
			"budget_ref": "google/gemini-2.5-flash",
			"max_output_tokens": 2048,
			"runtime_profile": "main"
		}`), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	if _, err := LoadOrchestratorModel(root, "main"); err == nil {
		t.Fatal("LoadOrchestratorModel() error = nil, want duplicate ID failure")
	}
}

func TestLoadWorkerModelFromYAML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "worker.yaml"), []byte(`id: reviewer
agent: gemini
invoke: google/gemini-2.5-flash
budget_ref: google/gemini-2.5-flash
max_output_tokens: 4096
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadWorkerModel(root, "reviewer")
	if err != nil {
		t.Fatalf("LoadWorkerModel() error = %v", err)
	}
	if got.Agent != "gemini" {
		t.Fatalf("Agent = %q, want %q", got.Agent, "gemini")
	}
}

func TestLoadPersona(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	personasDir := filepath.Join(root, "personas")
	if err := os.MkdirAll(personasDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(personasDir, "critic.md"), []byte(`---
id: critic
group: review
intent: identify risks
---
You are a critical reviewer.
Return only concise findings.
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadPersona(root, "critic")
	if err != nil {
		t.Fatalf("LoadPersona() error = %v", err)
	}
	if got.Group != "review" || got.Intent != "identify risks" {
		t.Fatalf("persona metadata = %#v, want populated frontmatter fields", got)
	}
	if got.Prompt == "" {
		t.Fatal("persona prompt is empty")
	}
}

func TestLoadPersonaRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	personasDir := filepath.Join(root, "personas")
	if err := os.MkdirAll(personasDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := []byte(`---
id: critic
group: review
intent: identify risks
---
Prompt text.
`)
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(personasDir, name), content, 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	if _, err := LoadPersona(root, "critic"); err == nil {
		t.Fatal("LoadPersona() error = nil, want duplicate ID failure")
	}
}

func TestLoadWorkerCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	personasDir := filepath.Join(root, "personas")
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(models) error = %v", err)
	}
	if err := os.MkdirAll(personasDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(personas) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "main.json"), []byte(`{
		"id": "main",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 2048,
		"runtime_profile": "main"
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(main.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "worker.json"), []byte(`{
		"id": "worker",
		"invoke": "github-copilot/gpt-5-mini",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 1024,
		"agent": "kilo"
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(worker.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "reviewer.json"), []byte(`{
		"id": "reviewer",
		"invoke": "kilo/kilo-auto/free",
		"budget_ref": "openai/gpt-4.1",
		"max_output_tokens": 1024,
		"agent": "kilo"
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(reviewer.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(personasDir, "critic.md"), []byte(`---
id: critic
group: review
intent: identify risks
---
You are a critical reviewer.
`), 0o600); err != nil {
		t.Fatalf("WriteFile(critic.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(personasDir, "summarizer.md"), []byte(`---
id: summarizer
group: synthesis
intent: summarize findings
---
You are a concise summarizer.
`), 0o600); err != nil {
		t.Fatalf("WriteFile(summarizer.md) error = %v", err)
	}

	catalog, err := LoadWorkerCatalog(root)
	if err != nil {
		t.Fatalf("LoadWorkerCatalog() error = %v", err)
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("worker catalog models = %d, want 2 worker models", len(catalog.Models))
	}
	if len(catalog.Personas) != 2 {
		t.Fatalf("worker catalog personas = %d, want 2 personas", len(catalog.Personas))
	}
	if catalog.Models[0].ID != "reviewer" || catalog.Models[1].ID != "worker" {
		t.Fatalf("worker catalog models = %#v, want sorted worker models only", catalog.Models)
	}
	if catalog.Personas[0].ID != "critic" || catalog.Personas[1].ID != "summarizer" {
		t.Fatalf("worker catalog personas = %#v, want sorted personas", catalog.Personas)
	}
}
