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
	"api-gateway/internal/router"
)

// TestRoute describes one route for NewTestGateway to wire up: a name (the path prefix
// requests must use, e.g. gw.URL+"/"+Name+"/..."), the stub backend it proxies to, and its
// optional handler chain.
type TestRoute struct {
	Name        string
	Description string
	Backend     *httptest.Server
	Handlers    []config.HandlerRef
}

// NewTestGateway wires up a real internal/router.Router — real worker-pool-supervised Python
// processes, one pool per (route, HandlerRef) pair — covering the given routes, and returns it
// as an httptest.Server. Pools run with a single worker (tests don't need concurrency) and are
// stopped automatically via t.Cleanup, same as the pools themselves.
func NewTestGateway(t *testing.T, routes []TestRoute) *httptest.Server {
	t.Helper()

	python := PythonExecutable(t)

	routerRoutes := make([]router.Route, 0, len(routes))
	for _, tr := range routes {
		destURL, err := url.Parse(tr.Backend.URL)
		if err != nil {
			t.Fatalf("parsing backend URL for route %q: %v", tr.Name, err)
		}

		handlers := make([]handler.Handler, 0, len(tr.Handlers))
		for _, ref := range tr.Handlers {
			pool, err := workerpool.New(tr.Name+"/"+ref.Name, ref.FilePath,
				workerpool.WithPythonPath(python), workerpool.WithSize(1))
			if err != nil {
				t.Fatalf("configuring pool for route %q handler %q: %v", tr.Name, ref.Name, err)
			}
			if err := pool.Start(context.Background()); err != nil {
				t.Fatalf("starting pool for route %q handler %q: %v", tr.Name, ref.Name, err)
			}
			t.Cleanup(pool.Stop)
			handlers = append(handlers, handler.Handler{Ref: ref, Client: pool})
		}

		runner := handler.NewRunner(tr.Name, proxy.New(destURL), handlers)
		routerRoutes = append(routerRoutes, router.Route{
			Config: config.RouteConfig{
				Name:        tr.Name,
				Description: tr.Description,
				Dest:        destURL,
				Handlers:    tr.Handlers,
			},
			Handler: runner,
		})
	}

	topRouter := router.New(routerRoutes)
	server := httptest.NewServer(topRouter)
	t.Cleanup(server.Close)
	return server
}
