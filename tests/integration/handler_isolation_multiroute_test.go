package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

// research.md §5t: a worker pool is scoped per (route, handler-file) pair, not shared across
// routes even when they reference the same file. Proven deterministically here: each route
// gets its own single-worker pool (WithSize(1) inside testutil), and pid_header.py stamps the
// worker's own OS PID onto the request. If pools were incorrectly shared by file path alone,
// both routes would report the same PID; independent pools report different PIDs.
func TestHandlerIsolation_SameFileDifferentRoutesGetIndependentPools(t *testing.T) {
	var pidA, pidB string
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pidA = r.Header.Get("X-Worker-Pid")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backendA.Close)
	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pidB = r.Header.Get("X-Worker-Pid")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backendB.Close)

	sameHandler := []config.HandlerRef{
		{Name: "pid-header", FilePath: "../fixtures/pid_header.py", TimeoutMS: 2000},
	}
	gw := testutil.NewTestGateway(t, []testutil.TestRoute{
		{Name: "routea", Backend: backendA, Handlers: sameHandler},
		{Name: "routeb", Backend: backendB, Handlers: sameHandler},
	})

	respA, err := http.Get(gw.URL + "/routea/anything")
	if err != nil {
		t.Fatalf("request to routea failed: %v", err)
	}
	respA.Body.Close()
	respB, err := http.Get(gw.URL + "/routeb/anything")
	if err != nil {
		t.Fatalf("request to routeb failed: %v", err)
	}
	respB.Body.Close()

	if pidA == "" || pidB == "" {
		t.Fatalf("expected both backends to receive X-Worker-Pid, got routea=%q routeb=%q", pidA, pidB)
	}
	if pidA == pidB {
		t.Fatalf("routea and routeb got the same worker PID (%s) for the same handler file — "+
			"pools must be scoped per (route, file), not shared", pidA)
	}
}
