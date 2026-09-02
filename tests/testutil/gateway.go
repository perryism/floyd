package testutil

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/handler/workerpool"
	"api-gateway/internal/proxy"
)

// NewTestGateway wires up a real handler.Runner (real worker-pool-supervised Python
// processes, one pool per HandlerRef) in front of backend, and returns it as an
// httptest.Server. Pools run with a single worker (tests don't need concurrency) and are
// stopped automatically via t.Cleanup.
func NewTestGateway(t *testing.T, backend *httptest.Server, refs []config.HandlerRef) *httptest.Server {
	t.Helper()

	python := PythonExecutable(t)
	dest, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("parsing backend URL: %v", err)
	}

	handlers := make([]handler.Handler, 0, len(refs))
	for _, ref := range refs {
		pool, err := workerpool.New(ref.Name, ref.FilePath,
			workerpool.WithPythonPath(python), workerpool.WithSize(1))
		if err != nil {
			t.Fatalf("configuring pool for %q: %v", ref.Name, err)
		}
		if err := pool.Start(context.Background()); err != nil {
			t.Fatalf("starting pool for %q: %v", ref.Name, err)
		}
		t.Cleanup(pool.Stop)
		handlers = append(handlers, handler.Handler{Ref: ref, Client: pool})
	}

	runner := handler.NewRunner(proxy.New(dest), handlers)
	server := httptest.NewServer(runner)
	t.Cleanup(server.Close)
	return server
}
