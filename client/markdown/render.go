package markdown

import (
	"bytes"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/niklasfasching/go-org/org"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
	"html"

	"github.com/josephburnett/gridwell/internal/doctype"
)

// The read-only rendered view: source bytes to sanitized HTML, which the wasm
// overlay hands to a DOM div. The decision lives here, js-free and unit-tested.
// goldmark does markdown, go-org (Hugo's org engine) does org files, and
// bluemonday sanitizes both as defense in depth; goldmark is already safe by
// default, omitting raw HTML rather than passing it through.

// gmRenderer holds this package's one GFM configuration, so rendering for the
// overlay and the task-marker scan in tasklist.go read the same dialect.
var gmRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(renderer.WithNodeRenderers(
		util.Prioritized(taskCheckboxRenderer{}, 100),
	)),
)

// taskCheckboxRenderer emits GFM's task-list checkbox without `disabled`.
// Task-list checkboxes are the one interactive control in the otherwise
// read-only rendered view, and a disabled input swallows clicks. tasklist.go
// maps a click back to the source marker.
type taskCheckboxRenderer struct{}

func (taskCheckboxRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindTaskCheckBox, renderTaskCheckbox)
}

func renderTaskCheckbox(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if node.(*east.TaskCheckBox).IsChecked {
		_, _ = w.WriteString(`<input checked="" type="checkbox"> `)
	} else {
		_, _ = w.WriteString(`<input type="checkbox"> `)
	}
	return ast.WalkContinue, nil
}

// htmlPolicy is bluemonday's user-generated-content policy plus the class and
// id attributes go-org's output leans on and goldmark's task-list checkboxes.
var htmlPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "id").Globally()
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")
	return p
}()

// IsOrg reports whether a tile's name marks it as an org-mode document. A tile
// has no filename, so its user-visible name carries the type.
func IsOrg(name string) bool { return doctype.IsOrg(name) }

// Renderable reports whether a name's document type renders. It and IsOrg
// re-export internal/doctype, the neutral home both sides of the plugin seam
// import, so classification and this package's render pipeline cannot disagree.
func Renderable(name string) bool { return doctype.Renderable(name) }

// RenderHTML renders source bytes to sanitized HTML. isOrg selects the org-mode
// renderer, anything else is GFM markdown. Errors degrade to an escaped <pre>
// of the source, because a document must never render as nothing.
func RenderHTML(src []byte, isOrg bool) string {
	var out string
	if isOrg {
		w := org.NewHTMLWriter()
		doc := org.New().Parse(bytes.NewReader(src), "")
		s, err := doc.Write(w)
		if err != nil {
			return renderFallback(src)
		}
		out = s
	} else {
		var buf bytes.Buffer
		if err := gmRenderer.Convert(src, &buf); err != nil {
			return renderFallback(src)
		}
		out = buf.String()
	}
	return htmlPolicy.Sanitize(out)
}

// renderFallback degrades to the raw source, escaped, in a <pre>.
func renderFallback(src []byte) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return "<pre>" + esc.Replace(string(src)) + "</pre>"
}

// RenderPlainHTML presents a body whose owning plugin declares
// text_presentation "plain" verbatim in a preformatted block. Nothing is
// interpreted as markdown, so a shell comment cannot become a heading, and the
// source is escaped, so it is inert HTML.
func RenderPlainHTML(src []byte) string {
	return `<pre class="gw-plain" style="margin:0;white-space:pre-wrap;word-break:break-word;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:0.9em;">` +
		html.EscapeString(string(src)) + `</pre>`
}
