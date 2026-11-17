package main

import (
	"io"
	"net"
	"os"
	"strings"
	"testing"
)

func TestReachableIPs_SpecificHostReturnedAsIs(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.1.2.3", "example.internal"} {
		got := reachableIPs(host)
		if len(got) != 1 || got[0] != host {
			t.Errorf("reachableIPs(%q) = %v, want [%q]", host, got, host)
		}
	}
}

func TestReachableIPs_WildcardListsNonLoopbackIPv4(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", ""} {
		for _, s := range reachableIPs(host) {
			ip := net.ParseIP(s)
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				t.Errorf("reachableIPs(%q) returned %q, want a non-loopback IPv4 address", host, s)
			}
		}
	}
}

func TestRun_VersionPrintsBuildTime(t *testing.T) {
	old := buildTime
	buildTime = "2026-01-02T03:04:05Z"
	defer func() { buildTime = old }()

	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	err := run([]string{"--version"})
	w.Close()
	os.Stdout = stdout
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "2026-01-02T03:04:05Z") {
		t.Fatalf("version output %q lacks build time", out)
	}
}
