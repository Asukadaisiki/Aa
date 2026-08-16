package core

import (
	"encoding/json"
	"sync"

	"a2aagent/agent/provider"
)

// Session is a thread-safe in-memory conversation history. Persistence is
// intentionally outside the session; the existing save tool can persist a
// caller-provided message array when needed.
type Session struct {
	mu       sync.RWMutex
	messages []provider.Message
}

func NewSession(initial ...provider.Message) *Session {
	messages := make([]provider.Message, 0, len(initial))
	for _, message := range initial {
		messages = append(messages, cloneMessage(message))
	}
	return &Session{messages: messages}
}

func (s *Session) Messages() []provider.Message {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]provider.Message, 0, len(s.messages))
	for _, message := range s.messages {
		result = append(result, cloneMessage(message))
	}
	return result
}

func (s *Session) Append(message provider.Message) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = append(s.messages, cloneMessage(message))
	s.mu.Unlock()
}

func (s *Session) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

func (s *Session) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.messages = nil
	s.mu.Unlock()
}

func cloneMessage(message provider.Message) provider.Message {
	message.ToolCalls = append([]provider.ToolCall(nil), message.ToolCalls...)
	for i := range message.ToolCalls {
		message.ToolCalls[i].Arguments = append(json.RawMessage(nil), message.ToolCalls[i].Arguments...)
	}
	return message
}
