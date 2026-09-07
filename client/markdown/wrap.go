package markdown

// WrapRawLine wraps one raw source line into the visual rows the editing
// <textarea> produces. The canvas painter must agree with the textarea or the
// text visibly reflows when pane focus moves. Chromium's UA stylesheet gives a
// textarea `white-space: pre-wrap; overflow-wrap: break-word`, so for a
// monospace face where every rune is one column:
//   - a soft break happens before a word whose end would pass cols;
//   - spaces at a soft break hang past the edge, staying on the earlier row;
//   - a word wider than a whole row is char-broken at the column limit, but
//     only once it has a row to itself.
//
// cols <= 0 disables wrapping, for a caller with no measurable width.
func WrapRawLine(line string, cols int) []string {
	if cols <= 0 {
		return []string{line}
	}
	r := []rune(line)
	wordStart := func(j int) bool { return r[j] != ' ' && r[j-1] == ' ' }
	var out []string
	pos := 0
	for len(r)-pos > cols {
		// window is the first column that no longer fits.
		window := pos + cols
		cut := -1
		switch {
		case wordStart(window):
			cut = window
		case r[window] == ' ':
			// Inside a space run: the spaces hang, so the row extends to the
			// next word start or swallows the rest.
			for j := window + 1; j < len(r); j++ {
				if wordStart(j) {
					cut = j
					break
				}
			}
		default:
			// Inside a word: move the whole word down when it started after pos.
			// A word owning the row from its start is char-broken.
			cut = window
			for j := window - 1; j > pos; j-- {
				if wordStart(j) {
					cut = j
					break
				}
			}
		}
		if cut == -1 {
			break // only spaces remain past the window: one hanging row
		}
		out = append(out, string(r[pos:cut]))
		pos = cut
	}
	return append(out, string(r[pos:]))
}

// WrapRawText applies WrapRawLine to every newline-delimited source line and
// returns the flattened visual rows, one per slot the canvas paints.
func WrapRawText(src string, cols int) []string {
	var out []string
	start := 0
	for i := 0; i <= len(src); i++ {
		if i == len(src) || src[i] == '\n' {
			out = append(out, WrapRawLine(src[start:i], cols)...)
			start = i + 1
		}
	}
	return out
}
