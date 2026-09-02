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
}

// GatewayConfig is the whole process's configuration: --host/--port plus the routes loaded
// from --config. One instance per running gateway process; a process now serves as many
// routes as its config file lists (previously exactly one — see research.md §5/§5r).
type GatewayConfig struct {
	ListenHost string
	ListenPort int
	Routes     []RouteConfig
}

// yamlRoute mirrors one route entry in the --config YAML file, before validation/conversion
// into RouteConfig.
type yamlRoute struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Dest        string        `yaml:"dest"`
	Handlers    handlersField `yaml:"handlers"`
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

	routes, err := loadRoutes(*configPath)
	if err != nil {
		return nil, err
	}

	return &GatewayConfig{
		ListenHost: *host,
		ListenPort: *port,
		Routes:     routes,
	}, nil
}

// loadRoutes reads and validates the --config YAML file into a list of RouteConfig.
func loadRoutes(path string) ([]RouteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading --config %q: %w", path, err)
	}

	var raw []yamlRoute
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing --config %q: %w", path, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("--config %q defines no routes", path)
	}

	seen := make(map[string]bool, len(raw))
	routes := make([]RouteConfig, 0, len(raw))
	for _, r := range raw {
		if err := validateRouteName(r.Name, seen); err != nil {
			return nil, fmt.Errorf("%q: %w", path, err)
		}
		seen[r.Name] = true

		destURL, err := url.Parse(r.Dest)
		if err != nil || destURL.Scheme == "" || destURL.Host == "" {
			return nil, fmt.Errorf("route %q: dest %q is not a well-formed address", r.Name, r.Dest)
		}

		handlers := make([]HandlerRef, 0, len(r.Handlers))
		for i, p := range r.Handlers {
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf("route %q: handlers %q could not be loaded: %w", r.Name, p, err)
			}
			handlers = append(handlers, HandlerRef{
				Name:      handlerName(p),
				FilePath:  p,
				TimeoutMS: DefaultHandlerTimeoutMS,
				Position:  i,
			})
		}

		routes = append(routes, RouteConfig{
			Name:        r.Name,
			Description: r.Description,
			Dest:        destURL,
			Handlers:    handlers,
		})
	}

	return routes, nil
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
