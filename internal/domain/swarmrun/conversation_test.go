package swarmrun

import "testing"

func TestConversationRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       ConversationRequest
		wantError string
	}{
		{
			name: "valid request",
			req: ConversationRequest{
				Model: "gemini-2.5-flash",
				Messages: []Message{
					{Role: RoleSystem, Content: "You are concise."},
					{Role: RoleUser, Content: "hello"},
				},
				Tools: []ToolDefinition{{Name: ToolNameRunWorkerNode, Parameters: []ToolParameter{{Name: "model", Required: true, Type: ToolParameterTypeString}}}},
			},
		},
		{
			name:      "missing model",
			req:       ConversationRequest{Messages: []Message{{Role: RoleUser, Content: "hello"}}},
			wantError: "model is required",
		},
		{
			name:      "missing messages",
			req:       ConversationRequest{Model: "gemini-2.5-flash"},
			wantError: "at least one message",
		},
		{
			name: "empty message content",
			req: ConversationRequest{
				Model:    "gemini-2.5-flash",
				Messages: []Message{{Role: RoleUser, Content: "   "}},
			},
			wantError: "message 0 content is required",
		},
		{
			name: "unknown role",
			req: ConversationRequest{
				Model:    "gemini-2.5-flash",
				Messages: []Message{{Role: Role("critic"), Content: "hello"}},
			},
			wantError: "message 0 role",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.req.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want %q", tt.wantError)
			}
			if got := err.Error(); got == "" || !contains(got, tt.wantError) {
				t.Fatalf("Validate() error = %q, want substring %q", got, tt.wantError)
			}
		})
	}
}

func TestToolCallValidate(t *testing.T) {
	t.Parallel()

	if err := (ToolCall{Name: ToolNameRunWorkerNode, Arguments: map[string]string{"model": "worker"}}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if err := (ToolCall{}).Validate(); err == nil {
		t.Fatal("Validate() error = nil, want failure")
	}
}

func TestValidateToolCallAgainstDefinitions(t *testing.T) {
	t.Parallel()

	defs := []ToolDefinition{{
		Name: ToolNameRunWorkerNode,
		Parameters: []ToolParameter{
			{Name: "model", Required: true, Type: ToolParameterTypeString, Enum: []string{"worker", "researcher"}},
			{Name: "persona", Required: true, Type: ToolParameterTypeString, Enum: []string{"critic", "summarizer"}},
			{Name: "input", Required: true, Type: ToolParameterTypeString},
		},
	}}

	if err := ValidateToolCallAgainstDefinitions(ToolCall{
		Name: ToolNameRunWorkerNode,
		Arguments: map[string]string{"model": "worker", "persona": "critic", "input": "inspect this patch"},
	}, defs); err != nil {
		t.Fatalf("ValidateToolCallAgainstDefinitions() error = %v, want nil", err)
	}

	if err := ValidateToolCallAgainstDefinitions(ToolCall{
		Name: ToolNameRunWorkerNode,
		Arguments: map[string]string{"model": "unknown", "persona": "critic", "input": "inspect this patch"},
	}, defs); err == nil || !contains(err.Error(), "must be one of") {
		t.Fatalf("ValidateToolCallAgainstDefinitions() error = %v, want enum validation failure", err)
	}

	if err := ValidateToolCallAgainstDefinitions(ToolCall{
		Name:      ToolName("other_tool"),
		Arguments: map[string]string{"model": "worker"},
	}, defs); err == nil || !contains(err.Error(), "is not defined") {
		t.Fatalf("ValidateToolCallAgainstDefinitions() error = %v, want unknown tool failure", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
