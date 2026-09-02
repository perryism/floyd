// Package router matches an inbound request to one of the gateway's configured routes by its
// first path segment, and serves the built-in discovery endpoint. See
// specs/001-handler-injection/research.md §5s (routing) and §5u (discovery).
package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"api-gateway/internal/config"
	"api-gateway/internal/observability"
)

// discoveryPath is the built-in, always-on endpoint listing every configured route. It is an
// exact match only — "_" is a reserved route name (config.Parse validates this), so no
// sub-path under it can ever collide with a real route.
const discoveryPath = "/_"

// Route pairs one configured route with the http.Handler that serves it (typically a
// *handler.Runner, but kept as the http.Handler interface here to avoid an import cycle and
// keep this package's contract minimal).
type Route struct {
	Config  config.RouteConfig
	Handler http.Handler
}

// Router is the gateway's top-level http.Handler.
type Router struct {
	byName map[string]Route
	routes []Route // preserves --config file order, for discovery
}

// New builds a Router from routes, in --config file order.
func New(routes []Route) *Router {
	byName := make(map[string]Route, len(routes))
	for _, rt := range routes {
		byName[rt.Config.Name] = rt
	}
	return &Router{byName: byName, routes: routes}
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == discoveryPath {
		rt.serveDiscovery(w)
		return
	}

	segment, rest := firstSegment(r.URL.Path)
	route, ok := rt.byName[segment]
	if !ok {
		observability.WriteError(w, observability.CodeRouteNotFound,
			fmt.Sprintf("no route configured for %q", segment), "", "")
		return
	}

	// Strip the matched segment before delegating, so the route's backend never needs to
	// know which prefix it happens to be mounted under (research.md §5s).
	forwarded := r.Clone(r.Context())
	forwarded.URL.Path = rest
	route.Handler.ServeHTTP(w, forwarded)
}

// firstSegment splits path into its first "/"-delimited segment and the remainder, which
// always begins with "/" (or is exactly "/" if nothing remains) — e.g. "/app1/foo" ->
// ("app1", "/foo"); "/app1" -> ("app1", "/"); "/" -> ("", "/").
func firstSegment(path string) (segment, rest string) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", "/"
	}
	if idx := strings.IndexByte(trimmed, '/'); idx >= 0 {
		return trimmed[:idx], trimmed[idx:]
	}
	return trimmed, "/"
}

type discoveryEndpoint struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type discoveryResponse struct {
	Endpoints []discoveryEndpoint `json:"endpoints"`
}

// serveDiscovery answers directly from the loaded routes, before any route matching or
// handler chain runs (contracts/discovery.md) — deliberately excludes dest/handlers.
func (rt *Router) serveDiscovery(w http.ResponseWriter) {
	endpoints := make([]discoveryEndpoint, 0, len(rt.routes))
	for _, rte := range rt.routes {
		endpoints = append(endpoints, discoveryEndpoint{
			Name:        rte.Config.Name,
			Description: rte.Config.Description,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(discoveryResponse{Endpoints: endpoints})
}
