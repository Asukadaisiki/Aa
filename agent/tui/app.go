package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"a2aagent/agent/core"
	"a2aagent/agent/message"
	"a2aagent/agent/tools"
)

type Options struct {
	Input          io.Reader
	Output         io.Writer
	WorkDir        string
	Width          int
	ANSI           bool
	Prompt         string
	ContextWindow  int
	Terminal       Terminal
	ApprovalBroker *ApprovalBroker
	OnCommand      func(Command) bool
}

type App struct {
	client   *Client
	input    *bufio.Reader
	output   io.Writer
	options  Options
	state    State
	terminal Terminal
	renderer *DiffRenderer
	editor   *Editor
	inputCh  chan string
	resizeCh chan struct{}
	events   <-chan message.Event
	cancel   context.CancelFunc
	pending  *ApprovalPrompt
}

type ApprovalPrompt struct {
	Request tools.ApprovalRequest
	respond chan bool
}

func (p *ApprovalPrompt) Respond(approved bool) {
	if p == nil || p.respond == nil {
		return
	}
	select {
	case p.respond <- approved:
	default:
	}
}

type ApprovalBroker struct {
	requests chan *ApprovalPrompt
}

func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{requests: make(chan *ApprovalPrompt)}
}

func (b *ApprovalBroker) Approve(ctx context.Context, request tools.ApprovalRequest) (bool, error) {
	if b == nil {
		return false, fmt.Errorf("approval broker is required")
	}
	prompt := &ApprovalPrompt{Request: request, respond: make(chan bool, 1)}
	select {
	case b.requests <- prompt:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case approved := <-prompt.respond:
		return approved, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (b *ApprovalBroker) Requests() <-chan *ApprovalPrompt {
	if b == nil {
		return nil
	}
	return b.requests
}

func New(agent *core.Agent, options Options) (*App, error) {
	client, err := NewClient(agent)
	if err != nil {
		return nil, err
	}
	if options.Input == nil {
		options.Input = os.Stdin
	}
	if options.Output == nil {
		options.Output = os.Stdout
	}
	if options.WorkDir == "" {
		options.WorkDir, _ = os.Getwd()
	}
	if options.Prompt == "" {
		options.Prompt = "> "
	}
	if options.Terminal == nil {
		options.Terminal = NewProcessTerminal(fileInput(options.Input), options.Output)
	}
	state := NewState()
	state.ContextWindow = options.ContextWindow
	commands := []Completion{
		{Value: "/help", Label: "/help", Description: "show commands"},
		{Value: "/clear", Label: "/clear", Description: "clear session"},
		{Value: "/count", Label: "/count", Description: "show message count"},
		{Value: "/stats", Label: "/stats", Description: "show token stats"},
		{Value: "/quit", Label: "/quit", Description: "exit Aa"},
	}
	return &App{
		client: client, input: bufio.NewReader(options.Input), output: options.Output,
		options: options, state: state, terminal: options.Terminal,
		renderer: NewDiffRenderer(options.Terminal), editor: NewEditor(options.WorkDir, commands),
		inputCh: make(chan string, 32), resizeCh: make(chan struct{}, 1),
	}, nil
}

func fileInput(input io.Reader) *os.File {
	if file, ok := input.(*os.File); ok {
		return file
	}
	return os.Stdin
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("tui context is required")
	}
	if a.options.ANSI && a.terminal.Interactive() {
		return a.runInteractive(ctx)
	}
	return a.runLineMode(ctx)
}

func (a *App) runLineMode(ctx context.Context) error {
	if err := a.renderLine(); err != nil {
		return err
	}
	for {
		if _, err := fmt.Fprint(a.output, "\n"+a.options.Prompt); err != nil {
			return err
		}
		line, err := a.input.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		command, input := parseInput(line)
		if command != CommandNone {
			if a.handleCommand(command) {
				return nil
			}
			if renderErr := a.renderLine(); renderErr != nil {
				return renderErr
			}
			if err == io.EOF {
				return nil
			}
			continue
		}
		if input == "" {
			if err == io.EOF {
				return nil
			}
			continue
		}
		if a.state.Busy {
			continue
		}
		a.startTurn(ctx, input)
		for event := range a.events {
			a.state.Apply(event)
			if renderErr := a.renderLine(); renderErr != nil {
				return renderErr
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (a *App) runInteractive(ctx context.Context) error {
	if err := a.terminal.Start(func(data string) {
		select {
		case a.inputCh <- data:
		case <-ctx.Done():
		}
	}, func() {
		select {
		case a.resizeCh <- struct{}{}:
		default:
		}
	}); err != nil {
		return err
	}
	defer a.terminal.Stop()

	dirty := true
	if err := a.renderInteractive(true); err != nil {
		return err
	}
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	for {
		var approvalCh <-chan *ApprovalPrompt
		if a.options.ApprovalBroker != nil {
			approvalCh = a.options.ApprovalBroker.Requests()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-a.inputCh:
			if a.pending != nil {
				a.handleApprovalInput(data)
				dirty = true
				continue
			}
			action := a.editor.HandleInput(data)
			switch action {
			case EditorSubmit:
				if !a.state.Busy {
					command, input := parseInput(a.editor.LastSubmitted())
					if command != CommandNone {
						if a.handleCommand(command) {
							return nil
						}
					} else if input != "" {
						a.startTurn(ctx, input)
					}
				}
			case EditorInterrupt:
				if a.cancel != nil {
					a.cancel()
					a.cancel = nil
					a.state.Busy = false
					a.state.Status = "ready"
				} else {
					return nil
				}
			case EditorQuit:
				return nil
			}
			dirty = true
		case event, ok := <-a.events:
			if !ok {
				a.events = nil
				a.cancel = nil
				if a.state.Busy {
					a.state.Busy = false
					a.state.Status = "ready"
				}
				dirty = true
				continue
			}
			a.state.Apply(event)
			dirty = true
		case prompt := <-approvalCh:
			a.pending = prompt
			a.state.ToolStatus = fmt.Sprintf("approve %s on %s? [y/N]", prompt.Request.Tool, prompt.Request.Path)
			dirty = true
		case <-a.resizeCh:
			a.renderer.Reset()
			dirty = true
		case <-ticker.C:
			if dirty {
				if err := a.renderInteractive(false); err != nil {
					return err
				}
				dirty = false
			}
		}
	}
}

func (a *App) startTurn(parent context.Context, input string) {
	a.state.StartTurn(input)
	turnCtx, cancel := context.WithCancel(parent)
	a.cancel = cancel
	events, err := a.client.Run(turnCtx, input)
	if err != nil {
		cancel()
		a.cancel = nil
		a.state.Busy = false
		a.state.Status = "error"
		a.state.LastError = err.Error()
		return
	}
	a.events = events
}

func (a *App) handleApprovalInput(data string) {
	value := strings.ToLower(strings.TrimSpace(data))
	if value == "y" || value == "n" || value == "\r" || value == "\n" {
		a.pending.Respond(value == "y")
		a.pending = nil
		a.state.ToolStatus = ""
	}
}

func (a *App) renderLine() error {
	a.updateEstimatedContext()
	return Render(a.output, a.state, a.options.WorkDir, RenderOptions{Width: a.options.Width, ANSI: false})
}

func (a *App) renderInteractive(force bool) error {
	a.updateEstimatedContext()
	width, _ := a.terminal.Size()
	if width < 40 {
		width = 80
	}
	style := func(code, text string) string { return code + text + reset }
	lines := frameLines(a.state, a.options.WorkDir, width, style)
	editorLines, cursorRow, cursorCol := a.editor.Render(width)
	baseRows := len(lines)
	lines = append(lines, editorLines...)
	return a.renderer.Render(lines, Cursor{Row: baseRows + cursorRow, Col: cursorCol}, force)
}

func (a *App) updateEstimatedContext() {
	if a.state.ContextTokens > 0 {
		return
	}
	tokens := 0
	for _, item := range a.client.agent.Messages() {
		text := item.Content + item.ReasoningContent
		if text != "" {
			tokens += (len([]rune(text)) + 3) / 4
		}
		for _, call := range item.ToolCalls {
			tokens += (len(call.Name) + len(call.Arguments) + 3) / 4
		}
	}
	if tokens > 0 {
		a.state.ContextTokens = tokens
	}
}

func (a *App) handleCommand(command Command) bool {
	switch command {
	case CommandHelp:
		a.state.Add(EntrySystem, "Commands: /help, /clear, /count, /stats, /quit. Ctrl+C exits when idle or cancels a running turn; Ctrl+D exits on empty input.")
	case CommandClear:
		a.client.agent.ClearSession()
		a.state.Clear()
		a.state.ContextWindow = a.options.ContextWindow
		a.editor.Clear()
		a.renderer.Reset()
	case CommandCount:
		a.state.Add(EntrySystem, fmt.Sprintf("Session messages: %d", len(a.client.agent.Messages())))
	case CommandStats:
		a.state.Add(EntrySystem, a.state.StatsText())
	case CommandQuit:
		return true
	}
	if a.options.OnCommand != nil && a.options.OnCommand(command) {
		return true
	}
	return false
}
