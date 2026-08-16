package tui

import (
	"fmt"
	"strings"

	"a2aagent/agent/message"
	"a2aagent/agent/provider"
)

type EntryKind string

const (
	EntrySystem    EntryKind = "system"
	EntryUser      EntryKind = "user"
	EntryAssistant EntryKind = "assistant"
	EntryReasoning EntryKind = "reasoning"
	EntryTool      EntryKind = "tool"
	EntryError     EntryKind = "error"
)

type Entry struct {
	Kind EntryKind
	Text string
}

type State struct {
	Entries       []Entry
	Busy          bool
	Status        string
	Reasoning     string
	ToolStatus    string
	LastError     string
	Totals        provider.Usage
	LastUsage     provider.Usage
	ContextWindow int
	ContextTokens int
	CacheKnown    bool
}

func NewState() State {
	return State{Status: "ready"}
}

func (s State) StatsText() string {
	return s.statsText(nil)
}

func (s State) StatsLine(style func(string, string) string) string {
	return s.statsText(style)
}

func (s State) statsText(style func(string, string) string) string {
	prompt := s.LastUsage.PromptTokens()
	cache := "n/a"
	if s.CacheKnown {
		rate := 0.0
		if prompt > 0 {
			rate = float64(s.LastUsage.CacheReadTokens) / float64(prompt) * 100
		}
		cache = fmt.Sprintf("read %s, write %s, hit %.1f%%", formatTokens(s.Totals.CacheReadTokens), formatTokens(s.Totals.CacheWriteTokens), rate)
	}
	context := "unknown"
	if s.ContextWindow > 0 {
		percent := 0.0
		if s.ContextTokens > 0 {
			percent = float64(s.ContextTokens) / float64(s.ContextWindow) * 100
		}
		context = fmt.Sprintf("%s/%s (%.1f%%)", formatTokens(s.ContextTokens), formatTokens(s.ContextWindow), percent)
		if style != nil {
			color := dim
			if percent > 90 {
				color = red
			} else if percent > 70 {
				color = yellow
			}
			context = style(color, context)
		}
	}
	return fmt.Sprintf("context %s · input %s · output %s · cache %s", context, formatTokens(s.Totals.InputTokens), formatTokens(s.Totals.OutputTokens), cache)
}

func formatTokens(value int) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	if value < 10000 {
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	}
	if value < 1_000_000 {
		return fmt.Sprintf("%dk", value/1000)
	}
	if value < 10_000_000 {
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	}
	return fmt.Sprintf("%dM", value/1_000_000)
}

func (s *State) Add(kind EntryKind, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	s.Entries = append(s.Entries, Entry{Kind: kind, Text: text})
}

func (s *State) StartTurn(input string) {
	s.Add(EntryUser, input)
	s.Busy = true
	s.Status = "thinking"
	s.Reasoning = ""
	s.ToolStatus = ""
	s.LastError = ""
	s.ContextTokens = 0
	s.Entries = append(s.Entries, Entry{Kind: EntryAssistant})
}

func (s *State) Clear() {
	s.Entries = nil
	s.Busy = false
	s.Status = "ready"
	s.Reasoning = ""
	s.ToolStatus = ""
	s.LastError = ""
	s.Totals = provider.Usage{}
	s.LastUsage = provider.Usage{}
	s.ContextTokens = 0
	s.CacheKnown = false
}

func (s *State) Apply(event message.Event) {
	switch event.Type {
	case message.EventUsage:
		s.LastUsage = event.Usage
		s.Totals.InputTokens += event.Usage.InputTokens
		s.Totals.OutputTokens += event.Usage.OutputTokens
		s.Totals.CacheReadTokens += event.Usage.CacheReadTokens
		s.Totals.CacheWriteTokens += event.Usage.CacheWriteTokens
		s.Totals.TotalTokens += event.Usage.TotalTokens
		s.ContextTokens = event.Usage.PromptTokens()
		s.CacheKnown = s.CacheKnown || event.Usage.HasCacheData()
	case message.EventReasoningDelta:
		s.Reasoning += event.Reasoning
		s.Status = "reasoning"
	case message.EventTextDelta:
		if len(s.Entries) == 0 || s.Entries[len(s.Entries)-1].Kind != EntryAssistant {
			s.Entries = append(s.Entries, Entry{Kind: EntryAssistant})
		}
		s.Entries[len(s.Entries)-1].Text += event.Text
		s.Status = "streaming"
	case message.EventToolCall:
		if event.ToolCall != nil {
			s.ToolStatus = "calling " + event.ToolCall.Name
			s.Status = "tool"
		}
	case message.EventToolResult:
		if event.ToolCall != nil {
			s.ToolStatus = "completed " + event.ToolCall.Name
			s.Status = "tool"
		}
	case message.EventDone:
		s.Busy = false
		s.Status = "ready"
		s.ToolStatus = ""
		if event.Response != nil && event.Response.Usage.PromptTokens() > 0 && event.Usage.PromptTokens() != s.LastUsage.PromptTokens() {
			s.LastUsage = event.Response.Usage
			s.ContextTokens = event.Response.Usage.PromptTokens()
		}
	case message.EventError:
		s.Busy = false
		s.Status = "error"
		if event.Err != nil {
			s.LastError = event.Err.Error()
		}
	}
}
