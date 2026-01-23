package tui

import "unicode"

type TextBuffer struct {
	lines      []string
	row        int
	col        int
	desiredCol int
}

func NewTextBuffer() TextBuffer {
	return TextBuffer{lines: []string{""}}
}

func (b *TextBuffer) SetText(text string) {
	b.lines = splitLinesText(text)
	if len(b.lines) == 0 {
		b.lines = []string{""}
	}
	b.row = len(b.lines) - 1
	b.col = runeCount(b.lines[b.row])
	b.desiredCol = b.col
}

func (b *TextBuffer) Text() string {
	return joinLinesText(b.lines)
}

func (b *TextBuffer) InsertRune(r rune) {
	if r == '\n' {
		b.insertNewline()
		return
	}
	line := b.lineRunes(b.row)
	if b.col > len(line) {
		b.col = len(line)
	}
	line = append(line[:b.col], append([]rune{r}, line[b.col:]...)...)
	b.lines[b.row] = string(line)
	b.col++
	b.desiredCol = b.col
}

func (b *TextBuffer) Backspace() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	line := b.lineRunes(b.row)
	if b.col > 0 {
		line = append(line[:b.col-1], line[b.col:]...)
		b.lines[b.row] = string(line)
		b.col--
		b.desiredCol = b.col
		return
	}
	if b.row == 0 {
		return
	}
	prev := b.lineRunes(b.row - 1)
	b.col = len(prev)
	b.lines[b.row-1] = string(append(prev, line...))
	b.lines = append(b.lines[:b.row], b.lines[b.row+1:]...)
	b.row--
	b.desiredCol = b.col
}

func (b *TextBuffer) MoveLeft() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	if b.col > 0 {
		b.col--
		b.desiredCol = b.col
		return
	}
	if b.row == 0 {
		return
	}
	b.row--
	b.col = runeCount(b.lines[b.row])
	b.desiredCol = b.col
}

func (b *TextBuffer) MoveRight() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	line := b.lineRunes(b.row)
	if b.col < len(line) {
		b.col++
		b.desiredCol = b.col
		return
	}
	if b.row >= len(b.lines)-1 {
		return
	}
	b.row++
	b.col = 0
	b.desiredCol = b.col
}

func (b *TextBuffer) MoveUp() {
	if b.row <= 0 {
		return
	}
	b.row--
	lineLen := runeCount(b.lines[b.row])
	if b.desiredCol > lineLen {
		b.col = lineLen
	} else {
		b.col = b.desiredCol
	}
}

func (b *TextBuffer) MoveDown() {
	if b.row >= len(b.lines)-1 {
		return
	}
	b.row++
	lineLen := runeCount(b.lines[b.row])
	if b.desiredCol > lineLen {
		b.col = lineLen
	} else {
		b.col = b.desiredCol
	}
}

func (b *TextBuffer) MoveWordLeft() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	for {
		line := b.lineRunes(b.row)
		if b.col == 0 {
			if b.row == 0 {
				return
			}
			b.row--
			b.col = runeCount(b.lines[b.row])
			continue
		}
		i := b.col - 1
		for i > 0 && unicode.IsSpace(line[i]) {
			i--
		}
		for i > 0 && !unicode.IsSpace(line[i-1]) {
			i--
		}
		b.col = i
		b.desiredCol = b.col
		return
	}
}

func (b *TextBuffer) MoveWordRight() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	for {
		line := b.lineRunes(b.row)
		if b.col >= len(line) {
			if b.row >= len(b.lines)-1 {
				return
			}
			b.row++
			b.col = 0
			continue
		}
		i := b.col
		for i < len(line) && !unicode.IsSpace(line[i]) {
			i++
		}
		for i < len(line) && unicode.IsSpace(line[i]) {
			i++
		}
		b.col = i
		b.desiredCol = b.col
		return
	}
}

func (b *TextBuffer) DeleteWordLeft() {
	if b.row < 0 || b.row >= len(b.lines) {
		return
	}
	line := b.lineRunes(b.row)
	if b.col == 0 {
		b.Backspace()
		return
	}
	i := b.col - 1
	for i > 0 && unicode.IsSpace(line[i]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(line[i-1]) {
		i--
	}
	line = append(line[:i], line[b.col:]...)
	b.lines[b.row] = string(line)
	b.col = i
	b.desiredCol = b.col
}

func (b *TextBuffer) Render(height int) []string {
	if height <= 0 {
		return []string{""}
	}
	if len(b.lines) == 0 {
		b.lines = []string{""}
	}
	start := 0
	if b.row >= height {
		start = b.row - height + 1
	}
	if start+height > len(b.lines) {
		if len(b.lines) > height {
			start = len(b.lines) - height
		} else {
			start = 0
		}
	}
	end := start + height
	if end > len(b.lines) {
		end = len(b.lines)
	}
	lines := []string{}
	for i := start; i < end; i++ {
		line := b.lines[i]
		if i == b.row {
			line = insertCursor(line, b.col)
		}
		lines = append(lines, line)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func (b *TextBuffer) CursorPosition() (int, int) {
	return b.row + 1, b.col + 1
}

func (b *TextBuffer) lineRunes(row int) []rune {
	if row < 0 || row >= len(b.lines) {
		return []rune{}
	}
	return []rune(b.lines[row])
}

func (b *TextBuffer) insertNewline() {
	line := b.lineRunes(b.row)
	left := string(line[:b.col])
	right := string(line[b.col:])
	b.lines[b.row] = left
	insertAt := b.row + 1
	b.lines = append(b.lines[:insertAt], append([]string{right}, b.lines[insertAt:]...)...)
	b.row++
	b.col = 0
	b.desiredCol = b.col
}

func insertCursor(line string, col int) string {
	runes := []rune(line)
	if col < 0 {
		col = 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	runes = append(runes[:col], append([]rune{'|'}, runes[col:]...)...)
	return string(runes)
}

func splitLinesText(text string) []string {
	lines := []string{}
	current := ""
	for _, r := range text {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
			continue
		}
		current += string(r)
	}
	lines = append(lines, current)
	return lines
}

func joinLinesText(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for i := 1; i < len(lines); i++ {
		out += "\n" + lines[i]
	}
	return out
}

func runeCount(s string) int {
	return len([]rune(s))
}
