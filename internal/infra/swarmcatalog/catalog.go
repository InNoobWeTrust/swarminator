package swarmcatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type BudgetOverride struct {
	Context int `json:"context,omitempty"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output,omitempty"`
}

type ModelDefinition struct {
	ID              string         `json:"id"`
	Invoke          string         `json:"invoke"`
	BudgetRef       string         `json:"budget_ref,omitempty"`
	BudgetOverride  BudgetOverride `json:"budget_override,omitempty"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	RuntimeProfile  string         `json:"runtime_profile,omitempty"`
	Agent           string         `json:"agent,omitempty"`
}

type PersonaDefinition struct {
	ID     string
	Group  string
	Intent string
	Prompt string
}

type WorkerCatalog struct {
	Models   []ModelDefinition
	Personas []PersonaDefinition
}

func LoadOrchestratorModel(swarmRoot, id string) (ModelDefinition, error) {
	models, err := loadModels(swarmRoot)
	if err != nil {
		return ModelDefinition{}, err
	}
	model, ok := models[strings.TrimSpace(id)]
	if !ok {
		return ModelDefinition{}, fmt.Errorf("orchestrator model %q was not found under %q", id, filepath.Join(swarmRoot, "models"))
	}
	if strings.TrimSpace(model.RuntimeProfile) == "" {
		return ModelDefinition{}, fmt.Errorf("orchestrator model %q is missing runtime_profile", model.ID)
	}
	return model, nil
}

func LoadWorkerModel(swarmRoot, id string) (ModelDefinition, error) {
	models, err := loadModels(swarmRoot)
	if err != nil {
		return ModelDefinition{}, err
	}
	model, ok := models[strings.TrimSpace(id)]
	if !ok {
		return ModelDefinition{}, fmt.Errorf("worker model %q was not found under %q", id, filepath.Join(swarmRoot, "models"))
	}
	if strings.TrimSpace(model.Agent) == "" {
		return ModelDefinition{}, fmt.Errorf("worker model %q is missing agent", model.ID)
	}
	return model, nil
}

func LoadPersona(swarmRoot, id string) (PersonaDefinition, error) {
	personas, err := loadPersonas(swarmRoot)
	if err != nil {
		return PersonaDefinition{}, err
	}
	persona, ok := personas[strings.TrimSpace(id)]
	if !ok {
		return PersonaDefinition{}, fmt.Errorf("persona %q was not found under %q", id, filepath.Join(swarmRoot, "personas"))
	}
	return persona, nil
}

func LoadWorkerCatalog(swarmRoot string) (WorkerCatalog, error) {
	models, err := loadModels(swarmRoot)
	if err != nil {
		return WorkerCatalog{}, err
	}
	personas, err := loadPersonas(swarmRoot)
	if err != nil {
		return WorkerCatalog{}, err
	}
	catalog := WorkerCatalog{
		Models:   make([]ModelDefinition, 0, len(models)),
		Personas: make([]PersonaDefinition, 0, len(personas)),
	}
	for _, model := range models {
		if strings.TrimSpace(model.Agent) == "" {
			continue
		}
		catalog.Models = append(catalog.Models, model)
	}
	for _, persona := range personas {
		catalog.Personas = append(catalog.Personas, persona)
	}
	sort.Slice(catalog.Models, func(i, j int) bool { return catalog.Models[i].ID < catalog.Models[j].ID })
	sort.Slice(catalog.Personas, func(i, j int) bool { return catalog.Personas[i].ID < catalog.Personas[j].ID })
	if len(catalog.Models) == 0 {
		return WorkerCatalog{}, fmt.Errorf("no worker models with agent were found under %q", filepath.Join(strings.TrimSpace(swarmRoot), "models"))
	}
	if len(catalog.Personas) == 0 {
		return WorkerCatalog{}, fmt.Errorf("no personas were found under %q", filepath.Join(strings.TrimSpace(swarmRoot), "personas"))
	}
	return catalog, nil
}

func loadModels(swarmRoot string) (map[string]ModelDefinition, error) {
	root, err := normalizeSwarmRoot(swarmRoot)
	if err != nil {
		return nil, err
	}
	modelsDir := filepath.Join(root, "models")
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("read models directory %q: %w", modelsDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	models := make(map[string]ModelDefinition, len(entries))
	paths := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(modelsDir, entry.Name())
		model, err := loadModelFromPath(path)
		if err != nil {
			return nil, err
		}
		if previous, exists := paths[model.ID]; exists {
			return nil, fmt.Errorf("duplicate model id %q in %q and %q", model.ID, previous, path)
		}
		models[model.ID] = model
		paths[model.ID] = path
	}
	return models, nil
}

func loadPersonas(swarmRoot string) (map[string]PersonaDefinition, error) {
	root, err := normalizeSwarmRoot(swarmRoot)
	if err != nil {
		return nil, err
	}
	personasDir := filepath.Join(root, "personas")
	entries, err := os.ReadDir(personasDir)
	if err != nil {
		return nil, fmt.Errorf("read personas directory %q: %w", personasDir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	personas := make(map[string]PersonaDefinition, len(entries))
	paths := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".md" && ext != ".markdown" {
			continue
		}
		path := filepath.Join(personasDir, entry.Name())
		persona, err := loadPersonaFromPath(path)
		if err != nil {
			return nil, err
		}
		if previous, exists := paths[persona.ID]; exists {
			return nil, fmt.Errorf("duplicate persona id %q in %q and %q", persona.ID, previous, path)
		}
		personas[persona.ID] = persona
		paths[persona.ID] = path
	}
	return personas, nil
}

func normalizeSwarmRoot(swarmRoot string) (string, error) {
	trimmed := strings.TrimSpace(swarmRoot)
	if trimmed == "" {
		return "", errors.New("swarm root is required")
	}
	root, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve swarm root %q: %w", swarmRoot, err)
	}
	return root, nil
}

func loadModelFromPath(path string) (ModelDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelDefinition{}, fmt.Errorf("read model file %q: %w", path, err)
	}

	var model ModelDefinition
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal(data, &model); err != nil {
			return ModelDefinition{}, fmt.Errorf("parse model file %q: %w", path, err)
		}
	case ".yaml", ".yml":
		parsed, err := parseSimpleYAML(string(data))
		if err != nil {
			return ModelDefinition{}, fmt.Errorf("parse model file %q: %w", path, err)
		}
		model = modelFromMap(parsed)
	default:
		return ModelDefinition{}, fmt.Errorf("unsupported model file %q", path)
	}

	if err := model.Validate(); err != nil {
		return ModelDefinition{}, fmt.Errorf("validate model file %q: %w", path, err)
	}
	return model, nil
}

func loadPersonaFromPath(path string) (PersonaDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PersonaDefinition{}, fmt.Errorf("read persona file %q: %w", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "---") {
		return PersonaDefinition{}, fmt.Errorf("persona file %q is missing frontmatter", path)
	}
	parts := strings.SplitN(trimmed, "---", 3)
	if len(parts) < 3 {
		return PersonaDefinition{}, fmt.Errorf("persona file %q has malformed frontmatter", path)
	}
	fields, err := parseSimpleYAML(parts[1])
	if err != nil {
		return PersonaDefinition{}, fmt.Errorf("parse persona frontmatter %q: %w", path, err)
	}
	persona := PersonaDefinition{
		ID:     strings.TrimSpace(fields["id"]),
		Group:  strings.TrimSpace(fields["group"]),
		Intent: strings.TrimSpace(fields["intent"]),
		Prompt: strings.TrimSpace(parts[2]),
	}
	if strings.TrimSpace(persona.ID) == "" {
		return PersonaDefinition{}, fmt.Errorf("persona file %q is missing id", path)
	}
	if strings.TrimSpace(persona.Group) == "" {
		return PersonaDefinition{}, fmt.Errorf("persona file %q is missing group", path)
	}
	if strings.TrimSpace(persona.Intent) == "" {
		return PersonaDefinition{}, fmt.Errorf("persona file %q is missing intent", path)
	}
	if strings.TrimSpace(persona.Prompt) == "" {
		return PersonaDefinition{}, fmt.Errorf("persona file %q is missing prompt body", path)
	}
	return persona, nil
}

func modelFromMap(fields map[string]string) ModelDefinition {
	return ModelDefinition{
		ID:              strings.TrimSpace(fields["id"]),
		Invoke:          strings.TrimSpace(fields["invoke"]),
		BudgetRef:       strings.TrimSpace(fields["budget_ref"]),
		MaxOutputTokens: atoi(fields["max_output_tokens"]),
		RuntimeProfile:  strings.TrimSpace(fields["runtime_profile"]),
		Agent:           strings.TrimSpace(fields["agent"]),
		BudgetOverride: BudgetOverride{
			Context: atoi(fields["budget_override.context"]),
			Input:   atoi(fields["budget_override.input"]),
			Output:  atoi(fields["budget_override.output"]),
		},
	}
}

func (m ModelDefinition) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(m.Invoke) == "" {
		return errors.New("invoke is required")
	}
	if strings.TrimSpace(m.BudgetRef) == "" && !m.BudgetOverride.IsSet() {
		return errors.New("budget_ref is required unless budget_override is set")
	}
	if m.MaxOutputTokens <= 0 {
		return errors.New("max_output_tokens must be > 0")
	}
	return nil
}

func (b BudgetOverride) IsSet() bool {
	return b.Context > 0 && b.Output > 0
}

func parseSimpleYAML(input string) (map[string]string, error) {
	result := make(map[string]string)
	section := ""
	for _, rawLine := range strings.Split(input, "\n") {
		line := strings.TrimRight(rawLine, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(strings.TrimSuffix(trimmed, ":"), " ") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid yaml line %q", rawLine)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if section != "" && strings.HasPrefix(rawLine, " ") {
			key = section + "." + key
		}
		if !strings.HasPrefix(rawLine, " ") {
			section = ""
		}
		result[key] = value
	}
	return result, nil
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}
