package dnsbench

import "net/netip"
import "net"
import "sync"

// NetworkCapabilities describes which IP address families are usable.
type NetworkCapabilities struct {
	IPv4 bool
	IPv6 bool
}

// DefaultServersForNetwork returns the built-in servers usable with the given
// network capabilities. Hostname-based servers are retained because they may
// resolve to either address family.
func DefaultServersForNetwork(capabilities NetworkCapabilities) []Server {
	servers := make([]Server, 0, len(DefaultServers))

	for _, server := range DefaultServers {
		addr, err := netip.ParseAddr(server.Address)
		if err == nil {
			if addr.Is4() && !capabilities.IPv4 {
				continue
			}
			if addr.Is6() && !capabilities.IPv6 {
				continue
			}
		}

		servers = append(servers, server)
	}

	return servers
}

func probeNetworkCapabilities(
	canDial func(network, address string) bool,
) NetworkCapabilities {
	return NetworkCapabilities{
		IPv4: canDial("udp4", "1.1.1.1:53"),
		IPv6: canDial("udp6", "[2606:4700:4700::1111]:53"),
	}
}

func canDialUDP(network, address string) bool {
	conn, err := net.Dial(network, address)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

var loadNetworkCapabilities = sync.OnceValue(func() NetworkCapabilities {
	return probeNetworkCapabilities(canDialUDP)
})

// DetectNetworkCapabilities returns the usable IP address families.
// The result is detected only once per process.
func DetectNetworkCapabilities() NetworkCapabilities {
	return loadNetworkCapabilities()
}
