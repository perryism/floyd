// Command gateway is the api-gateway entrypoint: a single-destination reverse proxy that
// runs a configurable chain of Python request/response handlers around every request. See
// specs/001-handler-injection/quickstart.md for usage.
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

	"api-gateway/internal/config"
	"api-gateway/internal/handler"
	"api-gateway/internal/handler/workerpool"
	"api-gateway/internal/proxy"
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

	pools := make([]*workerpool.Pool, 0, len(cfg.Handlers))
	handlers := make([]handler.Handler, 0, len(cfg.Handlers))
	for _, ref := range cfg.Handlers {
		pool, err := workerpool.New(ref.Name, ref.FilePath, pythonPathOption()...)
		if err != nil {
			return fmt.Errorf("configuring handler %q: %w", ref.Name, err)
		}
		if err := pool.Start(ctx); err != nil {
			stopAll(pools)
			return fmt.Errorf("starting handler %q: %w", ref.Name, err)
		}
		pools = append(pools, pool)
		handlers = append(handlers, handler.Handler{Ref: ref, Client: pool})
	}
	defer stopAll(pools)

	runner := handler.NewRunner(proxy.New(cfg.Dest), handlers)

	addr := net.JoinHostPort(cfg.ListenHost, fmt.Sprintf("%d", cfg.ListenPort))
	server := &http.Server{Addr: addr, Handler: runner}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("api-gateway: listening on %s, proxying to %s (%d handler(s))", addr, cfg.Dest, len(handlers))
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
