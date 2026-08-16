package tui

import "a2aagent/agent/message"

func eventLabel(event message.Event) string {
	switch event.Type {
	case message.EventReasoningDelta:
		return "reasoning"
	case message.EventTextDelta:
		return "assistant"
	case message.EventToolCall:
		return "tool"
	case message.EventToolResult:
		return "tool result"
	case message.EventUsage:
		return "usage"
	case message.EventDone:
		return "done"
	case message.EventError:
		return "error"
	default:
		return string(event.Type)
	}
}
