package dnsbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemDNSFromResolvConf(t *testing.T) {
	content := `# comment
nameserver 192.168.1.1
nameserver 8.8.8.8
options ndots:2
`
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := systemDNSFromResolvConf(path)
	want := []string{"192.168.1.1", "8.8.8.8"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSystemDNSFromResolvConfMissing(t *testing.T) {
	if got := systemDNSFromResolvConf(filepath.Join(t.TempDir(), "nope.conf")); got != nil {
		t.Fatalf("expected nil for missing file, got %v", got)
	}
}

func TestBuildSystemServers(t *testing.T) {
	const nameSingle = "System DNS"
	const nameFmt = "System DNS %d"

	// Dedupe + filter invalid + numbered naming.
	servers := buildSystemServers([]string{"1.1.1.1", "1.1.1.1", "", "not-an-ip", "8.8.8.8"}, nameSingle, nameFmt)
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d: %+v", len(servers), servers)
	}
	for _, s := range servers {
		if !s.IsSystem || s.Protocol != UDP {
			t.Fatalf("unexpected server fields: %+v", s)
		}
	}
	if servers[0].Name != "System DNS 1" || servers[1].Name != "System DNS 2" {
		t.Fatalf("unexpected names: %q, %q", servers[0].Name, servers[1].Name)
	}

	// A single server is not numbered.
	single := buildSystemServers([]string{"9.9.9.9"}, nameSingle, nameFmt)
	if len(single) != 1 || single[0].Name != nameSingle {
		t.Fatalf("unexpected single server: %+v", single)
	}

	// All invalid -> nil.
	if got := buildSystemServers([]string{"", "x"}, nameSingle, nameFmt); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestBuildSystemServersIPv6(t *testing.T) {
	const nameSingle = "System DNS"
	const nameFmt = "System DNS %d"

	got := buildSystemServers([]string{
		"2001:4860:4860::8888",
		"fe80::1%eth0",
	}, nameSingle, nameFmt)

	want := []Server{
		{
			Name:     "System DNS 1",
			Address:  "2001:4860:4860::8888",
			Protocol: UDP,
			IsSystem: true,
		},
		{
			Name:     "System DNS 2",
			Address:  "fe80::1%eth0",
			Protocol: UDP,
			IsSystem: true,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d servers %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("server %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
