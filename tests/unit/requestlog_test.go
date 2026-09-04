package unit

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api-gateway/internal/observability"
)

func TestVerboseRequestLogger_LogsMethodPathAndStatus(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(http.MethodPost, "/app1/foo", nil)
	rec := httptest.NewRecorder()

	observability.VerboseRequestLogger(stub).ServeHTTP(rec, req)

	logged := buf.String()
	if !strings.Contains(logged, http.MethodPost) || !strings.Contains(logged, "/app1/foo") ||
		!strings.Contains(logged, "418") {
		t.Fatalf("expected log line with method, path, and status 418, got: %q", logged)
	}
}

func TestVerboseRequestLogger_DefaultsStatusTo200WhenWriteHeaderNotCalled(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/app1", nil)
	rec := httptest.NewRecorder()

	observability.VerboseRequestLogger(stub).ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "200") {
		t.Fatalf("expected default status 200 in log line, got: %q", buf.String())
	}
}
