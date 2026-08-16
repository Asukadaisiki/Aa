package tui

import (
	"fmt"
	"io"
	"strings"
)

const (
	reset  = "\x1b[0m"
	cyan   = "\x1b[36m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
)

type RenderOptions struct {
	Width int
	ANSI  bool
}

func Render(w io.Writer, state State, workDir string, options RenderOptions) error {
	width := options.Width
	if width < 40 {
		width = 80
	}
	style := func(code, text string) string {
		if !options.ANSI {
			return text
		}
		return code + text + reset
	}
	if options.ANSI {
		if _, err := io.WriteString(w, "\x1b[2J\x1b[H"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "%s\n", style(cyan+bold, "Aa · Agent TUI")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s  %s\n", style(dim, "workspace:"), workDir); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s  %s\n", style(dim, "status:"), statusText(state, style)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", width)); err != nil {
		return err
	}
	for _, entry := range state.Entries {
		label, color := entryLabel(entry.Kind)
		if _, err := fmt.Fprintf(w, "%s\n", style(color+bold, label+":")); err != nil {
			return err
		}
		for _, line := range wrap(entry.Text, width-2) {
			if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
				return err
			}
		}
	}
	if state.Reasoning != "" && state.Busy {
		if _, err := fmt.Fprintf(w, "%s\n", style(dim, "reasoning: "+truncate(state.Reasoning, width-12))); err != nil {
			return err
		}
	}
	if state.ToolStatus != "" {
		if _, err := fmt.Fprintf(w, "%s\n", style(yellow, "• "+state.ToolStatus)); err != nil {
			return err
		}
	}
	if state.LastError != "" {
		if _, err := fmt.Fprintf(w, "%s\n", style(red, "error: "+state.LastError)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("─", width)); err != nil {
		return err
	}
	if state.Busy {
		_, err := fmt.Fprintln(w, style(cyan, "… Agent is working"))
		return err
	}
	_, err := fmt.Fprintln(w, style(dim, "/help  /clear  /count  /quit"))
	return err
}

func statusText(state State, style func(string, string) string) string {
	if state.Status == "error" {
		return style(red, state.Status)
	}
	if state.Busy {
		return style(cyan, state.Status)
	}
	return style(green, state.Status)
}

func entryLabel(kind EntryKind) (string, string) {
	switch kind {
	case EntryUser:
		return "you", green
	case EntryAssistant:
		return "assistant", cyan
	case EntryReasoning:
		return "reasoning", dim
	case EntryTool:
		return "tool", yellow
	case EntryError:
		return "error", red
	default:
		return "system", dim
	}
}

func wrap(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	if text == "" {
		return []string{""}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			result = append(result, "")
			continue
		}
		for len(runes) > width {
			cut := width
			for i := width; i > 0; i-- {
				if runes[i-1] == ' ' || runes[i-1] == '\t' {
					cut = i
					break
				}
			}
			result = append(result, strings.TrimRight(string(runes[:cut]), " \t"))
			runes = []rune(strings.TrimLeft(string(runes[cut:]), " \t"))
		}
		result = append(result, string(runes))
	}
	return result
}

func truncate(text string, width int) string {
	runes := []rune(text)
	if width < 1 || len(runes) <= width {
		return text
	}
	return string(runes[:width-1]) + "…"
}
