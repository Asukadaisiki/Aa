package message

import (
	"context"
	"encoding/json"
	"fmt"

	"a2aagent/agent/config"
	"a2aagent/agent/provider"
	"a2aagent/agent/tools"
)

const (
	ToolSearch = "tool_search"
	ToolLoad   = "tool_load"
)

// Client is the provider-facing business client. It owns the request
// configuration and tool registry, but deliberately does not own session
// history; session history belongs to core.Loop.
type Client struct {
	cfg      config.Config
	provider provider.Provider
	registry *tools.Registry
	toolCtx  tools.Context
}

// NewClient creates a message client with the default tool context.
func NewClient(cfg config.Config, p provider.Provider, registry *tools.Registry) (*Client, error) {
	return NewClientWithToolContext(cfg, p, registry, tools.Context{})
}

// NewClientWithToolContext creates a message client with the working
// directory and permission policy used when executing tools.
func NewClientWithToolContext(cfg config.Config, p provider.Provider, registry *tools.Registry, toolCtx tools.Context) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate message client config: %w", err)
	}
	if p == nil {
		return nil, fmt.Errorf("message client provider is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("message client tool registry is required")
	}
	return &Client{cfg: cfg, provider: p, registry: registry, toolCtx: toolCtx}, nil
}

// Post sends one provider request and returns its streamed events. Validation
// and request construction happen before the goroutine starts; provider and
// stream errors are delivered on the returned channel.
func (c *Client) Post(ctx context.Context, history []provider.Message, loaded map[string]tools.Definition) (<-chan Event, error) {
	if ctx == nil {
		return nil, fmt.Errorf("message client context is required")
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("message history is required")
	}
	request := c.cfg.Request(history, c.toolDefinitions(loaded))
	events := make(chan Event, 32)
	go func() {
		defer close(events)
		_, err := c.provider.Stream(ctx, request, func(event provider.StreamEvent) error {
			return sendProviderEvent(ctx, events, event)
		})
		if err != nil && ctx.Err() == nil {
			sendEvent(ctx, events, Event{Type: EventError, Err: err})
		}
	}()
	return events, nil
}

func (c *Client) toolDefinitions(loaded map[string]tools.Definition) []provider.ToolDefinition {
	definitions := make([]tools.Definition, 0, len(loaded)+2)
	definitions = append(definitions, discoveryDefinitions()...)
	for _, definition := range tools.LoadedDefinitions(loaded) {
		if definition.Name == ToolSearch || definition.Name == ToolLoad {
			continue
		}
		definitions = append(definitions, definition)
	}
	result := make([]provider.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, provider.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
		})
	}
	return result
}

// SearchTools exposes summaries without exposing full schemas.
func (c *Client) SearchTools(query string) []tools.ToolSummary {
	return c.registry.Search(query)
}

// LoadTool resolves one registry key and returns its complete definition.
func (c *Client) LoadTool(key string) (tools.Definition, bool) {
	return c.registry.Loaded(key)
}

// ExecuteTool runs a loaded registry definition with the configured tool
// context. Discovery tools are handled by core.Loop and are not executable
// through this method.
func (c *Client) ExecuteTool(ctx context.Context, name string, raw json.RawMessage) (tools.Result, error) {
	definition, ok := c.registry.Loaded(name)
	if !ok {
		return tools.Result{}, fmt.Errorf("tool %q is not loaded", name)
	}
	if definition.Execute == nil {
		return tools.Result{}, fmt.Errorf("tool %q has no executor", name)
	}
	return definition.Execute(ctx, c.toolCtx, raw)
}

func discoveryDefinitions() []tools.Definition {
	return []tools.Definition{
		{
			Name:        ToolSearch,
			Label:       ToolSearch,
			Description: "Search available tools by name or description. Returns keys that can be loaded.",
			Parameters: tools.Schema{
				Type: "object",
				Properties: map[string]tools.Property{
					"query": {Type: "string", Description: "Words describing the tool needed."},
				},
				Required:             []string{"query"},
				AdditionalProperties: false,
			},
		},
		{
			Name:        ToolLoad,
			Label:       ToolLoad,
			Description: "Load one tool by the key returned from tool_search.",
			Parameters: tools.Schema{
				Type: "object",
				Properties: map[string]tools.Property{
					"key": {Type: "string", Description: "Stable key returned by tool_search."},
				},
				Required:             []string{"key"},
				AdditionalProperties: false,
			},
		},
	}
}

func sendProviderEvent(ctx context.Context, events chan<- Event, event provider.StreamEvent) error {
	converted := Event{Usage: event.Usage}
	switch event.Type {
	case provider.EventReasoningDelta:
		converted.Type, converted.Reasoning = EventReasoningDelta, event.Reasoning
	case provider.EventTextDelta:
		converted.Type, converted.Text = EventTextDelta, event.Text
	case provider.EventToolCallDelta:
		converted.Type = EventToolCallDelta
		delta := event.ToolCall
		converted.ToolCallDelta = &delta
	case provider.EventDone:
		converted.Type, converted.Response = EventDone, event.Response
	default:
		return fmt.Errorf("unsupported provider event type %q", event.Type)
	}
	return sendEvent(ctx, events, converted)
}

func sendEvent(ctx context.Context, events chan<- Event, event Event) error {
	select {
	case events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
