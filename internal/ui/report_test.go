package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/palemoky/dnspick/internal/dnsbench"
)

func TestSystemDNSVerdict(t *testing.T) {
	// mk builds a result from avg latency (ms) and success rate; Successes is derived from the rate.
	mk := func(name string, avgMs int, rate float64, sys bool) dnsbench.Result {
		return dnsbench.Result{
			Name: name, Address: name,
			AvgTime:     time.Duration(avgMs) * time.Millisecond,
			SuccessRate: rate, Successes: int(rate * 10), Total: 10, IsSystem: sys,
		}
	}

	t.Run("no system dns", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false)}
		if _, ok := systemDNSVerdict(res); ok {
			t.Fatal("expected ok=false when no system DNS")
		}
	})

	t.Run("system is best", func(t *testing.T) {
		res := []dnsbench.Result{mk("sys", 5, 1.0, true), mk("B", 10, 1.0, false)}
		if msg, ok := systemDNSVerdict(res); !ok || !strings.Contains(msg, "is already the best") {
			t.Fatalf("got ok=%v msg=%q", ok, msg)
		}
	})

	// User scenario: only 2ms slower should NOT recommend switching.
	t.Run("system 2ms slower -> no switch", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false), mk("sys", 7, 1.0, true)}
		msg, ok := systemDNSVerdict(res)
		if !ok || strings.Contains(msg, "consider switching") {
			t.Fatalf("2ms gap should NOT recommend switching; got ok=%v msg=%q", ok, msg)
		}
		if !strings.Contains(msg, "no change needed") {
			t.Fatalf("expected \"no change needed\", got %q", msg)
		}
	})

	t.Run("system much slower -> switch", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false), mk("sys", 25, 1.0, true)}
		if msg, ok := systemDNSVerdict(res); !ok || !strings.Contains(msg, "consider switching") {
			t.Fatalf("got ok=%v msg=%q", ok, msg)
		}
	})

	// Close latency but clearly worse reliability should also recommend switching.
	t.Run("system flaky -> switch", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false), mk("sys", 6, 0.7, true)}
		if msg, ok := systemDNSVerdict(res); !ok || !strings.Contains(msg, "consider switching") {
			t.Fatalf("got ok=%v msg=%q", ok, msg)
		}
	})

	t.Run("system all failed -> switch", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false), mk("sys", 0, 0, true)}
		if msg, ok := systemDNSVerdict(res); !ok || !strings.Contains(msg, "failed every query") {
			t.Fatalf("got ok=%v msg=%q", ok, msg)
		}
	})

	// RFC 1918 and loopback addresses as system DNS should append a private-DNS
	// note when switching is recommended, so users on corporate networks (or
	// behind a local stub resolver like systemd-resolved's 127.0.0.53) are not
	// misled into a switch that breaks internal name resolution.
	for _, privateAddr := range []string{"10.0.0.1", "172.16.63.95", "192.168.1.1", "127.0.0.53", "127.0.0.1"} {
		addr := privateAddr
		t.Run("private system DNS switch includes note ("+addr+")", func(t *testing.T) {
			sys := dnsbench.Result{
				Name: "sys", Address: addr,
				AvgTime:     25 * time.Millisecond,
				SuccessRate: 1.0, Successes: 10, Total: 10, IsSystem: true,
			}
			res := []dnsbench.Result{mk("A", 5, 1.0, false), sys}
			msg, ok := systemDNSVerdict(res)
			if !ok {
				t.Fatal("expected ok=true")
			}
			if !strings.Contains(msg, "internal") {
				t.Fatalf("expected private DNS note in message; got %q", msg)
			}
		})
	}

	t.Run("private system DNS all-failed includes note", func(t *testing.T) {
		sys := dnsbench.Result{
			Name: "sys", Address: "172.16.63.95",
			AvgTime:     0,
			SuccessRate: 0, Successes: 0, Total: 10, IsSystem: true,
		}
		res := []dnsbench.Result{mk("A", 5, 1.0, false), sys}
		msg, ok := systemDNSVerdict(res)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !strings.Contains(msg, "internal") {
			t.Fatalf("expected private DNS note in message; got %q", msg)
		}
	})

	t.Run("public system DNS switch has no private note", func(t *testing.T) {
		res := []dnsbench.Result{mk("A", 5, 1.0, false), mk("8.8.8.8", 25, 1.0, true)}
		msg, ok := systemDNSVerdict(res)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if strings.Contains(msg, "internal") {
			t.Fatalf("public DNS should not trigger private note; got %q", msg)
		}
	})

	t.Run("private system DNS good-enough has no private note", func(t *testing.T) {
		sys := dnsbench.Result{
			Name: "sys", Address: "192.168.1.1",
			AvgTime:     7 * time.Millisecond,
			SuccessRate: 1.0, Successes: 10, Total: 10, IsSystem: true,
		}
		res := []dnsbench.Result{mk("A", 5, 1.0, false), sys}
		msg, ok := systemDNSVerdict(res)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if strings.Contains(msg, "internal") {
			t.Fatalf("good-enough verdict should not include private note; got %q", msg)
		}
	})

	t.Run("scoped link-local IPv6 system DNS switch includes note", func(t *testing.T) {
		sys := mk("sys", 25, 1.0, true)
		sys.Address = "fe80::1%eth0"

		res := []dnsbench.Result{
			mk("A", 5, 1.0, false),
			sys,
		}

		msg, ok := systemDNSVerdict(res)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !strings.Contains(msg, "internal") {
			t.Fatalf("expected internal DNS note for scoped link-local IPv6; got %q", msg)
		}
	})
}

// displayAddress should make the protocol obvious for DoT (which is otherwise a
// bare hostname users can't configure correctly), while leaving UDP IPs and the
// already-scheme'd DoH URLs untouched.
func TestDisplayAddress(t *testing.T) {
	cases := []struct {
		protocol      dnsbench.Protocol
		address, want string
	}{
		{dnsbench.UDP, "8.8.8.8", "8.8.8.8"},
		{dnsbench.DOT, "dns.google", "tls://dns.google"},
		{dnsbench.DOH, "https://dns.google/dns-query", "https://dns.google/dns-query"},
	}
	for _, c := range cases {
		got := displayAddress(dnsbench.Result{Address: c.address, Protocol: c.protocol})
		if got != c.want {
			t.Errorf("displayAddress(%s, %q) = %q; want %q", c.protocol, c.address, got, c.want)
		}
	}
}
