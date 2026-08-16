package tui

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
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

// Render is retained for deterministic/non-interactive mode. The live
// terminal path consumes frameLines and updates only changed rows.
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
	for _, line := range frameLines(state, workDir, width, style) {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func frameLines(state State, workDir string, width int, style func(string, string) string) []string {
	lines := []string{
		style(cyan+bold, "Aa · Agent TUI"),
		style(dim, "workspace:") + "  " + workDir,
		style(dim, "status:") + "  " + statusText(state, style),
		strings.Repeat("─", width),
	}
	for _, entry := range state.Entries {
		label, color := entryLabel(entry.Kind)
		lines = append(lines, style(color+bold, label+":"))
		for _, line := range wrap(entry.Text, width-2) {
			lines = append(lines, "  "+line)
		}
	}
	if state.Reasoning != "" && state.Busy {
		lines = append(lines, style(dim, "reasoning: "+truncate(state.Reasoning, width-12)))
	}
	if state.ToolStatus != "" {
		lines = append(lines, style(yellow, "• "+state.ToolStatus))
	}
	if state.LastError != "" {
		lines = append(lines, style(red, "error: "+state.LastError))
	}
	lines = append(lines, strings.Repeat("─", width))
	lines = append(lines, state.StatsLine(style))
	if state.Busy {
		lines = append(lines, style(cyan, "… Agent is working"))
	} else {
		lines = append(lines, style(dim, "/help  /clear  /count  /stats  /quit"))
	}
	return lines
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

func visibleWidth(text string) int {
	width := 0
	for i := 0; i < len(text); {
		if text[i] == '\x1b' {
			i = skipANSI(text, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if size == 0 {
			break
		}
		if r == '\t' {
			width += 4
		} else if r >= 0x1100 && (r <= 0x115f || r >= 0x2e80) {
			width += 2
		} else {
			width++
		}
		i += size
	}
	return width
}

func skipANSI(text string, start int) int {
	if start+1 >= len(text) {
		return len(text)
	}
	if text[start+1] == '[' {
		for i := start + 2; i < len(text); i++ {
			if text[i] >= 0x40 && text[i] <= 0x7e {
				return i + 1
			}
		}
		return len(text)
	}
	return start + 1
}
