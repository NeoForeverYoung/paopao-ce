# paopao-ce Feeds流性能压测方案

## 📋 概述

本文档详细介绍如何对paopao-ce的feeds流功能进行性能压测，包括假数据生成、压测脚本编写、性能分析等完整方案。

## 🎯 压测目标

### 主要指标
- **QPS**: 每秒查询数（queries per second）
- **响应时间**: P50、P95、P99延迟
- **并发处理能力**: 最大并发用户数
- **数据库性能**: CPU、内存、I/O使用率
- **缓存命中率**: Redis/BigCache命中率

### 测试场景
1. **小规模测试**: 1000用户，10000推文，模拟轻量级社交
2. **中规模测试**: 10000用户，100000推文，模拟中等社交平台
3. **大规模测试**: 100000用户，1000000推文，模拟大型社交平台

## 🗄️ 数据库表结构分析

### 核心表关系
```
p_user (用户表)
├── p_post (推文表) - user_id关联
├── p_following (关注表) - user_id, follow_id
├── p_contact (好友表) - user_id, friend_id
└── p_post_content (推文内容表) - post_id关联
```

### 关键字段说明
- **p_post.visibility**: 可见性控制 (0私密, 50好友, 60关注, 90公开)
- **p_post_content.type**: 内容类型 (1标题, 2文本, 3图片, 4视频, 7附件, 8收费附件)
- **p_contact.status**: 好友状态 (2为已好友)
- **feeds查询核心逻辑**: 聚合用户关注和好友的推文

## 📊 假数据生成方案

### 1. 数据规模设计

#### 小规模测试 (1K用户)
- 用户数: 1,000
- 推文数: 10,000 (平均每人10条)
- 关注关系: 5,000 (平均每人关注5人)
- 好友关系: 2,000 (平均每人2个好友)

#### 中规模测试 (10K用户)
- 用户数: 10,000
- 推文数: 100,000 (平均每人10条)
- 关注关系: 50,000 (平均每人关注5人)
- 好友关系: 20,000 (平均每人2个好友)

#### 大规模测试 (100K用户)
- 用户数: 100,000
- 推文数: 1,000,000 (平均每人10条)
- 关注关系: 500,000 (平均每人关注5人)
- 好友关系: 200,000 (平均每人2个好友)

### 2. 数据生成脚本

