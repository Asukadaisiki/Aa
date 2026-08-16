package tui

import "strings"

type Command string

const (
	CommandNone  Command = ""
	CommandHelp  Command = "help"
	CommandClear Command = "clear"
	CommandQuit  Command = "quit"
	CommandCount Command = "count"
)

func parseInput(line string) (Command, string) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "/") {
		return CommandNone, trimmed
	}
	parts := strings.Fields(strings.TrimPrefix(trimmed, "/"))
	if len(parts) == 0 {
		return CommandHelp, ""
	}
	switch strings.ToLower(parts[0]) {
	case "help", "?":
		return CommandHelp, ""
	case "clear":
		return CommandClear, ""
	case "quit", "exit", "q":
		return CommandQuit, ""
	case "count", "history":
		return CommandCount, ""
	default:
		return CommandNone, trimmed
	}
}
