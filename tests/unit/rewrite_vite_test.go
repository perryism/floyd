package unit

import (
	"strings"
	"testing"

	"api-gateway/internal/rewrite"
)

// A typical Vite build output.
func TestRewrite_ViteModuleScript(t *testing.T) {
	in := `<!doctype html><html><head><meta charset="UTF-8"><link rel="icon" href="/vite.svg">` +
		`<script type="module" crossorigin src="/assets/index-DMqM5t1_.js"></script>` +
		`<link rel="stylesheet" crossorigin href="/assets/index-abc.css"></head><body><div id="root"></div></body></html>`
	got := string(rewrite.HTML("/foo", []byte(in)))
	for _, want := range []string{
		`src="/foo/assets/index-DMqM5t1_.js"`, `href="/foo/assets/index-abc.css"`, `href="/foo/vite.svg"`,
		`type="module"`, `crossorigin`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
