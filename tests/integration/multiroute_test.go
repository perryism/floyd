package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/tests/testutil"
)

// Two routes, two distinct backends, one gateway process: each is reached independently at
// its own path prefix, with that prefix stripped before forwarding (research.md §5s).
func TestMultiRoute_IndependentDispatch(t *testing.T) {
	var gotPathA, gotPathB string
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPathA = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("A"))
	}))
	t.Cleanup(backendA.Close)
	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPathB = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("B"))
	}))
	t.Cleanup(backendB.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{
		{Name: "routea", Backend: backendA},
		{Name: "routeb", Backend: backendB},
	})

	respA, err := http.Get(gw.URL + "/routea/foo")
	if err != nil {
		t.Fatalf("request to routea failed: %v", err)
	}
	respA.Body.Close()
	if gotPathA != "/foo" {
		t.Fatalf("expected backendA to receive /foo, got %q", gotPathA)
	}

	respB, err := http.Get(gw.URL + "/routeb/bar")
	if err != nil {
		t.Fatalf("request to routeb failed: %v", err)
	}
	respB.Body.Close()
	if gotPathB != "/bar" {
		t.Fatalf("expected backendB to receive /bar, got %q", gotPathB)
	}
}

// contracts/discovery.md: GET /_ (and any other method) lists every configured route's name
// and description, in config order, regardless of method.
func TestMultiRoute_Discovery(t *testing.T) {
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backendA.Close)
	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backendB.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{
		{Name: "routea", Description: "first route", Backend: backendA},
		{Name: "routeb", Description: "second route", Backend: backendB},
	})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, gw.URL+"/_", nil)
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
		if len(body.Endpoints) != 2 ||
			body.Endpoints[0].Name != "routea" || body.Endpoints[0].Description != "first route" ||
			body.Endpoints[1].Name != "routeb" || body.Endpoints[1].Description != "second route" {
			t.Fatalf("%s /_: unexpected endpoints: %+v", method, body.Endpoints)
		}
	}
}

// research.md §5s: a request to an unconfigured first path segment, or to "/" itself, 404s
// with the standard error envelope.
func TestMultiRoute_RouteNotFound(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: testRouteName, Backend: backend}})

	for _, path := range []string{"/does-not-exist", "/"} {
		resp, err := http.Get(gw.URL + path)
		if err != nil {
			t.Fatalf("request to %s failed: %v", path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("path %s: expected 404, got %d", path, resp.StatusCode)
		}

		var body struct {
			Error struct {
				Code string `json:"code"`
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
	}
}
