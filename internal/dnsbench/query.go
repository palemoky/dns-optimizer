package dnsbench

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

// querier performs a single query for a domain and returns how long it took.
type querier func(domain string) (time.Duration, error)

// newQuerier builds a reusable query function and its cleanup function for a
// server. The server hostname is resolved to an IP up front so that the system
// DNS resolution time is not counted in the measurement.
func newQuerier(server Server, timeout time.Duration) (querier, func()) {
	switch server.Protocol {
	case UDP:
		ip := resolveHost(server.Address, timeout)
		client := &dns.Client{Net: "udp", Timeout: timeout}
		return reusableExchange(client, net.JoinHostPort(ip, "53"))

	case DOT:
		ip := resolveHost(server.Address, timeout)
		client := &dns.Client{
			Net:       "tcp-tls",
			Timeout:   timeout,
			TLSConfig: &tls.Config{ServerName: server.Address},
		}
		return reusableExchange(client, net.JoinHostPort(ip, "853"))

	case DOH:
		client := &http.Client{Timeout: timeout}
		q := func(domain string) (time.Duration, error) {
			start := time.Now()
			err := dohQuery(client, server.Address, domain)
			return time.Since(start), err
		}
		return q, client.CloseIdleConnections

	case DOH3:
		// HTTP/3 (QUIC) transport. The endpoint is a regular https:// URL and the
		// wire-format query is identical to DoH; only the transport differs.
		rt := &http3.Transport{}
		client := &http.Client{Timeout: timeout, Transport: rt}
		q := func(domain string) (time.Duration, error) {
			start := time.Now()
			err := dohQuery(client, server.Address, domain)
			return time.Since(start), err
		}
		return q, func() { rt.Close() }

	default:
		q := func(domain string) (time.Duration, error) {
			return 0, fmt.Errorf("unsupported protocol: %s", server.Protocol)
		}
		return q, func() {}
	}
}

// reusableExchange maintains a persistent connection (a UDP socket or DoT TLS
// connection) reused across queries, so each measurement reflects a single
// query round-trip rather than a fresh handshake every time. A broken
// connection is reconnected and retried once. A querier is used sequentially
// within a single goroutine, so no locking is needed.
func reusableExchange(client *dns.Client, addr string) (querier, func()) {
	var conn *dns.Conn

	exchange := func(m *dns.Msg) (*dns.Msg, error) {
		if conn == nil {
			c, err := client.Dial(addr)
			if err != nil {
				return nil, err
			}
			conn = c
		}
		r, _, err := client.ExchangeWithConn(m, conn)
		return r, err
	}

	query := func(domain string) (time.Duration, error) {
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(domain), dns.TypeA)

		start := time.Now()
		r, err := exchange(m)
		if err != nil {
			// The connection may have been closed by the peer; drop it and retry once.
			if conn != nil {
				conn.Close()
				conn = nil
			}
			start = time.Now() // reset so the measurement reflects only the retry
			r, err = exchange(m)
		}
		elapsed := time.Since(start)

		if err != nil {
			if conn != nil {
				conn.Close()
				conn = nil
			}
			return elapsed, err
		}
		if r.Rcode != dns.RcodeSuccess {
			return elapsed, fmt.Errorf("DNS response code %s", dns.RcodeToString[r.Rcode])
		}
		return elapsed, nil
	}

	closeFn := func() {
		if conn != nil {
			conn.Close()
			conn = nil
		}
	}
	return query, closeFn
}

// dohQuery sends a single DoH query per RFC 8484 in wire-format
// (application/dns-message) and validates the returned DNS message (not just
// the HTTP status code). Unlike the inconsistent JSON dialects across vendors,
// wire-format is the DoH standard and is supported by every server on the
// /dns-query endpoint.
func dohQuery(client *http.Client, endpoint, domain string) error {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	wire, err := q.Pack()
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(wire))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024)) // drain so the connection can be reused
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}
	var r dns.Msg
	if err := r.Unpack(body); err != nil {
		return fmt.Errorf("failed to parse DoH response: %w", err)
	}
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("DNS response code %s", dns.RcodeToString[r.Rcode])
	}
	return nil
}

// selectResolvedHost picks the address to dial from a hostname's resolved
// addresses, preferring a family the local network can actually reach. IPv4
// wins on a dual-stack host because it is the more widely reachable path for
// the public resolvers tested here.
func selectResolvedHost(
	addrs []string,
	capabilities NetworkCapabilities,
) string {
	if capabilities.IPv4 {
		for _, candidate := range addrs {
			addr, err := netip.ParseAddr(candidate)
			if err == nil && addr.Is4() {
				return candidate
			}
		}
	}
	if capabilities.IPv6 {
		for _, candidate := range addrs {
			addr, err := netip.ParseAddr(candidate)
			if err == nil && addr.Is6() {
				return candidate
			}
		}
	}

	if len(addrs) > 0 {
		return addrs[0]
	}
	return ""
}

// resolveHost resolves a hostname to an IP address of a family the local
// network supports; if it is already an IP or resolution fails, it is returned
// unchanged.
func resolveHost(host string, timeout time.Duration) string {
	if _, err := netip.ParseAddr(host); err == nil {
		return host
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		return host
	}

	return selectResolvedHost(addrs, DetectNetworkCapabilities())
}
