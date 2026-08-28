package dnsbench

import "testing"

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
