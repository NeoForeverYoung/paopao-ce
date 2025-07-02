package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	BaseURL       = "http://127.0.0.1:8008"
	TestDuration  = 120 * time.Second // 2分钟测试
	MaxConcurrent = 100               // 增加并发数
)

type TestResult struct {
	RequestCount  int64
	SuccessCount  int64
	ErrorCount    int64
	TotalLatency  int64
	ResponseTimes []int64
	StartTime     time.Time
	EndTime       time.Time
	mutex         sync.Mutex
	ScenarioStats map[string]*ScenarioStat
}

type ScenarioStat struct {
	Name         string
	Count        int64
	SuccessCount int64
	TotalLatency int64
}

type FeedsResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Data    struct {
		List  []interface{} `json:"list"`
		Pager struct {
			Page      int `json:"page"`
			PageSize  int `json:"page_size"`
			TotalRows int `json:"total_rows"`
		} `json:"pager"`
	} `json:"data"`
}

var (
	searchKeywords = []string{
		"技术", "生活", "美食", "旅行", "电影", "读书", "音乐", "摄影", "设计", "编程",
		"创业", "投资", "健身", "美妆", "育儿", "宠物", "游戏", "学习", "工作", "感悟",
		"今天", "分享", "推荐", "心得", "笔记", "体验", "感想", "见闻", "心情", "日常",
	}
)

func main() {
	fmt.Println("🚀 开始进行paopao-ce真实场景压测...")
	fmt.Printf("目标URL: %s\n", BaseURL)
	fmt.Printf("测试持续时间: %v\n", TestDuration)
	fmt.Printf("最大并发数: %d\n", MaxConcurrent)

	// 预热
	fmt.Println("正在进行预热...")
	warmup()

	// 初始化结果
	result := &TestResult{
		StartTime:     time.Now(),
		ScenarioStats: make(map[string]*ScenarioStat),
	}

	// 初始化场景统计
	scenarios := []string{
		"最新推文首页", "最新推文翻页", "热门推文首页", "热门推文翻页",
		"搜索推文", "关注推文流", "混合场景",
	}
	for _, scenario := range scenarios {
		result.ScenarioStats[scenario] = &ScenarioStat{Name: scenario}
	}

	fmt.Println("\n=== 开始真实场景压测 ===")

	// 控制并发数
	semaphore := make(chan struct{}, MaxConcurrent)
	var wg sync.WaitGroup

	// 开始时间
	start := time.Now()

	// 持续发送请求，模拟真实用户行为
	for time.Since(start) < TestDuration {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// 获取许可
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 模拟真实用户行为模式
			simulateUserBehavior(result)
		}()

		// 控制发送频率
		time.Sleep(time.Duration(rand.Intn(20)+5) * time.Millisecond)
	}

	// 等待所有请求完成
	wg.Wait()
	result.EndTime = time.Now()

	// 打印结果
	printDetailedResults(result)
}

func simulateUserBehavior(result *TestResult) {
	// 随机选择用户行为模式
	behaviors := []func(*TestResult){
		browseNewestFeeds,    // 30% - 浏览最新推文
		browseHotFeeds,       // 25% - 浏览热门推文
		searchFeeds,          // 20% - 搜索推文
		browseFollowingFeeds, // 15% - 浏览关注推文
		deepBrowsing,         // 10% - 深度浏览（翻页）
	}

	weights := []int{30, 25, 20, 15, 10}
	totalWeight := 100

	r := rand.Intn(totalWeight)
	cumulative := 0

	for i, weight := range weights {
		cumulative += weight
		if r < cumulative {
			behaviors[i](result)
			return
		}
	}

	// 默认行为
	browseNewestFeeds(result)
}

func browseNewestFeeds(result *TestResult) {
	endpoint := "/v1/posts?style=newest&page=1&page_size=20"
	executeRequest(BaseURL+endpoint, result, "最新推文首页")
}

func browseHotFeeds(result *TestResult) {
	endpoint := "/v1/posts?style=hots&page=1&page_size=20"
	executeRequest(BaseURL+endpoint, result, "热门推文首页")
}

