package core

import (
	"context"
	"fmt"

	"a2aagent/agent/config"
	"a2aagent/agent/message"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

// Agent is the public orchestration boundary of the backend.
//
// Agent owns the long-lived conversation session and delegates one user turn
// to Loop. Loop is responsible for streaming the provider response, executing
// tool calls, appending the resulting messages, and stopping when the model
// returns a response without tool calls.
type Agent struct {
	loop    *Loop
	session *Session
}

// NewAgent assembles an Agent from an already configured message client.
// Keeping the client as an input makes the orchestration layer independent of
// the provider implementation and keeps tests fast with a fake provider.
func NewAgent(client *message.Client, session *Session) (*Agent, error) {
	if client == nil {
		return nil, fmt.Errorf("agent message client is required")
	}
	if session == nil {
		session = NewSession()
	}
	return &Agent{loop: NewLoop(client, session), session: session}, nil
}

// NewAgentFromConfig builds the complete backend from application config.
// The provider is created once when the Agent is instantiated; every later
// Run call reuses that provider and sends the current session snapshot.
func NewAgentFromConfig(cfg config.Config, registry *tools.Registry, toolCtx tools.Context, session *Session) (*Agent, error) {
	if registry == nil {
		registry = tools.NewRegistry()
	}
	p, err := cfg.NewProvider()
	if err != nil {
		return nil, fmt.Errorf("create agent provider: %w", err)
	}
	client, err := message.NewClientWithToolContext(cfg, p, registry, toolCtx)
	if err != nil {
		return nil, fmt.Errorf("create agent message client: %w", err)
	}
	return NewAgent(client, session)
}

// Run starts one user turn and returns its ordered event stream immediately.
// The stream contains text/reasoning deltas, tool lifecycle events, and one
// final EventDone or EventError. The caller must consume the stream until it
// is closed so the session can finish updating and the next turn can start.
func (a *Agent) Run(ctx context.Context, input string) (<-chan message.Event, error) {
	if a == nil || a.loop == nil {
		return nil, fmt.Errorf("agent is not initialized")
	}
	return a.loop.Run(ctx, input)
}

// Session returns the conversation owned by the Agent. Session is safe for
// concurrent reads, but a normal caller should start the next Run only after
// consuming the previous event stream.
func (a *Agent) Session() *Session {
	if a == nil {
		return nil
	}
	return a.session
}

// Messages returns a defensive snapshot in provider-neutral, JSON-ready
// format. It is useful for persistence, debugging, or resuming a session.
func (a *Agent) Messages() []provider.Message {
	if a == nil || a.session == nil {
		return nil
	}
	return a.session.Messages()
}

func (a *Agent) ClearSession() {
	if a == nil || a.session == nil {
		return
	}
	a.session.Clear()
}
