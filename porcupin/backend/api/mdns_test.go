package api

import (
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
)

// TestNewMDNSServer verifies constructor sets fields correctly
func TestNewMDNSServer(t *testing.T) {
	server := NewMDNSServer(8085, "1.2.3", true)

	if server.port != 8085 {
		t.Errorf("expected port 8085, got %d", server.port)
	}

	if server.version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", server.version)
	}

	if server.useTLS != true {
		t.Errorf("expected useTLS true, got %v", server.useTLS)
	}

	if server.hostname == "" {
		t.Error("expected hostname to be set")
	}
}

// TestParseServiceEntry tests our conversion from zeroconf to DiscoveredServer
func TestParseServiceEntry(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "myserver-porcupin",
			Service:  "_porcupin._tcp",
			Domain:   "local.",
		},
		HostName: "myserver.local.",
		Port:     8085,
		Text:     []string{"version=2.0.0", "tls=true"},
		AddrIPv4: []net.IP{net.ParseIP("192.168.1.50")},
		AddrIPv6: []net.IP{net.ParseIP("fe80::1")}, // link-local, should be filtered
	}

	result := parseServiceEntry(entry)

	if result == nil {
		t.Fatal("parseServiceEntry returned nil for valid entry")
	}

	if result.Name != "myserver-porcupin" {
		t.Errorf("expected Name 'myserver-porcupin', got %q", result.Name)
	}

	if result.Port != 8085 {
		t.Errorf("expected Port 8085, got %d", result.Port)
	}

	if result.Version != "2.0.0" {
		t.Errorf("expected Version '2.0.0', got %q", result.Version)
	}

	if result.UseTLS != true {
		t.Errorf("expected UseTLS true, got %v", result.UseTLS)
	}

	// Should prefer IPv4 for Host
	if result.Host != "192.168.1.50" {
		t.Errorf("expected Host '192.168.1.50', got %q", result.Host)
	}

	// IPs should contain IPv4, link-local IPv6 should be filtered
	foundIPv4 := false
	for _, ip := range result.IPs {
		if ip == "192.168.1.50" {
			foundIPv4 = true
		}
		if ip == "fe80::1" {
			t.Error("link-local IPv6 should be filtered out")
		}
	}
	if !foundIPv4 {
		t.Error("IPv4 address should be in IPs list")
	}
}

// TestParseServiceEntryNil verifies nil handling
func TestParseServiceEntryNil(t *testing.T) {
	result := parseServiceEntry(nil)
	if result != nil {
		t.Error("expected nil for nil entry")
	}
}

// TestParseServiceEntryNoIPs verifies we return nil when no usable IPs
func TestParseServiceEntryNoIPs(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "noip",
		},
		Port:     8085,
		AddrIPv4: []net.IP{},
		AddrIPv6: []net.IP{},
	}

	result := parseServiceEntry(entry)
	if result != nil {
		t.Error("expected nil when no IPs available")
	}
}

// TestParseServiceEntryNoTLS verifies default TLS=false
func TestParseServiceEntryNoTLS(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "server",
		},
		Port:     8085,
		Text:     []string{"version=1.0.0"}, // no tls field
		AddrIPv4: []net.IP{net.ParseIP("10.0.0.1")},
	}

	result := parseServiceEntry(entry)

	if result.UseTLS != false {
		t.Error("expected UseTLS false when not specified")
	}
}

// TestParseServiceEntryIPv6Only verifies IPv6-only handling
func TestParseServiceEntryIPv6Only(t *testing.T) {
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "ipv6server",
		},
		Port: 8085,
		Text: []string{},
		AddrIPv6: []net.IP{
			net.ParseIP("fe80::1"),     // link-local (should skip)
			net.ParseIP("2001:db8::1"), // global (should use)
		},
	}

	result := parseServiceEntry(entry)

	if result == nil {
		t.Fatal("expected non-nil result for IPv6-only server")
	}

	if result.Host != "2001:db8::1" {
		t.Errorf("expected Host '2001:db8::1', got %q", result.Host)
	}
}

// TestGetLocalIPsFiltering verifies our IP filtering logic
func TestGetLocalIPsFiltering(t *testing.T) {
	ips := GetLocalIPs()

	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed == nil {
			t.Errorf("GetLocalIPs returned invalid IP: %q", ip)
			continue
		}

		if parsed.IsLoopback() {
			t.Errorf("loopback IP should be filtered: %s", ip)
		}

		if parsed.IsLinkLocalUnicast() {
			t.Errorf("link-local IP should be filtered: %s", ip)
		}
	}
}

// TestMDNSServerStopSafety verifies Stop doesn't panic
func TestMDNSServerStopSafety(t *testing.T) {
	server := NewMDNSServer(19099, "1.0.0", false)

	// Stop without start - should not panic
	server.Stop()

	// Double stop - should not panic
	server.Stop()
}
