package dnsbench

import (
	"net/netip"
	"testing"
)

func TestParseServers(t *testing.T) {
	got := ParseServers(" 1.1.1.1 , udp://8.8.8.8, tls://dns.google , https://cloudflare-dns.com/dns-query , h3://dns.alidns.com/dns-query ,, 1.1.1.1 ")
	want := []Server{
		{Name: "1.1.1.1 (UDP)", Address: "1.1.1.1", Protocol: UDP},
		{Name: "8.8.8.8 (UDP)", Address: "8.8.8.8", Protocol: UDP},
		{Name: "dns.google (DoT)", Address: "dns.google", Protocol: DOT},
		{Name: "cloudflare-dns.com (DoH)", Address: "https://cloudflare-dns.com/dns-query", Protocol: DOH},
		{Name: "dns.alidns.com (DoH3)", Address: "https://dns.alidns.com/dns-query", Protocol: DOH3},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d servers %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("server %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseServersEmpty(t *testing.T) {
	if got := ParseServers("  , , "); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestParseServersIPv6(t *testing.T) {
	got := ParseServers(
		"2606:4700:4700::1111, udp://[2001:4860:4860::8888], [2620:fe::fe]",
	)

	want := []Server{
		{
			Name:     "2606:4700:4700::1111 (UDP)",
			Address:  "2606:4700:4700::1111",
			Protocol: UDP,
		},
		{
			Name:     "2001:4860:4860::8888 (UDP)",
			Address:  "2001:4860:4860::8888",
			Protocol: UDP,
		},
		{
			Name:     "2620:fe::fe (UDP)",
			Address:  "2620:fe::fe",
			Protocol: UDP,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d servers %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("server %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDefaultServersIncludeIPv6(t *testing.T) {
	want := []Server{
		{Name: "AliDNS 1 (UDP/IPv6)", Address: "2400:3200::1", Protocol: UDP},
		{Name: "AliDNS 2 (UDP/IPv6)", Address: "2400:3200:baba::1", Protocol: UDP},
		{Name: "Google 1 (UDP/IPv6)", Address: "2001:4860:4860::8888", Protocol: UDP},
		{Name: "Google 2 (UDP/IPv6)", Address: "2001:4860:4860::8844", Protocol: UDP},
		{Name: "Cloudflare 1 (UDP/IPv6)", Address: "2606:4700:4700::1111", Protocol: UDP},
		{Name: "Cloudflare 2 (UDP/IPv6)", Address: "2606:4700:4700::1001", Protocol: UDP},
		{Name: "Quad9 1 (UDP/IPv6)", Address: "2620:fe::fe", Protocol: UDP},
		{Name: "Quad9 2 (UDP/IPv6)", Address: "2620:fe::9", Protocol: UDP},
	}

	byName := make(map[string]Server, len(DefaultServers))
	for _, server := range DefaultServers {
		byName[server.Name] = server
	}

	for _, expected := range want {
		got, ok := byName[expected.Name]
		if !ok {
			t.Errorf("missing default IPv6 server %q", expected.Name)
			continue
		}
		if got != expected {
			t.Errorf("server %q = %+v, want %+v", expected.Name, got, expected)
		}
	}
}

// Filter IPv6, keep IPv4 and hostname-based servers
func TestDefaultServersForIPv4Only(t *testing.T) {
	got := DefaultServersForNetwork(NetworkCapabilities{
		IPv4: true,
		IPv6: false,
	})

	foundIPv4 := false
	foundHostname := false

	for _, server := range got {
		addr, err := netip.ParseAddr(server.Address)
		if err == nil && addr.Is6() {
			t.Errorf("IPv4-only defaults contain IPv6 server %+v", server)
		}
		if server.Address == "1.1.1.1" {
			foundIPv4 = true
		}
		if server.Address == "https://dns.google/dns-query" {
			foundHostname = true
		}
	}

	if !foundIPv4 {
		t.Error("IPv4-only defaults removed IPv4 servers")
	}
	if !foundHostname {
		t.Error("IPv4-only defaults removed hostname-based servers")
	}
}

func TestDefaultServersForIPv6Only(t *testing.T) {
	got := DefaultServersForNetwork(NetworkCapabilities{
		IPv4: false,
		IPv6: true,
	})

	foundIPv6 := false
	foundHostname := false

	for _, server := range got {
		addr, err := netip.ParseAddr(server.Address)
		if err == nil && addr.Is4() {
			t.Errorf("IPv6-only defaults contain IPv4 server %+v", server)
		}

		if server.Address == "2606:4700:4700::1111" {
			foundIPv6 = true
		}
		if server.Address == "https://dns.google/dns-query" {
			foundHostname = true
		}
	}

	if !foundIPv6 {
		t.Error("IPv6-only defaults removed IPv6 servers")
	}
	if !foundHostname {
		t.Error("IPv6-only defaults removed hostname-based servers")
	}
}

// Equivalent spellings of one server must collapse into a single entry, so a
// bracketed IPv6 literal is not benchmarked twice alongside its bare form.
func TestParseServersDeduplicatesEquivalentSpellings(t *testing.T) {
	got := ParseServers(
		"2606:4700:4700::1111, udp://[2606:4700:4700::1111], [2606:4700:4700::1111]",
	)

	want := []Server{
		{
			Name:     "2606:4700:4700::1111 (UDP)",
			Address:  "2606:4700:4700::1111",
			Protocol: UDP,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d servers %+v, want %d", len(got), got, len(want))
	}
	if got[0] != want[0] {
		t.Errorf("server = %+v, want %+v", got[0], want[0])
	}
}

// The same address under different protocols is not a duplicate.
func TestParseServersKeepsSameHostAcrossProtocols(t *testing.T) {
	got := ParseServers("dns.google, tls://dns.google")

	if len(got) != 2 {
		t.Fatalf("got %d servers %+v, want 2", len(got), got)
	}
	if got[0].Protocol != UDP || got[1].Protocol != DOT {
		t.Errorf("got protocols %q/%q, want udp/dot", got[0].Protocol, got[1].Protocol)
	}
}

// A probe that detects neither family is more likely to be wrong than to
// describe a host that truly cannot reach anything, so the full list is kept.
func TestDefaultServersForNoDetectedFamily(t *testing.T) {
	got := DefaultServersForNetwork(NetworkCapabilities{})

	if len(got) != len(DefaultServers) {
		t.Fatalf("got %d servers, want the full list of %d", len(got), len(DefaultServers))
	}
}
