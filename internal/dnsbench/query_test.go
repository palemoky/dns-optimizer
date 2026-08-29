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

func TestSelectResolvedHostForIPv6OnlyNetwork(t *testing.T) {
	got := selectResolvedHost(
		[]string{
			"192.0.2.1",
			"2001:db8::1",
		},
		NetworkCapabilities{
			IPv4: false,
			IPv6: true,
		},
	)

	want := "2001:db8::1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSelectResolvedHostForIPv4OnlyNetwork(t *testing.T) {
	got := selectResolvedHost(
		[]string{
			"2001:db8::1",
			"192.0.2.1",
		},
		NetworkCapabilities{
			IPv4: true,
			IPv6: false,
		},
	)

	want := "192.0.2.1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSelectResolvedHostForDualStackNetwork(t *testing.T) {
	got := selectResolvedHost(
		[]string{
			"2001:db8::1",
			"192.0.2.1",
		},
		NetworkCapabilities{
			IPv4: true,
			IPv6: true,
		},
	)

	want := "192.0.2.1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
