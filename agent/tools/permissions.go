package tools

import (
	"context"
	"fmt"
)

// PermissionMode controls whether an allowed tool runs directly or needs an
// approval callback first.
type PermissionMode string

const (
	PermissionModeApproval   PermissionMode = "approval"
	PermissionModeAutonomous PermissionMode = "autonomous"
)

// Permission identifies the operation that is being requested.
type Permission string

const (
	PermissionRead            Permission = "read"
	PermissionWrite           Permission = "write"
	PermissionCreate          Permission = "create"
	PermissionDelete          Permission = "delete"
	PermissionSave            Permission = "save"
	PermissionRecursiveDelete Permission = "recursive_delete"
)

// ApprovalRequest is passed to the caller in approval mode.
type ApprovalRequest struct {
	Tool        string       `json:"tool"`
	Path        string       `json:"path"`
	Permissions []Permission `json:"permissions"`
	Recursive   bool         `json:"recursive,omitempty"`
}

// Approver decides whether one tool operation may proceed.
type Approver func(context.Context, ApprovalRequest) (bool, error)

func authorize(ctx context.Context, toolCtx Context, request ApprovalRequest) error {
	mode := toolCtx.Mode
	if mode == "" {
		mode = PermissionModeApproval
	}
	switch mode {
	case PermissionModeAutonomous:
		return nil
	case PermissionModeApproval:
		if toolCtx.Approver == nil {
			return fmt.Errorf("approval required for tool %q", request.Tool)
		}
		approved, err := toolCtx.Approver(ctx, request)
		if err != nil {
			return fmt.Errorf("approval failed for tool %q: %w", request.Tool, err)
		}
		if !approved {
			return fmt.Errorf("operation not approved for tool %q", request.Tool)
		}
		return nil
	default:
		return fmt.Errorf("unsupported permission mode: %q", mode)
	}
}
