// Package integration exercises the real handler.Runner end-to-end: a real gateway HTTP
// server, real Python worker processes, and a stub backend.
package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

const testRouteName = "test"

func newEchoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Received-Header", r.Header.Get("X-Injected"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)
	return backend
}

// US1 acceptance scenario 1: a request handler adds a header; the backend receives it.
func TestRequestHandler_AddHeader(t *testing.T) {
	backend := newEchoBackend(t)
	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: backend,
		Handlers: []config.HandlerRef{
			{Name: "add-header", FilePath: "../fixtures/add_header.py", TimeoutMS: 2000},
		},
	}})

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Received-Header"); got != "yes" {
		t.Fatalf("backend did not receive injected header; echoed back %q", got)
	}
}

// US1 acceptance scenario 2: a request handler rejects a request missing a required header;
// the backend is never called and the caller gets the standard error envelope.
func TestRequestHandler_RejectMissingField(t *testing.T) {
	backendCalled := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: backend,
		Handlers: []config.HandlerRef{
			{Name: "reject-missing-field", FilePath: "../fixtures/reject_missing_field.py", TimeoutMS: 2000},
		},
	}})

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", resp.StatusCode, body)
	}
	if backendCalled {
		t.Fatal("backend must not be called when a request handler rejects the request")
	}
}
