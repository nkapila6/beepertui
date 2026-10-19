package ui

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Vim normal mode for the compose box (the vim_compose setting). esc while
// writing lands here instead of leaving; esc again leaves. It's a subset:
//
//	motions   h l j k w b e 0 ^ $ gg G, with counts (3w)
//	operators d c y with any motion, dd cc yy, D C S s x X r~ J p P
//	insert    i a I A o O
//	undo      u, ctrl+r redo
//	enter     sends, as in insert
//
// Edits work on the text as runes with a cursor offset, then write it back
// into the textarea and put the cursor where it belongs.

type vimSnap struct {
	text string
	pos  int
}

type vimState struct {
	normal  bool
	pending string // operator or prefix waiting for the rest: d c y g r
	count   string
	undo    []vimSnap
	redo    []vimSnap
	reg     string
	regLine bool // register holds whole lines (dd/yy), pasted on their own line
}

const vimUndoMax = 100

// vimGet reads the compose box as runes plus the cursor offset into them.
func (m *Model) vimGet() ([]rune, int) {
	lines := strings.Split(m.compose.Value(), "\n")
	row, col := m.compose.Line(), m.compose.Column()
	pos := 0
	for i := 0; i < row && i < len(lines); i++ {
		pos += len([]rune(lines[i])) + 1
	}
	if row < len(lines) {
		pos += min(col, len([]rune(lines[row])))
	}
	return []rune(m.compose.Value()), pos
}

// vimSet writes runes back and moves the cursor to pos.
func (m *Model) vimSet(rs []rune, pos int) {
	m.compose.SetValue(string(rs))
	m.vimPlace(rs, pos)
}

func (m *Model) vimPlace(rs []rune, pos int) {
	pos = clamp(pos, 0, len(rs))
	row, col := 0, 0
	for _, r := range rs[:pos] {
		if r == '\n' {
			row, col = row+1, 0
		} else {
			col++
		}
	}
	m.compose.MoveToBegin()
	// CursorDown steps through wrapped rows too, so walk until the logical line matches
	for i := 0; m.compose.Line() < row && i < len(rs)+1; i++ {
		m.compose.CursorDown()
	}
	m.compose.SetCursorColumn(col)
}

func (m *Model) vimSave() {
	rs, pos := m.vimGet()
	m.vim.undo = append(m.vim.undo, vimSnap{string(rs), pos})
	if len(m.vim.undo) > vimUndoMax {
		m.vim.undo = m.vim.undo[1:]
	}
	m.vim.redo = nil
}

// enterVimNormal is esc from insert: the cursor steps back one, like vim.
func (m *Model) enterVimNormal() {
	m.vim.normal, m.vim.pending, m.vim.count = true, "", ""
	m.mention = nil
	rs, pos := m.vimGet()
	if pos > lineStart(rs, pos) {
		pos--
	}
	m.vimPlace(rs, pos)
}

func (m *Model) vimInsert(rs []rune, pos int) tea.Cmd {
	m.vim.normal = false
	m.vimPlace(rs, pos)
	return nil
}

