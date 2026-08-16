package tui

import (
	"context"
	"strings"
	"testing"

	"a2aagent/agent/config"
	"a2aagent/agent/core"
	"a2aagent/agent/message"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		line    string
		command Command
		input   string
	}{
		{"hello", CommandNone, "hello"},
		{"/help", CommandHelp, ""},
		{"/history", CommandCount, ""},
		{"/q", CommandQuit, ""},
		{"/clear", CommandClear, ""},
	}
	for _, test := range tests {
		command, input := parseInput(test.line)
		if command != test.command || input != test.input {
			t.Fatalf("parseInput(%q) = (%q, %q), want (%q, %q)", test.line, command, input, test.command, test.input)
		}
	}
}

func TestAppStreamsResponseAndClearResetsSession(t *testing.T) {
	client, err := message.NewClient(testConfig(), testProvider{}, tools.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := core.NewAgent(client, nil)
	if err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader("hello\n/count\n/clear\n/quit\n")
	var output strings.Builder
	app, err := New(agent, Options{Input: input, Output: &output, WorkDir: "/tmp", ANSI: false, Width: 60})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if !strings.Contains(rendered, "hello from test provider") {
		t.Fatalf("rendered output does not contain streamed response: %s", rendered)
	}
	if !strings.Contains(rendered, "Session messages: 2") {
		t.Fatalf("count command did not see the completed turn: %s", rendered)
	}
	if len(agent.Messages()) != 0 {
		t.Fatalf("clear command left %d session messages", len(agent.Messages()))
	}
}

type testProvider struct{}

func (testProvider) Name() string { return "test" }

func (testProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{Content: "hello from test provider"}, nil
}

func (testProvider) Stream(ctx context.Context, _ provider.Request, handler provider.StreamHandler) (provider.Response, error) {
	if err := handler(provider.StreamEvent{Type: provider.EventTextDelta, Text: "hello "}); err != nil {
		return provider.Response{}, err
	}
	response := provider.Response{Content: "hello from test provider"}
	if err := handler(provider.StreamEvent{Type: provider.EventTextDelta, Text: "from test provider"}); err != nil {
		return provider.Response{}, err
	}
	if err := handler(provider.StreamEvent{Type: provider.EventDone, Response: &response}); err != nil {
		return provider.Response{}, err
	}
	return response, nil
}

func testConfig() config.Config {
	return config.Config{Provider: config.ProviderDeepSeek, Model: "test-model", URL: "https://example.com/chat", ThinkingEffort: "disabled"}
}
