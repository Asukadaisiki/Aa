package tui

import (
	"strings"
	"testing"

	"a2aagent/agent/message"
	"a2aagent/agent/provider"
)

type testTerminal struct {
	width, height int
	writes        []string
}

func (t *testTerminal) Interactive() bool                { return true }
func (t *testTerminal) Start(func(string), func()) error { return nil }
func (t *testTerminal) Stop() error                      { return nil }
func (t *testTerminal) Size() (int, int)                 { return t.width, t.height }
func (t *testTerminal) Write(value string) error         { t.writes = append(t.writes, value); return nil }

func TestDiffRendererOnlyClearsOnFullRedraw(t *testing.T) {
	terminal := &testTerminal{width: 20, height: 4}
	renderer := NewDiffRenderer(terminal)
	if err := renderer.Render([]string{"one", "two"}, Cursor{Row: 1, Col: 3}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(terminal.writes[0], "\x1b[2J") {
		t.Fatalf("first render did not clear screen: %q", terminal.writes[0])
	}
	styled := frameLines(NewState(), ".", 20, func(code, text string) string { return code + text + reset })
	terminal = &testTerminal{width: 20, height: 12}
	renderer = NewDiffRenderer(terminal)
	if err := renderer.Render(styled, Cursor{Row: len(styled) - 1, Col: 2}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(terminal.writes[0], "Aa") {
		t.Fatalf("styled first render lost text: %q", terminal.writes[0])
	}
	terminal = &testTerminal{width: 20, height: 4}
	renderer = NewDiffRenderer(terminal)
	if err := renderer.Render([]string{"one", "two"}, Cursor{Row: 1, Col: 3}, true); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render([]string{"one", "changed"}, Cursor{Row: 1, Col: 7}, false); err != nil {
		t.Fatal(err)
	}
	last := terminal.writes[len(terminal.writes)-1]
	if strings.Contains(last, "\x1b[2J") || !strings.Contains(last, "changed") {
		t.Fatalf("incremental render was not differential: %q", last)
	}
}

func TestEditorCoreEditingAndPaste(t *testing.T) {
	editor := NewEditor(".", nil)
	for _, key := range []string{"a", "b", "c", "\x1b[D", "\x7f"} {
		editor.HandleInput(key)
	}
	if got := editor.Text(); got != "ac" {
		t.Fatalf("edited text = %q, want %q", got, "ac")
	}
	editor.SetText(editor.Text())
	editor.HandleInput("\x1b[13;2u")
	editor.HandleInput("d")
	if got := editor.Text(); got != "ac\nd" {
		t.Fatalf("multiline text = %q", got)
	}
	editor.HandleInput("\x1b[200~")
	editor.HandleInput(" 中")
	editor.HandleInput("\x1b[201~")
	if got := editor.Text(); got != "ac\nd 中" {
		t.Fatalf("pasted text = %q", got)
	}
}

func TestEditorSlashCompletion(t *testing.T) {
	editor := NewEditor(".", []Completion{{Value: "/help", Label: "/help"}, {Value: "/clear", Label: "/clear"}})
	for _, key := range []string{"/", "h", "e"} {
		editor.HandleInput(key)
	}
	if action := editor.HandleInput("\r"); action != EditorNone || editor.Text() != "/help" {
		t.Fatalf("completion accept = action %v text %q", action, editor.Text())
	}
	if action := editor.HandleInput("\r"); action != EditorSubmit || editor.LastSubmitted() != "/help" {
		t.Fatalf("completion submit = action %v submitted %q", action, editor.LastSubmitted())
	}
}

func TestEditorHistoryNavigation(t *testing.T) {
	editor := NewEditor(".", nil)
	editor.AddHistory("first")
	editor.AddHistory("second")
	editor.SetText("draft")
	editor.HandleInput("\x1b[A")
	if editor.Text() != "second" {
		t.Fatalf("up history = %q", editor.Text())
	}
	editor.HandleInput("\x1b[A")
	if editor.Text() != "first" {
		t.Fatalf("second up history = %q", editor.Text())
	}
	editor.HandleInput("\x1b[B")
	if editor.Text() != "second" {
		t.Fatalf("down history = %q", editor.Text())
	}
	editor.HandleInput("\x1b[B")
	if editor.Text() != "draft" {
		t.Fatalf("draft restore = %q", editor.Text())
	}
}

func TestStateDisplaysContextAndCacheMetrics(t *testing.T) {
	state := NewState()
	state.ContextWindow = 1000
	state.Apply(message.Event{Type: message.EventUsage, Usage: provider.Usage{
		InputTokens: 25, CacheReadTokens: 75, CacheReported: true,
		OutputTokens: 10, TotalTokens: 110,
	}})
	line := state.StatsText()
	if !strings.Contains(line, "context 100/1.0k") || !strings.Contains(line, "hit 75.0%") {
		t.Fatalf("metrics line = %q", line)
	}
}
