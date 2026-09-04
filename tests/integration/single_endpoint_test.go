package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/tests/testutil"
)

// data-model.md GatewayConfig.single_endpoint: with exactly one configured endpoint, mounting
// it at the gateway's root means every path (including "/") reaches its backend unchanged —
// no name-based subpath prefix required — while /_ discovery still works.
func TestSingleEndpoint_RoutesEveryPathUnprefixed(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: "app1", Backend: backend}}, testutil.WithRootRoute("app1"))

	for _, path := range []string{"/", "/orders/123"} {
		resp, err := http.Get(gw.URL + path)
		if err != nil {
			t.Fatalf("request to %s failed: %v", path, err)
		}
		resp.Body.Close()
		if gotPath != path {
			t.Fatalf("path %s: expected backend to receive it unchanged, got %q", path, gotPath)
		}
	}

	resp, err := http.Get(gw.URL + "/_")
	if err != nil {
		t.Fatalf("request to /_ failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/_: expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Endpoints []struct {
			Name string `json:"name"`
		} `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("/_: decoding body: %v", err)
	}
	if len(body.Endpoints) != 1 || body.Endpoints[0].Name != "app1" {
		t.Fatalf("/_: unexpected endpoints: %+v", body.Endpoints)
	}
}
