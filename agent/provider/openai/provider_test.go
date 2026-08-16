package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2aagent/agent/provider"
)

func TestStreamSupportsMultiTurnMessagesAndToolDefinitions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		var request struct {
			Stream   bool             `json:"stream"`
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !request.Stream || len(request.Messages) != 4 || len(request.Tools) != 1 {
			t.Fatalf("unexpected request: %+v", request)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chat-1\",\"model\":\"test-model\",\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"world\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, err := New(Config{APIKey: "test-key", URL: server.URL + "/chat/completions", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]provider.StreamEvent, 0)
	result, err := client.Stream(context.Background(), provider.Request{
		System:   "be concise",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "first"}, {Role: provider.RoleAssistant, Content: "reply"}, {Role: provider.RoleUser, Content: "second"}},
		Tools:    []provider.ToolDefinition{{Name: "lookup", Parameters: map[string]any{"type": "object"}}},
	}, func(event provider.StreamEvent) error { events = append(events, event); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "hello world" || result.StopReason != "stop" || result.Usage.TotalTokens != 6 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(events) != 3 || events[0].Text != "hello " || events[len(events)-1].Type != provider.EventDone {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestCompleteParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "test-key") {
			t.Error("missing authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat-2","model":"test-model","choices":[{"message":{"content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"go\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "test-key", URL: server.URL + "/chat/completions", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "search"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "lookup" || string(result.ToolCalls[0].Arguments) != `{"q":"go"}` {
		t.Fatalf("unexpected tool calls: %+v", result.ToolCalls)
	}
}

func TestStreamParsesPromptCacheUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":4,\"total_tokens\":104,\"prompt_cache_hit_tokens\":75}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "test-key", URL: server.URL, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hello"}}}, func(provider.StreamEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage.InputTokens != 25 || result.Usage.CacheReadTokens != 75 || !result.Usage.CacheReported {
		t.Fatalf("unexpected cache usage: %+v", result.Usage)
	}
}
