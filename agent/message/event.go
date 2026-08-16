package message

import (
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

// EventType identifies a message client or loop event.
type EventType string

const (
	EventReasoningDelta EventType = "reasoning_delta"
	EventTextDelta      EventType = "text_delta"
	EventToolCallDelta  EventType = "tool_call_delta"
	EventToolCall       EventType = "tool_call"
	EventToolResult     EventType = "tool_result"
	EventUsage          EventType = "usage"
	EventDone           EventType = "done"
	EventError          EventType = "error"
)

// Event is emitted in order by Client.Post and core.Loop.Run. Response is
// populated on EventDone; Err is populated on EventError.
type Event struct {
	Type          EventType               `json:"type"`
	Text          string                  `json:"text,omitempty"`
	Reasoning     string                  `json:"reasoning,omitempty"`
	ToolCall      *provider.ToolCall      `json:"tool_call,omitempty"`
	ToolCallDelta *provider.ToolCallDelta `json:"tool_call_delta,omitempty"`
	ToolResult    *tools.Result           `json:"tool_result,omitempty"`
	Usage         provider.Usage          `json:"usage,omitempty"`
	Response      *provider.Response      `json:"response,omitempty"`
	Err           error                   `json:"-"`
	Round         int                     `json:"round,omitempty"`
}
