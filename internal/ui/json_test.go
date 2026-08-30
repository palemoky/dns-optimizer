package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/palemoky/dnspick/internal/dnsbench"
)

func TestWriteJSON(t *testing.T) {
	results := []dnsbench.Result{
		{
			Name: "Fast", Address: "1.1.1.1", Protocol: dnsbench.UDP,
			AvgTime: 10 * time.Millisecond, SuccessRate: 1.0,
			Successes: 6, Total: 6, Score: 100,
		},
		{
			Name: "Slow", Address: "2.2.2.2", Protocol: dnsbench.UDP,
			AvgTime: 50 * time.Millisecond, SuccessRate: 0.8,
			Successes: 4, Total: 5, Score: 10, IsSystem: true,
		},
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, results, 3, 2, nil); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Verify the output is valid JSON and contains expected fields.
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	// Schema version.
	if v, ok := doc["schema"].(float64); !ok || int(v) != jsonSchemaVersion {
		t.Errorf("schema = %v, want %d", doc["schema"], jsonSchemaVersion)
	}

	// Number of results.
	arr, ok := doc["results"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("results = %v, want 2-element array", doc["results"])
	}

	// First result should be ranked 1.
	first := arr[0].(map[string]any)
	if first["name"] != "Fast" {
		t.Errorf("first result name = %v, want Fast", first["name"])
	}
	if first["rank"].(float64) != 1 {
		t.Errorf("first result rank = %v, want 1", first["rank"])
	}

	// Recommendation should have system_dns since one result is IsSystem.
	rec := doc["recommendation"].(map[string]any)
	if rec["system_dns"] == nil {
		t.Error("expected system_dns in recommendation")
	}
}

func TestLatencyMs(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want float64
	}{
		{10 * time.Millisecond, 10.0},
		{1500 * time.Microsecond, 1.5},
		{0, 0},
	}
	for _, tt := range tests {
		if got := latencyMs(tt.d); got != tt.want {
			t.Errorf("latencyMs(%v) = %v, want %v", tt.d, got, tt.want)
		}
	}
}

// The document must record why built-in servers were filtered out, and must omit
// the field entirely when a custom server list bypassed filtering.
func TestWriteJSONNetwork(t *testing.T) {
	results := []dnsbench.Result{
		{Name: "A", Address: "1.1.1.1", Protocol: dnsbench.UDP, AvgTime: time.Millisecond, SuccessRate: 1, Successes: 3, Total: 3, Score: 10},
	}

	t.Run("present", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteJSON(&buf, results, 3, 2, &NetworkInfo{IPv4: true, SkippedServers: 8}); err != nil {
			t.Fatalf("WriteJSON: %v", err)
		}

		var doc struct {
			Network *struct {
				IPv4           bool `json:"ipv4"`
				IPv6           bool `json:"ipv6"`
				SkippedServers int  `json:"skipped_servers"`
			} `json:"network"`
		}
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if doc.Network == nil {
			t.Fatal("network object missing")
		}
		if !doc.Network.IPv4 || doc.Network.IPv6 || doc.Network.SkippedServers != 8 {
			t.Errorf("network = %+v, want {true false 8}", *doc.Network)
		}
	})

	t.Run("omitted", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteJSON(&buf, results, 3, 2, nil); err != nil {
			t.Fatalf("WriteJSON: %v", err)
		}
		if strings.Contains(buf.String(), "network") {
			t.Errorf("network key should be omitted, got %s", buf.String())
		}
	})
}
