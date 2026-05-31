package orchestratortransport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"swarminator/internal/domain/swarmrun"
	"swarminator/internal/infra/orchestratorconfig"
)

const (
	BackendOpenAICompatible    = "openai-compatible"
	MessageAPIFormatOpenAIChat = "openai.chat.completions"
	openAIChatCompletionsPath  = "/chat/completions"
)

type Transport struct {
	cfg    orchestratorconfig.Config
	client *http.Client
	token  string
}

func New(cfg orchestratorconfig.Config) (*Transport, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Backend != BackendOpenAICompatible {
		return nil, fmt.Errorf("unsupported orchestrator backend %q", cfg.Backend)
	}
	if cfg.MessageAPIFormat != MessageAPIFormatOpenAIChat {
		return nil, fmt.Errorf("unsupported message API format %q for backend %q", cfg.MessageAPIFormat, cfg.Backend)
	}

	token, err := resolveToken(cfg.Auth)
	if err != nil {
		return nil, err
	}

	return &Transport{
		cfg:    cfg,
		client: &http.Client{Timeout: transportTimeout(cfg)},
		token:  token,
	}, nil
}

func (t *Transport) Send(ctx context.Context, req swarmrun.ConversationRequest) (swarmrun.ConversationResponse, error) {
	if err := req.Validate(); err != nil {
		return swarmrun.ConversationResponse{}, err
	}

	requestBody, err := buildOpenAIChatRequest(req)
	if err != nil {
		return swarmrun.ConversationResponse{}, err
	}

	endpoint, err := buildOpenAICompatibleEndpoint(t.cfg.BaseURL)
	if err != nil {
		return swarmrun.ConversationResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return swarmrun.ConversationResponse{}, fmt.Errorf("create orchestrator request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if mode := autoModeHeaderValue(req.Model); mode != "" {
		httpReq.Header.Set("x-kilocode-mode", mode)
	}
	applyAuth(httpReq, t.cfg.Auth.Method, t.token)

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return swarmrun.ConversationResponse{}, fmt.Errorf("send orchestrator request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return swarmrun.ConversationResponse{}, fmt.Errorf("read orchestrator response: %w", err)
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return swarmrun.ConversationResponse{}, fmt.Errorf("parse orchestrator response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if parsed.Error.Message != "" {
			return swarmrun.ConversationResponse{}, fmt.Errorf("orchestrator backend error (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return swarmrun.ConversationResponse{}, fmt.Errorf("orchestrator backend error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	toolCall, err := parsed.firstToolCall()
	if err != nil {
		return swarmrun.ConversationResponse{}, err
	}
	if toolCall != nil {
		return swarmrun.ConversationResponse{ToolCall: toolCall}, nil
	}

	text, err := parsed.firstText()
	if err != nil {
		return swarmrun.ConversationResponse{}, err
	}
	return swarmrun.ConversationResponse{Message: swarmrun.Message{Role: swarmrun.RoleAssistant, Content: text}}, nil
}

func buildOpenAICompatibleEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base_url must be absolute")
	}
	parsed.Path = path.Join(strings.TrimSuffix(parsed.Path, "/"), openAIChatCompletionsPath)
	return parsed.String(), nil
}

func resolveToken(auth orchestratorconfig.AuthConfig) (string, error) {
	token := strings.TrimSpace(os.Getenv(auth.CredentialRef))
	if token == "" {
		return "", fmt.Errorf("credential %q is empty or unset", auth.CredentialRef)
	}
	return token, nil
}

func applyAuth(httpReq *http.Request, method orchestratorconfig.AuthMethod, token string) {
	switch method {
	case orchestratorconfig.AuthMethodBearerTokenEnv:
		httpReq.Header.Set("Authorization", "Bearer "+token)
	case orchestratorconfig.AuthMethodAPIKeyEnv:
		q := httpReq.URL.Query()
		q.Set("key", token)
		httpReq.URL.RawQuery = q.Encode()
	}
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Tools       []openAIChatTool    `json:"tools,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIResponseTool `json:"tool_calls,omitempty"`
}

type openAIChatTool struct {
	Type     string                  `json:"type"`
	Function openAIFunctionDefinition `json:"function"`
}

type openAIFunctionDefinition struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Parameters  openAISchema `json:"parameters,omitempty"`
}

type openAISchema struct {
	Type        string                  `json:"type,omitempty"`
	Properties  map[string]openAISchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Description string                  `json:"description,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
}

type openAIChatResponse struct {
	Choices []openAIChatChoice `json:"choices"`
	Error   struct {
		Message string `json:"message"`
	} `json:"error"`
}

type openAIChatChoice struct {
	Message openAIChatMessage `json:"message"`
}

type openAIResponseTool struct {
	ID       string                `json:"id,omitempty"`
	Type     string                `json:"type,omitempty"`
	Function openAIResponseFunction `json:"function"`
}

type openAIResponseFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (r openAIChatResponse) firstText() (string, error) {
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("orchestrator backend returned no choices")
	}
	text := strings.TrimSpace(r.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("orchestrator backend returned empty text")
	}
	return text, nil
}

func (r openAIChatResponse) firstToolCall() (*swarmrun.ToolCall, error) {
	if len(r.Choices) == 0 {
		return nil, nil
	}
	calls := r.Choices[0].Message.ToolCalls
	if len(calls) == 0 {
		return nil, nil
	}
	call := calls[0]
	if strings.TrimSpace(call.Function.Name) == "" {
		return nil, fmt.Errorf("orchestrator backend returned a tool call without a function name")
	}
	args := map[string]string{}
	if strings.TrimSpace(call.Function.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return nil, fmt.Errorf("parse orchestrator tool arguments: %w", err)
		}
	}
	toolCall := &swarmrun.ToolCall{Name: swarmrun.ToolName(strings.TrimSpace(call.Function.Name)), Arguments: args}
	if err := toolCall.Validate(); err != nil {
		return nil, err
	}
	return toolCall, nil
}

