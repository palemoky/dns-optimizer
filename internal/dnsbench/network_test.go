package dnsbench

import "testing"

// canDial is injected, so the probe's mapping from reachable networks to
// capabilities is verified without touching the real network.
func TestProbeNetworkCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		canDial func(network, address string) bool
		want    NetworkCapabilities
	}{
		{"IPv4 only", func(network, _ string) bool { return network == "udp4" }, NetworkCapabilities{IPv4: true}},
		{"IPv6 only", func(network, _ string) bool { return network == "udp6" }, NetworkCapabilities{IPv6: true}},
		{"dual stack", func(string, string) bool { return true }, NetworkCapabilities{IPv4: true, IPv6: true}},
		{"nothing reachable", func(string, string) bool { return false }, NetworkCapabilities{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := probeNetworkCapabilities(tt.canDial)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
