// Package config resolves the gateway's entire configuration from CLI flags at process
// startup (FR-011): there is no config file and no runtime registration API. See
// specs/001-handler-injection/data-model.md for the GatewayConfig/HandlerRef shapes.
package config

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultHandlerTimeoutMS is used for a --handlers file when no per-handler override exists.
// CLI flags carry a single flat timeout for all handlers; see research.md §3.
const DefaultHandlerTimeoutMS = 100

// HandlerRef is one --handlers occurrence: a Python module the gateway loads at startup and
// calls for both the request and response phase of every request. A handler file that only
// implements one direction relies on the handlers-sdk's no-op default for the other (see
// handlers-sdk/gateway_handler_sdk/contracts.py).
type HandlerRef struct {
	Name      string
	FilePath  string
	TimeoutMS int
	Position  int
}

// GatewayConfig is the whole process's configuration. One instance per running gateway
// process: a process proxies to exactly one Dest. Multiple destinations are served by running
// multiple gateway processes, each with its own GatewayConfig (see plan.md, research.md §5).
type GatewayConfig struct {
	ListenHost string
	ListenPort int
	Dest       *url.URL
	Handlers   []HandlerRef
}

// handlerFlag collects repeated --handlers occurrences, preserving flag order as chain order
// (FR-005).
type handlerFlag struct {
	paths *[]string
}

func (h handlerFlag) String() string {
	if h.paths == nil {
		return ""
	}
	return strings.Join(*h.paths, ",")
}

func (h handlerFlag) Set(value string) error {
	if value == "" {
		return fmt.Errorf("--handlers value must not be empty")
	}
	*h.paths = append(*h.paths, value)
	return nil
}

// Parse resolves a GatewayConfig from CLI arguments (excluding the program name), failing
// with a descriptive error on a malformed --dest or an unloadable --handlers file (FR-011).
// It does not fail if --handlers is omitted entirely: a process with no handlers behaves as
// baseline pass-through proxying (FR-009).
func Parse(args []string) (*GatewayConfig, error) {
	fs := flag.NewFlagSet("api-gateway", flag.ContinueOnError)

	dest := fs.String("dest", "", "backend address this gateway proxies to (required)")
	host := fs.String("host", "", "host this gateway listens on (required)")
	port := fs.Int("port", 0, "port this gateway listens on (required)")

	var handlerPaths []string
	fs.Var(handlerFlag{paths: &handlerPaths}, "handlers",
		"path to a Python handler file; repeat to chain multiple, in order")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *dest == "" {
		return nil, fmt.Errorf("--dest is required")
	}
	destURL, err := url.Parse(*dest)
	if err != nil || destURL.Scheme == "" || destURL.Host == "" {
		return nil, fmt.Errorf("--dest %q is not a well-formed address", *dest)
	}

	if *host == "" {
		return nil, fmt.Errorf("--host is required")
	}
	if *port <= 0 || *port > 65535 {
		return nil, fmt.Errorf("--port %d is not a valid port", *port)
	}

	handlers := make([]HandlerRef, 0, len(handlerPaths))
	for i, p := range handlerPaths {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("--handlers %q could not be loaded: %w", p, err)
		}
		handlers = append(handlers, HandlerRef{
			Name:      handlerName(p),
			FilePath:  p,
			TimeoutMS: DefaultHandlerTimeoutMS,
			Position:  i,
		})
	}

	return &GatewayConfig{
		ListenHost: *host,
		ListenPort: *port,
		Dest:       destURL,
		Handlers:   handlers,
	}, nil
}

// handlerName derives a human-readable identifier from a handler file path for logs and error
// messages (data-model.md HandlerRef.name), e.g. "./handlers/v1.py" -> "v1".
func handlerName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
