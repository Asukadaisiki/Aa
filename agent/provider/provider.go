// Package provider defines the provider-neutral chat and streaming types used
// by the agent. Providers are stateless: a multi-turn conversation is
// represented by Request.Messages and sent on every call.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one turn in a conversation. ToolCalls is normally populated on
// assistant messages, while ToolCallID is used on tool result messages.
type Message struct {
	Role             Role       `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Usage            Usage      `json:"usage,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition uses a JSON-schema object but does not depend on the
// implementation of the agent's tool registry.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

type Request struct {
	Model           string           `json:"model,omitempty"`
	Messages        []Message        `json:"messages"`
	System          string           `json:"system,omitempty"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	MaxTokens       int              `json:"max_tokens,omitempty"`
	Temperature     *float64         `json:"temperature,omitempty"`
	Stop            []string         `json:"stop,omitempty"`
	Thinking        *Thinking        `json:"thinking,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
	ResponseFormat  *ResponseFormat  `json:"response_format,omitempty"`
}

type Thinking struct {
	Type string `json:"type"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Usage struct {
	// InputTokens contains prompt tokens that were not read from the cache.
	InputTokens      int  `json:"input_tokens,omitempty"`
	OutputTokens     int  `json:"output_tokens,omitempty"`
	CacheReadTokens  int  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int  `json:"cache_write_tokens,omitempty"`
	TotalTokens      int  `json:"total_tokens,omitempty"`
	CacheReported    bool `json:"-"`
}

func (u Usage) PromptTokens() int {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

func (u Usage) HasCacheData() bool {
	return u.CacheReported || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0
}

type Response struct {
	ID               string     `json:"id,omitempty"`
	Model            string     `json:"model,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	StopReason       string     `json:"stop_reason,omitempty"`
	Usage            Usage      `json:"usage,omitempty"`
}

type EventType string

const (
	EventReasoningDelta EventType = "reasoning_delta"
	EventTextDelta      EventType = "text_delta"
	EventToolCallDelta  EventType = "tool_call_delta"
	EventDone           EventType = "done"
)

// StreamEvent is emitted in order while a provider response is generated.
// ToolCallDelta.Arguments is a JSON fragment during streaming and a complete
// JSON document on the final Response attached to EventDone.
type StreamEvent struct {
	Type      EventType
	Text      string
	Reasoning string
	ToolCall  ToolCallDelta
	Usage     Usage
	Response  *Response
}

type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type StreamHandler func(StreamEvent) error

// Provider supports one-shot and streaming generation. Implementations do not
// retain conversation state; callers append the assistant response and tool
// results to Request.Messages for the next turn.
type Provider interface {
	Name() string
	Complete(context.Context, Request) (Response, error)
	Stream(context.Context, Request, StreamHandler) (Response, error)
}

type APIError struct {
	Provider   string
	StatusCode int
	RequestID  string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s provider returned HTTP %d: %s", e.Provider, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s provider returned HTTP %d", e.Provider, e.StatusCode)
}

func ValidateRequest(req Request) error {
	if len(req.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	for i, message := range req.Messages {
		switch message.Role {
		case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		default:
			return fmt.Errorf("messages[%d]: unsupported role %q", i, message.Role)
		}
		if message.Role == RoleTool && message.ToolCallID == "" {
			return fmt.Errorf("messages[%d]: tool_call_id is required for tool messages", i)
		}
	}
	for i, tool := range req.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tools[%d]: name is required", i)
		}
		if tool.Parameters == nil {
			return fmt.Errorf("tools[%d]: parameters are required", i)
		}
	}
	return nil
}
