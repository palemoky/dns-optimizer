package ui

import (
	"encoding/json"
	"io"
	"time"

	"github.com/palemoky/dnspick/internal/dnsbench"
)

// jsonSchemaVersion is the version of the --json document structure. It is the
// stability contract for automated consumers: bump it on any backward-incompatible
// change so they can guard on it.
const jsonSchemaVersion = 2

// NetworkInfo describes the address families detected on this host and how many
// built-in servers were skipped because of them, so that a shorter-than-expected
// server list is explainable rather than mysterious.
type NetworkInfo struct {
	IPv4           bool
	IPv6           bool
	SkippedServers int
}

// jsonReport is the top-level machine-readable benchmark output.
type jsonReport struct {
	Schema           int                `json:"schema"`
	QueriesPerDomain int                `json:"queries_per_domain"`
	ServersTested    int                `json:"servers_tested"`
	DomainsTested    int                `json:"domains_tested"`
	Network          *jsonNetwork       `json:"network,omitempty"`
	Results          []jsonResult       `json:"results"`
	Recommendation   jsonRecommendation `json:"recommendation"`
}

// jsonNetwork reports which address families were usable and how many built-in
// servers were filtered out as a result. It is omitted when a custom server list
// was supplied, since that list is never filtered.
type jsonNetwork struct {
	IPv4           bool `json:"ipv4"`
	IPv6           bool `json:"ipv6"`
	SkippedServers int  `json:"skipped_servers"`
}

// jsonResult is a single server's result. Latency is expressed in milliseconds
// (rounded to microsecond precision) so consumers needn't parse Go durations.
type jsonResult struct {
	Rank         int               `json:"rank"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Protocol     dnsbench.Protocol `json:"protocol"`
	IsSystem     bool              `json:"is_system"`
	AvgLatencyMs float64           `json:"avg_latency_ms"`
	SuccessRate  float64           `json:"success_rate"`
	Successes    int               `json:"successes"`
	Total        int               `json:"total"`
	Score        float64           `json:"score"`
}

type jsonRecommendation struct {
	Top       []jsonTop          `json:"top"`
	SystemDNS *jsonSystemVerdict `json:"system_dns,omitempty"`
}

type jsonTop struct {
	Rank     int               `json:"rank"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Protocol dnsbench.Protocol `json:"protocol"`
}

// jsonSystemVerdict is the conclusion about the system default DNS. Verdict is a
// stable enum (see VerdictKind); should_switch is the actionable boolean.
// is_internal_dns is true when the address is a private (RFC 1918 / RFC 4193) or
// loopback resolver, signalling that switching to an external DNS may break
// internal hostname resolution.
type jsonSystemVerdict struct {
	Name          string      `json:"name"`
	Address       string      `json:"address"`
	Rank          int         `json:"rank"`
	Verdict       VerdictKind `json:"verdict"`
	ShouldSwitch  bool        `json:"should_switch"`
	IsInternalDNS bool        `json:"is_internal_dns"`
}

// WriteJSON serializes the benchmark results as indented JSON to w. domains is the
// number of domains tested; network describes the address families detected for
// the built-in server list and may be nil. The rest of the metadata is derived
// from results, which must be sorted by score in descending order.
func WriteJSON(w io.Writer, results []dnsbench.Result, queriesPerDomain, domains int, network *NetworkInfo) error {
	rep := jsonReport{
		Schema:           jsonSchemaVersion,
		QueriesPerDomain: queriesPerDomain,
		ServersTested:    len(results),
		DomainsTested:    domains,
		Results:          make([]jsonResult, len(results)),
	}

	if network != nil {
		rep.Network = &jsonNetwork{
			IPv4:           network.IPv4,
			IPv6:           network.IPv6,
			SkippedServers: network.SkippedServers,
		}
	}

	for i, r := range results {
		rep.Results[i] = jsonResult{
			Rank:         i + 1,
			Name:         r.Name,
			Address:      r.Address,
			Protocol:     r.Protocol,
			IsSystem:     r.IsSystem,
			AvgLatencyMs: latencyMs(r.AvgTime),
			SuccessRate:  r.SuccessRate,
			Successes:    r.Successes,
			Total:        r.Total,
			Score:        r.Score,
		}
	}

	for _, best := range topRecommendations(results) {
		rep.Recommendation.Top = append(rep.Recommendation.Top, jsonTop{
			Rank:     best.Rank,
			Name:     best.Name,
			Address:  best.Address,
			Protocol: best.Protocol,
		})
	}

	if e, ok := evalSystemDNS(results); ok {
		rep.Recommendation.SystemDNS = &jsonSystemVerdict{
			Name:          e.sys.Name,
			Address:       e.sys.Address,
			Rank:          e.rank,
			Verdict:       e.kind,
			ShouldSwitch:  e.kind == VerdictSwitch || e.kind == VerdictAllFailed,
			IsInternalDNS: isInternalDNS(e.sys.Address),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// latencyMs converts a duration to milliseconds, rounded to microsecond precision
// to match the human-readable table.
func latencyMs(d time.Duration) float64 {
	return float64(d.Round(time.Microsecond)) / float64(time.Millisecond)
}
