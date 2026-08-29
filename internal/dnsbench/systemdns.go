package dnsbench

import (
	"fmt"
	"net/netip"
	"os/exec"
	"runtime"
	"strings"

	"github.com/miekg/dns"
)

// DetectSystemDNS probes the system's configured default DNS servers (handed
// out by the ISP or router) and returns Servers ready to be benchmarked.
// nameSingle is the display name for a single system DNS; nameFmt is a
// fmt.Sprintf pattern (with one %d) for numbering multiple entries.
// Returns nil when detection is not possible (the feature degrades gracefully).
func DetectSystemDNS(nameSingle, nameFmt string) []Server {
	var ips []string
	if runtime.GOOS == "windows" {
		ips = windowsDNS()
	} else {
		ips = systemDNSFromResolvConf("/etc/resolv.conf")
	}
	return buildSystemServers(ips, nameSingle, nameFmt)
}

// buildSystemServers deduplicates the IP list and converts it into Servers
// flagged with IsSystem.
func buildSystemServers(ips []string, nameSingle, nameFmt string) []Server {
	seen := make(map[string]struct{})
	var servers []Server
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if _, err := netip.ParseAddr(ip); err != nil {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		servers = append(servers, Server{Address: ip, Protocol: UDP, IsSystem: true})
	}
	// Naming: a single server is unnumbered, multiple servers are numbered.
	for i := range servers {
		if len(servers) == 1 {
			servers[i].Name = nameSingle
		} else {
			servers[i].Name = fmt.Sprintf(nameFmt, i+1)
		}
	}
	return servers
}

// systemDNSFromResolvConf parses a resolv.conf(5)-style file and returns its nameserver list.
func systemDNSFromResolvConf(path string) []string {
	cfg, err := dns.ClientConfigFromFile(path)
	if err != nil {
		return nil
	}
	return cfg.Servers
}

// windowsDNS reads the currently effective IPv4 and IPv6 DNS servers via PowerShell (best-effort).
func windowsDNS() []string {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"(Get-DnsClientServerAddress).ServerAddresses")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseWindowsDNSOutput(string(out))
}

// parseWindowsDNSOutput removes the legacy site-local DNS addresses that
// Windows automatically adds to some IPv6 interfaces.
func parseWindowsDNSOutput(out string) []string {
	var addresses []string
	for address := range strings.FieldsSeq(out) {
		if addr, err := netip.ParseAddr(address); err == nil && siteLocalV6.Contains(addr) {
			continue
		}
		addresses = append(addresses, address)
	}
	return addresses
}
