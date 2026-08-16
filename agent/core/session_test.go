package core

import (
	"sync"
	"testing"

	"a2aagent/agent/provider"
)

func TestSessionCopiesMessagesAndToolArguments(t *testing.T) {
	session := NewSession(provider.Message{
		Role: provider.RoleAssistant,
		ToolCalls: []provider.ToolCall{{
			ID:        "call-1",
			Arguments: []byte(`{"path":"notes.txt"}`),
		}},
	})
	messages := session.Messages()
	messages[0].ToolCalls[0].Arguments[0] = 'X'
	if got := string(session.Messages()[0].ToolCalls[0].Arguments); got != `{"path":"notes.txt"}` {
		t.Fatalf("session was mutated through returned copy: %q", got)
	}
}

func TestSessionConcurrentAppendAndRead(t *testing.T) {
	session := NewSession()
	var group sync.WaitGroup
	for i := 0; i < 50; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			session.Append(provider.Message{Role: provider.RoleUser, Content: "hello"})
			_ = session.Messages()
		}()
	}
	group.Wait()
	if session.Len() != 50 {
		t.Fatalf("session length = %d, want 50", session.Len())
	}
}
