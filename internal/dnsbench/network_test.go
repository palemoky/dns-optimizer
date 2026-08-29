package dnsbench

import "testing"

func TestProbeNetworkCapabilitiesIPv4Only(t *testing.T) {
	got := probeNetworkCapabilities(func(network, address string) bool {
		return network == "udp4"
	})

	want := NetworkCapabilities{
		IPv4: true,
		IPv6: false,
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestProbeNetworkCapabilitiesIPv6Only(t *testing.T) {
	got := probeNetworkCapabilities(func(network, address string) bool {
		return network == "udp6"
	})

	want := NetworkCapabilities{
		IPv4: false,
		IPv6: true,
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
