package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"a2aagent/agent/core"
)

type Options struct {
	Input     io.Reader
	Output    io.Writer
	WorkDir   string
	Width     int
	ANSI      bool
	Prompt    string
	OnCommand func(Command) bool
}

type App struct {
	client  *Client
	input   *bufio.Reader
	output  io.Writer
	options Options
	state   State
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
	return &App{client: client, input: bufio.NewReader(options.Input), output: options.Output, options: options, state: NewState()}, nil
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("tui context is required")
	}
	if err := a.render(); err != nil {
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
		a.state.StartTurn(input)
		if renderErr := a.render(); renderErr != nil {
			return renderErr
		}
		events, runErr := a.client.Run(ctx, input)
		if runErr != nil {
			a.state.Busy = false
			a.state.Status = "error"
			a.state.LastError = runErr.Error()
			if renderErr := a.render(); renderErr != nil {
				return renderErr
			}
			continue
		}
		for event := range events {
			a.state.Apply(event)
			if renderErr := a.render(); renderErr != nil {
				return renderErr
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (a *App) handleCommand(command Command) bool {
	switch command {
	case CommandHelp:
		a.state.Add(EntrySystem, "Commands: /help, /clear, /count, /quit. Ctrl+D also exits.")
	case CommandClear:
		a.client.agent.ClearSession()
		a.state.Clear()
	case CommandCount:
		a.state.Add(EntrySystem, fmt.Sprintf("Session messages: %d", len(a.client.agent.Messages())))
	case CommandQuit:
		return true
	}
	if a.options.OnCommand != nil && a.options.OnCommand(command) {
		return true
	}
	_ = a.render()
	return false
}

func (a *App) render() error {
	return Render(a.output, a.state, a.options.WorkDir, RenderOptions{Width: a.options.Width, ANSI: a.options.ANSI})
}
