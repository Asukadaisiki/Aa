package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"a2aagent/agent/config"
	"a2aagent/agent/core"
	"a2aagent/agent/tools"
	"a2aagent/agent/tui"
)

func main() {
	configPath := flag.String("config", "configs/config.example.yaml", "path to YAML config")
	workDir := flag.String("workdir", ".", "agent working directory")
	permissions := flag.String("permissions", "approval", "tool permissions: approval or autonomous")
	noANSI := flag.Bool("no-ansi", false, "disable ANSI screen redraw and colors")
	flag.Parse()

	absWorkDir, err := filepath.Abs(*workDir)
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(fmt.Errorf("load config: %w", err))
	}
	mode := tools.PermissionMode(strings.ToLower(strings.TrimSpace(*permissions)))
	if mode != tools.PermissionModeApproval && mode != tools.PermissionModeAutonomous {
		fatal(fmt.Errorf("unsupported permissions mode %q", *permissions))
	}

	input := bufio.NewReader(os.Stdin)
	toolContext := tools.Context{WorkDir: absWorkDir, Mode: mode}
	terminal := tui.NewProcessTerminal(os.Stdin, os.Stdout)
	var approvalBroker *tui.ApprovalBroker
	if mode == tools.PermissionModeApproval {
		if *noANSI || !terminal.Interactive() {
			toolContext.Approver = terminalApprover(input)
		} else {
			approvalBroker = tui.NewApprovalBroker()
			toolContext.Approver = approvalBroker.Approve
		}
	}
	agent, err := core.NewAgentFromConfig(cfg, tools.NewRegistry(), toolContext, nil)
	if err != nil {
		fatal(err)
	}

	app, err := tui.New(agent, tui.Options{
		Input: input, Output: os.Stdout, WorkDir: absWorkDir, ANSI: !*noANSI,
		ContextWindow: cfg.EffectiveContextWindow(), Terminal: terminal, ApprovalBroker: approvalBroker,
	})
	if err != nil {
		fatal(err)
	}
	if err := app.Run(context.Background()); err != nil {
		fatal(err)
	}
}

func terminalApprover(input *bufio.Reader) tools.Approver {
	return func(ctx context.Context, request tools.ApprovalRequest) (bool, error) {
		if _, err := fmt.Fprintf(os.Stdout, "\nApprove %s on %s [%s]? [y/N] ", request.Tool, request.Path, strings.Join(permissionNames(request.Permissions), ", ")); err != nil {
			return false, err
		}
		line, err := input.ReadString('\n')
		if err != nil {
			return false, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		return strings.EqualFold(strings.TrimSpace(line), "y"), nil
	}
}

func permissionNames(values []tools.Permission) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = string(value)
	}
	return result
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Aa TUI:", err)
	os.Exit(1)
}
