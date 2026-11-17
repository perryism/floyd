// Package config resolves the gateway's entire configuration from CLI flags and a routes
// YAML file at process startup (FR-011): there is no runtime registration API. See
// specs/001-handler-injection/data-model.md for the GatewayConfig/RouteConfig/HandlerRef
// shapes, and research.md §5r for why this superseded the earlier CLI-flags-only design.
package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultHandlerTimeoutMS is used for a handler when no per-handler override exists.
// The routes config carries no per-handler timeout override yet; see research.md §3.
const DefaultHandlerTimeoutMS = 100

// discoveryName is reserved: it's the built-in discovery endpoint's path, not a route
// (research.md §5u).
const discoveryName = "_"

// HandlerRef is one entry in a route's ordered handler list: a Python module the gateway
// loads at startup and calls for both the request and response phase of every request that
// route handles. A handler file that only implements one direction relies on the
// handlers-sdk's no-op default for the other (see handlers-sdk/gateway_handler_sdk/contracts.py).
type HandlerRef struct {
	Name      string
	FilePath  string
	TimeoutMS int
	Position  int
}

// RouteConfig is one named route from the --config YAML file: a destination with its own
// optional handler chain. Name is also the first path segment inbound requests are matched
// against (see internal/router).
type RouteConfig struct {
	Name        string
	Description string
	Dest        *url.URL
	Handlers    []HandlerRef
	// BackendTimeoutMS is how long the gateway waits for this route's backend to respond
	// before giving up, in milliseconds. Zero means "use proxy.DefaultTimeout" — routes
	// with unusually slow backends (e.g. an LLM API without streaming) can override it here
	// rather than everyone getting stuck with one fixed global value.
	BackendTimeoutMS int
	// Headers are injected into every request forwarded to this route's backend, overriding
	// any same-named header from the caller or a request handler. Values from the YAML may
	// reference environment variables as ${NAME}; they are expanded at startup.
	Headers map[string]string
	// RewritePaths re-prefixes root-relative URLs in this route's HTML/CSS responses (and
	// Location/Set-Cookie headers) with "/<name>" so pages that reference "/assets.css" work
	// when mounted under /<name>/ (research.md §5v).
	RewritePaths bool
}

// GatewayConfig is the whole process's configuration: --host/--port plus the routes and
// top-level options loaded from --config. One instance per running gateway process; a process
// now serves as many routes as its config file lists (previously exactly one — see
// research.md §5/§5r).
type GatewayConfig struct {
	ListenHost     string
	ListenPort     int
	Routes         []RouteConfig
	Verbose        bool
	SingleEndpoint string
}

// yamlConfig mirrors the --config YAML file's top-level shape: top-level options alongside the
// "endpoints" list, so more top-level keys can be added later without changing the file's
// shape from a bare list to a map more than once (contracts/discovery.md already uses
// "endpoints" for the same data in the /_ response, so this aligns the two).
type yamlConfig struct {
	Verbose        bool        `yaml:"verbose"`
	SingleEndpoint string      `yaml:"single_endpoint"`
	Endpoints      []yamlRoute `yaml:"endpoints"`
}

// yamlRoute mirrors one route entry in the --config YAML file, before validation/conversion
// into RouteConfig.
type yamlRoute struct {
	Name             string            `yaml:"name"`
	Description      string            `yaml:"description"`
	Dest             string            `yaml:"dest"`
	Handlers         handlersField     `yaml:"handlers"`
	BackendTimeoutMS int               `yaml:"backend_timeout_ms"`
	Headers          map[string]string `yaml:"headers"`
	RewritePaths     bool              `yaml:"rewrite_paths"`
}

// handlersField accepts a route's "handlers" YAML value as either one scalar path or a list
// of paths, so `handlers: ./a.py` and `handlers: [./a.py, ./b.py]` both work (research.md §5r).
type handlersField []string

func (h *handlersField) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		if s == "" {
			*h = nil
			return nil
		}
		*h = handlersField{s}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*h = handlersField(list)
		return nil
	default:
		return fmt.Errorf("handlers must be a string or a list of strings")
	}
}

// Parse resolves a GatewayConfig from CLI arguments (excluding the program name), failing
// with a descriptive error on any invalid --host/--port/--config or a malformed route
// (FR-011). The gateway fails to start rather than silently running with a misconfigured or
// missing route.
func Parse(args []string) (*GatewayConfig, error) {
	fs := flag.NewFlagSet("api-gateway", flag.ContinueOnError)

	host := fs.String("host", "", "host this gateway listens on (required)")
	port := fs.Int("port", 0, "port this gateway listens on (required)")
	configPath := fs.String("config", "", "path to the routes YAML config file (required)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *host == "" {
		return nil, fmt.Errorf("--host is required")
	}
	if *port <= 0 || *port > 65535 {
		return nil, fmt.Errorf("--port %d is not a valid port", *port)
	}
	if *configPath == "" {
		return nil, fmt.Errorf("--config is required")
	}

	routes, raw, err := loadConfig(*configPath)
	if err != nil {
		return nil, err
	}

	return &GatewayConfig{
		ListenHost:     *host,
		ListenPort:     *port,
		Routes:         routes,
		Verbose:        raw.Verbose,
		SingleEndpoint: raw.SingleEndpoint,
	}, nil
}

