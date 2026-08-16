package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type EditorAction int

const (
	EditorNone EditorAction = iota
	EditorSubmit
	EditorInterrupt
	EditorQuit
)

type Completion struct {
	Value       string
	Label       string
	Description string
}

type completionState struct {
	items []Completion
	start int
	end   int
	index int
}

type Editor struct {
	value         []rune
	cursor        int
	history       []string
	historyIndex  int
	draft         []rune
	completions   completionState
	workDir       string
	commands      []Completion
	lastSubmitted string
	pasting       bool
	pasteBuffer   strings.Builder
}

func NewEditor(workDir string, commands []Completion) *Editor {
	return &Editor{workDir: workDir, commands: append([]Completion(nil), commands...), historyIndex: -1}
}

func (e *Editor) Text() string { return string(e.value) }

func (e *Editor) LastSubmitted() string { return e.lastSubmitted }

func (e *Editor) SetText(value string) {
	e.value = []rune(value)
	e.cursor = len(e.value)
	e.historyIndex = -1
	e.completions = completionState{}
}

func (e *Editor) AddHistory(value string) {
	value = strings.TrimSpace(value)
	if value == "" || (len(e.history) > 0 && e.history[len(e.history)-1] == value) {
		return
	}
	e.history = append(e.history, value)
	if len(e.history) > 100 {
		e.history = e.history[len(e.history)-100:]
	}
}

func (e *Editor) Clear() {
	e.value = nil
	e.cursor = 0
	e.historyIndex = -1
	e.completions = completionState{}
}

func (e *Editor) HandleInput(data string) EditorAction {
	if data == "\x1b[200~" {
		e.pasting = true
		e.pasteBuffer.Reset()
		return EditorNone
	}
	if e.pasting {
		if data == "\x1b[201~" {
			text := strings.ReplaceAll(strings.ReplaceAll(e.pasteBuffer.String(), "\r\n", "\n"), "\r", "\n")
			text = strings.ReplaceAll(text, "\t", "    ")
			e.insert([]rune(text))
			e.pasting = false
			e.pasteBuffer.Reset()
		} else {
			e.pasteBuffer.WriteString(data)
		}
		return EditorNone
	}
	if strings.HasPrefix(data, "\x1b[200~") && strings.HasSuffix(data, "\x1b[201~") {
		text := strings.TrimSuffix(strings.TrimPrefix(data, "\x1b[200~"), "\x1b[201~")
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
		text = strings.ReplaceAll(text, "\t", "    ")
		e.insert([]rune(text))
		return EditorNone
	}

	if e.completions.items != nil {
		switch data {
		case "\x1b[A":
			e.moveCompletion(-1)
			return EditorNone
		case "\x1b[B":
			e.moveCompletion(1)
			return EditorNone
		case "\t", "\r", "\n":
			e.acceptCompletion()
			if data == "\t" {
				return EditorNone
			}
			return EditorNone
		case "\x1b":
			e.completions = completionState{}
			return EditorNone
		}
	}

	switch data {
	case "\x1b[13;2u", "\x1b[27;2u":
		e.insert([]rune{'\n'})
		return EditorNone
	case "\r", "\n":
		if strings.TrimSpace(string(e.value)) == "" {
			return EditorNone
		}
		value := string(e.value)
		e.AddHistory(value)
		e.lastSubmitted = value
		e.Clear()
		return EditorSubmit
	case "\x03":
		return EditorInterrupt
	case "\x04":
		if len(e.value) == 0 {
			return EditorQuit
		}
		e.deleteForward()
		return EditorNone
	case "\x01", "\x1b[H", "\x1b[1~":
		e.cursorLineStart()
	case "\x05", "\x1b[F", "\x1b[4~":
		e.cursorLineEnd()
	case "\x02", "\x1b[D":
		e.moveLeft()
	case "\x06", "\x1b[C":
		e.moveRight()
	case "\x10", "\x1b[A":
		e.moveUpOrHistory()
	case "\x0e", "\x1b[B":
		e.moveDownOrHistory()
	case "\x7f", "\x08", "\x1b[3~":
		e.deleteBackward()
	case "\x0b":
		e.deleteToEnd()
	case "\x15":
		e.deleteToStart()
	case "\x17", "\x1b[3;5~":
		e.deleteWordBackward()
	case "\x1b[1;5D", "\x1b[5D":
		e.moveWordBackward()
	case "\x1b[1;5C", "\x1b[5C":
		e.moveWordForward()
	case "\t":
		e.refreshCompletions(true)
	default:
		if value, ok := decodeKittyPrintable(data); ok {
			e.insert([]rune{value})
			break
		}
		if isPrintableInput(data) {
			e.insert([]rune(data))
		} else {
			return EditorNone
		}
	}
	e.refreshCompletions(false)
	return EditorNone
}

