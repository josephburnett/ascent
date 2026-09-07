// alttext.go derives alt-text for text tiles, so the store can auto-title one
// without importing the client tree. The parse dialect is GFM, matching the
// client renderer's parser, so the derived title agrees with the rendered view.

package doctype

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gmtext "github.com/yuin/goldmark/text"
)

// altParser parses with the same dialect the client renders, GFM.
var altParser = goldmark.New(goldmark.WithExtensions(extension.GFM))

// AltFromSource derives a short one-line alt-text from a markdown document: the
// plain text of the first block, markers stripped so "# Heading" becomes
// "Heading", whitespace runs collapsed to one space, clamped to altMaxLen runes.
// It returns "" for content-free input. The single line matters, since a
// code-block-first document would otherwise yield a multi-line alt.
func AltFromSource(src string) string {
	source := []byte(src)
	root := altParser.Parser().Parse(gmtext.NewReader(source))
	for b := root.FirstChild(); b != nil; b = b.NextSibling() {
		s := strings.Join(strings.Fields(blockPlainText(b, source)), " ")
		if s == "" {
			continue
		}
		return clampRunes(s, altMaxLen)
	}
	return ""
}

// blockPlainText is the concatenated plain text of one block-level AST node.
// Images and everything inside them are skipped.
func blockPlainText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			b.Write(t.Label(src))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(src))
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

const altMaxLen = 100

func clampRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
