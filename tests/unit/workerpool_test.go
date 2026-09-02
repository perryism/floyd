// Package unit tests individual Go components in isolation (no HTTP layer).
package unit

import (
	"context"
	"testing"
	"time"

	"api-gateway/internal/handler/workerpool"
	"api-gateway/tests/testutil"
)

// TestWorkerPool_CrashRestartDoesNotHangOrLeak drives a handler that hard-crashes its worker
// process on every call (os._exit, not a caught Python exception) through several
// crash/restart cycles. workerpool.Pool is a black box from this test's package (by this
// project's structure, tests live outside the packages they test), so this asserts the
// externally-visible safety contract from data-model.md's HandlerWorkerPool state machine —
// every call fails closed promptly, the pool never hangs waiting on a dead worker, and it
// settles into consistently failing (Degraded, internally) rather than restart-looping
// forever — rather than asserting the internal state enum directly.
func TestWorkerPool_CrashRestartDoesNotHangOrLeak(t *testing.T) {
	pool, err := workerpool.New("crash-process", "../fixtures/crash_process.py",
		workerpool.WithPythonPath(testutil.PythonExecutable(t)), workerpool.WithSize(1))
	if err != nil {
		t.Fatalf("configuring pool: %v", err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatalf("starting pool: %v", err)
	}
	t.Cleanup(pool.Stop)

	deadline := time.Now().Add(8 * time.Second)
	calls := 0
	for time.Now().Before(deadline) && calls < 20 {
		calls++
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, _, err := pool.ProcessRequest(ctx, "req", workerpool.Message{Method: "GET", Path: "/x"})
		cancel()
		if err == nil {
			t.Fatalf("call %d: expected an error (handler always crashes), got none", calls)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if calls < 5 {
		t.Fatalf("expected to drive at least 5 calls through crash/restart cycles, only completed %d", calls)
	}

	// The pool must still respond promptly (not hang) even well after repeated crashes —
	// whether the sole worker is mid-restart or has been marked Degraded, pick() must fail
	// fast rather than block.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	_, _, err = pool.ProcessRequest(ctx, "req-final", workerpool.Message{Method: "GET", Path: "/x"})
	cancel()
	if err == nil {
		t.Fatal("expected the final call to also fail (handler always crashes)")
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("final call took %s; pool appears to be hanging instead of failing fast", elapsed)
	}
}
