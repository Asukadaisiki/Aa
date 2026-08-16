package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"a2aagent/agent/config"
	"a2aagent/agent/message"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

type loopProvider struct {
	mu       sync.Mutex
	requests []provider.Request
	respond  func(provider.Request, int) provider.Response
}

func (p *loopProvider) Name() string { return "loop-test" }

func (p *loopProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, fmt.Errorf("complete is not used")
}

func (p *loopProvider) Stream(ctx context.Context, req provider.Request, handler provider.StreamHandler) (provider.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	index := len(p.requests)
	response := p.respond(req, index)
	p.mu.Unlock()
	if response.Content != "" {
		if err := handler(provider.StreamEvent{Type: provider.EventTextDelta, Text: response.Content}); err != nil {
			return provider.Response{}, err
		}
	}
	if err := handler(provider.StreamEvent{Type: provider.EventDone, Response: &response}); err != nil {
		return provider.Response{}, err
	}
	return response, nil
}

func loopConfig() config.Config {
	return config.Config{
		Provider:       config.ProviderDeepSeek,
		Model:          "test-model",
		URL:            "https://example.test/chat/completions",
		ThinkingEffort: "disabled",
	}
}

func toolCall(id, name, args string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: name, Arguments: json.RawMessage(args)}
}

func TestLoopSearchLoadExecuteAndFinish(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "notes.txt"), []byte("hello from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	providerClient := &loopProvider{respond: func(_ provider.Request, index int) provider.Response {
		switch index {
		case 1:
			return provider.Response{ToolCalls: []provider.ToolCall{toolCall("call-search", message.ToolSearch, `{"query":"text file"}`)}}
		case 2:
			return provider.Response{ToolCalls: []provider.ToolCall{toolCall("call-load", message.ToolLoad, `{"key":"read"}`)}}
		case 3:
			return provider.Response{ToolCalls: []provider.ToolCall{toolCall("call-read", "read", `{"path":"notes.txt"}`)}}
		default:
			return provider.Response{Content: "finished"}
		}
	}}
	client, err := message.NewClientWithToolContext(loopConfig(), providerClient, tools.NewRegistry(), tools.Context{WorkDir: workDir, Mode: tools.PermissionModeAutonomous})
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession()
	loop := NewLoop(client, session)
	stream, err := loop.Run(context.Background(), "inspect notes")
	if err != nil {
		t.Fatal(err)
	}
	var events []message.Event
	for event := range stream {
		events = append(events, event)
	}
	if len(events) == 0 || events[len(events)-1].Type != message.EventDone || events[len(events)-1].Response.Content != "finished" {
		t.Fatalf("loop did not finish: %+v", events)
	}

	providerClient.mu.Lock()
	requests := append([]provider.Request(nil), providerClient.requests...)
	providerClient.mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("provider request count = %d, want 4", len(requests))
	}
	wantToolCounts := []int{2, 2, 3, 3}
	for i, request := range requests {
		if len(request.Tools) != wantToolCounts[i] {
			t.Fatalf("request %d tool count = %d, want %d", i+1, len(request.Tools), wantToolCounts[i])
		}
	}
	if len(session.Messages()) != 8 {
		t.Fatalf("session message count = %d, want 8", session.Len())
	}
	messages := session.Messages()
	for _, index := range []int{2, 4, 6} {
		if messages[index].ToolCallID == "" {
			t.Fatalf("tool message %d has no tool_call_id", index)
		}
	}
	wantRoles := []provider.Role{
		provider.RoleUser,
		provider.RoleAssistant,
		provider.RoleTool,
		provider.RoleAssistant,
		provider.RoleTool,
		provider.RoleAssistant,
		provider.RoleTool,
		provider.RoleAssistant,
	}
	for i, current := range session.Messages() {
		if current.Role != wantRoles[i] {
			t.Fatalf("message %d role = %q, want %q", i, current.Role, wantRoles[i])
		}
	}
}

func TestLoopStopsAfterEightToolRounds(t *testing.T) {
	providerClient := &loopProvider{respond: func(_ provider.Request, _ int) provider.Response {
		return provider.Response{ToolCalls: []provider.ToolCall{toolCall("call-search", message.ToolSearch, `{"query":"read"}`)}}
	}}
	client, err := message.NewClient(loopConfig(), providerClient, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := NewLoop(client, NewSession()).Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatal(err)
	}
	var last message.Event
	for event := range stream {
		last = event
	}
	if last.Type != message.EventError || last.Err == nil || !strings.Contains(last.Err.Error(), "maximum tool loop rounds") {
		t.Fatalf("unexpected max-round event: %+v", last)
	}
	providerClient.mu.Lock()
	count := len(providerClient.requests)
	providerClient.mu.Unlock()
	if count != MaxRounds {
		t.Fatalf("provider request count = %d, want %d", count, MaxRounds)
	}
}
