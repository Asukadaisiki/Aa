package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteDefinition() Definition {
	return Definition{
		Name:        "write",
		Label:       "write",
		Description: "Write text content to a file, creating parent directories when needed.",
		Parameters: Schema{
			Type: "object",
			Properties: map[string]Property{
				"path":    {Type: "string", Description: "Path to the file to write."},
				"content": {Type: "string", Description: "Complete text content to write."},
			},
			Required:             []string{"path", "content"},
			AdditionalProperties: false,
		},
		Execute: executeWrite,
	}
}

func executeWrite(ctx context.Context, toolCtx Context, raw json.RawMessage) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var input writeInput
	if err := decodeArgs(raw, &input); err != nil {
		return Result{}, err
	}
	pathValue, err := resolvePath(toolCtx, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := authorize(ctx, toolCtx, ApprovalRequest{
		Tool:        "write",
		Path:        pathValue,
		Permissions: []Permission{PermissionWrite},
	}); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent directory: %w", err)
	}
	data := []byte(input.Content)
	if err := os.WriteFile(pathValue, data, 0o644); err != nil {
		return Result{}, fmt.Errorf("write %q: %w", input.Path, err)
	}
	return textResult(fmt.Sprintf("Wrote %d bytes to %s", len(data), pathValue), map[string]any{
		"path":  pathValue,
		"bytes": len(data),
	}), nil
}