func (m *Model) vimKey(key string) tea.Cmd {
	v := &m.vim
	rs, pos := m.vimGet()

	if v.pending == "r" {
		v.pending = ""
		if r := []rune(key); len(r) == 1 && pos < len(rs) && rs[pos] != '\n' {
			m.vimSave()
			rs[pos] = r[0]
			m.vimSet(rs, pos)
		}
		return nil
	}
	if key == "esc" {
		if v.pending != "" || v.count != "" {
			v.pending, v.count = "", ""
			return nil
		}
		v.normal = false
		m.mode = modeNormal
		m.compose.Blur()
		return nil
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' || key == "0" && v.count != "" {
		v.count += key
		return nil
	}
	n := 1
	if v.count != "" {
		n, _ = strconv.Atoi(v.count)
		v.count = ""
	}

	// an operator is waiting for its motion
	if op := v.pending; op == "d" || op == "c" || op == "y" {
		v.pending = ""
		if key == op { // dd cc yy work on whole lines
			return m.vimLines(op, rs, pos, n)
		}
		to, incl, ok := vimMotion(key, rs, pos, n)
		if !ok {
			return nil
		}
		a, b := min(pos, to), max(pos, to)
		if incl {
			b = min(b+1, len(rs))
		}
		// w/b/e stop at the end of the line rather than eating the newline
		if key == "w" || key == "e" || key == "b" {
			if nl := strings.IndexRune(string(rs[a:b]), '\n'); nl >= 0 {
				b = a + len([]rune(string(rs[a:b])[:nl]))
			}
		}
		if op == "c" && key == "w" && pos < len(rs) && runeClass(rs[pos]) != 0 {
			// cw on a word changes to its end and keeps the space after, like ce
			e := pos
			for i := 0; i < n; i++ {
				if i > 0 {
					if e = wordNext(rs, e); e >= len(rs) {
						e = len(rs) - 1
						break
					}
				}
				for c := runeClass(rs[e]); e+1 < len(rs) && runeClass(rs[e+1]) == c; e++ {
				}
			}
			a, b = pos, min(e+1, lineEnd(rs, pos))
		}
		v.reg, v.regLine = string(rs[a:b]), false
		if op == "y" {
			return nil
		}
		m.vimSave()
		rs = append(rs[:a:a], rs[b:]...)
		if op == "c" {
			m.compose.SetValue(string(rs))
			return m.vimInsert(rs, a)
		}
		m.vimSet(rs, clampNormal(rs, a))
		return nil
	}
	if v.pending == "g" {
		v.pending = ""
		if key == "g" {
			m.vimPlace(rs, 0)
		}
		return nil
	}

	if to, _, ok := vimMotion(key, rs, pos, n); ok {
		m.vimPlace(rs, clampNormal(rs, to))
		return nil
	}

	switch key {
	case "d", "c", "y", "g", "r":
		v.pending = key
	case "i":
		m.vimSave()
		return m.vimInsert(rs, pos)
	case "a":
		m.vimSave()
		return m.vimInsert(rs, min(pos+1, lineEnd(rs, pos)))
	case "I":
		m.vimSave()
		return m.vimInsert(rs, firstNonBlank(rs, pos))
	case "A":
		m.vimSave()
		return m.vimInsert(rs, lineEnd(rs, pos))
	case "o", "O":
		m.vimSave()
		at := lineEnd(rs, pos)
		if key == "O" {
			at = lineStart(rs, pos)
		}
		rs = append(rs[:at:at], append([]rune{'\n'}, rs[at:]...)...)
		m.compose.SetValue(string(rs))
		if key == "O" {
			return m.vimInsert(rs, at)
		}
		return m.vimInsert(rs, at+1)
	case "x", "X":
		a, b := pos, min(pos+n, lineEnd(rs, pos))
		if key == "X" {
			a, b = max(lineStart(rs, pos), pos-n), pos
		}
		if a < b {
			m.vimSave()
			v.reg, v.regLine = string(rs[a:b]), false
			rs = append(rs[:a:a], rs[b:]...)
			m.vimSet(rs, clampNormal(rs, a))
		}
	case "D", "C":
		m.vimSave()
		e := lineEnd(rs, pos)
		v.reg, v.regLine = string(rs[pos:e]), false
		rs = append(rs[:pos:pos], rs[e:]...)
		if key == "C" {
			m.compose.SetValue(string(rs))
			return m.vimInsert(rs, pos)
		}
		m.vimSet(rs, clampNormal(rs, pos))
	case "s":
		m.vimSave()
		e := min(pos+n, lineEnd(rs, pos))
		rs = append(rs[:pos:pos], rs[e:]...)
		m.compose.SetValue(string(rs))
		return m.vimInsert(rs, pos)
	case "S":
		return m.vimLines("c", rs, pos, n)
	case "~":
		e := min(pos+n, lineEnd(rs, pos))
		if pos < e {
			m.vimSave()
			for i := pos; i < e; i++ {
				if unicode.IsUpper(rs[i]) {
					rs[i] = unicode.ToLower(rs[i])
				} else {
					rs[i] = unicode.ToUpper(rs[i])
				}
			}
			m.vimSet(rs, clampNormal(rs, e))
		}
	case "J":
		if e := lineEnd(rs, pos); e < len(rs) {
			m.vimSave()
			next := e + 1
			for next < len(rs) && rs[next] == ' ' {
				next++
			}
			rs = append(append(rs[:e:e], ' '), rs[next:]...)
			m.vimSet(rs, e)
		}
	case "p", "P":
		if v.reg == "" {
			return nil
		}
		m.vimSave()
		paste := []rune(strings.Repeat(v.reg, n))
		var at, cur int
		switch {
		case v.regLine && key == "p":
			at = lineEnd(rs, pos)
			paste = append([]rune{'\n'}, paste...)
			if strings.HasSuffix(v.reg, "\n") {
				paste = paste[:len(paste)-1]
			}
			cur = at + 1
		case v.regLine:
			at, cur = lineStart(rs, pos), lineStart(rs, pos)
		case key == "p":
			at = min(pos+1, lineEnd(rs, pos))
			cur = at + len(paste) - 1
		default:
			at, cur = pos, pos+len(paste)-1
		}
		rs = append(rs[:at:at], append(paste, rs[at:]...)...)
		m.vimSet(rs, cur)
	case "u", "ctrl+r":
		from, to := &v.undo, &v.redo
		if key == "ctrl+r" {
			from, to = to, from
		}
		if len(*from) == 0 {
			return nil
		}
		s := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		*to = append(*to, vimSnap{string(rs), pos})
		r := []rune(s.text)
		m.vimSet(r, clampNormal(r, s.pos))
	case "enter":
		return m.submitCompose()
	}
	return nil
}

