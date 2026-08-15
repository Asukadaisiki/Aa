package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolvePath(toolCtx Context, rawPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		return "", fmt.Errorf("path is required")
	}

	base := toolCtx.WorkDir
	if strings.TrimSpace(base) == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	pathValue := rawPath
	if !filepath.IsAbs(pathValue) {
		pathValue = filepath.Join(base, pathValue)
	}
	resolved, err := filepath.Abs(pathValue)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	relative, err := filepath.Rel(base, resolved)
	if err != nil {
		return "", fmt.Errorf("check path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay inside working directory: %s", rawPath)
	}
	return resolved, nil
}
