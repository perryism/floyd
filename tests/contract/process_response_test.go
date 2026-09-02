package contract

import (
	"context"
	"testing"

	"api-gateway/internal/handler/workerpool"
)

func TestProcessResponse_Modify(t *testing.T) {
	pool := newTestPool(t, "add-response-header", "../fixtures/add_response_header.py")

	result, rejection, err := pool.ProcessResponse(context.Background(), "req-3", workerpool.Message{
		StatusCode: 200,
		Headers:    map[string]string{},
		Body:       []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("ProcessResponse returned error: %v", err)
	}
	if rejection != nil {
		t.Fatalf("expected no rejection, got %+v", rejection)
	}
	if result.Headers["x-cache"] != "miss" {
		t.Fatalf("expected x-cache=miss, got headers %+v", result.Headers)
	}
}

func TestProcessResponse_Reject(t *testing.T) {
	pool := newTestPool(t, "reject-response", "../fixtures/reject_response.py")

	result, rejection, err := pool.ProcessResponse(context.Background(), "req-5", workerpool.Message{
		StatusCode: 200,
		Body:       []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("ProcessResponse returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no modified message on rejection, got %+v", result)
	}
	if rejection == nil || rejection.ErrorCode != "BLOCKED_RESPONSE" {
		t.Fatalf("expected BLOCKED_RESPONSE rejection, got %+v", rejection)
	}
}

func TestProcessResponse_PassthroughWhenDirectionNotImplemented(t *testing.T) {
	// add_header.py only implements RequestHandler; the SDK's default ResponseHandler
	// must pass the response through unchanged (FR-010/FR-011 "may implement one or both").
	pool := newTestPool(t, "add-header-response-passthrough", "../fixtures/add_header.py")

	result, rejection, err := pool.ProcessResponse(context.Background(), "req-4", workerpool.Message{
		StatusCode: 200,
		Body:       []byte("hello"),
	})
	if err != nil {
		t.Fatalf("ProcessResponse returned error: %v", err)
	}
	if rejection != nil {
		t.Fatalf("expected no rejection, got %+v", rejection)
	}
	if string(result.Body) != "hello" || result.StatusCode != 200 {
		t.Fatalf("expected unmodified passthrough, got %+v", result)
	}
}
