// Package contract validates the Go<->Python handler boundary (api/handler/v1/handler.proto)
// directly against real Python workers, independent of the full HTTP proxy path.
package contract

import (
	"context"
	"testing"

	"api-gateway/internal/handler/workerpool"
	"api-gateway/tests/testutil"
)

func newTestPool(t *testing.T, name, fixturePath string) *workerpool.Pool {
	t.Helper()
	pool, err := workerpool.New(name, fixturePath,
		workerpool.WithPythonPath(testutil.PythonExecutable(t)), workerpool.WithSize(1))
	if err != nil {
		t.Fatalf("configuring pool: %v", err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatalf("starting pool: %v", err)
	}
	t.Cleanup(pool.Stop)
	return pool
}

func TestProcessRequest_Modify(t *testing.T) {
	pool := newTestPool(t, "add-header", "../fixtures/add_header.py")

	result, rejection, err := pool.ProcessRequest(context.Background(), "req-1", workerpool.Message{
		Method:  "GET",
		Path:    "/x",
		Headers: map[string]string{},
	})
	if err != nil {
		t.Fatalf("ProcessRequest returned error: %v", err)
	}
	if rejection != nil {
		t.Fatalf("expected no rejection, got %+v", rejection)
	}
	if result.Headers["x-injected"] != "yes" {
		t.Fatalf("expected x-injected=yes, got headers %+v", result.Headers)
	}
}

func TestProcessRequest_Reject(t *testing.T) {
	pool := newTestPool(t, "reject-missing-field", "../fixtures/reject_missing_field.py")

	result, rejection, err := pool.ProcessRequest(context.Background(), "req-2", workerpool.Message{
		Method:  "GET",
		Path:    "/x",
		Headers: map[string]string{},
	})
	if err != nil {
		t.Fatalf("ProcessRequest returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no modified message on rejection, got %+v", result)
	}
	if rejection == nil || rejection.ErrorCode != "MISSING_FIELD" {
		t.Fatalf("expected MISSING_FIELD rejection, got %+v", rejection)
	}
}
