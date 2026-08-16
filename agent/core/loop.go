package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"a2aagent/agent/message"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

const MaxRounds = 8

// Loop owns a session and serializes complete user turns. The message client
// remains responsible for one provider request; Loop handles tool calls and
// starts the next request when necessary.
type Loop struct {
	client  *message.Client
	session *Session
	runMu   sync.Mutex
}

func NewLoop(client *message.Client, session *Session) *Loop {
	if session == nil {
		session = NewSession()
	}
	return &Loop{client: client, session: session}
}

func (l *Loop) Run(ctx context.Context, input string) (<-chan message.Event, error) {
	if l == nil || l.client == nil {
		return nil, fmt.Errorf("loop client is required")
	}
	if ctx == nil {
		return nil, fmt.Errorf("loop context is required")
	}
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("loop input is required")
	}

	// Holding runMu until the goroutine exits keeps the user message and all
	// subsequent tool/assistant messages from interleaving with another Run.
	l.runMu.Lock()
	l.session.Append(provider.Message{Role: provider.RoleUser, Content: input})
	events := make(chan message.Event, 32)
	go func() {
		defer l.runMu.Unlock()
		defer close(events)
		l.run(ctx, events)
	}()
	return events, nil
}

func (l *Loop) run(ctx context.Context, events chan<- message.Event) {
	loaded := make(map[string]tools.Definition)
	for round := 1; round <= MaxRounds; round++ {
		stream, err := l.client.Post(ctx, l.session.Messages(), loaded)
		if err != nil {
			sendLoopEvent(ctx, events, message.Event{Type: message.EventError, Err: err, Round: round})
			return
		}

		var response *provider.Response
		for event := range stream {
			event.Round = round
			switch event.Type {
			case message.EventTextDelta, message.EventReasoningDelta, message.EventToolCallDelta:
				if !sendLoopEvent(ctx, events, event) {
					return
				}
			case message.EventDone:
				response = event.Response
				usage := event.Usage
				if response != nil {
					usage = response.Usage
				}
				if usage.TotalTokens != 0 || usage.PromptTokens() != 0 {
					if !sendLoopEvent(ctx, events, message.Event{Type: message.EventUsage, Usage: usage, Round: round}) {
						return
					}
				}
			case message.EventError:
				if !sendLoopEvent(ctx, events, event) {
					return
				}
				return
			}
		}
		if response == nil {
			sendLoopEvent(ctx, events, message.Event{Type: message.EventError, Err: fmt.Errorf("provider stream ended without a response"), Round: round})
			return
		}

		assistant := provider.Message{
			Role:             provider.RoleAssistant,
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
			Usage:            response.Usage,
			ToolCalls:        cloneToolCalls(response.ToolCalls),
		}
		l.session.Append(assistant)
		if len(response.ToolCalls) == 0 {
			finalResponse := cloneResponse(*response)
			sendLoopEvent(ctx, events, message.Event{Type: message.EventDone, Response: &finalResponse, Usage: response.Usage, Round: round})
			return
		}

		for _, call := range response.ToolCalls {
			callCopy := cloneToolCall(call)
			if !sendLoopEvent(ctx, events, message.Event{Type: message.EventToolCall, ToolCall: &callCopy, Round: round}) {
				return
			}
			result, loadedDefinition, err := l.executeToolCall(ctx, call, loaded)
			if ctx.Err() != nil {
				sendLoopEvent(ctx, events, message.Event{Type: message.EventError, Err: ctx.Err(), Round: round})
				return
			}
			if err != nil {
				result = errorResult(err)
			}
			if loadedDefinition != nil {
				loaded[loadedDefinition.Name] = *loadedDefinition
			}
			resultCopy := result
			if !sendLoopEvent(ctx, events, message.Event{Type: message.EventToolResult, ToolCall: &callCopy, ToolResult: &resultCopy, Round: round}) {
				return
			}
			l.session.Append(provider.Message{
				Role:       provider.RoleTool,
				Name:       call.Name,
				ToolCallID: call.ID,
				Content:    resultContent(result),
			})
		}

		if round == MaxRounds {
			sendLoopEvent(ctx, events, message.Event{Type: message.EventError, Err: fmt.Errorf("maximum tool loop rounds (%d) exceeded", MaxRounds), Round: round})
			return
		}
	}
}

func (l *Loop) executeToolCall(ctx context.Context, call provider.ToolCall, loaded map[string]tools.Definition) (tools.Result, *tools.Definition, error) {
	switch call.Name {
	case message.ToolSearch:
		var input struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return tools.Result{}, nil, fmt.Errorf("decode %s arguments: %w", call.Name, err)
		}
		return jsonResult(l.client.SearchTools(input.Query)), nil, nil
	case message.ToolLoad:
		var input struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(call.Arguments, &input); err != nil {
			return tools.Result{}, nil, fmt.Errorf("decode %s arguments: %w", call.Name, err)
		}
		definition, ok := l.client.LoadTool(input.Key)
		if !ok {
			return tools.Result{}, nil, fmt.Errorf("tool %q was not found", input.Key)
		}
		return jsonResult(tools.ToolSummary{Key: definition.Name, Label: definition.Label, Description: definition.Description}), &definition, nil
	default:
		if _, ok := loaded[call.Name]; !ok {
			return tools.Result{}, nil, fmt.Errorf("tool %q is not loaded; call %s first", call.Name, message.ToolLoad)
		}
		result, err := l.client.ExecuteTool(ctx, call.Name, call.Arguments)
		return result, nil, err
	}
}

func jsonResult(value any) tools.Result {
	data, err := json.Marshal(value)
	if err != nil {
		return errorResult(fmt.Errorf("encode tool result: %w", err))
	}
	return tools.Result{Content: []tools.Content{{Type: "text", Text: string(data)}}}
}

func errorResult(err error) tools.Result {
	return tools.Result{Content: []tools.Content{{Type: "text", Text: "tool error: " + err.Error()}}, Details: map[string]any{"error": err.Error()}}
}

func resultContent(result tools.Result) string {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func sendLoopEvent(ctx context.Context, events chan<- message.Event, event message.Event) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func cloneToolCalls(calls []provider.ToolCall) []provider.ToolCall {
	result := make([]provider.ToolCall, len(calls))
	for i, call := range calls {
		result[i] = cloneToolCall(call)
	}
	return result
}

func cloneToolCall(call provider.ToolCall) provider.ToolCall {
	call.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return call
}

func cloneResponse(response provider.Response) provider.Response {
	response.ToolCalls = cloneToolCalls(response.ToolCalls)
	return response
}
