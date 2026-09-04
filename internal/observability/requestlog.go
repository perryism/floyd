package observability

import (
	"log"
	"net/http"
)

// LogVerboseRequest writes one log line for a completed request, gated behind the --config
// file's top-level "verbose" flag (data-model.md GatewayConfig.verbose). Kept as a small seam
// (rather than a direct log.Printf call at its call site) for the same reason as
// LogHandlerExecution: the output format changes in exactly one place if this ever needs to
// become structured/shipped elsewhere.
func LogVerboseRequest(method, path string, status int) {
	log.Printf("api-gateway: %s %s -> %d", method, path, status)
}

// statusRecorder wraps an http.ResponseWriter to capture the status code passed to
// WriteHeader, since http.ResponseWriter itself exposes no way to read it back.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

// VerboseRequestLogger wraps next so every request it serves is logged via LogVerboseRequest
// once next has written its response. r.Method/r.URL.Path are read before next runs: this
// middleware is intended to wrap the gateway's top-level handler, ahead of
// internal/router.Router, which clones the request before stripping the matched route segment
// (research.md §5s) — so the path logged here is always the original, un-stripped inbound
// path, not what a matched route's backend ultimately saw.
//
// If next never calls WriteHeader (e.g. it only calls Write, which implicitly sends a 200),
// the recorded status defaults to http.StatusOK, matching net/http's own default.
func VerboseRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		LogVerboseRequest(method, path, rec.status)
	})
}
