// Package proxy implements the gateway's core reverse-proxy: forwarding a (possibly
// handler-modified) request to the single configured --dest and returning the backend's raw
// response for the response-handler chain to see (FR-009 baseline pass-through behavior when
// no handlers are configured).
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Forwarder forwards requests to a single fixed destination.
type Forwarder struct {
	dest   *url.URL
	client *http.Client
}

// New returns a Forwarder that sends every request to dest.
func New(dest *url.URL) *Forwarder {
	return &Forwarder{
		dest:   dest,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Response is the backend's raw response, read fully into memory so response handlers can
// inspect and modify the body before it's written back to the caller.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Forward sends method/path/headers/body to the destination and returns its response.
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
		return nil, fmt.Errorf("calling backend: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading backend response: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}
