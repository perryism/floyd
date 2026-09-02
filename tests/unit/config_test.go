package unit

import (
	"os"
	"path/filepath"
	"testing"

	"api-gateway/internal/config"
)

func TestConfig_Parse_Valid(t *testing.T) {
	handlerFile := writeTempFile(t, "print('hi')")

	cfg, err := config.Parse([]string{
		"--dest", "http://127.0.0.1:1234",
		"--host", "0.0.0.0",
		"--port", "1235",
		"--handlers", handlerFile,
	})
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.Dest.String() != "http://127.0.0.1:1234" {
		t.Fatalf("unexpected dest: %v", cfg.Dest)
	}
	if cfg.ListenHost != "0.0.0.0" || cfg.ListenPort != 1235 {
		t.Fatalf("unexpected listen address: %s:%d", cfg.ListenHost, cfg.ListenPort)
	}
	if len(cfg.Handlers) != 1 || cfg.Handlers[0].FilePath != handlerFile {
		t.Fatalf("unexpected handlers: %+v", cfg.Handlers)
	}
}

func TestConfig_Parse_NoHandlers(t *testing.T) {
	cfg, err := config.Parse([]string{
		"--dest", "http://127.0.0.1:1234",
		"--host", "0.0.0.0",
		"--port", "1235",
	})
	if err != nil {
		t.Fatalf("expected a handlerless config to be valid (FR-009), got error: %v", err)
	}
	if len(cfg.Handlers) != 0 {
		t.Fatalf("expected no handlers, got %+v", cfg.Handlers)
	}
}

func TestConfig_Parse_RepeatedHandlersPreservesOrder(t *testing.T) {
	a := writeTempFile(t, "print('a')")
	b := writeTempFile(t, "print('b')")

	cfg, err := config.Parse([]string{
		"--dest", "http://127.0.0.1:1234",
		"--host", "0.0.0.0",
		"--port", "1235",
		"--handlers", a,
		"--handlers", b,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Handlers) != 2 || cfg.Handlers[0].FilePath != a || cfg.Handlers[1].FilePath != b {
		t.Fatalf("expected handlers in flag order [a, b], got %+v", cfg.Handlers)
	}
}

func TestConfig_Parse_MalformedDest(t *testing.T) {
	cases := []string{"", "not-a-url", "/just/a/path"}
	for _, dest := range cases {
		_, err := config.Parse([]string{"--dest", dest, "--host", "0.0.0.0", "--port", "1235"})
		if err == nil {
			t.Errorf("expected an error for --dest %q, got none", dest)
		}
	}
}

func TestConfig_Parse_MissingRequiredFlags(t *testing.T) {
	cases := [][]string{
		{"--host", "0.0.0.0", "--port", "1235"},                      // missing --dest
		{"--dest", "http://x:1", "--port", "1235"},                   // missing --host
		{"--dest", "http://x:1", "--host", "0.0.0.0"},                // missing --port
		{"--dest", "http://x:1", "--host", "0.0.0.0", "--port", "0"}, // invalid port
	}
	for _, args := range cases {
		if _, err := config.Parse(args); err == nil {
			t.Errorf("expected an error for args %v, got none", args)
		}
	}
}

func TestConfig_Parse_UnloadableHandlerFile(t *testing.T) {
	_, err := config.Parse([]string{
		"--dest", "http://127.0.0.1:1234",
		"--host", "0.0.0.0",
		"--port", "1235",
		"--handlers", "/does/not/exist.py",
	})
	if err == nil {
		t.Fatal("expected an error for a nonexistent --handlers file, got none")
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "handler.py")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}