// vimLines is dd, cc and yy over n lines.
func (m *Model) vimLines(op string, rs []rune, pos, n int) tea.Cmd {
	a, b := lineStart(rs, pos), pos
	for i := 0; i < n; i++ {
		b = lineEnd(rs, b)
		if i < n-1 && b < len(rs) {
			b++
		}
	}
	m.vim.reg, m.vim.regLine = string(rs[a:b])+"\n", true
	if op == "y" {
		return nil
	}
	m.vimSave()
	if op == "c" {
		rs = append(rs[:a:a], rs[b:]...)
		m.compose.SetValue(string(rs))
		return m.vimInsert(rs, a)
	}
	// take the line break with it: the one after, or before on the last line
	switch {
	case b < len(rs):
		b++
	case a > 0:
		a--
	}
	rs = append(rs[:a:a], rs[b:]...)
	m.vimSet(rs, clampNormal(rs, firstNonBlank(rs, min(a, len(rs)))))
	return nil
}

// vimMotion moves from pos. incl says the target char belongs in an
// operator's range (e, $).
func vimMotion(key string, rs []rune, pos, n int) (to int, incl, ok bool) {
	to = pos
	switch key {
	case "h", "left":
		to = max(lineStart(rs, pos), pos-n)
	case "l", "right":
		to = min(pos+n, max(lineStart(rs, pos), lineEnd(rs, pos)-1))
	case "0":
		to = lineStart(rs, pos)
	case "^":
		to = firstNonBlank(rs, pos)
	case "$":
		to, incl = max(lineStart(rs, pos), lineEnd(rs, pos)-1), true
	case "w":
		for i := 0; i < n; i++ {
			to = wordNext(rs, to)
		}
	case "b":
		for i := 0; i < n; i++ {
			to = wordPrev(rs, to)
		}
	case "e":
		for i := 0; i < n; i++ {
			to = wordEnd(rs, to)
		}
		incl = true
	case "j", "down", "k", "up":
		col := pos - lineStart(rs, pos)
		for i := 0; i < n; i++ {
			if key == "j" || key == "down" {
				if e := lineEnd(rs, to); e < len(rs) {
					to = e + 1
				}
			} else if s := lineStart(rs, to); s > 0 {
				to = lineStart(rs, s-1)
			}
		}
		to = min(lineStart(rs, to)+col, lineEnd(rs, to))
	case "G":
		to = lineStart(rs, len(rs))
	default:
		return pos, false, false
	}
	return to, incl, true
}

func lineStart(rs []rune, pos int) int {
	for pos > 0 && rs[pos-1] != '\n' {
		pos--
	}
	return pos
}

func lineEnd(rs []rune, pos int) int {
	for pos < len(rs) && rs[pos] != '\n' {
		pos++
	}
	return pos
}

func firstNonBlank(rs []rune, pos int) int {
	p := lineStart(rs, pos)
	for p < len(rs) && (rs[p] == ' ' || rs[p] == '\t') {
		p++
	}
	return p
}

// clampNormal keeps the cursor on a character: normal mode can't sit past the end of a line.
func clampNormal(rs []rune, pos int) int {
	pos = clamp(pos, 0, len(rs))
	if pos == lineEnd(rs, pos) && pos > lineStart(rs, pos) {
		pos--
	}
	return pos
}

// runeClass sorts runes the way vim's w/b/e do: blanks, word chars, punctuation.
func runeClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	}
	return 2
}

func wordNext(rs []rune, pos int) int {
	if pos >= len(rs) {
		return len(rs)
	}
	c := runeClass(rs[pos])
	for pos < len(rs) && c != 0 && runeClass(rs[pos]) == c {
		pos++
	}
	for pos < len(rs) && runeClass(rs[pos]) == 0 {
		pos++
	}
	return pos
}

func wordPrev(rs []rune, pos int) int {
	for pos > 0 && runeClass(rs[pos-1]) == 0 {
		pos--
	}
	if pos == 0 {
		return 0
	}
	c := runeClass(rs[pos-1])
	for pos > 0 && runeClass(rs[pos-1]) == c {
		pos--
	}
	return pos
}

func wordEnd(rs []rune, pos int) int {
	pos++
	for pos < len(rs) && runeClass(rs[pos]) == 0 {
		pos++
	}
	if pos >= len(rs) {
		return max(0, len(rs)-1)
	}
	c := runeClass(rs[pos])
	for pos+1 < len(rs) && runeClass(rs[pos+1]) == c {
		pos++
	}
	return pos
}
