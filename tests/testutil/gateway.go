package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/handler/workerpool"
	"api-gateway/internal/observability"
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
	// RewritePaths mirrors the route's rewrite_paths config option.
	RewritePaths bool
}

// Option configures NewTestGateway beyond its routes, mirroring a top-level --config option
// from cmd/gateway/main.go.
type Option func(*options)

type options struct {
	verbose  bool
	rootName string
}

// WithVerbose wraps the test gateway with observability.VerboseRequestLogger, mirroring
// cmd/gateway/main.go's own wrapping when GatewayConfig.Verbose is true.
func WithVerbose() Option {
	return func(o *options) { o.verbose = true }
}

// WithRootRoute mounts the named route at the gateway's root, mirroring cmd/gateway/main.go's
// own router.WithRootRoute wiring when GatewayConfig.SingleEndpoint is set.
func WithRootRoute(name string) Option {
	return func(o *options) { o.rootName = name }
}

// NewTestGateway wires up a real internal/router.Router — real worker-pool-supervised Python
// processes, one pool per (route, HandlerRef) pair — covering the given routes, and returns it
// as an httptest.Server. Pools run with a single worker (tests don't need concurrency) and are
// stopped automatically via t.Cleanup, same as the pools themselves.
func NewTestGateway(t *testing.T, routes []TestRoute, opts ...Option) *httptest.Server {
	t.Helper()

	var o options
	for _, opt := range opts {
		opt(&o)
	}

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

		var fopts []proxy.Option
		if tr.RewritePaths {
			fopts = append(fopts, proxy.WithHeaders(map[string]string{"Accept-Encoding": "identity"}))
		}
		runner := handler.NewRunner(tr.Name, proxy.New(destURL, fopts...), handlers)
		if tr.RewritePaths {
			runner.WithPathRewrite("/"+tr.Name, destURL.Scheme+"://"+destURL.Host)
		}
		routerRoutes = append(routerRoutes, router.Route{
			Config: config.RouteConfig{
				Name:        tr.Name,
				Description: tr.Description,
				Dest:        destURL,
				Handlers:    tr.Handlers,

				RewritePaths: tr.RewritePaths,
			},
			Handler: runner,
		})
	}

	var routerOpts []router.Option
	if o.rootName != "" {
		routerOpts = append(routerOpts, router.WithRootRoute(o.rootName))
	}
	topRouter := router.New(routerRoutes, routerOpts...)
	var topHandler http.Handler = topRouter
	if o.verbose {
		topHandler = observability.VerboseRequestLogger(topRouter)
	}
	server := httptest.NewServer(topHandler)
	t.Cleanup(server.Close)
	return server
}
