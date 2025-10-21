package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

const (
	UDP = "udp"
	DOT = "dot"
	DOH = "doh"
)

type DNSServer struct{ Name, Address, Protocol string }

type QueryResult struct {
	Server   DNSServer
	Domain   string
	Duration time.Duration
	Err      error
}

type BenchmarkResult struct {
	Name, Address      string
	AvgTime            time.Duration
	SuccessRate, Score float64
	Successes, Total   int
}

var (
	dnsServers = []DNSServer{
		{Name: "AliDNS 1 (UDP)", Address: "223.5.5.5", Protocol: UDP},
		{Name: "AliDNS 2 (UDP)", Address: "223.6.6.6", Protocol: UDP},
		{Name: "BaiduDNS (UDP)", Address: "180.76.76.76", Protocol: UDP},
		{Name: "DNSPod 1 (UDP)", Address: "119.28.28.28", Protocol: UDP},
		{Name: "DNSPod 2 (UDP)", Address: "119.29.29.29", Protocol: UDP},
		{Name: "114DNS 1 (UDP)", Address: "114.114.114.114", Protocol: UDP},
		{Name: "114DNS 2 (UDP)", Address: "114.114.115.115", Protocol: UDP},
		{Name: "114DNS Safe 1 (UDP)", Address: "114.114.114.119", Protocol: UDP},
		{Name: "114DNS Safe 2 (UDP)", Address: "114.114.115.119", Protocol: UDP},
		{Name: "114DNS Family 1 (UDP)", Address: "114.114.114.110", Protocol: UDP},
		{Name: "114DNS Family 2 (UDP)", Address: "114.114.115.110", Protocol: UDP},
		{Name: "Bytedance 1 (UDP)", Address: "180.184.1.1", Protocol: UDP},
		{Name: "Bytedance 2 (UDP)", Address: "180.184.2.2", Protocol: UDP},
		{Name: "Google 1 (UDP)", Address: "8.8.8.8", Protocol: UDP},
		{Name: "Google 2 (UDP)", Address: "8.8.4.4", Protocol: UDP},
		{Name: "Cloudflare 1 (UDP)", Address: "1.1.1.1", Protocol: UDP},
		{Name: "Cloudflare 2 (UDP)", Address: "1.0.0.1", Protocol: UDP},
		{Name: "Freenom 1 (UDP)", Address: "80.80.80.80", Protocol: UDP},
		{Name: "Freenom 2 (UDP)", Address: "80.80.81.81", Protocol: UDP},

		{Name: "AliDNS (DoT)", Address: "dns.alidns.com", Protocol: DOT},
		{Name: "DNSPod (DoT)", Address: "dot.pub", Protocol: DOT},
		{Name: "Google (DoT)", Address: "dns.google", Protocol: DOT},
		{Name: "Cloudflare 1 (DoT)", Address: "1.1.1.1", Protocol: DOT},
		{Name: "Cloudflare 2 (DoT)", Address: "one.one.one.one", Protocol: DOT},

		{Name: "AliDNS 1 (DoH)", Address: "https://dns.alidns.com/dns-query", Protocol: DOH},
		{Name: "AliDNS 2 (DoH)", Address: "https://223.5.5.5/dns-query", Protocol: DOH},
		{Name: "AliDNS 3 (DoH)", Address: "https://223.6.6.6/dns-query", Protocol: DOH},
		{Name: "DNSPod (DoH)", Address: "https://doh.pub/dns-query", Protocol: DOH},
		{Name: "Cloudflare 1 (DoH)", Address: "https://cloudflare-dns.com/dns-query", Protocol: DOH},
		{Name: "Cloudflare 2 (DoH)", Address: "https://1.1.1.1/dns-query", Protocol: DOH},
		{Name: "Cloudflare 3 (DoH)", Address: "https://1.0.0.1/dns-query", Protocol: DOH},
		{Name: "Google (DoH)", Address: "https://dns.google/resolve", Protocol: DOH},
	}

	defaultDomains = []string{
		"douyin.com", "kuaishou.com", "baidu.com", "taobao.com", "mi.com", "aliyun.com",
		"bilibili.com", "jd.com", "qq.com", "ithome.com", "hupu.com", "feishu.cn",
		"sohu.com", "163.com", "sina.com", "weibo.com", "bilibili.com", "xiaohongshu.com",
		"douban.com", "zhihu.com", "youku.com", "youdao.com", "mp.weixin.qq.com",
		"iqiyi.com", "v.qq.com", "y.qq.com", "www.ctrip.com", "autohome.com.cn",
		"google.com", "facebook.com", "x.com", "github.com", "youtube.com", "chatgpt.com",
		"apple.com", "bing.com", "tiktok.com",
	}
)

