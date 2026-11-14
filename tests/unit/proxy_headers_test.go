package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"api-gateway/internal/proxy"
)

func TestProxy_InjectsHeadersOverridingCaller(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	f := proxy.New(u, proxy.WithHeaders(map[string]string{"X_HEADER": "foo", "Api_Token": "tok"}))
	_, err := f.Forward(context.Background(), "GET", "/x", http.Header{"Api_Token": {"caller"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("X_HEADER") != "foo" || got.Get("Api_Token") != "tok" {
		t.Fatalf("unexpected headers: %v", got)
	}
}
