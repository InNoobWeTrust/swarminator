package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"swarminator/internal/domain/agent"
	"swarminator/internal/domain/protocol/acp"
)

const acpProcessWaitTimeout = 5 * time.Second

type ACPProvider struct {
	binary string
	args   []string
}

func NewACPProvider(binary string, args ...string) LLMAdapter {
	return &ACPProvider{
		binary: binary,
		args:   args,
	}
}

func (p *ACPProvider) Complete(ctx context.Context, req agent.CompletionRequest) (string, error) {
	ctx, cancelCtx := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx, p.binary, p.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to open stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to open stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancelCtx()
		return "", fmt.Errorf("failed to start agent %s: %w", p.binary, err)
	}

	var wg sync.WaitGroup
	defer func() {
		cancelCtx()
		stdin.Close()
		stdout.Close()
		wg.Wait()
		waitForProcessExit(cmd, acpProcessWaitTimeout)
	}()

	responseChan := make(chan *acp.Response, 10)
	permissionChan := make(chan *acp.Response, 10)
	errChan := make(chan error, 1)
	var responseText strings.Builder
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(responseChan)
		defer close(permissionChan)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 128*1024), 1024*1024)
		for scanner.Scan() {
			var resp acp.Response
			if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
				continue
			}
			if resp.ID == nil {
				if resp.Method == "session/update" {
					var params struct {
						Update struct {
							SessionUpdate string `json:"sessionUpdate"`
							Content       struct {
								Type string `json:"type"`
								Text string `json:"text"`
							} `json:"content"`
						} `json:"update"`
					}
					if err := json.Unmarshal(resp.Params, &params); err == nil &&
						params.Update.SessionUpdate == "agent_message_chunk" &&
						params.Update.Content.Type == "text" {
						responseText.WriteString(params.Update.Content.Text)
					}
				}
				continue
			}
			if resp.Method != "" && resp.Result == nil && resp.Error == nil {
				select {
				case permissionChan <- &resp:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case responseChan <- &resp:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			if !errors.Is(err, os.ErrClosed) {
				errChan <- err
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case permReq, ok := <-permissionChan:
				if !ok {
					return
				}
				handlePermissionRequest(stdin, permReq, string(req.AgentMode))
			}
		}
	}()

	nextID := 3
	if req.AgentMode != "" {
		nextID = 4
	}

	initParams := acp.InitializeParams{
		ClientInfo: acp.ClientInfo{
			Name:    "swarminator",
			Version: "1.0.0",
			Title:   "Swarminator",
		},
		ClientCapabilities: acp.ClientCapabilities{
			Fs: &acp.FsCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
		ProtocolVersion: 1,
	}
	initParamsJSON, _ := json.Marshal(initParams)
	if err := sendRequest(stdin, "initialize", initParamsJSON, 1); err != nil {
		return "", err
	}
	resp, err := waitResponse(ctx, 1, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("initialize: %w", err)
	}
	_ = resp

	sessionParams := acp.SessionNewParams{
		Cwd:        getcwd(),
		McpServers: []string{},
	}
	sessionParamsJSON, _ := json.Marshal(sessionParams)
	if err := sendRequest(stdin, "session/new", sessionParamsJSON, 2); err != nil {
		return "", err
	}
	resp, err = waitResponse(ctx, 2, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("session/new: %w", err)
	}
	var sessionResult acp.SessionNewResult
	if err := json.Unmarshal(resp.Result, &sessionResult); err != nil {
		return "", fmt.Errorf("failed to parse session result: %w", err)
	}

	if req.AgentMode != "" {
		modeParams := acp.SessionSetModeParams{
			SessionId: sessionResult.SessionId,
			ModeId:    string(req.AgentMode),
		}
		modeParamsJSON, _ := json.Marshal(modeParams)
		if err := sendRequest(stdin, "session/set_mode", modeParamsJSON, nextID); err != nil {
			return "", err
		}
		resp, err = waitResponse(ctx, nextID, responseChan, errChan)
		if err != nil {
			return "", fmt.Errorf("session/set_mode: invalid or unavailable mode %q: %w", req.AgentMode, err)
		}
		nextID++
	}

	promptParams := acp.SessionPromptParams{
		SessionId: sessionResult.SessionId,
		Prompt: []map[string]string{
			{"type": "text", "text": fmt.Sprintf("PERSONA: %s\n\nINPUT: %s", req.Persona, req.Input)},
		},
	}
	promptParamsJSON, _ := json.Marshal(promptParams)
	if err := sendRequest(stdin, "session/prompt", promptParamsJSON, nextID); err != nil {
		return "", err
	}
	resp, err = waitPromptResponse(ctx, nextID, sessionResult.SessionId, stdin, responseChan, errChan)
	if err != nil {
		return "", fmt.Errorf("session/prompt: %w", err)
	}
	_ = resp

	return responseText.String(), nil
}

func getcwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func handlePermissionRequest(w io.Writer, req *acp.Response, agentMode string) {
	type permResponse struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Result  json.RawMessage `json:"result"`
	}

	var chosen json.RawMessage
	if strings.ToLower(agentMode) == "yolo" {
		var params acp.SessionRequestPermissionParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			for _, opt := range params.Options {
				if strings.HasPrefix(strings.ToLower(opt.Kind), "allow") {
					chosenJSON, _ := json.Marshal(opt.Kind)
					chosen = chosenJSON
					break
				}
			}
		}
		if chosen == nil {
			chosen = json.RawMessage(`null`)
		}
	} else {
		chosen = json.RawMessage(`null`)
	}

	resp := permResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  chosen,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func sendRequest(w io.Writer, method string, params json.RawMessage, id interface{}) error {
	req := acp.Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}
	return json.NewEncoder(w).Encode(req)
}

func waitResponse(ctx context.Context, id interface{}, responseChan chan *acp.Response, errChan chan error) (*acp.Response, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errChan:
			return nil, err
		case resp, ok := <-responseChan:
			if !ok {
				responseChan = nil
				continue
			}
			if fmt.Sprintf("%v", resp.ID) == fmt.Sprintf("%v", id) {
				if resp.Error != nil {
					return nil, resp.Error
				}
				return resp, nil
			}
		}
	}
}

func waitPromptResponse(ctx context.Context, id interface{}, sessionId string, stdin io.Writer, responseChan chan *acp.Response, errChan chan error) (*acp.Response, error) {
	for {
		select {
		case <-ctx.Done():
			cancelParams, _ := json.Marshal(acp.SessionCancelParams{SessionId: sessionId})
			_ = sendNotification(stdin, "session/cancel", cancelParams)
			return nil, ctx.Err()
		case err := <-errChan:
			return nil, err
		case resp, ok := <-responseChan:
			if !ok {
				responseChan = nil
				continue
			}
			if fmt.Sprintf("%v", resp.ID) == fmt.Sprintf("%v", id) {
				if resp.Error != nil {
					return nil, resp.Error
				}
				return resp, nil
			}
		}
	}
}

func sendNotification(w io.Writer, method string, params json.RawMessage) error {
	notif := struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return json.NewEncoder(w).Encode(notif)
}

func waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
}