// loadConfig reads and validates the --config YAML file into a list of RouteConfig plus the
// decoded top-level options (yamlConfig, so Parse can read Verbose/SingleEndpoint/... without
// this return signature growing every time a new top-level option is added).
func loadConfig(path string) ([]RouteConfig, yamlConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, yamlConfig{}, fmt.Errorf("reading --config %q: %w", path, err)
	}

	var raw yamlConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, yamlConfig{}, fmt.Errorf("parsing --config %q: %w", path, err)
	}
	if len(raw.Endpoints) == 0 {
		return nil, yamlConfig{}, fmt.Errorf("--config %q defines no endpoints", path)
	}

	seen := make(map[string]bool, len(raw.Endpoints))
	routes := make([]RouteConfig, 0, len(raw.Endpoints))
	for _, r := range raw.Endpoints {
		if err := validateRouteName(r.Name, seen); err != nil {
			return nil, yamlConfig{}, fmt.Errorf("%q: %w", path, err)
		}
		seen[r.Name] = true

		destURL, err := url.Parse(r.Dest)
		if err != nil || destURL.Scheme == "" || destURL.Host == "" {
			return nil, yamlConfig{}, fmt.Errorf("route %q: dest %q is not a well-formed address", r.Name, r.Dest)
		}

		handlers := make([]HandlerRef, 0, len(r.Handlers))
		for i, p := range r.Handlers {
			if _, err := os.Stat(p); err != nil {
				return nil, yamlConfig{}, fmt.Errorf("route %q: handlers %q could not be loaded: %w", r.Name, p, err)
			}
			handlers = append(handlers, HandlerRef{
				Name:      handlerName(p),
				FilePath:  p,
				TimeoutMS: DefaultHandlerTimeoutMS,
				Position:  i,
			})
		}

		headers, err := expandHeaders(r.Headers)
		if err != nil {
			return nil, yamlConfig{}, fmt.Errorf("route %q: %w", r.Name, err)
		}

		routes = append(routes, RouteConfig{
			Name:             r.Name,
			Description:      r.Description,
			Dest:             destURL,
			Handlers:         handlers,
			BackendTimeoutMS: r.BackendTimeoutMS,
			Headers:          headers,
			RewritePaths:     r.RewritePaths,
		})
	}

	if raw.SingleEndpoint != "" {
		if len(routes) != 1 {
			return nil, yamlConfig{}, fmt.Errorf(
				"%q: single_endpoint %q is only valid when exactly one endpoint is configured, got %d",
				path, raw.SingleEndpoint, len(routes))
		}
		if routes[0].RewritePaths {
			return nil, yamlConfig{}, fmt.Errorf(
				"%q: rewrite_paths has no effect with single_endpoint (the route is mounted at /)", path)
		}
		if raw.SingleEndpoint != routes[0].Name {
			return nil, yamlConfig{}, fmt.Errorf(
				"%q: single_endpoint %q does not match the configured endpoint's name %q",
				path, raw.SingleEndpoint, routes[0].Name)
		}
	}

	return routes, raw, nil
}

// expandHeaders resolves ${VAR} references in header values from the process environment.
// An unset variable is a startup error, so a missing secret can't silently produce a request
// with an empty credential.
func expandHeaders(in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for name, val := range in {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("headers: empty header name")
		}
		var missing string
		expanded := os.Expand(val, func(key string) string {
			v, ok := os.LookupEnv(key)
			if !ok && missing == "" {
				missing = key
			}
			return v
		})
		if missing != "" {
			return nil, fmt.Errorf("header %q references unset environment variable %q", name, missing)
		}
		out[name] = expanded
	}
	return out, nil
}

func validateRouteName(name string, seen map[string]bool) error {
	if name == "" {
		return fmt.Errorf("a route has an empty name")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("route name %q must not contain '/'", name)
	}
	if name == discoveryName {
		return fmt.Errorf("route name %q is reserved for the discovery endpoint", name)
	}
	if seen[name] {
		return fmt.Errorf("duplicate route name %q", name)
	}
	return nil
}

// handlerName derives a human-readable identifier from a handler file path for logs and error
// messages (data-model.md HandlerRef.name), e.g. "./handlers/v1.py" -> "v1".
func handlerName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
