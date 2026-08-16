package tui

import (
	"fmt"
	"strings"
)

type Cursor struct {
	Row int
	Col int
}

// DiffRenderer keeps a fixed-height visible viewport. The logical frame may
// grow without bound; only its tail is mapped to terminal rows, which makes
// scrolling deterministic and prevents stale rows from being overwritten.
type DiffRenderer struct {
	terminal    Terminal
	previous    []string
	previousTop int
	width       int
	height      int
	initialized bool
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
	if width != r.width || height != r.height {
		force = true
	}
	visible, top := viewportLines(lines, width, height)
	if !force && r.initialized && len(visible) == len(r.previous) {
		if top != r.previousTop {
			force = true
		}
	}
	if !force && r.initialized && len(visible) == len(r.previous) {
		var buffer strings.Builder
		buffer.WriteString("\x1b[?2026h")
		for i, line := range visible {
			if line == r.previous[i] {
				continue
			}
			fmt.Fprintf(&buffer, "\x1b[%d;1H\x1b[2K%s", i+1, line)
		}
		buffer.WriteString(cursorSequence(cursor, top, height))
		buffer.WriteString("\x1b[?2026l")
		if buffer.Len() > len("\x1b[?2026h\x1b[?2026l") {
			if err := r.terminal.Write(buffer.String()); err != nil {
				return err
			}
		}
	} else {
		var buffer strings.Builder
		buffer.WriteString("\x1b[?2026h\x1b[2J\x1b[H")
		for i, line := range visible {
			if i > 0 {
				buffer.WriteString("\r\n")
			}
			buffer.WriteString("\x1b[2K")
			buffer.WriteString(line)
		}
		buffer.WriteString(cursorSequence(cursor, top, height))
		buffer.WriteString("\x1b[?2026l")
		if err := r.terminal.Write(buffer.String()); err != nil {
			return err
		}
	}
	r.previous = visible
	r.previousTop = top
	r.width, r.height = width, height
	r.initialized = true
	return nil
}

func (r *DiffRenderer) Reset() {
	r.previous = nil
	r.initialized = false
	r.width, r.height = 0, 0
	r.previousTop = 0
}

func viewportLines(lines []string, width, height int) ([]string, int) {
	start := len(lines) - height
	if start < 0 {
		start = 0
	}
	visible := make([]string, 0, minInt(height, len(lines)))
	for i := start; i < len(lines); i++ {
		visible = append(visible, fitLine(lines[i], width))
	}
	return visible, start
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func cursorSequence(cursor Cursor, viewportTop, height int) string {
	row := cursor.Row - viewportTop + 1
	if row < 1 {
		row = 1
	}
	if row > height {
		row = height
	}
	col := cursor.Col + 1
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}
