// Package dnsbench provides a concurrent benchmarking engine for DNS servers:
// concurrent queries, connection reuse, result aggregation and scoring. It
// contains no command-line or terminal presentation logic.
package dnsbench

import (
	"net"
	"net/url"
	"strings"
)

// Protocol identifies the DNS transport used by a server.
type Protocol string

// Supported protocols.
const (
	UDP  Protocol = "udp"
	DOT  Protocol = "dot"
	DOH  Protocol = "doh"
	DOH3 Protocol = "doh3" // DNS-over-HTTPS carried over HTTP/3 (QUIC)
)

// Server describes a DNS server to be tested.
type Server struct {
	Name     string
	Address  string
	Protocol Protocol
	IsSystem bool // whether this is the detected system default DNS
}

// Domain categories. These are stable internal keys; use CategoryLabel for
// localized display text.
const (
	CategoryDomestic = "domestic"
	CategoryForeign  = "foreign"
	CategoryCustom   = "custom"
)

// Domain is a test domain with its category.
type Domain struct{ Name, Category string }

// DefaultDomains is the built-in list of test domains (a balanced selection per category, deduplicated across same-company domains).
var DefaultDomains = []Domain{
	{"baidu.com", CategoryDomestic},
	{"qq.com", CategoryDomestic},
	{"taobao.com", CategoryDomestic},
	{"jd.com", CategoryDomestic},
	{"bilibili.com", CategoryDomestic},
	{"douyin.com", CategoryDomestic},
	{"weibo.com", CategoryDomestic},
	{"163.com", CategoryDomestic},
	{"zhihu.com", CategoryDomestic},
	{"aliyun.com", CategoryDomestic},

	{"google.com", CategoryForeign},
	{"youtube.com", CategoryForeign},
	{"github.com", CategoryForeign},
	{"facebook.com", CategoryForeign},
	{"x.com", CategoryForeign},
	{"apple.com", CategoryForeign},
	{"chatgpt.com", CategoryForeign},
	{"bing.com", CategoryForeign},
	{"tiktok.com", CategoryForeign},
	{"cloudflare.com", CategoryForeign},
}

// DefaultServers is the built-in list of default DNS servers.
var DefaultServers = []Server{
	{Name: "AliDNS 1 (UDP)", Address: "223.5.5.5", Protocol: UDP},
	{Name: "AliDNS 2 (UDP)", Address: "223.6.6.6", Protocol: UDP},
	{Name: "BaiduDNS (UDP)", Address: "180.76.76.76", Protocol: UDP},
	{Name: "DNSPod 1 (UDP)", Address: "119.28.28.28", Protocol: UDP},
	{Name: "DNSPod 2 (UDP)", Address: "119.29.29.29", Protocol: UDP},
	{Name: "114DNS 1 (UDP)", Address: "114.114.114.114", Protocol: UDP},
	{Name: "114DNS 2 (UDP)", Address: "114.114.115.115", Protocol: UDP},
	{Name: "Bytedance 1 (UDP)", Address: "180.184.1.1", Protocol: UDP},
	{Name: "Bytedance 2 (UDP)", Address: "180.184.2.2", Protocol: UDP},
	{Name: "OneDNS 1 (UDP)", Address: "117.50.10.10", Protocol: UDP},
	{Name: "OneDNS 2 (UDP)", Address: "52.80.52.52", Protocol: UDP},
	{Name: "Google 1 (UDP)", Address: "8.8.8.8", Protocol: UDP},
	{Name: "Google 2 (UDP)", Address: "8.8.4.4", Protocol: UDP},
	{Name: "Cloudflare 1 (UDP)", Address: "1.1.1.1", Protocol: UDP},
	{Name: "Cloudflare 2 (UDP)", Address: "1.0.0.1", Protocol: UDP},
	{Name: "OpenDNS 1 (UDP)", Address: "208.67.222.222", Protocol: UDP},
	{Name: "OpenDNS 2 (UDP)", Address: "208.67.220.220", Protocol: UDP},
	{Name: "Quad9 (UDP)", Address: "9.9.9.9", Protocol: UDP},

	// Plain DNS over IPv6.
	{Name: "AliDNS 1 (UDP/IPv6)", Address: "2400:3200::1", Protocol: UDP},
	{Name: "AliDNS 2 (UDP/IPv6)", Address: "2400:3200:baba::1", Protocol: UDP},
	{Name: "Google 1 (UDP/IPv6)", Address: "2001:4860:4860::8888", Protocol: UDP},
	{Name: "Google 2 (UDP/IPv6)", Address: "2001:4860:4860::8844", Protocol: UDP},
	{Name: "Cloudflare 1 (UDP/IPv6)", Address: "2606:4700:4700::1111", Protocol: UDP},
	{Name: "Cloudflare 2 (UDP/IPv6)", Address: "2606:4700:4700::1001", Protocol: UDP},
	{Name: "Quad9 1 (UDP/IPv6)", Address: "2620:fe::fe", Protocol: UDP},
	{Name: "Quad9 2 (UDP/IPv6)", Address: "2620:fe::9", Protocol: UDP},

	{Name: "AliDNS (DoT)", Address: "dns.alidns.com", Protocol: DOT},
	{Name: "DNSPod (DoT)", Address: "dot.pub", Protocol: DOT},
	{Name: "Google (DoT)", Address: "dns.google", Protocol: DOT},
	{Name: "Cloudflare (DoT)", Address: "one.one.one.one", Protocol: DOT},
	{Name: "Quad9 (DoT)", Address: "dns.quad9.net", Protocol: DOT},

	// All DoH servers use the RFC 8484 standard /dns-query endpoint (wire-format, application/dns-message).
	{Name: "AliDNS (DoH)", Address: "https://dns.alidns.com/dns-query", Protocol: DOH},
	{Name: "DNSPod (DoH)", Address: "https://doh.pub/dns-query", Protocol: DOH},
	{Name: "Cloudflare (DoH)", Address: "https://cloudflare-dns.com/dns-query", Protocol: DOH},
	{Name: "Google (DoH)", Address: "https://dns.google/dns-query", Protocol: DOH},
	{Name: "Quad9 (DoH)", Address: "https://dns.quad9.net/dns-query", Protocol: DOH},

	// DoH3 (DNS-over-HTTP/3). Same RFC 8484 /dns-query endpoint, carried over QUIC.
	{Name: "AliDNS (DoH3)", Address: "https://dns.alidns.com/dns-query", Protocol: DOH3},
	{Name: "Cloudflare (DoH3)", Address: "https://cloudflare-dns.com/dns-query", Protocol: DOH3},
	{Name: "Google (DoH3)", Address: "https://dns.google/dns-query", Protocol: DOH3},
}

