package tui

import (
	"fmt"
	"strings"
)

type Cursor struct {
	Row int
	Col int
}

// DiffRenderer keeps the complete logical frame and updates it in place. New
// lines are written with normal CRLF output, so the terminal itself owns the
// scrollback buffer just like a regular CLI.
type DiffRenderer struct {
	terminal          Terminal
	previous          []string
	previousCursor    Cursor
	width             int
	height            int
	initialized       bool
	clearOnNextRender bool
}

func NewDiffRenderer(terminal Terminal) *DiffRenderer {
	return &DiffRenderer{terminal: terminal}
}

func (r *DiffRenderer) Render(lines []string, cursor Cursor, force bool) error {
	width, height := r.terminal.Size()
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	normalized := make([]string, len(lines))
	for i, line := range lines {
		normalized[i] = fitLine(line, width)
	}
	if len(normalized) == 0 {
		normalized = []string{""}
	}
	if !r.initialized {
		if err := r.fullRender(normalized, cursor, r.clearOnNextRender); err != nil {
			return err
		}
	} else if force || width != r.width || height != r.height || r.clearOnNextRender {
		if err := r.fullRender(normalized, cursor, true); err != nil {
			return err
		}
	} else if err := r.diffRender(normalized, cursor); err != nil {
		return err
	}
	r.clearOnNextRender = false
	r.previous = normalized
	r.previousCursor = cursor
	r.width, r.height = width, height
	r.initialized = true
	return nil
}

func (r *DiffRenderer) Reset() {
	r.previous = nil
	r.initialized = false
	r.width, r.height = 0, 0
	r.clearOnNextRender = true
}

func (r *DiffRenderer) fullRender(lines []string, cursor Cursor, clear bool) error {
	var buffer strings.Builder
	buffer.WriteString("\x1b[?2026h")
	if clear {
		// Same normal-screen strategy as pi: clear the visible screen and its
		// scrollback only when a full redraw is unavoidable.
		buffer.WriteString("\x1b[2J\x1b[H\x1b[3J")
	}
	for i, line := range lines {
		if i > 0 {
			buffer.WriteString("\r\n")
		}
		buffer.WriteString(line)
	}
	moveCursorFromEnd(&buffer, len(lines)-1, cursor)
	buffer.WriteString("\x1b[?2026l")
	return r.terminal.Write(buffer.String())
}

func (r *DiffRenderer) diffRender(lines []string, cursor Cursor) error {
	firstChanged := -1
	maxLines := len(lines)
	if len(r.previous) > maxLines {
		maxLines = len(r.previous)
	}
	for i := 0; i < maxLines; i++ {
		oldLine, newLine := "", ""
		if i < len(r.previous) {
			oldLine = r.previous[i]
		}
		if i < len(lines) {
			newLine = lines[i]
		}
		if oldLine != newLine {
			if firstChanged == -1 {
				firstChanged = i
			}
		}
	}
	if firstChanged == -1 {
		var buffer strings.Builder
		buffer.WriteString("\x1b[?2026h")
		moveCursorBetween(&buffer, r.previousCursor, cursor)
		buffer.WriteString("\x1b[?2026l")
		return r.terminal.Write(buffer.String())
	}
	if firstChanged < len(r.previous)-r.height {
		return r.fullRender(lines, cursor, true)
	}

	var buffer strings.Builder
	buffer.WriteString("\x1b[?2026h")
	previousLast := len(r.previous) - 1
	moveCursorDown := previousLast - r.previousCursor.Row
	if moveCursorDown > 0 {
		fmt.Fprintf(&buffer, "\x1b[%dB", moveCursorDown)
	}
	if firstChanged == len(r.previous) {
		buffer.WriteString("\r\n")
	} else {
		moveUp := previousLast - firstChanged
		if moveUp > 0 {
			fmt.Fprintf(&buffer, "\x1b[%dA", moveUp)
		}
		buffer.WriteString("\r")
	}
	for i := firstChanged; i < len(lines); i++ {
		if i > firstChanged {
			buffer.WriteString("\r\n")
		}
		buffer.WriteString("\x1b[2K")
		buffer.WriteString(lines[i])
	}
	for i := len(lines); i < len(r.previous); i++ {
		buffer.WriteString("\r\n\x1b[2K")
	}
	moveCursorFromEnd(&buffer, len(lines)-1, cursor)
	buffer.WriteString("\x1b[?2026l")
	return r.terminal.Write(buffer.String())
}

func moveCursorFromEnd(buffer *strings.Builder, lastRow int, cursor Cursor) {
	row := cursor.Row
	if row < 0 {
		row = 0
	}
	if row > lastRow {
		row = lastRow
	}
	if moveUp := lastRow - row; moveUp > 0 {
		fmt.Fprintf(buffer, "\x1b[%dA", moveUp)
	}
	buffer.WriteString("\r")
	col := cursor.Col + 1
	if col < 1 {
		col = 1
	}
	fmt.Fprintf(buffer, "\x1b[%dG", col)
}

func moveCursorBetween(buffer *strings.Builder, from, to Cursor) {
	if delta := to.Row - from.Row; delta > 0 {
		fmt.Fprintf(buffer, "\x1b[%dB", delta)
	} else if delta < 0 {
		fmt.Fprintf(buffer, "\x1b[%dA", -delta)
	}
	buffer.WriteString("\r")
	col := to.Col + 1
	if col < 1 {
		col = 1
	}
	fmt.Fprintf(buffer, "\x1b[%dG", col)
}

func fitLine(line string, width int) string {
	if visibleWidth(line) <= width {
		return line + strings.Repeat(" ", width-visibleWidth(line))
	}
	return truncateANSI(line, width)
}

func truncateANSI(line string, width int) string {
	if width <= 0 {
		return ""
	}
	var result strings.Builder
	used := 0
	for i := 0; i < len(line) && used < width; {
		if line[i] == '\x1b' {
			end := skipANSI(line, i)
			result.WriteString(line[i:end])
			i = end
			continue
		}
		next := i + 1
		for next < len(line) && line[next]&0xc0 == 0x80 {
			next++
		}
		part := line[i:next]
		w := visibleWidth(part)
		if used+w > width {
			break
		}
		result.WriteString(part)
		used += w
		i = next
	}
	return result.String() + reset + strings.Repeat(" ", width-used)
}
