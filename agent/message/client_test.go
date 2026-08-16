package message

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"a2aagent/agent/config"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

type recordingProvider struct {
	mu       sync.Mutex
	requests []provider.Request
	response provider.Response
	err      error
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, fmt.Errorf("complete is not used")
}

func (p *recordingProvider) Stream(ctx context.Context, req provider.Request, handler provider.StreamHandler) (provider.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	response, streamErr := p.response, p.err
	p.mu.Unlock()
	if streamErr != nil {
		return provider.Response{}, streamErr
	}
	if response.ReasoningContent != "" {
		if err := handler(provider.StreamEvent{Type: provider.EventReasoningDelta, Reasoning: response.ReasoningContent}); err != nil {
			return provider.Response{}, err
		}
	}
	if response.Content != "" {
		if err := handler(provider.StreamEvent{Type: provider.EventTextDelta, Text: response.Content}); err != nil {
			return provider.Response{}, err
		}
	}
	for i, call := range response.ToolCalls {
		if err := handler(provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: provider.ToolCallDelta{Index: i, ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)}}); err != nil {
			return provider.Response{}, err
		}
	}
	if err := handler(provider.StreamEvent{Type: provider.EventDone, Response: &response, Usage: response.Usage}); err != nil {
		return provider.Response{}, err
	}
	return response, nil
}

func testConfig() config.Config {
	return config.Config{
		Provider:       config.ProviderDeepSeek,
		Model:          "test-model",
		URL:            "https://example.test/chat/completions",
		ThinkingEffort: "disabled",
	}
}

func TestPostInjectsOnlyDiscoveryAndLoadedTools(t *testing.T) {
	providerClient := &recordingProvider{response: provider.Response{Content: "answer"}}
	client, err := NewClient(testConfig(), providerClient, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	read, ok := tools.NewRegistry().Find("read")
	if !ok {
		t.Fatal("read tool is not registered")
	}
	stream, err := client.Post(context.Background(), []provider.Message{{Role: provider.RoleUser, Content: "hello"}}, map[string]tools.Definition{"read": read})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for event := range stream {
		events = append(events, event)
	}

	if len(events) != 2 || events[0].Type != EventTextDelta || events[1].Type != EventDone {
		// The provider emits one text delta and one done event. This branch also
		// gives a useful diagnostic if the event contract changes unexpectedly.
		t.Fatalf("unexpected events: %+v", events)
	}
	request := providerClient.requests[0]
	if len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", request.Messages)
	}
	if len(request.Tools) != 3 {
		t.Fatalf("tool count = %d, want 3", len(request.Tools))
	}
	for _, definition := range request.Tools {
		if definition.Name == "create" || definition.Name == "write" || definition.Name == "delete" || definition.Name == "save" {
			t.Fatalf("unloaded tool %q was injected", definition.Name)
		}
	}
}

func TestPostDeliversProviderError(t *testing.T) {
	providerClient := &recordingProvider{err: fmt.Errorf("upstream failed")}
	client, err := NewClient(testConfig(), providerClient, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Post(context.Background(), []provider.Message{{Role: provider.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	event := <-stream
	if event.Type != EventError || event.Err == nil || event.Err.Error() != "upstream failed" {
		t.Fatalf("unexpected error event: %+v", event)
	}
	if _, ok := <-stream; ok {
		t.Fatal("event stream should close after provider error")
	}
}

func TestDiscoveryDefinitionsHaveStrictSchemas(t *testing.T) {
	providerClient := &recordingProvider{response: provider.Response{Content: "ok"}}
	client, err := NewClient(testConfig(), providerClient, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Post(context.Background(), []provider.Message{{Role: provider.RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	for _, definition := range providerClient.requests[0].Tools {
		data, err := json.Marshal(definition.Parameters)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == "null" {
			t.Fatalf("tool %q has no parameters", definition.Name)
		}
	}
}
