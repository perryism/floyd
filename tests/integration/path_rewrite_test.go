package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api-gateway/tests/testutil"
)

// research.md §5v: a page served under /foo/ that references "/assets.css" must be rewritten
// to "/foo/assets.css", with redirects/cookies re-scoped and stray JS requests redirected via
// their Referer.
func TestPathRewrite_HTMLRedirectCookieAndRefererFallback(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Set-Cookie", "sid=1; Path=/; HttpOnly")
			_, _ = io.WriteString(w, `<link rel="stylesheet" href="/assets.css"><img src="//cdn/x.png">`)
		case "/login":
			http.Redirect(w, r, "/home", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`"}`)
		}
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{
		{Name: "foo", Backend: backend, RewritePaths: true},
		{Name: "plain", Backend: backend},
	})
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	resp, err := noFollow.Get(gw.URL + "/foo/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `href="/foo/assets.css"`) || !strings.Contains(string(body), `src="//cdn/x.png"`) {
		t.Fatalf("html not rewritten as expected: %s", body)
	}
	if got := resp.Header.Get("Set-Cookie"); !strings.Contains(got, "Path=/foo/") {
		t.Fatalf("cookie path not rewritten: %q", got)
	}

	resp, _ = noFollow.Get(gw.URL + "/foo/login")
	resp.Body.Close()
	if got := resp.Header.Get("Location"); got != "/foo/home" {
		t.Fatalf("Location = %q, want /foo/home", got)
	}

	// Route without rewrite_paths is untouched.
	resp, _ = noFollow.Get(gw.URL + "/plain/")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `href="/assets.css"`) {
		t.Fatalf("plain route should not be rewritten: %s", body)
	}

	// Referer fallback: JS-initiated /api/items?x=1 from a /foo/ page.
	req, _ := http.NewRequest("GET", gw.URL+"/api/items?x=1", nil)
	req.Header.Set("Referer", gw.URL+"/foo/dashboard")
	resp, _ = noFollow.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect || resp.Header.Get("Location") != "/foo/api/items?x=1" {
		t.Fatalf("expected 307 to /foo/api/items?x=1, got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// No Referer, or Referer under a non-rewrite route: still 404.
	for _, ref := range []string{"", gw.URL + "/plain/x"} {
		req, _ = http.NewRequest("GET", gw.URL+"/api/items", nil)
		if ref != "" {
			req.Header.Set("Referer", ref)
		}
		resp, _ = noFollow.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("referer %q: expected 404, got %d", ref, resp.StatusCode)
		}
	}
}
