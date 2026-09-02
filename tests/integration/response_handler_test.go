package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

func newJSONBackend(t *testing.T, body string) *httptest.Server {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(backend.Close)
	return backend
}

// US2 acceptance scenario 1: a response handler removes a field from the backend's JSON
// response; the caller doesn't see it, and the backend's own response is unaffected.
func TestResponseHandler_RemoveField(t *testing.T) {
	backend := newJSONBackend(t, `{"visible":"yes","secret":"shh"}`)
	gw := testutil.NewTestGateway(t, backend, []config.HandlerRef{
		{Name: "remove-field", FilePath: "../fixtures/remove_field.py", TimeoutMS: 2000},
	})

	resp, err := http.Get(gw.URL + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body=%s)", err, body)
	}
	if got["visible"] != "yes" {
		t.Fatalf("expected visible field preserved, got %v", got)
	}
	if _, present := got["secret"]; present {
		t.Fatalf("expected secret field removed, got %v", got)
	}
}

// US2 acceptance scenario 2: a response handler adds a header; the caller receives it.
func TestResponseHandler_AddHeader(t *testing.T) {
	backend := newJSONBackend(t, `{}`)
	gw := testutil.NewTestGateway(t, backend, []config.HandlerRef{
		{Name: "add-response-header", FilePath: "../fixtures/add_response_header.py", TimeoutMS: 2000},
	})

	resp, err := http.Get(gw.URL + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Cache"); got != "miss" {
		t.Fatalf("expected X-Cache=miss, got %q (headers=%v)", got, resp.Header)
	}
}
