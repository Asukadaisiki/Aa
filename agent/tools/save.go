package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type saveInput struct {
	Path     string            `json:"path"`
	Messages []json.RawMessage `json:"messages"`
}

func SaveDefinition() Definition {
	return Definition{
		Name:        "save",
		Label:       "save",
		Description: "Save the current multi-turn message array as a JSON file.",
		Parameters: Schema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "Path to the JSON session file.",
				},
				"messages": {
					Type:        "array",
					Description: "Conversation messages to persist.",
					Items:       &Schema{Type: "object", AdditionalProperties: true},
				},
			},
			Required:             []string{"path", "messages"},
			AdditionalProperties: false,
		},
		Execute: executeSave,
	}
}

func executeSave(ctx context.Context, toolCtx Context, raw json.RawMessage) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var input saveInput
	if err := decodeArgs(raw, &input); err != nil {
		return Result{}, err
	}
	pathValue, err := resolvePath(toolCtx, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := authorize(ctx, toolCtx, ApprovalRequest{
		Tool:        "save",
		Path:        pathValue,
		Permissions: []Permission{PermissionSave},
	}); err != nil {
		return Result{}, err
	}
	data, err := json.MarshalIndent(input.Messages, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("encode messages: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(pathValue, append(data, '\n'), 0o644); err != nil {
		return Result{}, fmt.Errorf("save messages to %q: %w", input.Path, err)
	}
	return textResult(fmt.Sprintf("Saved %d messages to %s", len(input.Messages), pathValue), map[string]any{
		"path":          pathValue,
		"message_count": len(input.Messages),
	}), nil
}