func buildOpenAIChatRequest(req swarmrun.ConversationRequest) ([]byte, error) {
	normalizedModel := normalizeOpenAICompatibleModel(req.Model)
	messages := make([]openAIChatMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case swarmrun.RoleSystem:
			messages = append(messages, openAIChatMessage{Role: "system", Content: strings.TrimSpace(msg.Content)})
		case swarmrun.RoleUser:
			messages = append(messages, openAIChatMessage{Role: "user", Content: strings.TrimSpace(msg.Content)})
		case swarmrun.RoleAssistant:
			messages = append(messages, openAIChatMessage{Role: "assistant", Content: strings.TrimSpace(msg.Content)})
		case swarmrun.RoleTool:
			messages = append(messages, openAIChatMessage{Role: "tool", Content: strings.TrimSpace(msg.Content), ToolCallID: strings.TrimSpace(msg.Name)})
		default:
			return nil, fmt.Errorf("message role %q is not supported", msg.Role)
		}
	}
	reqBody := openAIChatRequest{Model: normalizedModel, Messages: messages, Stream: false}
	if len(req.Tools) > 0 {
		tools := make([]openAIChatTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			tools = append(tools, openAIChatTool{Type: "function", Function: buildOpenAIFunctionDefinition(tool)})
		}
		reqBody.Tools = tools
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		reqBody.Temperature = &temp
	}
	if req.MaxOutputTokens > 0 {
		reqBody.MaxTokens = req.MaxOutputTokens
	}
	return json.Marshal(reqBody)
}

func buildOpenAIFunctionDefinition(tool swarmrun.ToolDefinition) openAIFunctionDefinition {
	properties := make(map[string]openAISchema, len(tool.Parameters))
	required := make([]string, 0, len(tool.Parameters))
	for _, param := range tool.Parameters {
		schema := openAISchema{Type: string(parameterTypeOrDefault(param.Type)), Description: strings.TrimSpace(param.Description)}
		if len(param.Enum) > 0 {
			schema.Enum = append([]string(nil), param.Enum...)
		}
		properties[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
	}
	return openAIFunctionDefinition{
		Name:        strings.TrimSpace(string(tool.Name)),
		Description: strings.TrimSpace(tool.Description),
		Parameters: openAISchema{
			Type:       "object",
			Properties: properties,
			Required:   required,
		},
	}
}

func parameterTypeOrDefault(kind swarmrun.ToolParameterType) swarmrun.ToolParameterType {
	if strings.TrimSpace(string(kind)) == "" {
		return swarmrun.ToolParameterTypeString
	}
	return kind
}

func normalizeOpenAICompatibleModel(model string) string {
	trimmed := strings.TrimSpace(model)
	if strings.HasPrefix(trimmed, "kilo/kilo-auto/") {
		return strings.TrimPrefix(trimmed, "kilo/")
	}
	return trimmed
}

func autoModeHeaderValue(model string) string {
	if strings.HasPrefix(normalizeOpenAICompatibleModel(model), "kilo-auto/") {
		return "orchestrator"
	}
	return ""
}

func transportTimeout(cfg orchestratorconfig.Config) time.Duration {
	if cfg.TimeoutSeconds > 0 {
		return time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return 30 * time.Second
}
