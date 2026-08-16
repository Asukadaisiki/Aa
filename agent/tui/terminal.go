package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// Terminal is the small boundary between the TUI and the real terminal.
// Keeping it injectable makes rendering and key handling testable without a
// real console.
type Terminal interface {
	Interactive() bool
	Start(onInput func(string), onResize func()) error
	Stop() error
	Write(string) error
	Size() (int, int)
}

// ProcessTerminal puts a tty into raw mode and translates its byte stream into
// complete UTF-8 characters or ANSI key sequences.
type ProcessTerminal struct {
	input  *os.File
	output io.Writer

	mu       sync.Mutex
	stopOnce sync.Once
	stopCh   chan struct{}
	rawState *term.State
}

func NewProcessTerminal(input *os.File, output io.Writer) *ProcessTerminal {
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	return &ProcessTerminal{input: input, output: output, stopCh: make(chan struct{})}
}

func (t *ProcessTerminal) Interactive() bool {
	return t != nil && t.input != nil && term.IsTerminal(int(t.input.Fd()))
}

func (t *ProcessTerminal) Size() (int, int) {
	if t == nil || t.input == nil {
		return 80, 24
	}
	width, height, err := term.GetSize(int(t.input.Fd()))
	if err != nil || width < 1 || height < 1 {
		return 80, 24
	}
	return width, height
}

func (t *ProcessTerminal) Start(onInput func(string), onResize func()) error {
	if !t.Interactive() {
		return fmt.Errorf("interactive terminal is required")
	}
	state, err := term.MakeRaw(int(t.input.Fd()))
	if err != nil {
		return fmt.Errorf("enable raw terminal mode: %w", err)
	}
	t.rawState = state
	if err := t.Write("\x1b[?2004h\x1b[?25l"); err != nil {
		_ = term.Restore(int(t.input.Fd()), state)
		return err
	}

	go t.readInput(onInput)
	go t.watchResize(onResize)
	return nil
}

func (t *ProcessTerminal) readInput(onInput func(string)) {
	decoder := terminalInputDecoder{emit: onInput}
	buffer := make([]byte, 4096)
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}
		n, err := t.input.Read(buffer)
		if n > 0 {
			decoder.feed(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

func (t *ProcessTerminal) watchResize(onResize func()) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	lastWidth, lastHeight := t.Size()
	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			width, height := t.Size()
			if width != lastWidth || height != lastHeight {
				lastWidth, lastHeight = width, height
				if onResize != nil {
					onResize()
				}
			}
		}
	}
}

func (t *ProcessTerminal) Write(value string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := io.WriteString(t.output, value)
	return err
}

func (t *ProcessTerminal) Stop() error {
	if t == nil {
		return nil
	}
	var err error
	t.stopOnce.Do(func() {
		close(t.stopCh)
		if t.rawState != nil {
			err = term.Restore(int(t.input.Fd()), t.rawState)
			t.rawState = nil
		}
		writeErr := t.Write("\x1b[?2004l\x1b[?25h\r\n")
		if err == nil {
			err = writeErr
		}
	})
	return err
}

type terminalInputDecoder struct {
	buffer []byte
	emit   func(string)
}

func (d *terminalInputDecoder) feed(data []byte) {
	d.buffer = append(d.buffer, data...)
	for len(d.buffer) > 0 {
		if d.buffer[0] != 0x1b {
			size := 1
			if d.buffer[0] >= 0x80 {
				if !utf8.FullRune(d.buffer) {
					return
				}
				_, size = utf8.DecodeRune(d.buffer)
			}
			d.emit(string(d.buffer[:size]))
			d.buffer = d.buffer[size:]
			continue
		}

		if len(d.buffer) == 1 {
			return
		}
		if d.buffer[1] == '[' {
			end := ansiSequenceEnd(d.buffer[2:])
			if end < 0 {
				return
			}
			end += 2
			d.emit(string(d.buffer[:end+1]))
			d.buffer = d.buffer[end+1:]
			continue
		}
		if d.buffer[1] == ']' {
			end := oscSequenceEnd(d.buffer[2:])
			if end < 0 {
				return
			}
			end += 2
			d.emit(string(d.buffer[:end+1]))
			d.buffer = d.buffer[end+1:]
			continue
		}
		// A bare Escape is a complete key. Alt+key is represented as two
		// events, which is sufficient for the editor's core bindings.
		d.emit("\x1b")
		d.buffer = d.buffer[1:]
	}
}

func ansiSequenceEnd(data []byte) int {
	for i, value := range data {
		if value >= 0x40 && value <= 0x7e {
			return i
		}
	}
	return -1
}

func oscSequenceEnd(data []byte) int {
	for i, value := range data {
		if value == 0x07 {
			return i
		}
		if value == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
			return i + 1
		}
	}
	return -1
}
