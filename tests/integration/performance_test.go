package integration

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"api-gateway/internal/config"
	"api-gateway/tests/testutil"
)

// T036 / Constitution Principle IV: baseline proxy overhead (no handlers) must stay within
// budget, and a configured handler must add no more than SC-004's 20ms p95. This is a
// local-machine sanity check, not a load test — it validates the budget is achievable in
// principle, not that it holds under production load/hardware.
const sampleCount = 30

func TestPerformance_BaselineProxyOverhead(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: testRouteName, Backend: backend}})
	client := gw.Client()

	backendLatency := measureP95(t, sampleCount, func() {
		resp, err := client.Get(backend.URL)
		if err != nil {
			t.Fatalf("direct backend request failed: %v", err)
		}
		resp.Body.Close()
	})
	gatewayLatency := measureP95(t, sampleCount, func() {
		resp, err := client.Get(gw.URL + "/" + testRouteName)
		if err != nil {
			t.Fatalf("gateway request failed: %v", err)
		}
		resp.Body.Close()
	})

	overhead := gatewayLatency - backendLatency
	t.Logf("backend p95=%s, gateway (no handlers) p95=%s, overhead=%s", backendLatency, gatewayLatency, overhead)
	if overhead > 5*time.Millisecond {
		t.Fatalf("baseline proxy overhead %s exceeds the 5ms p95 budget (plan.md Performance Goals)", overhead)
	}
}

func TestPerformance_PerHandlerOverhead(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	baseline := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: testRouteName, Backend: backend}})
	withHandler := testutil.NewTestGateway(t, []testutil.TestRoute{{
		Name:    testRouteName,
		Backend: backend,
		Handlers: []config.HandlerRef{
			{Name: "add-header", FilePath: "../fixtures/add_header.py", TimeoutMS: 2000},
		},
	}})

	baselineLatency := measureP95(t, sampleCount, func() {
		resp, err := http.Get(baseline.URL + "/" + testRouteName)
		if err != nil {
			t.Fatalf("baseline gateway request failed: %v", err)
		}
		resp.Body.Close()
	})
	handlerLatency := measureP95(t, sampleCount, func() {
		resp, err := http.Get(withHandler.URL + "/" + testRouteName)
		if err != nil {
			t.Fatalf("gateway-with-handler request failed: %v", err)
		}
		resp.Body.Close()
	})

	overhead := handlerLatency - baselineLatency
	t.Logf("baseline p95=%s, with 1 handler p95=%s, overhead=%s", baselineLatency, handlerLatency, overhead)
	if overhead > 20*time.Millisecond {
		t.Fatalf("per-handler overhead %s exceeds SC-004's 20ms p95 budget", overhead)
	}
}

func measureP95(t *testing.T, n int, call func()) time.Duration {
	t.Helper()
	durations := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		call()
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	idx := int(float64(n) * 0.95)
	if idx >= n {
		idx = n - 1
	}
	return durations[idx]
}