#### 用户生成 (Go脚本)
```go
package main

import (
    "database/sql"
    "fmt"
    "math/rand"
    "time"
    _ "github.com/go-sql-driver/mysql"
)

func generateUsers(db *sql.DB, count int) error {
    stmt, err := db.Prepare(`
        INSERT INTO p_user (nickname, username, phone, password, salt, status, avatar, balance, is_admin, created_on, modified_on) 
        VALUES (?, ?, ?, ?, ?, 1, '', 0, false, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    now := time.Now().Unix()
    for i := 1; i <= count; i++ {
        nickname := fmt.Sprintf("测试用户%d", i)
        username := fmt.Sprintf("testuser%d", i)
        phone := fmt.Sprintf("1888888%04d", i)
        password := "e10adc3949ba59abbe56e057f20f883e" // MD5: 123456
        salt := "salt123"
        
        _, err = stmt.Exec(nickname, username, phone, password, salt, now, now)
        if err != nil {
            return err
        }
        
        if i%1000 == 0 {
            fmt.Printf("Generated %d users\n", i)
        }
    }
    return nil
}
```

#### 推文生成 (Go脚本)
```go
func generatePosts(db *sql.DB, userCount, postCount int) error {
    // 生成推文主体
    postStmt, err := db.Prepare(`
        INSERT INTO p_post (user_id, comment_count, collection_count, upvote_count, share_count, 
                           visibility, is_top, is_essence, is_lock, latest_replied_on, tags, 
                           attachment_price, ip, ip_loc, created_on, modified_on) 
        VALUES (?, 0, 0, ?, 0, ?, 0, 0, 0, ?, '', 0, '127.0.0.1', '北京', ?, ?)
    `)
    if err != nil {
        return err
    }
    defer postStmt.Close()

    // 生成推文内容
    contentStmt, err := db.Prepare(`
        INSERT INTO p_post_content (post_id, user_id, content, type, sort, created_on, modified_on) 
        VALUES (?, ?, ?, ?, 100, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer contentStmt.Close()

    texts := []string{
        "今天天气真不错，心情也很好！",
        "分享一下最近的学习心得",
        "刚刚看了一部很棒的电影",
        "周末打算去爬山，有人一起吗？",
        "最近在学习Go语言，感觉很有趣",
        "今天的晚餐特别美味",
        "推荐一本好书给大家",
        "工作虽然忙碌，但很充实",
        "喜欢这样安静的午后时光",
        "期待即将到来的假期",
    }

    visibilities := []int{50, 60, 90} // 好友可见、关注可见、公开
    
    for i := 1; i <= postCount; i++ {
        userId := rand.Intn(userCount) + 1
        upvoteCount := rand.Intn(50)
        visibility := visibilities[rand.Intn(len(visibilities))]
        now := time.Now().Unix() - int64(rand.Intn(86400*30)) // 最近30天内
        
        // 插入推文
        result, err := postStmt.Exec(userId, upvoteCount, visibility, now, now, now)
        if err != nil {
            return err
        }
        
        postId, _ := result.LastInsertId()
        
        // 插入推文内容（标题）
        title := fmt.Sprintf("推文标题 #%d", i)
        _, err = contentStmt.Exec(postId, userId, title, 1, now, now)
        if err != nil {
            return err
        }
        
        // 插入推文内容（文本）
        content := texts[rand.Intn(len(texts))]
        _, err = contentStmt.Exec(postId, userId, content, 2, now, now)
        if err != nil {
            return err
        }
        
        if i%1000 == 0 {
            fmt.Printf("Generated %d posts\n", i)
        }
    }
    return nil
}
```

#### 关注关系生成 (Go脚本)
```go
func generateFollowing(db *sql.DB, userCount, followCount int) error {
    stmt, err := db.Prepare(`
        INSERT IGNORE INTO p_following (user_id, follow_id, is_del, created_on, modified_on) 
        VALUES (?, ?, 0, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    now := time.Now().Unix()
    generated := 0
    
    for generated < followCount {
        userId := rand.Intn(userCount) + 1
        followId := rand.Intn(userCount) + 1
        
        // 避免自己关注自己
        if userId == followId {
            continue
        }
        
        _, err = stmt.Exec(userId, followId, now, now)
        if err == nil {
            generated++
        }
        
        if generated%1000 == 0 {
            fmt.Printf("Generated %d following relationships\n", generated)
        }
    }
    return nil
}
```

#### 好友关系生成 (Go脚本)
```go
func generateContacts(db *sql.DB, userCount, contactCount int) error {
    stmt, err := db.Prepare(`
        INSERT IGNORE INTO p_contact (user_id, friend_id, group_id, remark, status, 
                                    is_top, is_black, is_del, notice_enable, created_on, modified_on) 
        VALUES (?, ?, 0, '', 2, 0, 0, 0, 0, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    now := time.Now().Unix()
    generated := 0
    
    for generated < contactCount {
        userId := rand.Intn(userCount) + 1
        friendId := rand.Intn(userCount) + 1
        
        // 避免自己加自己为好友
        if userId == friendId {
            continue
        }
        
        // 创建双向好友关系
        _, err1 := stmt.Exec(userId, friendId, now, now)
        _, err2 := stmt.Exec(friendId, userId, now, now)
        
        if err1 == nil && err2 == nil {
            generated++
        }
        
        if generated%1000 == 0 {
            fmt.Printf("Generated %d contact relationships\n", generated)
        }
    }
    return nil
}
```

### 3. 完整数据生成脚本

创建 `scripts/generate_test_data.go`:

```go
package main

import (
    "database/sql"
    "flag"
    "fmt"
    "log"
    "math/rand"
    "time"
    _ "github.com/go-sql-driver/mysql"
)

var (
    dsn = flag.String("dsn", "paopao:paopao@tcp(127.0.0.1:3306)/paopao?charset=utf8mb4&parseTime=True&loc=Local", "数据库连接字符串")
    scale = flag.String("scale", "small", "测试规模: small, medium, large")
)

type TestScale struct {
    Users     int
    Posts     int
    Following int
    Contacts  int
}

func main() {
    flag.Parse()
    
    scales := map[string]TestScale{
        "small":  {1000, 10000, 5000, 2000},
        "medium": {10000, 100000, 50000, 20000},
        "large":  {100000, 1000000, 500000, 200000},
    }
    
    s, exists := scales[*scale]
    if !exists {
        log.Fatal("Invalid scale. Use: small, medium, large")
    }
    
    db, err := sql.Open("mysql", *dsn)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    rand.Seed(time.Now().UnixNano())
    
    fmt.Printf("Generating test data for %s scale...\n", *scale)
    fmt.Printf("Users: %d, Posts: %d, Following: %d, Contacts: %d\n", 
              s.Users, s.Posts, s.Following, s.Contacts)
    
    // 清理现有数据
    if err := cleanData(db); err != nil {
        log.Fatal("清理数据失败:", err)
    }
    
    // 生成用户
    fmt.Println("Generating users...")
    if err := generateUsers(db, s.Users); err != nil {
        log.Fatal("生成用户失败:", err)
    }
    
    // 生成推文
    fmt.Println("Generating posts...")
    if err := generatePosts(db, s.Users, s.Posts); err != nil {
        log.Fatal("生成推文失败:", err)
    }
    
    // 生成关注关系
    fmt.Println("Generating following relationships...")
    if err := generateFollowing(db, s.Users, s.Following); err != nil {
        log.Fatal("生成关注关系失败:", err)
    }
    
    // 生成好友关系
    fmt.Println("Generating contact relationships...")
    if err := generateContacts(db, s.Users, s.Contacts); err != nil {
        log.Fatal("生成好友关系失败:", err)
    }
    
    fmt.Println("Test data generation completed!")
}

func cleanData(db *sql.DB) error {
    tables := []string{"p_post_content", "p_post", "p_following", "p_contact", "p_user"}
    for _, table := range tables {
        _, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id > 0", table))
        if err != nil {
            return err
        }
    }
    return nil
}

// 这里插入之前定义的生成函数...
```

## 🚀 压测脚本方案

### 1. HTTP压测脚本 (wrk)

创建 `scripts/feeds_benchmark.lua`:

```lua
-- feeds_benchmark.lua
local counter = 1
local users = 1000 -- 根据测试规模调整

-- 随机选择用户ID进行测试
function request()
    local user_id = math.random(1, users)
    local page = math.random(1, 5)
    local path = string.format("/v1/posts?type=%d&style=following&page=%d&page_size=20", 
                              os.time() * 1000, page)
    
    return wrk.format("GET", path, {
        ["Authorization"] = "Bearer test_token_" .. user_id,
        ["Content-Type"] = "application/json"
    })
end

function response(status, headers, body)
    if status ~= 200 then
        print("Error: " .. status .. " - " .. body)
    end
end
```

压测命令:
```bash
# 基础压测
wrk -t12 -c100 -d30s -s scripts/feeds_benchmark.lua http://127.0.0.1:8008

# 高并发压测
wrk -t24 -c500 -d60s -s scripts/feeds_benchmark.lua http://127.0.0.1:8008

# 长时间压测
wrk -t12 -c200 -d300s -s scripts/feeds_benchmark.lua http://127.0.0.1:8008
```

### 2. Go压测脚本

创建 `scripts/feeds_load_test.go`:

```go
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "log"
    "math/rand"
    "net/http"
    "sync"
    "time"
)

type Config struct {
    BaseURL     string
    Concurrency int
    Duration    time.Duration
    UserCount   int
}

type Result struct {
    TotalRequests int64
    SuccessCount  int64
    ErrorCount    int64
    TotalTime     time.Duration
    Latencies     []time.Duration
}

func main() {
    var (
        baseURL     = flag.String("url", "http://127.0.0.1:8008", "目标URL")
        concurrency = flag.Int("c", 100, "并发数")
        duration    = flag.Duration("d", 30*time.Second, "测试时长")
        userCount   = flag.Int("users", 1000, "用户总数")
    )
    flag.Parse()

    config := Config{
        BaseURL:     *baseURL,
        Concurrency: *concurrency,
        Duration:    *duration,
        UserCount:   *userCount,
    }

    fmt.Printf("开始压测: %s\n", config.BaseURL)
    fmt.Printf("并发数: %d, 时长: %v, 用户数: %d\n", 
               config.Concurrency, config.Duration, config.UserCount)

    result := runLoadTest(config)
    printResults(result)
}

func runLoadTest(config Config) *Result {
    result := &Result{
        Latencies: make([]time.Duration, 0),
    }
    var mu sync.Mutex
    var wg sync.WaitGroup

    startTime := time.Now()
    endTime := startTime.Add(config.Duration)

    // 启动并发goroutine
    for i := 0; i < config.Concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            client := &http.Client{Timeout: 10 * time.Second}
            
            for time.Now().Before(endTime) {
                latency, success := makeRequest(client, config)
                
                mu.Lock()
                result.TotalRequests++
                result.Latencies = append(result.Latencies, latency)
                if success {
                    result.SuccessCount++
                } else {
                    result.ErrorCount++
                }
                mu.Unlock()
            }
        }()
    }

    wg.Wait()
    result.TotalTime = time.Since(startTime)
    return result
}

func makeRequest(client *http.Client, config Config) (time.Duration, bool) {
    userID := rand.Intn(config.UserCount) + 1
    page := rand.Intn(5) + 1
    timestamp := time.Now().UnixNano() / 1000000
    
    url := fmt.Sprintf("%s/v1/posts?type=%d&style=following&page=%d&page_size=20",
                      config.BaseURL, timestamp, page)
    
    start := time.Now()
    resp, err := client.Get(url)
    latency := time.Since(start)
    
    if err != nil {
        return latency, false
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return latency, false
    }
    
    // 读取响应体，模拟真实使用
    io.Copy(io.Discard, resp.Body)
    return latency, true
}

func printResults(result *Result) {
    if len(result.Latencies) == 0 {
        log.Fatal("没有收到任何响应")
    }

    // 计算延迟统计
    latencies := result.Latencies
    qps := float64(result.TotalRequests) / result.TotalTime.Seconds()
    
    fmt.Printf("\n=== 压测结果 ===\n")
    fmt.Printf("总请求数: %d\n", result.TotalRequests)
    fmt.Printf("成功请求: %d\n", result.SuccessCount)
    fmt.Printf("失败请求: %d\n", result.ErrorCount)
    fmt.Printf("成功率: %.2f%%\n", float64(result.SuccessCount)/float64(result.TotalRequests)*100)
    fmt.Printf("QPS: %.2f\n", qps)
    fmt.Printf("平均延迟: %v\n", averageLatency(latencies))
    fmt.Printf("P50延迟: %v\n", percentileLatency(latencies, 50))
    fmt.Printf("P95延迟: %v\n", percentileLatency(latencies, 95))
    fmt.Printf("P99延迟: %v\n", percentileLatency(latencies, 99))
}
```

### 3. 数据库监控脚本

创建 `scripts/monitor_db.sh`:

```bash
#!/bin/bash

LOG_FILE="db_performance.log"
INTERVAL=5  # 监控间隔(秒)

echo "开始监控数据库性能，日志文件: $LOG_FILE"
echo "时间,QPS,慢查询,连接数,CPU使用率,内存使用率" > $LOG_FILE

while true; do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # 获取MySQL状态
    QPS=$(mysql -u paopao -ppaopao -e "SHOW GLOBAL STATUS LIKE 'Questions';" | tail -1 | awk '{print $2}')
    SLOW_QUERIES=$(mysql -u paopao -ppaopao -e "SHOW GLOBAL STATUS LIKE 'Slow_queries';" | tail -1 | awk '{print $2}')
    THREADS_CONNECTED=$(mysql -u paopao -ppaopao -e "SHOW STATUS LIKE 'Threads_connected';" | tail -1 | awk '{print $2}')
    
    # 获取系统资源使用率
    CPU_USAGE=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | awk -F'%' '{print $1}')
    MEM_USAGE=$(free | grep Mem | awk '{printf "%.1f", $3/$2 * 100.0}')
    
    echo "$TIMESTAMP,$QPS,$SLOW_QUERIES,$THREADS_CONNECTED,$CPU_USAGE,$MEM_USAGE" >> $LOG_FILE
    
    sleep $INTERVAL
done
```

## 📈 性能分析方案

### 1. 性能指标收集

创建监控脚本收集关键指标:

```bash
# 应用性能监控
curl -s http://127.0.0.1:8008/debug/pprof/profile?seconds=30 > cpu.prof
curl -s http://127.0.0.1:8008/debug/pprof/heap > heap.prof

# 数据库查询分析
mysql -u paopao -ppaopao -e "
SELECT 
    sql_text,
    avg_timer_wait/1000000000 as avg_time_sec,
    count_star as exec_count
FROM performance_schema.events_statements_summary_by_digest 
WHERE avg_timer_wait > 1000000000
ORDER BY avg_timer_wait DESC 
LIMIT 10;
"
```

### 2. 压测报告模板

```markdown
## 压测报告

### 测试环境
- 数据规模: {small/medium/large}
- 用户数: {count}
- 推文数: {count}
- 关注关系: {count}

### 测试配置
- 并发数: {concurrency}
- 测试时长: {duration}
- 测试工具: {wrk/custom}

### 性能结果
- QPS: {qps}
- 平均延迟: {avg_latency}ms
- P95延迟: {p95_latency}ms
- P99延迟: {p99_latency}ms
- 成功率: {success_rate}%

### 资源使用
- CPU使用率: {cpu_usage}%
- 内存使用率: {memory_usage}%
- 数据库连接数: {db_connections}
- 慢查询数: {slow_queries}

### 性能瓶颈分析
1. 数据库层面: 
2. 应用层面:
3. 缓存层面:

### 优化建议
1. 短期优化:
2. 中期优化:
3. 长期优化:
```

## 🔧 压测执行步骤

### 1. 环境准备
```bash
# 1. 确保服务正常运行
curl http://127.0.0.1:8008/v1/site/profile

# 2. 安装压测工具
brew install wrk  # macOS
# 或者使用Go脚本

# 3. 清理日志
rm -f *.log *.prof
```

### 2. 数据生成
```bash
# 编译数据生成脚本
cd scripts
go build generate_test_data.go

# 生成小规模测试数据
./generate_test_data -scale=small

# 验证数据
mysql -u paopao -ppaopao paopao -e "
SELECT 
    (SELECT COUNT(*) FROM p_user) as users,
    (SELECT COUNT(*) FROM p_post) as posts,
    (SELECT COUNT(*) FROM p_following) as following,
    (SELECT COUNT(*) FROM p_contact) as contacts;
"
```

### 3. 基准测试
```bash
# 单次请求测试
curl -w "@curl-format.txt" -s -o /dev/null \
  "http://127.0.0.1:8008/v1/posts?type=$(date +%s)000&style=following&page=1&page_size=20"

# 小并发测试
wrk -t4 -c10 -d10s http://127.0.0.1:8008/v1/posts?type=1751446531796&style=newest&page=1&page_size=20
```

### 4. 完整压测
```bash
# 启动监控
./scripts/monitor_db.sh &
MONITOR_PID=$!

# 执行压测
wrk -t12 -c100 -d60s -s scripts/feeds_benchmark.lua http://127.0.0.1:8008

# 停止监控
kill $MONITOR_PID
```

### 5. 结果分析
```bash
# 分析应用性能
go tool pprof cpu.prof
go tool pprof heap.prof

# 分析数据库性能
tail -50 db_performance.log

# 生成压测报告
./scripts/generate_report.sh
```

## 🎯 性能优化方向

### 1. 数据库优化
- **索引优化**: 针对feeds查询的复合索引
- **查询优化**: 减少IN查询，优化JOIN逻辑
- **分区表**: 按时间分区减少查询范围
- **读写分离**: 读请求分发到从库

### 2. 缓存优化
- **查询缓存**: 缓存用户关系数据
- **结果缓存**: 缓存feeds查询结果
- **预热策略**: 活跃用户数据预加载
- **缓存层级**: L1(内存) + L2(Redis)

### 3. 应用优化
- **连接池**: 优化数据库连接池配置
- **并发控制**: 限制单用户并发请求
- **异步处理**: 非核心逻辑异步化
- **批量操作**: 批量查询减少网络开销

### 4. 架构优化
- **微服务**: feeds服务独立部署
- **消息队列**: 解耦写入和查询
- **CDN加速**: 静态资源分发
- **负载均衡**: 多实例水平扩展

## 📋 压测检查清单

### 准备阶段 ✅
- [ ] 测试环境搭建完成
- [ ] 假数据生成脚本就绪
- [ ] 压测工具安装配置
- [ ] 监控脚本准备完成
- [ ] 基准性能数据收集

### 执行阶段 ✅
- [ ] 数据生成并验证
- [ ] 基准测试完成
- [ ] 渐进式压测 (低→高并发)
- [ ] 长时间稳定性测试
- [ ] 极限压测找到瓶颈

### 分析阶段 ✅
- [ ] 性能数据收集完整
- [ ] 瓶颈点识别清晰
- [ ] 优化方案制定
- [ ] 压测报告编写
- [ ] 优化效果验证

---

## 🔗 相关文档
- [paopao feeds流技术QA](paopao-feeds-qa.md) - feeds实现原理
- [paopao-ce 本地部署配置指南](paopao-ce-local-deployment-config.md) - 环境配置
- [Go Context和Select模式](go-context-and-select-patterns.md) - 并发优化

## 📝 使用示例

```bash
# 快速开始压测
cd paopao-ce

# 1. 生成测试数据
go run scripts/generate_test_data.go -scale=small

# 2. 执行压测
wrk -t8 -c50 -d30s "http://127.0.0.1:8008/v1/posts?type=1751446531796&style=following&page=1&page_size=20"

# 3. 查看结果
cat db_performance.log | tail -10
```

这个压测方案可以帮助你全面评估paopao-ce feeds流的性能表现，找到性能瓶颈并制定针对性的优化策略。 