package orchestratorconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AuthMethod string

const (
	AuthMethodAPIKeyEnv      AuthMethod = "api_key_env"
	AuthMethodBearerTokenEnv AuthMethod = "bearer_token_env"
)

type AuthConfig struct {
	Method        AuthMethod `json:"method"`
	CredentialRef string     `json:"credential_ref"`
}

type RoutingConfig struct {
	ProjectID string `json:"project_id,omitempty"`
	Region    string `json:"region,omitempty"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

type Config struct {
	Backend          string        `json:"backend"`
	MessageAPIFormat string        `json:"message_api_format"`
	BaseURL          string        `json:"base_url,omitempty"`
	BaseURLRef       string        `json:"base_url_ref,omitempty"`
	EnvFile          string        `json:"env_file,omitempty"`
	TimeoutSeconds   int           `json:"timeout_seconds,omitempty"`
	Auth             AuthConfig    `json:"auth"`
	Routing          RoutingConfig `json:"routing,omitempty"`
}

func ResolvePath() (string, error) {
	configHome, err := resolveConfigHome()
	if err != nil {
		return "", err
	}

	return filepath.Join(configHome, "swarminator", "orchestrator.json"), nil
}

func ResolveProfilePath(profile string) (string, error) {
	configHome, err := resolveConfigHome()
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(profile)
	if trimmed == "" || trimmed == "default" {
		return filepath.Join(configHome, "swarminator", "orchestrator.json"), nil
	}

	return filepath.Join(configHome, "swarminator", "orchestrators", trimmed+".json"), nil
}

func resolveConfigHome() (string, error) {
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		var err error
		configHome, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve XDG config dir: %w", err)
		}
	}

	if configHome == "" {
		return "", errors.New("XDG config dir is required")
	}

	return configHome, nil
}

func Load() (Config, error) {
	path, err := ResolvePath()
	if err != nil {
		return Config{}, err
	}
	return LoadFromPath(path)
}

func LoadProfile(profile string) (Config, error) {
	path, err := ResolveProfilePath(profile)
	if err != nil {
		return Config{}, err
	}
	return LoadFromPath(path)
}

func LoadFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read orchestrator config %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse orchestrator config %q: %w", path, err)
	}
	if err := cfg.LoadEnvFile(filepath.Dir(path)); err != nil {
		return Config{}, fmt.Errorf("load orchestrator env file for %q: %w", path, err)
	}
	if err := cfg.ResolveEnvReferences(); err != nil {
		return Config{}, fmt.Errorf("resolve orchestrator env references for %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate orchestrator config %q: %w", path, err)
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Backend) == "" {
		return errors.New("backend is required")
	}
	if strings.TrimSpace(c.MessageAPIFormat) == "" {
		return errors.New("message_api_format is required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errors.New("base_url is required")
	}
	if c.TimeoutSeconds < 0 {
		return errors.New("timeout_seconds must be >= 0")
	}
	if err := c.Auth.Validate(); err != nil {
		return err
	}
	return nil
}

func (a AuthConfig) Validate() error {
	switch a.Method {
	case AuthMethodAPIKeyEnv, AuthMethodBearerTokenEnv:
	default:
		return fmt.Errorf("unsupported auth method %q", a.Method)
	}
	if strings.TrimSpace(a.CredentialRef) == "" {
		return errors.New("credential_ref is required")
	}
	return nil
}

func (c Config) LoadEnvFile(baseDir string) error {
	envPath := strings.TrimSpace(c.EnvFile)
	if envPath == "" {
		return nil
	}
	if !filepath.IsAbs(envPath) {
		envPath = filepath.Join(strings.TrimSpace(baseDir), envPath)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("read env file %q: %w", envPath, err)
	}
	for idx, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid env line %d", idx+1)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return fmt.Errorf("invalid env line %d", idx+1)
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %q: %w", key, err)
		}
	}
	return nil
}

func (c *Config) ResolveEnvReferences() error {
	if strings.TrimSpace(c.BaseURL) == "" && strings.TrimSpace(c.BaseURLRef) != "" {
		resolved, err := resolveRequiredEnv(c.BaseURLRef)
		if err != nil {
			return fmt.Errorf("resolve base_url_ref: %w", err)
		}
		c.BaseURL = resolved
	}
	return nil
}

func resolveRequiredEnv(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", errors.New("env key is required")
	}
	value := strings.TrimSpace(os.Getenv(trimmed))
	if value == "" {
		return "", fmt.Errorf("env %q is empty or unset", trimmed)
	}
	return value, nil
}
