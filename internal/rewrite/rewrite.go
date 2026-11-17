// Package rewrite re-prefixes root-relative URLs (e.g. "/assets.css") in proxied responses so a
// backend mounted under "/<route>/" on the gateway keeps working. See
// specs/001-handler-injection/research.md §5v and contracts/path-rewriting.md.
package rewrite

import (
	"bytes"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// MaxBodyBytes is the largest body that will be rewritten; larger ones pass through untouched.
const MaxBodyBytes = 10 << 20

// URL prefixes a single URL with prefix (e.g. "/foo") iff it is root-relative: it starts with
// exactly one "/" and isn't already under prefix. Everything else (absolute, protocol-relative,
// data:, fragments, relative paths) is returned unchanged.
func URL(prefix, u string) string {
	t := strings.TrimSpace(u)
	if !strings.HasPrefix(t, "/") || strings.HasPrefix(t, "//") {
		return u
	}
	if t == prefix || strings.HasPrefix(t, prefix+"/") || strings.HasPrefix(t, prefix+"?") {
		return u
	}
	return prefix + t
}

var (
	cssURL    = regexp.MustCompile(`(?i)url\(\s*(["']?)([^)"']*)(["']?)\s*\)`)
	cssImport = regexp.MustCompile(`(?i)@import\s+(["'])([^"']*)(["'])`)
)

// CSS rewrites url(...) and @import "..." references in a stylesheet.
func CSS(prefix string, css []byte) []byte {
	out := cssURL.ReplaceAllFunc(css, func(m []byte) []byte {
		g := cssURL.FindSubmatch(m)
		return []byte("url(" + string(g[1]) + URL(prefix, string(g[2])) + string(g[3]) + ")")
	})
	return cssImport.ReplaceAllFunc(out, func(m []byte) []byte {
		g := cssImport.FindSubmatch(m)
		return []byte("@import " + string(g[1]) + URL(prefix, string(g[2])) + string(g[3]))
	})
}

var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true, "poster": true, "data": true,
}

func srcset(prefix, v string) string {
	parts := strings.Split(v, ",")
	for i, p := range parts {
		f := strings.Fields(p)
		if len(f) == 0 {
			continue
		}
		f[0] = URL(prefix, f[0])
		parts[i] = strings.Join(f, " ")
	}
	return strings.Join(parts, ", ")
}

// HTML rewrites URL-valued attributes, inline <style> blocks and style="" attributes. Inline
// <script> content is deliberately left alone.
func HTML(prefix string, doc []byte) []byte {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var out bytes.Buffer
	inStyle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			// io.EOF (normal) or a malformed tail: emit what's left verbatim.
			out.Write(z.Raw())
			return out.Bytes()
		}
		raw := append([]byte(nil), z.Raw()...)
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			changed := false
			for i, a := range t.Attr {
				if a.Namespace != "" {
					continue
				}
				nv := a.Val
				switch {
				case urlAttrs[a.Key]:
					nv = URL(prefix, a.Val)
				case a.Key == "srcset":
					nv = srcset(prefix, a.Val)
				case a.Key == "style":
					nv = string(CSS(prefix, []byte(a.Val)))
				}
				if nv != a.Val {
					t.Attr[i].Val = nv
					changed = true
				}
			}
			inStyle = t.Data == "style" && tt == html.StartTagToken
			if changed {
				out.WriteString(t.String())
			} else {
				out.Write(raw)
			}
		case html.TextToken:
			if inStyle {
				out.Write(CSS(prefix, raw))
			} else {
				out.Write(raw)
			}
		default:
			if tt == html.EndTagToken {
				inStyle = false
			}
			out.Write(raw)
		}
	}
}

// Location rewrites a redirect target: root-relative paths, and absolute URLs on destOrigin
// (e.g. "http://localhost:8080"), are mapped under prefix. Other origins are untouched.
func Location(prefix, destOrigin, loc string) string {
	if destOrigin != "" && strings.HasPrefix(loc, destOrigin) {
		rest := strings.TrimPrefix(loc, destOrigin)
		if rest == "" || rest[0] == '/' || rest[0] == '?' {
			if rest == "" || rest[0] == '?' {
				rest = "/" + rest
			}
			return URL(prefix, rest)
		}
		return loc
	}
	return URL(prefix, loc)
}

var cookiePath = regexp.MustCompile(`(?i)(;\s*path=)(/[^;,]*)`)

// SetCookie rewrites the Path attribute of a Set-Cookie value (or several joined with ", " /
// newlines) so the cookie is scoped to the route's prefix.
func SetCookie(prefix, v string) string {
	return cookiePath.ReplaceAllStringFunc(v, func(m string) string {
		g := cookiePath.FindStringSubmatch(m)
		return g[1] + URL(prefix, g[2])
	})
}
