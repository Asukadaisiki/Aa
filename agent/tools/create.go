package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type createInput struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
}

func CreateDefinition() Definition {
	return Definition{
		Name:        "create",
		Label:       "create",
		Description: "Create a new file. Fails when the target already exists.",
		Parameters: Schema{
			Type: "object",
			Properties: map[string]Property{
				"path":    {Type: "string", Description: "Path of the new file."},
				"content": {Type: "string", Description: "Optional initial text content."},
			},
			Required:             []string{"path"},
			AdditionalProperties: false,
		},
		Execute: executeCreate,
	}
}

func executeCreate(ctx context.Context, toolCtx Context, raw json.RawMessage) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var input createInput
	if err := decodeArgs(raw, &input); err != nil {
		return Result{}, err
	}
	pathValue, err := resolvePath(toolCtx, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := authorize(ctx, toolCtx, ApprovalRequest{
		Tool:        "create",
		Path:        pathValue,
		Permissions: []Permission{PermissionCreate},
	}); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent directory: %w", err)
	}
	file, err := os.OpenFile(pathValue, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Result{}, fmt.Errorf("create %q: %w", input.Path, err)
	}
	if _, err := file.WriteString(input.Content); err != nil {
		_ = file.Close()
		return Result{}, fmt.Errorf("write initial content: %w", err)
	}
	if err := file.Close(); err != nil {
		return Result{}, fmt.Errorf("close created file: %w", err)
	}
	return textResult(fmt.Sprintf("Created %s", pathValue), map[string]any{
		"path": pathValue,
	}), nil
}