func searchFeeds(result *TestResult) {
	keyword := searchKeywords[rand.Intn(len(searchKeywords))]
	endpoint := "/v1/posts?query=" + url.QueryEscape(keyword) + "&style=newest&page=1&page_size=20"
	executeRequest(BaseURL+endpoint, result, "搜索推文")
}

func browseFollowingFeeds(result *TestResult) {
	endpoint := "/v1/posts?style=following&page=1&page_size=20"
	executeRequest(BaseURL+endpoint, result, "关注推文流")
}

func deepBrowsing(result *TestResult) {
	// 模拟用户翻页行为
	page := rand.Intn(5) + 2 // 第2-6页
	style := []string{"newest", "hots"}[rand.Intn(2)]

	endpoint := fmt.Sprintf("/v1/posts?style=%s&page=%d&page_size=20", style, page)
	scenarioName := "最新推文翻页"
	if style == "hots" {
		scenarioName = "热门推文翻页"
	}

	executeRequest(BaseURL+endpoint, result, scenarioName)
}

func warmup() {
	client := &http.Client{Timeout: 10 * time.Second}

	endpoints := []string{
		"/v1/posts?style=newest&page=1&page_size=5",
		"/v1/posts?style=hots&page=1&page_size=5",
		"/v1/posts?query=技术&style=newest&page=1&page_size=5",
	}

	for _, endpoint := range endpoints {
		resp, err := client.Get(BaseURL + endpoint)
		if err != nil {
			fmt.Printf("预热请求失败: %v\n", err)
			continue
		}
		resp.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("预热完成")
}

func executeRequest(url string, result *TestResult, scenarioName string) {
	client := &http.Client{Timeout: 30 * time.Second}

	start := time.Now()
	atomic.AddInt64(&result.RequestCount, 1)

	// 更新场景统计
	result.mutex.Lock()
	if stat, exists := result.ScenarioStats[scenarioName]; exists {
		atomic.AddInt64(&stat.Count, 1)
	}
	result.mutex.Unlock()

	resp, err := client.Get(url)
	if err != nil {
		atomic.AddInt64(&result.ErrorCount, 1)
		log.Printf("请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		atomic.AddInt64(&result.ErrorCount, 1)
		log.Printf("读取响应失败: %v", err)
		return
	}

	latency := time.Since(start).Milliseconds()

	// 检查响应状态
	if resp.StatusCode == 200 {
		var feedsResp FeedsResponse
		if err := json.Unmarshal(body, &feedsResp); err != nil {
			atomic.AddInt64(&result.ErrorCount, 1)
			log.Printf("JSON解析失败: %v", err)
			return
		}

		if feedsResp.Code == 0 {
			atomic.AddInt64(&result.SuccessCount, 1)

			// 更新场景成功统计
			result.mutex.Lock()
			if stat, exists := result.ScenarioStats[scenarioName]; exists {
				atomic.AddInt64(&stat.SuccessCount, 1)
				atomic.AddInt64(&stat.TotalLatency, latency)
			}
			result.mutex.Unlock()
		} else {
			atomic.AddInt64(&result.ErrorCount, 1)
			log.Printf("业务错误: code=%d, msg=%s", feedsResp.Code, feedsResp.Message)
			return
		}
	} else {
		atomic.AddInt64(&result.ErrorCount, 1)
		log.Printf("HTTP错误: status=%d", resp.StatusCode)
		return
	}

	// 记录响应时间
	result.mutex.Lock()
	result.ResponseTimes = append(result.ResponseTimes, latency)
	result.mutex.Unlock()

	atomic.AddInt64(&result.TotalLatency, latency)
}

func printDetailedResults(result *TestResult) {
	duration := result.EndTime.Sub(result.StartTime)

	fmt.Printf("\n🎯 真实场景压测结果总览\n")
	fmt.Printf("========================================\n")
	fmt.Printf("⏱️  测试持续时间: %v\n", duration)
	fmt.Printf("📤 总请求数: %d\n", result.RequestCount)
	fmt.Printf("✅ 成功请求数: %d\n", result.SuccessCount)
	fmt.Printf("❌ 失败请求数: %d\n", result.ErrorCount)

	if result.RequestCount > 0 {
		successRate := float64(result.SuccessCount) / float64(result.RequestCount) * 100
		fmt.Printf("✔️  总体成功率: %.2f%%\n", successRate)

		qps := float64(result.RequestCount) / duration.Seconds()
		fmt.Printf("⚡ 总体QPS: %.2f\n", qps)

		if result.SuccessCount > 0 {
			avgLatency := float64(result.TotalLatency) / float64(result.SuccessCount)
			fmt.Printf("⏰ 总体平均响应时间: %.2f ms\n", avgLatency)

			// 计算百分位数
			if len(result.ResponseTimes) > 0 {
				sort.Slice(result.ResponseTimes, func(i, j int) bool {
					return result.ResponseTimes[i] < result.ResponseTimes[j]
				})

				p50 := percentile(result.ResponseTimes, 0.5)
				p95 := percentile(result.ResponseTimes, 0.95)
				p99 := percentile(result.ResponseTimes, 0.99)

				fmt.Printf("📈 P50响应时间: %d ms\n", p50)
				fmt.Printf("📈 P95响应时间: %d ms\n", p95)
				fmt.Printf("📈 P99响应时间: %d ms\n", p99)
			}
		}
	}

	// 分场景详细统计
	fmt.Printf("\n📊 分场景详细统计\n")
	fmt.Printf("========================================\n")

	for _, stat := range result.ScenarioStats {
		if stat.Count > 0 {
			fmt.Printf("\n🎭 场景: %s\n", stat.Name)
			fmt.Printf("   📤 请求数: %d\n", stat.Count)
			fmt.Printf("   ✅ 成功数: %d\n", stat.SuccessCount)

			if stat.Count > 0 {
				successRate := float64(stat.SuccessCount) / float64(stat.Count) * 100
				fmt.Printf("   ✔️  成功率: %.2f%%\n", successRate)

				qps := float64(stat.Count) / duration.Seconds()
				fmt.Printf("   ⚡ QPS: %.2f\n", qps)
			}

			if stat.SuccessCount > 0 {
				avgLatency := float64(stat.TotalLatency) / float64(stat.SuccessCount)
				fmt.Printf("   ⏰ 平均响应时间: %.2f ms\n", avgLatency)
			}
		}
	}

	// 性能评估
	fmt.Printf("\n📈 性能评估\n")
	fmt.Printf("========================================\n")

	totalQPS := float64(result.RequestCount) / duration.Seconds()
	avgLatency := float64(result.TotalLatency) / float64(result.SuccessCount)

	if totalQPS > 80 {
		fmt.Printf("🔥 QPS表现: 优秀 (%.2f > 80)\n", totalQPS)
	} else if totalQPS > 50 {
		fmt.Printf("👍 QPS表现: 良好 (%.2f > 50)\n", totalQPS)
	} else {
		fmt.Printf("⚠️  QPS表现: 需要优化 (%.2f < 50)\n", totalQPS)
	}

	if avgLatency < 10 {
		fmt.Printf("🔥 响应时间: 优秀 (%.2fms < 10ms)\n", avgLatency)
	} else if avgLatency < 50 {
		fmt.Printf("👍 响应时间: 良好 (%.2fms < 50ms)\n", avgLatency)
	} else {
		fmt.Printf("⚠️  响应时间: 需要优化 (%.2fms > 50ms)\n", avgLatency)
	}

	if result.SuccessCount == result.RequestCount {
		fmt.Printf("🎯 稳定性: 优秀 (100%%成功率)\n")
	} else if float64(result.SuccessCount)/float64(result.RequestCount) > 0.99 {
		fmt.Printf("👍 稳定性: 良好 (>99%%成功率)\n")
	} else {
		fmt.Printf("⚠️  稳定性: 需要优化 (成功率过低)\n")
	}

	fmt.Printf("\n🎉 压测完成！\n")
}

func percentile(sortedTimes []int64, p float64) int64 {
	if len(sortedTimes) == 0 {
		return 0
	}

	index := int(float64(len(sortedTimes)) * p)
	if index >= len(sortedTimes) {
		index = len(sortedTimes) - 1
	}

	return sortedTimes[index]
}