func decodeKittyPrintable(data string) (rune, bool) {
	if !strings.HasPrefix(data, "\x1b[") || !strings.HasSuffix(data, "u") {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(data, "\x1b["), "u")
	if strings.Contains(value, ";") {
		value = strings.Split(value, ";")[0]
	}
	code, err := strconv.Atoi(value)
	if err != nil || code < 0x20 || code > 0x10ffff {
		return 0, false
	}
	return rune(code), true
}

func (e *Editor) Render(width int) ([]string, int, int) {
	if width < 20 {
		width = 80
	}
	available := width - 2
	if available < 1 {
		available = 1
	}
	lines := strings.Split(string(e.value), "\n")
	result := make([]string, 0, len(lines)+len(e.completions.items)+1)
	cursorRow, cursorCol := 0, 2
	runeOffset := 0
	for lineIndex, line := range lines {
		lineRunes := []rune(line)
		if len(lineRunes) == 0 {
			lineRunes = []rune{}
		}
		for len(lineRunes) > available {
			part := append([]rune(nil), lineRunes[:available]...)
			prefix := "  "
			if lineIndex == 0 && len(result) == 0 {
				prefix = "> "
			}
			result = append(result, prefix+string(part))
			lineRunes = lineRunes[available:]
			runeOffset += available
		}
		prefix := "  "
		if lineIndex == 0 && len(result) == 0 {
			prefix = "> "
		}
		row := len(result)
		text := string(lineRunes)
		cursorLine := strings.Count(string(e.value[:e.cursor]), "\n")
		if e.cursor >= runeOffset && e.cursor <= runeOffset+len(lineRunes) && e.cursor == runeOffset+len(lineRunes) && lineIndex == cursorLine {
			text += "\x1b[7m \x1b[27m"
			cursorRow, cursorCol = row, len(prefix)+cellWidth(string(lineRunes))
		} else if e.cursor >= runeOffset && e.cursor < runeOffset+len(lineRunes) {
			at := e.cursor - runeOffset
			before := string(lineRunes[:at])
			cursor := string(lineRunes[at : at+1])
			after := string(lineRunes[at+1:])
			text = before + "\x1b[7m" + cursor + "\x1b[27m" + after
			cursorRow, cursorCol = row, len(prefix)+cellWidth(before)
		}
		result = append(result, prefix+text)
		runeOffset += len(lineRunes)
	}
	if len(result) == 0 {
		result = append(result, "> \x1b[7m \x1b[27m")
		cursorRow, cursorCol = 0, 2
	}
	for i, item := range e.completions.items {
		marker := "  "
		if i == e.completions.index {
			marker = "> "
		}
		line := marker + item.Label
		if item.Description != "" {
			line += "  " + item.Description
		}
		result = append(result, line)
	}
	return result, cursorRow, cursorCol
}

