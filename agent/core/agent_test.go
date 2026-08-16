package core

import (
	"context"
	"testing"

	"a2aagent/agent/message"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

func TestAgentRunsLoopAndExposesSessionSnapshot(t *testing.T) {
	providerClient := &loopProvider{respond: func(_ provider.Request, index int) provider.Response {
		if index == 1 {
			return provider.Response{Content: "hello"}
		}
		return provider.Response{Content: "unexpected"}
	}}
	client, err := message.NewClient(loopConfig(), providerClient, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	agent, err := NewAgent(client, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := agent.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatal(err)
	}
	var events []message.Event
	for event := range stream {
		events = append(events, event)
	}

	if len(events) != 2 || events[0].Type != message.EventTextDelta || events[1].Type != message.EventDone {
		t.Fatalf("unexpected agent events: %+v", events)
	}
	messages := agent.Messages()
	if len(messages) != 2 {
		t.Fatalf("agent session length = %d, want 2", len(messages))
	}
	if messages[0].Role != provider.RoleUser || messages[0].Content != "say hello" {
		t.Fatalf("unexpected user message: %+v", messages[0])
	}
	if messages[1].Role != provider.RoleAssistant || messages[1].Content != "hello" {
		t.Fatalf("unexpected assistant message: %+v", messages[1])
	}
}

func TestNewAgentRequiresClient(t *testing.T) {
	if _, err := NewAgent(nil, NewSession()); err == nil {
		t.Fatal("expected an error for a nil message client")
	}
}
