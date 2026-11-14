// Command gateway is the api-gateway entrypoint: a reverse proxy serving one or more named
// routes (each with its own destination and optional Python request/response handler chain),
// loaded from a YAML file. See specs/001-handler-injection/quickstart.md for usage.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/handler/workerpool"
	"api-gateway/internal/observability"
	"api-gateway/internal/proxy"
	"api-gateway/internal/router"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "api-gateway:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pools := make([]*workerpool.Pool, 0)
	routes := make([]router.Route, 0, len(cfg.Routes))
	for _, rc := range cfg.Routes {
		handlers := make([]handler.Handler, 0, len(rc.Handlers))
		for _, ref := range rc.Handlers {
			// Pools are scoped per (route, handler-file): looping per RouteConfig gives
			// each route its own *workerpool.Pool even if two routes reference the same
			// handler file, so a crash-prone handler on one route can't degrade another
			// route's supply of workers (research.md §5t, FR-006).
			pool, err := workerpool.New(rc.Name+"/"+ref.Name, ref.FilePath, pythonPathOption()...)
			if err != nil {
				return fmt.Errorf("route %q: configuring handler %q: %w", rc.Name, ref.Name, err)
			}
			if err := pool.Start(ctx); err != nil {
				stopAll(pools)
				return fmt.Errorf("route %q: starting handler %q: %w", rc.Name, ref.Name, err)
			}
			pools = append(pools, pool)
			handlers = append(handlers, handler.Handler{Ref: ref, Client: pool})
		}

		var proxyOpts []proxy.Option
		if rc.BackendTimeoutMS > 0 {
			proxyOpts = append(proxyOpts, proxy.WithTimeout(time.Duration(rc.BackendTimeoutMS)*time.Millisecond))
		}
		if len(rc.Headers) > 0 {
			proxyOpts = append(proxyOpts, proxy.WithHeaders(rc.Headers))
		}
		runner := handler.NewRunner(rc.Name, proxy.New(rc.Dest, proxyOpts...), handlers)
		routes = append(routes, router.Route{Config: rc, Handler: runner})
	}
	defer stopAll(pools)

	var routerOpts []router.Option
	if cfg.SingleEndpoint != "" {
		routerOpts = append(routerOpts, router.WithRootRoute(cfg.SingleEndpoint))
	}
	topRouter := router.New(routes, routerOpts...)

	var topHandler http.Handler = topRouter
	if cfg.Verbose {
		topHandler = observability.VerboseRequestLogger(topRouter)
	}

	addr := net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.ListenPort))
	server := &http.Server{Addr: addr, Handler: topHandler}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("api-gateway: listening on %s (%d route(s))", addr, len(cfg.Routes))
		for _, ip := range reachableIPs(cfg.ListenHost) {
			log.Printf("api-gateway: reachable at http://%s", net.JoinHostPort(ip, fmt.Sprintf("%d", cfg.ListenPort)))
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		log.Println("api-gateway: shutting down")
		return server.Shutdown(context.Background())
	}
}

func stopAll(pools []*workerpool.Pool) {
	for _, p := range pools {
		p.Stop()
	}
}

// pythonPathOption lets an operator override the Python interpreter used to launch workers
// (e.g. to point at a specific virtualenv) via GATEWAY_PYTHON, defaulting to "python3" on
// $PATH otherwise.
func pythonPathOption() []workerpool.Option {
	if p := os.Getenv("GATEWAY_PYTHON"); p != "" {
		return []workerpool.Option{workerpool.WithPythonPath(p)}
	}
	return nil
}

// reachableIPs returns the addresses the gateway can be reached at: the host itself if it
// was bound to a specific address, or every non-loopback IPv4 address of this machine when
// bound to a wildcard (0.0.0.0, ::, or empty).
func reachableIPs(host string) []string {
	if ip := net.ParseIP(host); host != "" && (ip == nil || !ip.IsUnspecified()) {
		return []string{host}
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var ips []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			ips = append(ips, ipn.IP.String())
		}
	}
	return ips
}
