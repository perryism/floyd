package unit

import (
	"strings"
	"testing"

	"api-gateway/internal/rewrite"
)

func TestRewrite_HTML(t *testing.T) {
	in := `<html><head><link rel="stylesheet" href="/assets.css"><style>body{background:url(/bg.png)}</style>` +
		`<script>fetch("/api")</script></head><body style="background:url('/x.png')">` +
		`<img src="//cdn.x/y.png"><a href="page2">a</a><a href="/foo/already">b</a><a href="https://e.com/z">c</a>` +
		`<img srcset="/a.png 1x, /b.png 2x"><form action="/submit"></form></body></html>`
	got := string(rewrite.HTML("/foo", []byte(in)))
	for _, want := range []string{
		`href="/foo/assets.css"`, `url(/foo/bg.png)`, `fetch("/api")`, `url(&#39;/foo/x.png&#39;)`,
		`src="//cdn.x/y.png"`, `href="page2"`, `href="/foo/already"`, `href="https://e.com/z"`,
		`srcset="/foo/a.png 1x, /foo/b.png 2x"`, `action="/foo/submit"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/foo/foo/") {
		t.Errorf("double-prefixed:\n%s", got)
	}
}

func TestRewrite_CSS(t *testing.T) {
	got := string(rewrite.CSS("/foo", []byte(`@import "/base.css"; a{background:url("/i.png")} b{background:url(data:x)} c{background:url(rel.png)}`)))
	want := `@import "/foo/base.css"; a{background:url("/foo/i.png")} b{background:url(data:x)} c{background:url(rel.png)}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestRewrite_LocationAndCookie(t *testing.T) {
	cases := map[string]string{
		"/login":                      "/foo/login",
		"http://localhost:8080/login": "/foo/login",
		"http://localhost:8080":       "/foo/",
		"https://other.com/x":         "https://other.com/x",
		"/foo/login":                  "/foo/login",
	}
	for in, want := range cases {
		if got := rewrite.Location("/foo", "http://localhost:8080", in); got != want {
			t.Errorf("Location(%q) = %q, want %q", in, got, want)
		}
	}
	if got := rewrite.SetCookie("/foo", "sid=1; Path=/; HttpOnly"); got != "sid=1; Path=/foo/; HttpOnly" {
		t.Errorf("SetCookie = %q", got)
	}
}
