package integration

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api-gateway/tests/testutil"
)

// data-model.md GatewayConfig.verbose: when true, the gateway logs each request's method,
// path, and response status code to its standard log output; when omitted, it logs nothing.
func TestVerboseLogging_LogsRequestWhenEnabled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: testRouteName, Backend: backend}}, testutil.WithVerbose())

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	logged := buf.String()
	if !strings.Contains(logged, http.MethodGet) ||
		!strings.Contains(logged, "/"+testRouteName+"/anything") ||
		!strings.Contains(logged, "200") {
		t.Fatalf("expected a verbose log line with method, path, and status, got: %q", logged)
	}
}

func TestVerboseLogging_SilentWhenOmitted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	gw := testutil.NewTestGateway(t, []testutil.TestRoute{{Name: testRouteName, Backend: backend}})

	resp, err := http.Get(gw.URL + "/" + testRouteName + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if buf.Len() != 0 {
		t.Fatalf("expected no log output when verbose is omitted, got: %q", buf.String())
	}
}
