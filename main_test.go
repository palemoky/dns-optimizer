package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/palemoky/dnspick/internal/ui"
)

func TestLangFromArgs(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"--version"}, ""},
		{[]string{"--lang", "zh"}, "zh"},
		{[]string{"--lang=zh"}, "zh"},
		{[]string{"--lang=en"}, "en"},
		{[]string{"-d", "example.com", "--lang", "zh"}, "zh"},
		{[]string{"--lang"}, ""}, // --lang without value
	}
	for _, tt := range tests {
		if got := langFromArgs(tt.args); got != tt.want {
			t.Errorf("langFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// A shorter-than-usual server list must be explained, and a complete one must
// not print a spurious notice.
func TestNetworkNotice(t *testing.T) {
	tests := []struct {
		name    string
		network *ui.NetworkInfo
		want    string // substring the notice must contain; "" means no notice
	}{
		{"custom server list", nil, ""},
		{"dual stack", &ui.NetworkInfo{IPv4: true, IPv6: true}, ""},
		{"no IPv6", &ui.NetworkInfo{IPv4: true, SkippedServers: 8}, "IPv6"},
		{"no IPv4", &ui.NetworkInfo{IPv6: true, SkippedServers: 18}, "IPv4"},
		// Neither family detected: DefaultServersForNetwork keeps the full list,
		// so there is nothing to explain.
		{"nothing detected", &ui.NetworkInfo{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := networkNotice(tt.network)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("expected no notice, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("notice %q does not mention %q", got, tt.want)
			}
			if !strings.Contains(got, strconv.Itoa(tt.network.SkippedServers)) {
				t.Errorf("notice %q omits the skipped count", got)
			}
		})
	}
}
