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

// The built-in list is filtered down to the reachable address families, while
// hostname-based servers survive either way because they can resolve to both.
func TestDefaultServersForNetwork(t *testing.T) {
	const hostname = "https://dns.google/dns-query"

	tests := []struct {
		name         string
		capabilities NetworkCapabilities
		excluded     func(netip.Addr) bool // literal addresses that must be gone
		mustKeep     []string
		wantFullList bool
	}{
		{
			name:         "IPv4-only drops IPv6 literals",
			capabilities: NetworkCapabilities{IPv4: true},
			excluded:     netip.Addr.Is6,
			mustKeep:     []string{"1.1.1.1", hostname},
		},
		{
			name:         "IPv6-only drops IPv4 literals",
			capabilities: NetworkCapabilities{IPv6: true},
			excluded:     netip.Addr.Is4,
			mustKeep:     []string{"2606:4700:4700::1111", hostname},
		},
		{
			name:         "dual stack keeps everything",
			capabilities: NetworkCapabilities{IPv4: true, IPv6: true},
			mustKeep:     []string{"1.1.1.1", "2606:4700:4700::1111", hostname},
			wantFullList: true,
		},
		{
			// A probe that detects neither family is likelier to be wrong than
			// to describe a host that truly reaches nothing, so nothing is cut.
			name:         "no detected family keeps everything",
			capabilities: NetworkCapabilities{},
			mustKeep:     []string{"1.1.1.1", "2606:4700:4700::1111", hostname},
			wantFullList: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultServersForNetwork(tt.capabilities)

			if tt.wantFullList && len(got) != len(DefaultServers) {
				t.Errorf("got %d servers, want the full list of %d", len(got), len(DefaultServers))
			}

			present := make(map[string]bool, len(got))
			for _, server := range got {
				present[server.Address] = true
				if tt.excluded == nil {
					continue
				}
				if addr, err := netip.ParseAddr(server.Address); err == nil && tt.excluded(addr) {
					t.Errorf("server %+v should have been filtered out", server)
				}
			}

			for _, address := range tt.mustKeep {
				if !present[address] {
					t.Errorf("server %q was dropped", address)
				}
			}
		})
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