func (e *Editor) insert(value []rune) {
	if len(value) == 0 {
		return
	}
	e.value = append(e.value[:e.cursor], append(value, e.value[e.cursor:]...)...)
	e.cursor += len(value)
	e.historyIndex = -1
	e.completions = completionState{}
}

func (e *Editor) moveLeft() {
	if e.cursor > 0 {
		e.cursor--
	}
}

func (e *Editor) moveRight() {
	if e.cursor < len(e.value) {
		e.cursor++
	}
}

func (e *Editor) cursorLineStart() {
	start := e.cursor
	for start > 0 && e.value[start-1] != '\n' {
		start--
	}
	e.cursor = start
}

func (e *Editor) cursorLineEnd() {
	end := e.cursor
	for end < len(e.value) && e.value[end] != '\n' {
		end++
	}
	e.cursor = end
}

func (e *Editor) moveVertical(direction int) {
	column := 0
	start := e.cursor
	for start > 0 && e.value[start-1] != '\n' {
		start--
		column++
	}
	if direction < 0 {
		if start == 0 {
			return
		}
		prevEnd := start - 1
		prevStart := prevEnd
		for prevStart > 0 && e.value[prevStart-1] != '\n' {
			prevStart--
		}
		e.cursor = prevStart + min(column, prevEnd-prevStart)
		return
	}
	end := e.cursor
	for end < len(e.value) && e.value[end] != '\n' {
		end++
	}
	if end == len(e.value) {
		return
	}
	nextStart := end + 1
	nextEnd := nextStart
	for nextEnd < len(e.value) && e.value[nextEnd] != '\n' {
		nextEnd++
	}
	e.cursor = nextStart + min(column, nextEnd-nextStart)
}

func (e *Editor) moveUpOrHistory() {
	if !strings.ContainsRune(string(e.value), '\n') || e.cursorLine() == 0 {
		e.historyPrevious()
		return
	}
	e.moveVertical(-1)
}

func (e *Editor) moveDownOrHistory() {
	if !strings.ContainsRune(string(e.value), '\n') || e.cursorLine() == strings.Count(string(e.value), "\n") {
		e.historyNext()
		return
	}
	e.moveVertical(1)
}

func (e *Editor) cursorLine() int {
	return strings.Count(string(e.value[:e.cursor]), "\n")
}

func (e *Editor) historyPrevious() {
	if len(e.history) == 0 {
		return
	}
	if e.historyIndex == -1 {
		e.draft = append([]rune(nil), e.value...)
		e.historyIndex = len(e.history) - 1
	} else if e.historyIndex > 0 {
		e.historyIndex--
	}
	e.value = []rune(e.history[e.historyIndex])
	e.cursor = len(e.value)
	e.completions = completionState{}
}

func (e *Editor) historyNext() {
	if e.historyIndex == -1 {
		return
	}
	if e.historyIndex < len(e.history)-1 {
		e.historyIndex++
		e.value = []rune(e.history[e.historyIndex])
	} else {
		e.historyIndex = -1
		e.value = append([]rune(nil), e.draft...)
	}
	e.cursor = len(e.value)
	e.completions = completionState{}
}

func (e *Editor) moveWordBackward() {
	for e.cursor > 0 && unicode.IsSpace(e.value[e.cursor-1]) {
		e.cursor--
	}
	for e.cursor > 0 && !unicode.IsSpace(e.value[e.cursor-1]) {
		e.cursor--
	}
}

func (e *Editor) moveWordForward() {
	for e.cursor < len(e.value) && unicode.IsSpace(e.value[e.cursor]) {
		e.cursor++
	}
	for e.cursor < len(e.value) && !unicode.IsSpace(e.value[e.cursor]) {
		e.cursor++
	}
}

func (e *Editor) deleteBackward() {
	if e.cursor == 0 {
		return
	}
	e.value = append(e.value[:e.cursor-1], e.value[e.cursor:]...)
	e.cursor--
}

