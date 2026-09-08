package markdown

// Interactive task-list checkboxes: the rendered view's <input> elements map
// back to "[ ]" and "[x]" markers in the source. The N-th checkbox the renderer
// emits is the N-th TaskCheckBox node in the AST, because RenderHTML and this
// scan share gmRenderer's parser; a literal "- [ ]" in a code fence is neither
// and cannot shift the numbering.

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// taskMarkerOffsets returns each marker's "[" offset, in DOM checkbox order.
func taskMarkerOffsets(src []byte) []int {
	root := gmRenderer.Parser().Parse(gmtext.NewReader(src))
	var offs []int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*east.TaskCheckBox); !ok {
			return ast.WalkContinue, nil
		}
		// A TaskCheckBox is the first inline of its list item's first text
		// block, whose first line segment starts at the "[".
		p := n.Parent()
		if p == nil || p.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		if start := p.Lines().At(0).Start; isTaskMarker(src, start) {
			offs = append(offs, start)
		}
		return ast.WalkContinue, nil
	})
	return offs
}

// isTaskMarker checks the bytes before anything writes there, so segment
// arithmetic that drifts refuses instead of corrupting a document.
func isTaskMarker(src []byte, off int) bool {
	return off >= 0 && off+2 < len(src) &&
		src[off] == '[' && src[off+2] == ']' &&
		(src[off+1] == ' ' || src[off+1] == 'x' || src[off+1] == 'X')
}

// ToggleTask flips the index-th task checkbox in document order. src is never
// mutated and the output differs in exactly one byte, so every other byte
// stays as the user left it.
func ToggleTask(src []byte, index int) ([]byte, bool) {
	offs := taskMarkerOffsets(src)
	if index < 0 || index >= len(offs) {
		return nil, false
	}
	out := append([]byte(nil), src...)
	i := offs[index] + 1
	if out[i] == ' ' {
		out[i] = 'x'
	} else {
		out[i] = ' '
	}
	return out, true
}
