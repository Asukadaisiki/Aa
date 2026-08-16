package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"a2aagent/agent/config"
	"a2aagent/agent/core"
	"a2aagent/agent/message"
	"a2aagent/agent/tools"
)

func main() {
	configPath := flag.String("config", "configs/config.example.yaml", "path to YAML config")
	workDir := flag.String("workdir", ".", "agent working directory")
	permissions := flag.String("permissions", "approval", "tool permissions: approval or autonomous")
	flag.Parse()

	absWorkDir, err := filepath.Abs(*workDir)
	if err != nil {
		fatal(err)
	}
	mode := tools.PermissionMode(strings.ToLower(strings.TrimSpace(*permissions)))
	if mode != tools.PermissionModeApproval && mode != tools.PermissionModeAutonomous {
		fatal(fmt.Errorf("unsupported permissions mode %q", *permissions))
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(fmt.Errorf("load config: %w", err))
	}

	input := bufio.NewReader(os.Stdin)
	toolContext := tools.Context{WorkDir: absWorkDir, Mode: mode}
	if mode == tools.PermissionModeApproval {
		toolContext.Approver = terminalApprover(input)
	}
	agent, err := core.NewAgentFromConfig(cfg, tools.NewRegistry(), toolContext, nil)
	if err != nil {
		fatal(err)
	}

	fmt.Println("Aa agent (line mode). Type /quit to exit.")
	for {
		fmt.Print("> ")
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return
			}
			fatal(err)
		}
		line = strings.TrimSpace(line)
		if line == "/quit" || line == "/exit" {
			return
		}
		if line == "" {
			if err != nil {
				return
			}
			continue
		}
		events, runErr := agent.Run(context.Background(), line)
		if runErr != nil {
			fmt.Fprintln(os.Stderr, "error:", runErr)
			continue
		}
		printEvents(events)
		if err == io.EOF {
			return
		}
	}
}

func printEvents(events <-chan message.Event) {
	for event := range events {
		switch event.Type {
		case message.EventTextDelta:
			fmt.Print(event.Text)
		case message.EventReasoningDelta:
			fmt.Fprint(os.Stderr, event.Reasoning)
		case message.EventToolCall:
			if event.ToolCall != nil {
				fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", event.ToolCall.Name)
			}
		case message.EventToolResult:
			fmt.Fprintln(os.Stderr, "[tool done]")
		case message.EventError:
			fmt.Fprintln(os.Stderr, "\nerror:", event.Err)
		case message.EventDone:
			fmt.Println()
		}
	}
}

func terminalApprover(input *bufio.Reader) tools.Approver {
	return func(ctx context.Context, request tools.ApprovalRequest) (bool, error) {
		fmt.Fprintf(os.Stdout, "\nApprove %s on %s [%s]? [y/N] ", request.Tool, request.Path, strings.Join(permissionNames(request.Permissions), ", "))
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
	fmt.Fprintln(os.Stderr, "Aa agent:", err)
	os.Exit(1)
}