func (e *Editor) deleteForward() {
	if e.cursor < len(e.value) {
		e.value = append(e.value[:e.cursor], e.value[e.cursor+1:]...)
	}
}

func (e *Editor) deleteToStart() {
	start := e.cursor
	e.cursorLineStart()
	e.value = append(e.value[:e.cursor], e.value[start:]...)
}

func (e *Editor) deleteToEnd() {
	start := e.cursor
	e.cursorLineEnd()
	e.value = append(e.value[:start], e.value[e.cursor:]...)
	e.cursor = start
}

func (e *Editor) deleteWordBackward() {
	end := e.cursor
	e.moveWordBackward()
	e.value = append(e.value[:e.cursor], e.value[end:]...)
}

func (e *Editor) refreshCompletions(force bool) {
	start, end, prefix := e.completionToken()
	if !force && prefix == "" {
		e.completions = completionState{}
		return
	}
	items := e.suggestions(prefix, force)
	if len(items) == 0 {
		e.completions = completionState{}
		return
	}
	selected := 0
	if e.completions.start == start && e.completions.end == end && e.completions.index < len(items) {
		selected = e.completions.index
	}
	e.completions = completionState{items: items, start: start, end: end, index: selected}
}

func (e *Editor) completionToken() (int, int, string) {
	lineStart := e.cursor
	for lineStart > 0 && e.value[lineStart-1] != '\n' {
		lineStart--
	}
	start := e.cursor
	for start > lineStart && !unicode.IsSpace(e.value[start-1]) {
		start--
	}
	return start, e.cursor, string(e.value[start:e.cursor])
}

func (e *Editor) suggestions(prefix string, force bool) []Completion {
	if strings.HasPrefix(prefix, "/") {
		if strings.Contains(prefix, " ") {
			return nil
		}
		query := strings.ToLower(strings.TrimPrefix(prefix, "/"))
		items := make([]Completion, 0)
		for _, command := range e.commands {
			if query == "" || strings.Contains(strings.ToLower(command.Value), query) {
				items = append(items, command)
			}
		}
		return items
	}
	if strings.HasPrefix(prefix, "@") || force && strings.Contains(prefix, "/") {
		query := strings.TrimPrefix(prefix, "@")
		base := e.workDir
		pathQuery := query
		if index := strings.LastIndexAny(query, "/\\"); index >= 0 {
			base = filepath.Join(e.workDir, query[:index])
			pathQuery = query[index+1:]
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return nil
		}
		items := make([]Completion, 0, len(entries))
		for _, entry := range entries {
			if !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(pathQuery)) {
				continue
			}
			value := entry.Name()
			if index := strings.LastIndexAny(query, "/\\"); index >= 0 {
				value = query[:index+1] + value
			}
			if entry.IsDir() {
				value += "/"
			}
			if strings.HasPrefix(prefix, "@") {
				value = "@" + value
			}
			items = append(items, Completion{Value: value, Label: value})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
		if len(items) > 8 {
			items = items[:8]
		}
		return items
	}
	return nil
}

func (e *Editor) moveCompletion(delta int) {
	if len(e.completions.items) == 0 {
		return
	}
	e.completions.index = (e.completions.index + delta + len(e.completions.items)) % len(e.completions.items)
}

func (e *Editor) acceptCompletion() {
	if len(e.completions.items) == 0 {
		return
	}
	item := e.completions.items[e.completions.index]
	e.value = append(append(append([]rune(nil), e.value[:e.completions.start]...), []rune(item.Value)...), e.value[e.completions.end:]...)
	e.cursor = e.completions.start + len([]rune(item.Value))
	e.completions = completionState{}
}

func isPrintableInput(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func cellWidth(value string) int {
	width := 0
	for _, r := range value {
		if r == '\t' {
			width += 4
		} else if r >= 0x1100 && (r <= 0x115f || r >= 0x2e80) {
			width += 2
		} else {
			width++
		}
	}
	return width
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
