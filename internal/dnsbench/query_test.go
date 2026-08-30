package dnsbench

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// startTestDNS starts a local UDP DNS server that replies via handler and returns its listen address.
func startTestDNS(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: handler}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })
	return pc.LocalAddr().String()
}

func TestReusableExchange(t *testing.T) {
	var calls atomic.Int32
	addr := startTestDNS(t, func(w dns.ResponseWriter, r *dns.Msg) {
		calls.Add(1)
		m := new(dns.Msg)
		m.SetReply(r)
		rr, _ := dns.NewRR(r.Question[0].Name + " 60 IN A 1.2.3.4")
		m.Answer = append(m.Answer, rr)
		w.WriteMsg(m)
	})

	client := &dns.Client{Net: "udp", Timeout: time.Second}
	q, closeFn := reusableExchange(client, addr)
	defer closeFn()

	for i := range 3 {
		d, err := q("example.com")
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		if d <= 0 {
			t.Fatalf("query %d: non-positive duration %v", i, d)
		}
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("server received %d queries, want 3", got)
	}
}

func TestReusableExchangeServfail(t *testing.T) {
	addr := startTestDNS(t, func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = dns.RcodeServerFailure
		w.WriteMsg(m)
	})

	client := &dns.Client{Net: "udp", Timeout: time.Second}
	q, closeFn := reusableExchange(client, addr)
	defer closeFn()

	if _, err := q("example.com"); err == nil {
		t.Fatal("expected error for SERVFAIL rcode")
	}
}

func TestDohQuery(t *testing.T) {
	client := &http.Client{Timeout: time.Second}

	// dnsWireReply parses the wire-format query from the request and builds a reply message with the given Rcode.
	dnsWireReply := func(t *testing.T, r *http.Request, rcode int) []byte {
		t.Helper()
		body, _ := io.ReadAll(r.Body)
		req := new(dns.Msg)
		if err := req.Unpack(body); err != nil {
			t.Fatalf("server failed to unpack request: %v", err)
		}
		reply := new(dns.Msg)
		reply.SetReply(req)
		reply.Rcode = rcode
		b, err := reply.Pack()
		if err != nil {
			t.Fatalf("pack reply: %v", err)
		}
		return b
	}

	t.Run("valid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/dns-message")
			w.Write(dnsWireReply(t, r, dns.RcodeSuccess))
		}))
		defer srv.Close()
		if err := dohQuery(client, srv.URL, "example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nxdomain rcode", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(dnsWireReply(t, r, dns.RcodeNameError))
		}))
		defer srv.Close()
		if err := dohQuery(client, srv.URL, "example.com"); err == nil {
			t.Fatal("expected error for NXDOMAIN rcode")
		}
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		if err := dohQuery(client, srv.URL, "example.com"); err == nil {
			t.Fatal("expected error for HTTP 500")
		}
	})

	t.Run("garbage body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not a dns message"))
		}))
		defer srv.Close()
		if err := dohQuery(client, srv.URL, "example.com"); err == nil {
			t.Fatal("expected error for non-wire body")
		}
	})
}

// The address family a hostname resolves to must follow what the local network
// can actually reach, with IPv4 preferred when both are available.
func TestSelectResolvedHost(t *testing.T) {
	const (
		v4 = "192.0.2.1"
		v6 = "2001:db8::1"
	)

	tests := []struct {
		name         string
		addrs        []string
		capabilities NetworkCapabilities
		want         string
	}{
		{"IPv6-only network takes the AAAA", []string{v4, v6}, NetworkCapabilities{IPv6: true}, v6},
		{"IPv4-only network takes the A", []string{v6, v4}, NetworkCapabilities{IPv4: true}, v4},
		{"dual stack prefers IPv4", []string{v6, v4}, NetworkCapabilities{IPv4: true, IPv6: true}, v4},
		// Neither family detected: the probe is likelier to be wrong than the
		// host to be unreachable, so dial something rather than nothing.
		{"no usable family falls back to the first address", []string{v6, v4}, NetworkCapabilities{}, v6},
		{"nothing resolved", nil, NetworkCapabilities{IPv4: true}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectResolvedHost(tt.addrs, tt.capabilities)
			if got != tt.want {
				t.Errorf("selectResolvedHost(%v, %+v) = %q, want %q", tt.addrs, tt.capabilities, got, tt.want)
			}
		})
	}
}
