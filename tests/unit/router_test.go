package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/internal/router"
)

func TestRouter_MatchesFirstSegmentAndStripsIt(t *testing.T) {
	var gotPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	rt := router.New([]router.Route{{Config: config.RouteConfig{Name: "app1"}, Handler: stub}})
	srv := httptest.NewServer(rt)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/app1/foo/bar")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotPath != "/foo/bar" {
		t.Fatalf("expected the matched segment stripped to /foo/bar, got %q", gotPath)
	}
}

func TestRouter_ExactRouteNameStripsToRoot(t *testing.T) {
	var gotPath string
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	rt := router.New([]router.Route{{Config: config.RouteConfig{Name: "app1"}, Handler: stub}})
	srv := httptest.NewServer(rt)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/app1")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotPath != "/" {
		t.Fatalf("expected nothing-remains to strip to /, got %q", gotPath)
	}
}

func TestRouter_NoMatchReturnsRouteNotFound(t *testing.T) {
	rt := router.New([]router.Route{{Config: config.RouteConfig{Name: "app1"}, Handler: http.NotFoundHandler()}})
	srv := httptest.NewServer(rt)
	defer srv.Close()

	for _, path := range []string{"/does-not-exist", "/"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("request to %s failed: %v", path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("path %s: expected 404, got %d", path, resp.StatusCode)
		}

		var body struct {
			Error struct {
				Code    string `json:"code"`
				Route   string `json:"route"`
				Handler string `json:"handler"`
			} `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			t.Errorf("path %s: decoding body: %v", path, err)
			continue
		}
		if body.Error.Code != "ROUTE_NOT_FOUND" {
			t.Errorf("path %s: expected ROUTE_NOT_FOUND, got %q", path, body.Error.Code)
		}
		if body.Error.Route != "" || body.Error.Handler != "" {
			t.Errorf("path %s: expected route/handler omitted, got route=%q handler=%q",
				path, body.Error.Route, body.Error.Handler)
		}
	}
}

func TestRouter_DiscoveryTakesPrecedenceAndListsRoutes(t *testing.T) {
	rt := router.New([]router.Route{
		{Config: config.RouteConfig{Name: "app1", Description: "first"}, Handler: http.NotFoundHandler()},
		{Config: config.RouteConfig{Name: "app2", Description: "second"}, Handler: http.NotFoundHandler()},
	})
	srv := httptest.NewServer(rt)
	defer srv.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, srv.URL+"/_", nil)
		if err != nil {
			t.Fatalf("building %s request: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s /_ failed: %v", method, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s /_: expected 200, got %d", method, resp.StatusCode)
		}

		var body struct {
			Endpoints []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"endpoints"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("%s /_: decoding body: %v", method, err)
		}
		if len(body.Endpoints) != 2 || body.Endpoints[0].Name != "app1" || body.Endpoints[1].Name != "app2" {
			t.Fatalf("%s /_: unexpected endpoints: %+v", method, body.Endpoints)
		}
		if body.Endpoints[0].Description != "first" || body.Endpoints[1].Description != "second" {
			t.Fatalf("%s /_: unexpected descriptions: %+v", method, body.Endpoints)
		}
	}
}
