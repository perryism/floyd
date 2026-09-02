package workerpool

import "context"

// Message is the Go-side, language-agnostic representation of an HTTP message that crosses
// the gRPC boundary to a Python worker (mirrors api/handler/v1/handler.proto's HttpMessage).
type Message struct {
	Method     string
	Path       string
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// Rejection is a structured handler rejection (mirrors handler.proto's Rejection), rendered
// into the gateway's standard error envelope (contracts/error-format.md) before it reaches
// the caller.
type Rejection struct {
	ErrorCode string
	Message   string
}

// Client is one handler's callable interface, implemented by *Pool. A nil Rejection with a
// non-nil Message means the handler passed the message through (unmodified or modified); a
// non-nil Rejection means FR-003/FR-004's "reject" case. A non-nil error means the call itself
// failed (crash, transport error, deadline exceeded) and the caller MUST fail closed per
// FR-012.
type Client interface {
	ProcessRequest(ctx context.Context, requestID string, msg Message) (*Message, *Rejection, error)
	ProcessResponse(ctx context.Context, requestID string, msg Message) (*Message, *Rejection, error)
	Name() string
}