// ParseServers parses a comma-separated custom server list, inferring each
// server's protocol from its URL scheme, preserving input order and skipping
// duplicates:
//
//	h3://host/path    -> DoH3 (rewritten to https:// for the request)
//	https://host/path -> DoH
//	tls://host        -> DoT (scheme stripped; host used for TLS SNI)
//	host or IP        -> UDP
//
// Entries that cannot be parsed are skipped.
func ParseServers(raw string) []Server {
	seen := make(map[string]struct{})
	var servers []Server
	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		s, ok := parseServer(entry)
		if !ok {
			continue
		}
		// Deduplicate on the parsed result rather than the raw text, so that
		// equivalent spellings of one server collapse into a single entry
		// (e.g. "2606:4700:4700::1111" and "udp://[2606:4700:4700::1111]").
		key := string(s.Protocol) + "|" + s.Address
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		servers = append(servers, s)
	}
	return servers
}

func normalizeHost(host string) string {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		unbracketed := host[1 : len(host)-1]
		if net.ParseIP(unbracketed) != nil {
			return unbracketed
		}
	}

	return host
}

// parseServer turns a single user-supplied entry into a Server.
func parseServer(entry string) (Server, bool) {
	switch {
	case strings.HasPrefix(entry, "h3://"):
		// HTTP/3 transport requires an https URL; rewrite the scheme but keep
		// the rest of the endpoint intact.
		addr := "https://" + strings.TrimPrefix(entry, "h3://")
		return Server{Name: customName(hostOf(addr), DOH3), Address: addr, Protocol: DOH3}, true

	case strings.HasPrefix(entry, "https://"):
		return Server{Name: customName(hostOf(entry), DOH), Address: entry, Protocol: DOH}, true

	case strings.HasPrefix(entry, "tls://"):
		host := normalizeHost(strings.TrimPrefix(entry, "tls://"))
		return Server{Name: customName(host, DOT), Address: host, Protocol: DOT}, true

	case strings.HasPrefix(entry, "udp://"):
		host := normalizeHost(strings.TrimPrefix(entry, "udp://"))
		return Server{Name: customName(host, UDP), Address: host, Protocol: UDP}, true

	default:
		host := normalizeHost(entry)
		return Server{Name: customName(host, UDP), Address: host, Protocol: UDP}, true
	}
}

// hostOf extracts the host portion of an https URL, falling back to the raw
// string when it cannot be parsed.
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}

// customName builds a display name for a user-supplied server, e.g.
// "dns.google (DoT)".
func customName(host string, p Protocol) string {
	label := map[Protocol]string{UDP: "UDP", DOT: "DoT", DOH: "DoH", DOH3: "DoH3"}[p]
	return host + " (" + label + ")"
}
