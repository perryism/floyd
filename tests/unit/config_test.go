package unit

import (
	"os"
	"path/filepath"
	"testing"

	"api-gateway/internal/config"
)

func writeConfigFile(t *testing.T, yamlContent string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
	return path
}

func TestConfig_Parse_ValidMultiRoute(t *testing.T) {
	handlerA := writeTempFile(t, "print('a')")
	cfgPath := writeConfigFile(t, `
- name: app1
  description: The first app.
  dest: http://localhost:3000
- name: app2
  description: The second app.
  dest: http://localhost:3001
  handlers: `+handlerA+`
`)

	cfg, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath})
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if cfg.ListenHost != "0.0.0.0" || cfg.ListenPort != 1235 {
		t.Fatalf("unexpected listen address: %s:%d", cfg.ListenHost, cfg.ListenPort)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Name != "app1" || cfg.Routes[0].Description != "The first app." {
		t.Fatalf("unexpected route[0]: %+v", cfg.Routes[0])
	}
	if len(cfg.Routes[0].Handlers) != 0 {
		t.Fatalf("expected app1 to have no handlers, got %+v", cfg.Routes[0].Handlers)
	}
	if cfg.Routes[1].Dest.String() != "http://localhost:3001" {
		t.Fatalf("unexpected app2 dest: %v", cfg.Routes[1].Dest)
	}
	if len(cfg.Routes[1].Handlers) != 1 || cfg.Routes[1].Handlers[0].FilePath != handlerA {
		t.Fatalf("unexpected app2 handlers: %+v", cfg.Routes[1].Handlers)
	}
}

func TestConfig_Parse_HandlersAsListPreservesOrder(t *testing.T) {
	a := writeTempFile(t, "print('a')")
	b := writeTempFile(t, "print('b')")
	cfgPath := writeConfigFile(t, `
- name: app1
  dest: http://localhost:3000
  handlers:
    - `+a+`
    - `+b+`
`)

	cfg, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	handlers := cfg.Routes[0].Handlers
	if len(handlers) != 2 || handlers[0].FilePath != a || handlers[1].FilePath != b {
		t.Fatalf("expected handlers [a, b] in order, got %+v", handlers)
	}
	if handlers[0].Position != 0 || handlers[1].Position != 1 {
		t.Fatalf("expected positions 0,1, got %d,%d", handlers[0].Position, handlers[1].Position)
	}
}

func TestConfig_Parse_MissingConfigFlag(t *testing.T) {
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235"}); err == nil {
		t.Fatal("expected an error when --config is omitted")
	}
}

func TestConfig_Parse_UnparseableConfigFile(t *testing.T) {
	cfgPath := writeConfigFile(t, "not: [valid, yaml: because of this")
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for unparseable YAML")
	}
}

func TestConfig_Parse_NonexistentConfigFile(t *testing.T) {
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", "/does/not/exist.yaml"}); err == nil {
		t.Fatal("expected an error for a nonexistent --config file")
	}
}

func TestConfig_Parse_EmptyRoutesList(t *testing.T) {
	cfgPath := writeConfigFile(t, "[]")
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for an empty routes list")
	}
}

func TestConfig_Parse_DuplicateRouteName(t *testing.T) {
	cfgPath := writeConfigFile(t, `
- name: app1
  dest: http://localhost:3000
- name: app1
  dest: http://localhost:3001
`)
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for a duplicate route name")
	}
}

func TestConfig_Parse_RouteNameContainsSlash(t *testing.T) {
	cfgPath := writeConfigFile(t, `
- name: app1/sub
  dest: http://localhost:3000
`)
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for a route name containing '/'")
	}
}

func TestConfig_Parse_ReservedRouteName(t *testing.T) {
	cfgPath := writeConfigFile(t, `
- name: "_"
  dest: http://localhost:3000
`)
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for the reserved route name '_'")
	}
}

func TestConfig_Parse_MalformedDest(t *testing.T) {
	cases := []string{"", "not-a-url", "/just/a/path"}
	for _, dest := range cases {
		cfgPath := writeConfigFile(t, "- name: app1\n  dest: "+dest+"\n")
		if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
			t.Errorf("expected an error for dest %q, got none", dest)
		}
	}
}

func TestConfig_Parse_UnloadableHandlerFile(t *testing.T) {
	cfgPath := writeConfigFile(t, `
- name: app1
  dest: http://localhost:3000
  handlers: /does/not/exist.py
`)
	if _, err := config.Parse([]string{"--host", "0.0.0.0", "--port", "1235", "--config", cfgPath}); err == nil {
		t.Fatal("expected an error for a nonexistent handlers file")
	}
}

func TestConfig_Parse_MissingHostOrPort(t *testing.T) {
	cfgPath := writeConfigFile(t, "- name: app1\n  dest: http://localhost:3000\n")
	cases := [][]string{
		{"--port", "1235", "--config", cfgPath},                   // missing --host
		{"--host", "0.0.0.0", "--config", cfgPath},                // missing --port
		{"--host", "0.0.0.0", "--port", "0", "--config", cfgPath}, // invalid port
	}
	for _, args := range cases {
		if _, err := config.Parse(args); err == nil {
			t.Errorf("expected an error for args %v, got none", args)
		}
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
