// Package doctype owns the filename-to-document-type classifications both
// sides of the plugin seam read. It is importable from anywhere, so a
// server-side plugin never imports a client package; client/markdown
// re-exports.
package doctype

import "strings"

func IsOrg(name string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(name)), ".org")
}

// Renderable is the one rule, so the bytes fs serves and the tiles the client
// colors cannot disagree.
func Renderable(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(n, ".md") || strings.HasSuffix(n, ".markdown") || IsOrg(n)
}