var rootCmd = &cobra.Command{
	Use:   "dns-optimizer",
	Short: "一个跨平台的 DNS 选优工具",
	Long:  `通过对一组常用域名进行并发测试，为您的网络环境推荐最快、最稳定的DNS服务器。`,
	Run:   runBenchmark, // Cobra会调用这个函数来执行主逻辑
}

var (
	domainsStr       string
	queriesPerDomain int
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&domainsStr, "domains", "d", strings.Join(defaultDomains, ","), "用于测试的域名列表, 以逗号分隔")
	rootCmd.PersistentFlags().IntVarP(&queriesPerDomain, "queries", "q", 3, "每个域名的查询次数")
}

// runBenchmark
func runBenchmark(cmd *cobra.Command, args []string) {
	testDomains := strings.Split(domainsStr, ",")
	totalQueries := len(dnsServers) * len(testDomains) * queriesPerDomain

	fmt.Println("DNS 选优工具: 开始对", len(dnsServers), "个 DNS 服务器进行综合基准测试...")
	fmt.Printf("测试域名 (%d个): %s\n", len(testDomains), strings.Join(testDomains, ", "))
	fmt.Printf("每个域名查询 %d 次, 总计 %d 次查询。\n\n", queriesPerDomain, totalQueries)

	// --- 1. 初始化进度条 ---
	bar := progressbar.NewOptions(totalQueries,
		progressbar.OptionSetWriter(color.Output),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription("[cyan]Running queries[reset]"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// --- 2. 并发执行测试 ---
	resultsChan := make(chan QueryResult, totalQueries)
	var wg sync.WaitGroup

	for _, server := range dnsServers {
		for _, domain := range testDomains {
			for range queriesPerDomain {
				wg.Add(1)
				go func(s DNSServer, d string) {
					defer wg.Done()
					query(s, d, resultsChan)
					bar.Add(1)
				}(server, domain)
			}
		}
	}

	wg.Wait()
	close(resultsChan)
	fmt.Println()

	// --- 3. 聚合结果时显示 Spinner 动画 ---
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " 正在聚合和计算评分..."
	s.Start()

	serverStats := aggregateResults(resultsChan)

	s.Stop()
	fmt.Println()

	// --- 4. 计算最终结果和评分 ---
	benchmarkResults := calculateScores(serverStats)

	// --- 5. 使用 tablewriter 打印结果 ---
	fmt.Println("--- 综合测试结果 ---")
	printResultsTable(benchmarkResults)

	fmt.Println("\n--- 最佳DNS推荐 (Top 3) ---")
	printRecommendations(benchmarkResults)
}

// aggregateResults 负责从 channel 收集并聚合数据
func aggregateResults(resultsChan <-chan QueryResult) map[string]*struct {
	totalTime        time.Duration
	successes, total int
	address          string
} {
	serverStats := make(map[string]*struct {
		totalTime time.Duration
		successes int
		total     int
		address   string
	})
	for result := range resultsChan {
		if _, ok := serverStats[result.Server.Name]; !ok {
			serverStats[result.Server.Name] = &struct {
				totalTime time.Duration
				successes int
				total     int
				address   string
			}{address: result.Server.Address}
		}
		stats := serverStats[result.Server.Name]
		stats.total++
		if result.Err == nil {
			stats.totalTime += result.Duration
			stats.successes++
		}
	}
	return serverStats
}

// calculateScores 计算最终的 BenchmarkResult 列表
func calculateScores(serverStats map[string]*struct {
	totalTime        time.Duration
	successes, total int
	address          string
}) []BenchmarkResult {
	var benchmarkResults []BenchmarkResult
	for name, stats := range serverStats {
		res := BenchmarkResult{
			Name: name, Address: stats.address, Successes: stats.successes, Total: stats.total,
		}

		if stats.successes > 0 {
			res.AvgTime = stats.totalTime / time.Duration(stats.successes)
			res.SuccessRate = float64(stats.successes) / float64(stats.total)
			latencyScore := 1.0 / res.AvgTime.Seconds()
			res.Score = latencyScore * (res.SuccessRate * res.SuccessRate)
		}
		benchmarkResults = append(benchmarkResults, res)
	}

	sort.Slice(benchmarkResults, func(i, j int) bool {
		return benchmarkResults[i].Score > benchmarkResults[j].Score
	})

	return benchmarkResults
}

// printResultsTable 使用 tablewriter 打印漂亮的表格
func printResultsTable(results []BenchmarkResult) {
	table := tablewriter.NewWriter(os.Stdout)
	data := [][]string{
		{"DNS服务器", "地址", "平均延迟", "成功率", "综合评分"},
	}

	for _, r := range results {
		// 准备原始字符串
		rateStr := fmt.Sprintf("%.1f%% (%d/%d)", r.SuccessRate*100, r.Successes, r.Total)
		avgTimeStr := r.AvgTime.Round(time.Microsecond).String()
		scoreStr := fmt.Sprintf("%.2f", r.Score)

		// 根据成功率，为字符串添加颜色
		green := color.New(color.FgGreen).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		if r.SuccessRate < 1.0 {
			rateStr = red(rateStr)
		} else {
			rateStr = green(rateStr)
		}

		// 将处理好的字符串切片 append 到表格
		table.Append([]string{
			r.Name,
			r.Address,
			avgTimeStr,
			rateStr,
			scoreStr,
		})
	}
	table.Header(data[0])
	table.Bulk(data[1:])
	table.Render()
}

// printRecommendations 打印推荐
func printRecommendations(results []BenchmarkResult) {
	green := color.New(color.FgGreen, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan) 
	red := color.New(color.FgRed)

	found := 0
	for i, best := range results {
		if best.SuccessRate > 0.98 {
			var c *color.Color
			switch i {
			case 0:
				c = green
			case 1:
				c = yellow
			default:
				c = cyan
			}
			c.Printf("#%d: %s (%s)\n", i+1, best.Name, best.Address)
			fmt.Printf("    综合评分: %.2f, 平均延迟: %s, 成功率: %.1f%%\n",
				best.Score, best.AvgTime.Round(time.Microsecond).String(), best.SuccessRate*100)
			found++
		}
		if found >= 3 {
			break
		}
	}
	if found == 0 {
		red.Println("没有找到表现足够好的DNS服务器，请检查网络连接。")
	}
}

// query
func query(server DNSServer, domain string, ch chan<- QueryResult) {
	var duration time.Duration
	var err error
	start := time.Now()

	switch server.Protocol {
	case UDP:
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return net.DialTimeout("udp", net.JoinHostPort(server.Address, "53"), 2*time.Second)
			},
		}
		_, err = resolver.LookupIP(context.Background(), "ip4", domain)
	case DOT:
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", net.JoinHostPort(server.Address, "853"), nil)
			},
		}
		_, err = resolver.LookupIP(context.Background(), "ip4", domain)
	case DOH:
		client := &http.Client{Timeout: 2 * time.Second}
		reqURL := fmt.Sprintf("%s?name=%s&type=A", server.Address, domain)
		req, _ := http.NewRequest("GET", reqURL, nil)
		req.Header.Set("Accept", "application/dns-json")
		resp, doErr := client.Do(req)
		err = doErr
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("HTTP status %d", resp.StatusCode)
			}
		}
	default:
		err = fmt.Errorf("不支持的协议: %s", server.Protocol)
	}
	duration = time.Since(start)

	ch <- QueryResult{
		Server:   server,
		Domain:   domain,
		Duration: duration,
		Err:      err,
	}
}

// main
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
