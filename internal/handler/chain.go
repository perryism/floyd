package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"api-gateway/internal/config"
	"api-gateway/internal/handler/workerpool"
	"api-gateway/internal/observability"
	"api-gateway/internal/proxy"
)

// Handler pairs one configured --handlers file with its running worker pool.
type Handler struct {
	Ref    config.HandlerRef
	Client workerpool.Client
}

// Runner is the gateway's top-level http.Handler: it runs the configured request-handler
// chain (FR-001, FR-005), forwards to the backend (FR-009), then runs the configured
// response-handler chain (FR-002, FR-005) before writing the final response. A route with no
// configured handlers behaves as plain pass-through proxying (FR-009).
type Runner struct {
	forwarder *proxy.Forwarder
	handlers  []Handler
}

// NewRunner builds a Runner. handlers runs in the given order for both the request phase and
// the response phase (FR-005) — a handler file that only implements one direction relies on
// the handlers-sdk's no-op default for the other.
func NewRunner(forwarder *proxy.Forwarder, handlers []Handler) *Runner {
	return &Runner{forwarder: forwarder, handlers: handlers}
}

func (rn *Runner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := newRequestID()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	msg := workerpool.Message{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: flattenHeaders(r.Header),
		Body:    body,
	}

	for _, h := range rn.handlers {
		result, rejection, outcome, err := invoke(r.Context(), h, msg,
			func(ctx context.Context) (*workerpool.Message, *workerpool.Rejection, error) {
				return h.Client.ProcessRequest(ctx, requestID, msg)
			})
		logInvocation(requestID, h.Ref.Name, observability.DirectionRequest, outcome, err)
		if err != nil {
			writeFailClosed(w, err, h.Ref.Name)
			return
		}
		if rejection != nil {
			observability.WriteError(w, observability.CodeHandlerRejectedRequest, rejection.Message, h.Ref.Name)
			return
		}
		msg = *result
	}

	resp, err := rn.forwarder.Forward(r.Context(), msg.Method, msg.Path, headersFromMap(msg.Headers), msg.Body)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}

	respMsg := workerpool.Message{
		StatusCode: resp.StatusCode,
		Headers:    flattenHeaders(resp.Headers),
		Body:       resp.Body,
	}

	for _, h := range rn.handlers {
		result, rejection, outcome, err := invoke(r.Context(), h, respMsg,
			func(ctx context.Context) (*workerpool.Message, *workerpool.Rejection, error) {
				return h.Client.ProcessResponse(ctx, requestID, respMsg)
			})
		logInvocation(requestID, h.Ref.Name, observability.DirectionResponse, outcome, err)
		if err != nil {
			writeFailClosed(w, err, h.Ref.Name)
			return
		}
		if rejection != nil {
			observability.WriteError(w, observability.CodeHandlerRejectedResponse, rejection.Message, h.Ref.Name)
			return
		}
		respMsg = *result
	}

	for k, v := range headersFromMap(respMsg.Headers) {
		w.Header()[k] = v
	}
	w.WriteHeader(respMsg.StatusCode)
	_, _ = w.Write(respMsg.Body)
}

// invoke calls fn with a context deadline derived from the handler's configured timeout
// (FR-012) and classifies the outcome for logging (FR-008).
func invoke(
	ctx context.Context,
	h Handler,
	before workerpool.Message,
	fn func(context.Context) (*workerpool.Message, *workerpool.Rejection, error),
) (*workerpool.Message, *workerpool.Rejection, observability.Outcome, error) {
	timeout := time.Duration(h.Ref.TimeoutMS) * time.Millisecond
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, rejection, err := fn(callCtx)
	if err != nil {
		return nil, nil, observability.OutcomeErrored, err
	}
	if rejection != nil {
		return nil, rejection, observability.OutcomeRejected, nil
	}
	if messagesEqual(before, *result) {
		return result, nil, observability.OutcomePassed, nil
	}
	return result, nil, observability.OutcomeModified, nil
}

// messagesEqual reports whether two messages are the same for outcome-classification
// purposes (FR-008's "passed" vs "modified"). It treats a nil and an empty (non-nil) Body or
// Headers as equal: an empty gRPC bytes/map field deserializes as nil on one side of the wire
// and as a zero-length value on the other, which reflect.DeepEqual would otherwise report as
// "modified" even when a handler passed the message through untouched.
func messagesEqual(a, b workerpool.Message) bool {
	if a.Method != b.Method || a.Path != b.Path || a.StatusCode != b.StatusCode {
		return false
	}
	if !bytes.Equal(a.Body, b.Body) {
		return false
	}
	if len(a.Headers) != len(b.Headers) {
		return false
	}
	for k, v := range a.Headers {
		if bv, ok := b.Headers[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func logInvocation(requestID, handlerName string, dir observability.Direction, outcome observability.Outcome, err error) {
	if err != nil {
		outcome = observability.OutcomeErrored
	}
	observability.LogHandlerExecution(observability.HandlerExecutionRecord{
		RequestID: requestID,
		Handler:   handlerName,
		Direction: dir,
		Outcome:   outcome,
		Timestamp: time.Now(),
	})
}

// writeFailClosed renders a handler error/timeout using the standard error envelope,
// implementing FR-012's fail-closed requirement.
func writeFailClosed(w http.ResponseWriter, err error, handlerName string) {
	code := observability.CodeHandlerError
	if errors.Is(err, context.DeadlineExceeded) {
		code = observability.CodeHandlerTimeout
	}
	observability.WriteError(w, code, "the configured handler failed to process this request", handlerName)
}

// flattenHeaders converts Go's canonicalized (Title-Case) http.Header into the lowercase
// keys handlers see, so authors can check for a header by its plain HTTP-spec name (headers
// are case-insensitive) without guessing Go's canonicalization — e.g. request.headers["x-required"],
// not request.headers["X-Required"].
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			key := strings.ToLower(k)
			out[key] = v[0]
			for _, extra := range v[1:] {
				out[key] += ", " + extra
			}
		}
	}
	return out
}

// headersFromMap converts a handler-facing header map back into http.Header for an outgoing
// request/response. It drops content-length and transfer-encoding: both describe how the
// original body was framed on the wire, and every message here has already been fully
// buffered into a concrete []byte that may have a different length after a handler modified
// it — Go's net/http sets the correct Content-Length itself from the actual bytes written.
func headersFromMap(m map[string]string) http.Header {
	h := make(http.Header, len(m))
	for k, v := range m {
		if k == "content-length" || k == "transfer-encoding" {
			continue
		}
		h.Set(k, v)
	}
	return h
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
