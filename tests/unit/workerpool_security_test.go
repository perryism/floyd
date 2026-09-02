package unit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"api-gateway/internal/handler/workerpool"
	"api-gateway/tests/testutil"
)

// T037: the worker's socket must not be network-exposed (it's a Unix domain socket by
// construction — grpc-go only dials it as "unix:<path>", never a TCP address) and its
// containing directory must be private to the current user, so another local user on a
// shared host can't reach the socket regardless of the socket file's own permission bits.
func TestWorkerPool_SocketDirectoryIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets and POSIX permission bits don't apply on Windows")
	}

	pool, err := workerpool.New("add-header", "../fixtures/add_header.py",
		workerpool.WithPythonPath(testutil.PythonExecutable(t)), workerpool.WithSize(1))
	if err != nil {
		t.Fatalf("configuring pool: %v", err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatalf("starting pool: %v", err)
	}
	t.Cleanup(pool.Stop)

	sockPath := pool.DebugSocketPath()
	if sockPath == "" {
		t.Fatal("expected a non-empty socket path")
	}

	dir := filepath.Dir(sockPath)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat socket directory: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected socket directory to be 0700 (owner-only), got %o", perm)
	}
}
