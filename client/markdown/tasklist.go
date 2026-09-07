package markdown

// Interactive task-list checkboxes: the rendered view's <input> elements map
// back to "[ ]" and "[x]" markers in the source, and toggling one edits the
// source through the normal text-edit door. The wasm overlay owns that wiring;
// this file owns the mapping.
//
// The N-th checkbox the renderer emits is the N-th TaskCheckBox node in the
// parsed AST, because RenderHTML and this scan share gmRenderer's parser
// configuration. A literal "- [ ]" inside a code fence is neither, so it
// cannot shift the numbering. TestToggleTaskRenderParity pins that.

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// taskMarkerOffsets returns the source byte offset of every task-list marker's
// "[", in the order the rendered view's checkboxes appear in the DOM.
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

// isTaskMarker verifies the bytes at off spell a task marker before anything
// writes there. If the AST's segment arithmetic drifts from the source, the
// toggle refuses instead of corrupting a document.
func isTaskMarker(src []byte, off int) bool {
	return off >= 0 && off+2 < len(src) &&
		src[off] == '[' && src[off+2] == ']' &&
		(src[off+1] == ' ' || src[off+1] == 'x' || src[off+1] == 'X')
}

// ToggleTask flips the index-th task checkbox in src, counting from 0 in
// document order, and returns (nil, false) when index addresses no checkbox.
// src is never mutated and the output differs in exactly one byte, so every
// other byte of the document stays as the user left it.
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
