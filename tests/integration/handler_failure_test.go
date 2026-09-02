package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

// US3 acceptance scenario 2: a handler that always fails makes the gateway fail closed with
// a well-defined error, and the gateway process itself keeps serving normally afterward —
// both for this instance's own subsequent requests, and (since this architecture serves
// multiple destinations by running one process per destination, per plan.md) for an
// unrelated gateway instance whose handler is healthy.
func TestHandlerFailure_IsolatesAndKeepsServing(t *testing.T) {
	backendCalled := false
	failingBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(failingBackend.Close)

	failingGW := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: failingBackend,
		Handlers: []config.HandlerRef{
			{Name: "always-fail", FilePath: "../fixtures/always_fail.py", TimeoutMS: 2000},
		},
	}})

	// An unrelated, healthy gateway instance (a different destination in this
	// architecture), running concurrently.
	healthyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthyBackend.Close)
	healthyGW := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: healthyBackend,
		Handlers: []config.HandlerRef{
			{Name: "add-header", FilePath: "../fixtures/add_header.py", TimeoutMS: 2000},
		},
	}})

	for i := 0; i < 2; i++ {
		resp, err := http.Get(failingGW.URL + "/" + testRouteName + "/anything")
		if err != nil {
			t.Fatalf("request %d to failing gateway errored at the transport level (process likely crashed): %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("request %d: expected 502 HANDLER_ERROR, got %d", i, resp.StatusCode)
		}
	}
	if backendCalled {
		t.Fatal("backend must not be called when a request handler errors")
	}

	healthyResp, err := http.Get(healthyGW.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("unrelated healthy gateway failed: %v", err)
	}
	defer healthyResp.Body.Close()
	if healthyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected the unrelated healthy gateway to be unaffected, got %d", healthyResp.StatusCode)
	}
}

// FR-012: a handler that exceeds its configured timeout gets HANDLER_TIMEOUT (504), distinct
// from HANDLER_ERROR (502) for an outright failure — both fail closed, but with a different
// code so operators can tell "too slow" apart from "broken" in logs/metrics.
func TestHandlerFailure_Timeout(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: backend,
		Handlers: []config.HandlerRef{
			{Name: "slow", FilePath: "../fixtures/slow.py", TimeoutMS: 100},
		},
	}})

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 HANDLER_TIMEOUT, got %d", resp.StatusCode)
	}
}
