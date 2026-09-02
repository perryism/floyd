// Package workerpool supervises a bounded pool of long-lived Python worker processes for one
// --handlers file, calling into them over gRPC on a local Unix domain socket. See
// specs/001-handler-injection/research.md §§1-3 for the design rationale.
package workerpool

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	handlerv1 "api-gateway/api/handler/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// maxConsecutiveCrashes is the reasonable default after which a worker slot stops
// auto-restarting and is marked Degraded instead (data-model.md HandlerWorkerPool state
// transitions).
const maxConsecutiveCrashes = 5

// startupTimeout bounds how long Pool.Start waits for each worker's socket to become
// connectable before treating the handler file as unloadable (FR-011).
const startupTimeout = 10 * time.Second

type state int

const (
	stateStarting state = iota
	stateReady
	stateBusy
	stateCrashed
	stateRestarting
	stateDegraded
)

// PoolSize returns the default worker-pool size: the number of CPU cores available to the
// gateway process (research.md §2).
func PoolSize() int {
	if n := runtime.NumCPU(); n > 0 {
		return n
	}
	return 1
}

// Pool supervises one handler file's fixed-size worker pool.
type Pool struct {
	name       string
	handlerAbs string
	pythonPath string
	size       int

	mu      sync.Mutex
	workers []*worker
	next    atomic.Uint64
}

type worker struct {
	mu         sync.Mutex
	state      state
	sockPath   string
	cmd        *exec.Cmd
	conn       *grpc.ClientConn
	client     handlerv1.HandlerClient
	crashCount int
}

// Option customizes Pool construction; used by tests to point at a specific Python
// interpreter (e.g. a project venv) instead of relying on $PATH.
type Option func(*Pool)

// WithPythonPath overrides the Python interpreter used to launch workers.
func WithPythonPath(path string) Option {
	return func(p *Pool) { p.pythonPath = path }
}

// WithSize overrides the default (CPU-core-count) pool size.
func WithSize(size int) Option {
	return func(p *Pool) { p.size = size }
}

// New constructs a Pool for the handler file at handlerFilePath, but does not start any
// workers yet — call Start.
func New(name, handlerFilePath string, opts ...Option) (*Pool, error) {
	abs, err := filepath.Abs(handlerFilePath)
	if err != nil {
		return nil, fmt.Errorf("resolving handler path %q: %w", handlerFilePath, err)
	}
	p := &Pool{
		name:       name,
		handlerAbs: abs,
		pythonPath: "python3",
		size:       PoolSize(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// Name returns this pool's handler name, for logging and error messages.
func (p *Pool) Name() string { return p.name }

// Start spawns the pool's workers and waits for each to become ready. It returns an error
// (rather than starting with fewer workers) if any worker's handler file fails to load —
// per FR-011 and data-model.md, the gateway must fail to start rather than silently run
// without a handler it was configured to have.
func (p *Pool) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Workers start in parallel: each spawns its own Python interpreter and waits for its
	// socket, so starting size=NumCPU() of them sequentially would make gateway startup time
	// scale with core count for no reason.
	p.workers = make([]*worker, p.size)
	type result struct {
		index int
		w     *worker
		err   error
	}
	results := make(chan result, p.size)
	for i := 0; i < p.size; i++ {
		go func(i int) {
			w, err := p.startWorker(ctx)
			results <- result{index: i, w: w, err: err}
		}(i)
	}

	var firstErr error
	for i := 0; i < p.size; i++ {
		r := <-results
		if r.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("starting worker for handler %q: %w", p.name, r.err)
			}
			continue
		}
		p.workers[r.index] = r.w
	}
	if firstErr != nil {
		for _, w := range p.workers {
			if w != nil && w.cmd != nil && w.cmd.Process != nil {
				_ = w.cmd.Process.Kill()
			}
		}
		return firstErr
	}

	for _, w := range p.workers {
		go p.watch(w)
	}
	return nil
}

// Stop terminates every worker process in the pool.
func (p *Pool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		w.mu.Lock()
		if w.conn != nil {
			_ = w.conn.Close()
		}
		if w.cmd != nil && w.cmd.Process != nil {
			_ = w.cmd.Process.Kill()
		}
		_ = os.Remove(w.sockPath)
		w.mu.Unlock()
	}
}

func (p *Pool) startWorker(ctx context.Context) (*worker, error) {
	sockPath := filepath.Join(os.TempDir(), fmt.Sprintf("gateway-%s-%d-%d.sock", p.name, os.Getpid(), rand.Int63()))
	_ = os.Remove(sockPath)

	cmd := exec.CommandContext(context.Background(), p.pythonPath, "-m", "gateway_handler_sdk.server",
		"--handler-file", p.handlerAbs, "--socket", sockPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching worker process: %w", err)
	}

	conn, client, err := dialWithRetry(ctx, sockPath, startupTimeout)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	return &worker{
		state:    stateReady,
		sockPath: sockPath,
		cmd:      cmd,
		conn:     conn,
		client:   client,
	}, nil
}

