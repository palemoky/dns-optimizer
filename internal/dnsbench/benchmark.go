package dnsbench

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

// unreachableFailStreak is the number of consecutive failed queries (the warm-up
// counts as the first strike) after which a server is declared unreachable: the
// remaining queries are then recorded as failures immediately instead of each
// waiting out the full timeout. This stops a dead server — e.g. a DoH3 endpoint
// on a network that blocks QUIC — from dominating the total run time.
const unreachableFailStreak = 5

// errUnreachable marks the queries skipped after a server is ruled unreachable.
var errUnreachable = errors.New("server unreachable; remaining queries skipped")

// Options controls a single benchmark run.
type Options struct {
	Servers     []Server      // servers to test; uses network-compatible defaults when empty
	Domains     []Domain      // test domains
	Queries     int           // number of queries per domain
	Timeout     time.Duration // timeout per query
	Concurrency int           // maximum number of servers tested concurrently
}

// Result is the final benchmark result for a single DNS server.
type Result struct {
	Name, Address      string
	Protocol           Protocol // UDP, DOT or DOH
	AvgTime            time.Duration
	SuccessRate, Score float64
	Successes, Total   int
	IsSystem           bool // whether this is the system default DNS
}

// queryResult is the raw result of a single query.
type queryResult struct {
	server   Server
	duration time.Duration
	err      error
}

// serverStat aggregates the benchmark data for a single DNS server.
type serverStat struct {
	totalTime time.Duration
	successes int
	total     int
	address   string
	protocol  Protocol
	isSystem  bool
}

// ParseDomains splits, trims and deduplicates a custom domain list, preserving
// the original order. Custom domains are all assigned the CategoryCustom category.
func ParseDomains(raw string) []Domain {
	seen := make(map[string]struct{})
	var domains []Domain
	for d := range strings.SplitSeq(raw, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		domains = append(domains, Domain{Name: d, Category: CategoryCustom})
	}
	return domains
}

// Run benchmarks all servers concurrently and returns results sorted by score
// in descending order. progress is called after each completed query with that
// query's domain (may be nil); it drives the live progress UI.
func Run(opts Options, progress func(domain string)) []Result {
	servers := opts.Servers
	if len(servers) == 0 {
		servers = DefaultServersForNetwork(DetectNetworkCapabilities())
	}
	if progress == nil {
		progress = func(string) {}
	}
	concurrency := max(opts.Concurrency, 1)

	// One goroutine per server, querying sequentially inside it, so connections
	// (DoT/DoH) are reused and we avoid firing thousands of requests at once
	// that would contend with each other and pollute the latency measurement.
	// Server-level concurrency is bounded by sem.
	totalQueries := len(servers) * len(opts.Domains) * opts.Queries
	resultsChan := make(chan queryResult, totalQueries)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, server := range servers {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			benchmarkServer(server, opts, resultsChan, progress)
		})
	}

	wg.Wait()
	close(resultsChan)

	return calculateScores(aggregateResults(resultsChan))
}

// benchmarkServer runs all queries sequentially against a single server.
func benchmarkServer(server Server, opts Options, ch chan<- queryResult, progress func(domain string)) {
	q, closeFn := newQuerier(server, opts.Timeout)
	defer closeFn()
	runQueries(server, q, opts, ch, progress)
}

// runQueries drives the warm-up and the measured queries for a single server
// using the provided querier. It first does one warm-up query (excluded from the
// results) so that DoT/DoH/DoH3 establish their connections and the server
// hostname resolution is cached, bringing each protocol's measurement into a
// steady, comparable state. Once a server has failed unreachableFailStreak
// queries in a row it is declared unreachable and the remaining queries are
// recorded as failures without paying the per-query timeout.
func runQueries(server Server, q querier, opts Options, ch chan<- queryResult, progress func(domain string)) {
	// Warm-up (result discarded). A failed warm-up is the first strike toward the
	// unreachable check below.
	streak := 0
	if len(opts.Domains) > 0 {
		if _, err := q(opts.Domains[0].Name); err != nil {
			streak++
		}
	}

	unreachable := false
	for _, domain := range opts.Domains {
		for range opts.Queries {
			if unreachable {
				ch <- queryResult{server: server, err: errUnreachable}
				progress(domain.Name)
				continue
			}
			d, err := q(domain.Name)
			ch <- queryResult{server: server, duration: d, err: err}
			progress(domain.Name)
			if err != nil {
				streak++
				unreachable = streak >= unreachableFailStreak
			} else {
				streak = 0
			}
		}
	}
}

// aggregateResults collects and aggregates data from the channel.
func aggregateResults(resultsChan <-chan queryResult) map[string]*serverStat {
	serverStats := make(map[string]*serverStat)
	for result := range resultsChan {
		stats, ok := serverStats[result.server.Name]
		if !ok {
			stats = &serverStat{address: result.server.Address, protocol: result.server.Protocol, isSystem: result.server.IsSystem}
			serverStats[result.server.Name] = stats
		}
		stats.total++
		if result.err == nil {
			stats.totalTime += result.duration
			stats.successes++
		}
	}
	return serverStats
}

// calculateScores computes the final Result list and sorts it by score in descending order.
func calculateScores(serverStats map[string]*serverStat) []Result {
	var results []Result
	for name, stats := range serverStats {
		res := Result{
			Name: name, Address: stats.address, Protocol: stats.protocol,
			Successes: stats.successes, Total: stats.total, IsSystem: stats.isSystem,
		}

		if stats.successes > 0 {
			res.AvgTime = stats.totalTime / time.Duration(stats.successes)
			res.SuccessRate = float64(stats.successes) / float64(stats.total)
			latencyScore := 1.0 / res.AvgTime.Seconds()
			res.Score = latencyScore * (res.SuccessRate * res.SuccessRate)
		}
		results = append(results, res)
	}

	slices.SortFunc(results, func(a, b Result) int {
		return cmp.Compare(b.Score, a.Score)
	})

	return results
}
