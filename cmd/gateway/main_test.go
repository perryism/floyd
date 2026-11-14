package main

import (
	"net"
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