// dialWithRetry waits for the worker's socket file to exist, then makes a single gRPC dial.
// It deliberately avoids probing with a raw net.Dial-then-close: gRPC-go's server-side
// listener logs a (harmless but noisy) error for a connection that closes before completing
// any handshake, and a socket file existing is a reliable enough readiness signal here since
// this process is the only client and the worker created the file itself right before
// accepting.
func dialWithRetry(ctx context.Context, sockPath string, timeout time.Duration) (*grpc.ClientConn, handlerv1.HandlerClient, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			conn, err := grpc.NewClient("unix:"+sockPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return nil, nil, fmt.Errorf("dialing worker socket: %w", err)
			}
			return conn, handlerv1.NewHandlerClient(conn), nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil, nil, fmt.Errorf("worker socket %q never became ready: %w", sockPath, lastErr)
}

// watch waits for a worker's process to exit, then restarts it with backoff (FR-006), marking
// it Degraded after maxConsecutiveCrashes consecutive failures instead of restart-looping
// forever (data-model.md HandlerWorkerPool state transitions).
func (p *Pool) watch(w *worker) {
	for {
		_ = w.cmd.Wait()

		w.mu.Lock()
		w.crashCount++
		crashCount := w.crashCount
		w.state = stateCrashed
		if w.conn != nil {
			_ = w.conn.Close()
			w.conn = nil
		}
		_ = os.Remove(w.sockPath)
		w.mu.Unlock()

		if crashCount > maxConsecutiveCrashes {
			w.mu.Lock()
			w.state = stateDegraded
			w.mu.Unlock()
			log.Printf("workerpool[%s]: worker degraded after %d consecutive crashes; giving up", p.name, crashCount)
			return
		}

		backoff := time.Duration(1<<min(crashCount, 6)) * 50 * time.Millisecond
		w.mu.Lock()
		w.state = stateRestarting
		w.mu.Unlock()
		log.Printf("workerpool[%s]: worker crashed (count=%d), restarting in %s", p.name, crashCount, backoff)
		time.Sleep(backoff)

		nw, err := p.startWorker(context.Background())
		if err != nil {
			log.Printf("workerpool[%s]: failed to restart worker: %v", p.name, err)
			w.mu.Lock()
			w.state = stateCrashed
			w.mu.Unlock()
			continue
		}
		nw.crashCount = crashCount

		w.mu.Lock()
		w.state = nw.state
		w.sockPath = nw.sockPath
		w.cmd = nw.cmd
		w.conn = nw.conn
		w.client = nw.client
		w.mu.Unlock()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// pick returns the next available (non-degraded) worker via round robin, or an error if none
// are usable — the caller treats this as a handler failure and fails closed (FR-012).
func (p *Pool) pick() (*worker, error) {
	p.mu.Lock()
	workers := p.workers
	p.mu.Unlock()

	if len(workers) == 0 {
		return nil, fmt.Errorf("workerpool[%s]: no workers configured", p.name)
	}
	for i := 0; i < len(workers); i++ {
		idx := int(p.next.Add(1)-1) % len(workers)
		w := workers[idx]
		w.mu.Lock()
		usable := w.state == stateReady || w.state == stateBusy
		client := w.client
		w.mu.Unlock()
		if usable && client != nil {
			return w, nil
		}
	}
	return nil, fmt.Errorf("workerpool[%s]: no ready worker available", p.name)
}

func toProto(m Message) *handlerv1.HttpMessage {
	return &handlerv1.HttpMessage{
		Method:     m.Method,
		Path:       m.Path,
		StatusCode: int32(m.StatusCode),
		Headers:    m.Headers,
		Body:       m.Body,
	}
}

func fromProto(m *handlerv1.HttpMessage) *Message {
	if m == nil {
		return nil
	}
	return &Message{
		Method:     m.Method,
		Path:       m.Path,
		StatusCode: int(m.StatusCode),
		Headers:    m.Headers,
		Body:       m.Body,
	}
}

func rejectionFromProto(r *handlerv1.Rejection) *Rejection {
	if r == nil {
		return nil
	}
	return &Rejection{ErrorCode: r.ErrorCode, Message: r.Message}
}

// ProcessRequest calls the pool's Handler.ProcessRequest RPC on an available worker. The
// caller (internal/handler/chain.go) is responsible for setting ctx's deadline from the
// handler's configured timeout (FR-012).
func (p *Pool) ProcessRequest(ctx context.Context, requestID string, msg Message) (*Message, *Rejection, error) {
	w, err := p.pick()
	if err != nil {
		return nil, nil, err
	}
	resp, err := w.client.ProcessRequest(ctx, &handlerv1.ProcessRequestReq{
		RequestId: requestID,
		Request:   toProto(msg),
	})
	if err != nil {
		return nil, nil, err
	}
	if rej := resp.GetRejection(); rej != nil {
		return nil, rejectionFromProto(rej), nil
	}
	return fromProto(resp.GetModifiedRequest()), nil, nil
}

// ProcessResponse calls the pool's Handler.ProcessResponse RPC on an available worker.
func (p *Pool) ProcessResponse(ctx context.Context, requestID string, msg Message) (*Message, *Rejection, error) {
	w, err := p.pick()
	if err != nil {
		return nil, nil, err
	}
	resp, err := w.client.ProcessResponse(ctx, &handlerv1.ProcessResponseReq{
		RequestId: requestID,
		Response:  toProto(msg),
	})
	if err != nil {
		return nil, nil, err
	}
	if rej := resp.GetRejection(); rej != nil {
		return nil, rejectionFromProto(rej), nil
	}
	return fromProto(resp.GetModifiedResponse()), nil, nil
}
