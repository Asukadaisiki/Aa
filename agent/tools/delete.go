package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type deleteInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

func DeleteDefinition() Definition {
	return Definition{
		Name:        "delete",
		Label:       "delete",
		Description: "Delete a file, or delete an empty directory. Recursive directory deletion must be explicitly enabled.",
		Parameters: Schema{
			Type: "object",
			Properties: map[string]Property{
				"path":      {Type: "string", Description: "Path to the file or directory to delete."},
				"recursive": {Type: "boolean", Description: "Allow deleting a non-empty directory."},
			},
			Required:             []string{"path"},
			AdditionalProperties: false,
		},
		Execute: executeDelete,
	}
}

func executeDelete(ctx context.Context, toolCtx Context, raw json.RawMessage) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var input deleteInput
	if err := decodeArgs(raw, &input); err != nil {
		return Result{}, err
	}
	pathValue, err := resolvePath(toolCtx, input.Path)
	if err != nil {
		return Result{}, err
	}
	workDir, err := resolvePath(toolCtx, ".")
	if err != nil {
		return Result{}, err
	}
	if pathValue == workDir {
		return Result{}, fmt.Errorf("refuse to delete the working directory")
	}
	requestedPermissions := []Permission{PermissionDelete}
	if input.Recursive {
		requestedPermissions = append(requestedPermissions, PermissionRecursiveDelete)
	}
	if err := authorize(ctx, toolCtx, ApprovalRequest{
		Tool:        "delete",
		Path:        pathValue,
		Permissions: requestedPermissions,
		Recursive:   input.Recursive,
	}); err != nil {
		return Result{}, err
	}

	info, err := os.Stat(pathValue)
	if err != nil {
		return Result{}, fmt.Errorf("stat %q: %w", input.Path, err)
	}
	if info.IsDir() {
		if !input.Recursive {
			if err := os.Remove(pathValue); err != nil {
				return Result{}, fmt.Errorf("delete directory %q: %w", input.Path, err)
			}
		} else if err := os.RemoveAll(pathValue); err != nil {
			return Result{}, fmt.Errorf("delete directory %q: %w", input.Path, err)
		}
	} else if err := os.Remove(pathValue); err != nil {
		return Result{}, fmt.Errorf("delete file %q: %w", input.Path, err)
	}

	return textResult(fmt.Sprintf("Deleted %s", pathValue), map[string]any{
		"path":      pathValue,
		"recursive": input.Recursive,
	}), nil
}
