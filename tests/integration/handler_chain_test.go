package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

// US3 acceptance scenario 1: two request handlers on the same process run in the order
// they're configured, with the second seeing the first's modification.
func TestHandlerChain_Order(t *testing.T) {
	var gotOrder string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrder = r.Header.Get("X-Order")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: backend,
		Handlers: []config.HandlerRef{
			{Name: "append-a", FilePath: "../fixtures/append_suffix_a.py", TimeoutMS: 2000},
			{Name: "append-b", FilePath: "../fixtures/append_suffix_b.py", TimeoutMS: 2000},
		},
	}})

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotOrder != "-A-B" {
		t.Fatalf("expected handlers to run in configured order (-A-B), got %q", gotOrder)
	}
}
