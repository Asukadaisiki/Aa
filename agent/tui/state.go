package tui

import (
	"strings"

	"a2aagent/agent/message"
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
	Entries    []Entry
	Busy       bool
	Status     string
	Reasoning  string
	ToolStatus string
	LastError  string
}

func NewState() State {
	return State{Status: "ready"}
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
	s.Entries = append(s.Entries, Entry{Kind: EntryAssistant})
}

func (s *State) Clear() {
	s.Entries = nil
	s.Busy = false
	s.Status = "ready"
	s.Reasoning = ""
	s.ToolStatus = ""
	s.LastError = ""
}

func (s *State) Apply(event message.Event) {
	switch event.Type {
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
	case message.EventError:
		s.Busy = false
		s.Status = "error"
		if event.Err != nil {
			s.LastError = event.Err.Error()
		}
	}
}
