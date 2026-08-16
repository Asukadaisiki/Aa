package tui

import (
	"context"
	"fmt"

	"a2aagent/agent/core"
	"a2aagent/agent/message"
)

type Client struct {
	agent *core.Agent
}

func NewClient(agent *core.Agent) (*Client, error) {
	if agent == nil {
		return nil, fmt.Errorf("tui agent is required")
	}
	return &Client{agent: agent}, nil
}

func (c *Client) Run(ctx context.Context, input string) (<-chan message.Event, error) {
	return c.agent.Run(ctx, input)
}
