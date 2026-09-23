// Package proxy implements the gateway's core reverse-proxy: forwarding a (possibly
// handler-modified) request to the single configured --dest and returning the backend's raw
// response for the response-handler chain to see (FR-009 baseline pass-through behavior when
// no handlers are configured).
package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// DefaultTimeout is used when a route doesn't specify its own backend timeout. 30s (the
// previous fixed value) is too short for many real LLM-backend responses (e.g. Claude
// completions without streaming, or with extended thinking/tool-use loops), which routinely
// take well over 30s and would otherwise surface as a misleading "backend unavailable" 502.
const DefaultTimeout = 120 * time.Second

// ErrTimeout is returned (wrapped) by Forward when the backend didn't respond within the
// configured timeout, so callers can report a 504 (Gateway Timeout) instead of lumping every
// failure mode into a generic 502.
var ErrTimeout = errors.New("backend request timed out")

// Forwarder forwards requests to a single fixed destination.
type Forwarder struct {
	dest   *url.URL
	client *http.Client
}

// Option customizes Forwarder construction.
type Option func(*Forwarder)

// WithTimeout overrides DefaultTimeout for this Forwarder's backend requests.
func WithTimeout(d time.Duration) Option {
	return func(f *Forwarder) { f.client.Timeout = d }
}

// New returns a Forwarder that sends every request to dest.
func New(dest *url.URL, opts ...Option) *Forwarder {
	f := &Forwarder{
		dest: dest,
		client: &http.Client{
			Timeout: DefaultTimeout,
			// The default Transport's MaxIdleConnsPerHost is 2, which forces a fresh
			// TCP+TLS handshake for every request beyond 2-in-flight to the same backend
			// host — expensive, and under load a source of exactly the kind of
			// intermittent connection failures that get misreported as a flat "backend
			// unavailable".
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Response is the backend's raw response, read fully into memory so response handlers can
// inspect and modify the body before it's written back to the caller.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Forward sends method/path/headers/body to the destination and returns its response. On
// failure it always wraps the underlying error (so callers can log the real cause), and when
// the failure was a timeout, the returned error also satisfies errors.Is(err, ErrTimeout) so
// callers can distinguish "the backend took too long" from "the backend refused/reset the
// connection".
func (f *Forwarder) Forward(ctx context.Context, method, path string, headers http.Header, body []byte) (*Response, error) {
	target := *f.dest
	target.Path = path

	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building backend request: %w", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling backend: %w", classifyErr(err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading backend response: %w", classifyErr(err))
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

// classifyErr wraps err with ErrTimeout when it represents a client-side timeout (either the
// request context's deadline expiring, or a lower-level net.Error reporting Timeout(), which
// is how http.Client surfaces its own Timeout firing mid-request), so Forward's caller can
// tell "too slow" apart from every other connection failure without string-matching on error
// text.
func classifyErr(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return err
}
