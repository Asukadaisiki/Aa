package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// Property describes one JSON-schema property exposed to the model.
type Property struct {
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
	Items       *Schema `json:"items,omitempty"`
}

// Schema is the small JSON-schema subset needed by the built-in tools.
type Schema struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties bool                `json:"additionalProperties"`
	Items                *Schema             `json:"items,omitempty"`
}

// Content is the model-facing tool output, following the same content shape
// used by pi-agent's tool results.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result is returned by every tool execution.
type Result struct {
	Content []Content      `json:"content"`
	Details map[string]any `json:"details,omitempty"`
}

// Context contains dependencies shared by built-in tools.
type Context struct {
	WorkDir  string
	Mode     PermissionMode
	Approver Approver
}

// Definition is both the provider-facing tool declaration and its executor.
type Definition struct {
	Name        string                                                          `json:"name"`
	Label       string                                                          `json:"label"`
	Description string                                                          `json:"description"`
	Parameters  Schema                                                          `json:"parameters"`
	Execute     func(context.Context, Context, json.RawMessage) (Result, error) `json:"-"`
}

func textResult(text string, details map[string]any) Result {
	return Result{
		Content: []Content{{Type: "text", Text: text}},
		Details: details,
	}
}

func decodeArgs(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return fmt.Errorf("tool arguments are required")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
