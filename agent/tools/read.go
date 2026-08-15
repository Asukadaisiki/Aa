package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func ReadDefinition() Definition {
	return Definition{
		Name:        "read",
		Label:       "read",
		Description: "Read the contents of a text file. Paths are relative to the agent working directory unless absolute.",
		Parameters: Schema{
			Type: "object",
			Properties: map[string]Property{
				"path":   {Type: "string", Description: "Path to the file to read."},
				"offset": {Type: "integer", Description: "1-indexed line to start from. Defaults to 1."},
				"limit":  {Type: "integer", Description: "Maximum number of lines to return. Defaults to the rest of the file."},
			},
			Required:             []string{"path"},
			AdditionalProperties: false,
		},
		Execute: executeRead,
	}
}

func executeRead(ctx context.Context, toolCtx Context, raw json.RawMessage) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var input readInput
	if err := decodeArgs(raw, &input); err != nil {
		return Result{}, err
	}
	if input.Offset < 0 || input.Limit < 0 {
		return Result{}, fmt.Errorf("offset and limit cannot be negative")
	}
	if input.Offset == 0 {
		input.Offset = 1
	}

	pathValue, err := resolvePath(toolCtx, input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := authorize(ctx, toolCtx, ApprovalRequest{
		Tool:        "read",
		Path:        pathValue,
		Permissions: []Permission{PermissionRead},
	}); err != nil {
		return Result{}, err
	}
	data, err := os.ReadFile(pathValue)
	if err != nil {
		return Result{}, fmt.Errorf("read %q: %w", input.Path, err)
	}

	lines := strings.Split(string(data), "\n")
	start := input.Offset - 1
	if start >= len(lines) {
		return Result{}, fmt.Errorf("offset %d is beyond end of file (%d lines)", input.Offset, len(lines))
	}
	end := len(lines)
	if input.Limit > 0 && start+input.Limit < end {
		end = start + input.Limit
	}

	return textResult(strings.Join(lines[start:end], "\n"), map[string]any{
		"path":        pathValue,
		"start_line":  input.Offset,
		"end_line":    end,
		"total_lines": len(lines),
		"truncated":   end < len(lines),
	}), nil
}
