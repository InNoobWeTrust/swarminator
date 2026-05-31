package orchestratorconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.json")
	data := []byte(`{
		"backend": "openai-compatible",
		"message_api_format": "openai.chat.completions",
		"base_url": "http://127.0.0.1:8080",
		"timeout_seconds": 90,
		"auth": {
			"method": "bearer_token_env",
			"credential_ref": "KILO_API_KEY"
		},
		"routing": {
			"project_id": "proj-123",
			"region": "us-central1",
			"runtime_id": "runtime-1"
		}
	}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}

	if got.Backend != "openai-compatible" {
		t.Fatalf("Backend = %q, want %q", got.Backend, "openai-compatible")
	}
	if got.MessageAPIFormat != "openai.chat.completions" {
		t.Fatalf("MessageAPIFormat = %q, want %q", got.MessageAPIFormat, "openai.chat.completions")
	}
	if got.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "http://127.0.0.1:8080")
	}
	if got.TimeoutSeconds != 90 {
		t.Fatalf("TimeoutSeconds = %d, want %d", got.TimeoutSeconds, 90)
	}
	if got.Auth.Method != AuthMethodBearerTokenEnv {
		t.Fatalf("Auth.Method = %q, want %q", got.Auth.Method, AuthMethodBearerTokenEnv)
	}
	if got.Auth.CredentialRef != "KILO_API_KEY" {
		t.Fatalf("Auth.CredentialRef = %q, want %q", got.Auth.CredentialRef, "KILO_API_KEY")
	}
	if got.Routing.ProjectID != "proj-123" || got.Routing.Region != "us-central1" || got.Routing.RuntimeID != "runtime-1" {
		t.Fatalf("Routing = %#v, want populated routing fields", got.Routing)
	}
}

func TestLoadFromPathLoadsEnvFileWithoutOverwritingExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "orchestrator.env")
	if err := os.WriteFile(envPath, []byte("KILO_TEST_API_KEY=from-file\nexport KILO_TEST_BASE_URL=https://ignored.example\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}
	t.Setenv("KILO_TEST_BASE_URL", "https://already-set.example")

	path := filepath.Join(dir, "orchestrator.json")
	data := []byte(`{
		"backend": "openai-compatible",
		"message_api_format": "openai.chat.completions",
		"base_url_ref": "KILO_TEST_BASE_URL",
		"env_file": "orchestrator.env",
		"auth": {
			"method": "bearer_token_env",
			"credential_ref": "KILO_TEST_API_KEY"
		}
	}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	if _, err := LoadFromPath(path); err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}
	if got := os.Getenv("KILO_TEST_API_KEY"); got != "from-file" {
		t.Fatalf("KILO_TEST_API_KEY = %q, want %q", got, "from-file")
	}
	if got := os.Getenv("KILO_TEST_BASE_URL"); got != "https://already-set.example" {
		t.Fatalf("KILO_TEST_BASE_URL = %q, want preexisting value", got)
	}
	loaded, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() second pass error = %v", err)
	}
	if loaded.BaseURL != "https://already-set.example" {
		t.Fatalf("BaseURL = %q, want env-resolved preexisting value", loaded.BaseURL)
	}
}

func TestLoadFromPathRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.json")
	if err := os.WriteFile(path, []byte(`{"backend":"openai-compatible"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := LoadFromPath(path); err == nil {
		t.Fatal("LoadFromPath() error = nil, want validation failure")
	}
}

func TestLoadFromPathRejectsNegativeTimeout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator.json")
	if err := os.WriteFile(path, []byte(`{
		"backend": "openai-compatible",
		"message_api_format": "openai.chat.completions",
		"base_url": "http://127.0.0.1:8080",
		"timeout_seconds": -1,
		"auth": {"method": "bearer_token_env", "credential_ref": "KILO_API_KEY"}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := LoadFromPath(path); err == nil {
		t.Fatal("LoadFromPath() error = nil, want negative timeout validation failure")
	}
}

func TestResolvePathUsesXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := ResolvePath()
	if err != nil {
		t.Fatalf("ResolvePath() error = %v", err)
	}

	want := filepath.Join(dir, "swarminator", "orchestrator.json")
	if got != want {
		t.Fatalf("ResolvePath() = %q, want %q", got, want)
	}
}

func TestLoadProfileUsesDefaultConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "swarminator", "orchestrator.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{
		"backend": "openai-compatible",
		"message_api_format": "openai.chat.completions",
		"base_url": "http://127.0.0.1:8080",
		"auth": {"method": "bearer_token_env", "credential_ref": "KILO_API_KEY"}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if got.Backend != "openai-compatible" {
		t.Fatalf("Backend = %q, want %q", got.Backend, "openai-compatible")
	}
}

func TestLoadProfileUsesNamedProfilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "swarminator", "orchestrators", "main.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{
		"backend": "openai-compatible",
		"message_api_format": "openai.chat.completions",
		"base_url": "http://127.0.0.1:8080",
		"auth": {"method": "bearer_token_env", "credential_ref": "KILO_API_KEY"}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadProfile("main")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if got.MessageAPIFormat != "openai.chat.completions" {
		t.Fatalf("MessageAPIFormat = %q, want %q", got.MessageAPIFormat, "openai.chat.completions")
	}
}
