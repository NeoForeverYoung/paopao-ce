## 问题2：描述一个完整的 feeds 系统缓存策略，包括缓存层次、失效机制和一致性保证。

### 参考答案

基于 PaoPao-CE 的实际实现，feeds 系统缓存策略主要针对**时间线列表**而非单个推文详情。

#### **缓存内容分析**

**❌ 不缓存单个推文详情**
- 单个推文通过 `TweetDetail` 接口实时查询数据库
- 保证推文内容的强一致性（点赞数、评论数等实时更新）

**✅ 缓存时间线列表**
```go
// 缓存键设计
func (s *looseSrv) indexTweetsFromCache(req *web.TimelineReq, limit int, offset int) (res *web.TimelineResp, key string, ok bool) {
    username := "_"
    if req.User != nil {
        username = req.User.Username
    }
    switch req.Style {
    case web.StyleTweetsFollowing:
        key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsFollowing, username, offset, limit)
    case web.StyleTweetsNewest:
        key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsNewest, username, offset, limit)
    case web.StyleTweetsHots:
        key = fmt.Sprintf("%s%s:%d:%d", s.prefixIdxTweetsHots, username, offset, limit)
    }
}
```

#### **缓存架构层次**

**1. 多级缓存体系**
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   应用内存缓存    │ -> │    Redis缓存     │ -> │    数据库查询    │
│  (BigCache)     │    │ (rueidis客户端)  │    │   (MySQL)      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
        L1                     L2                     L3
    秒级访问               毫秒级访问              毫秒-秒级访问
```

**2. 缓存粒度设计**

| 缓存类型 | 缓存键模式 | 缓存内容 | 过期时间 |
|----------|------------|----------|----------|
| **广场时间线** | `idx_tweets_newest:用户名:offset:limit` | 分页推文列表 | 5分钟 |
| **热门时间线** | `idx_tweets_hots:用户名:offset:limit` | 热门推文列表 | 10分钟 |
| **关注时间线** | `idx_tweets_following:用户名:offset:limit` | 关注推文列表 | 3分钟 |
| **用户推文** | `user_tweets:用户ID:样式:关系:页码:数量` | 用户发布列表 | 5分钟 |
| **推文评论** | `tweet_comment:推文ID:样式:limit:offset` | 评论列表 | 3分钟 |

#### **缓存失效机制**

**1. 主动失效策略**
```go
// 发布新推文时触发缓存失效
func OnExpireIndexTweetEvent(userId int64) {
    events.OnEvent(&expireIndexTweetsEvent{
        ac: _appCache,
        keysPattern: []string{
            conf.PrefixIdxTweetsNewest + "*",    // 最新时间线
            conf.PrefixIdxTweetsHots + "*",      // 热门时间线  
            conf.PrefixIdxTweetsFollowing + "*", // 关注时间线
            fmt.Sprintf("%s%d:*", conf.PrefixUserTweets, userId), // 用户推文
        },
    })
}
```

**2. 级联失效场景**
- **发布推文**: 失效全部时间线缓存
- **删除推文**: 失效发布者相关缓存
- **互动操作**: 仅失效热门排序缓存
- **关注关系变更**: 失效关注时间线缓存

#### **一致性保证**

**1. 最终一致性模型**
```go
// 缓存未命中时从数据库加载并异步回填
func (s *looseSrv) getIndexTweets(req *web.TimelineReq, limit int, offset int) (*web.TimelineResp, error) {
    // 1. 尝试缓存命中
    if res, key, ok := s.indexTweetsFromCache(req, limit, offset); ok {
        return res, nil
    }
    
    // 2. 查询数据库
    posts, total, err := s.Ds.ListIndexNewestTweets(limit, offset)
    
    // 3. 异步回填缓存
    base.OnCacheRespEvent(s.ac, key, resp, s.idxTweetsExpire)
    return resp, nil
}
```

**2. 写入时一致性**
- **强一致性要求**: 推文内容、互动数据直接查询数据库
- **弱一致性允许**: 时间线列表可以短暂不一致

#### **性能优化策略**

**1. 预热机制**
```go
// 系统启动时预热热门内容
func (s *cacheIndexSrv) warmupCache() {
    // 预加载热门时间线前3页
    for page := 1; page <= 3; page++ {
        s.IndexPosts(nil, (page-1)*20, 20)
    }
}
```

**2. 分级存储**
- **热数据**: 存储在内存缓存 (BigCache)
- **温数据**: 存储在 Redis
- **冷数据**: 直接查询数据库

**3. 压缩存储**
```go
// 使用 gob 编码压缩缓存数据
func (s *cacheIndexSrv) setPosts(entry *postsEntry) {
    var buf bytes.Buffer
    enc := gob.NewEncoder(&buf)
    enc.Encode(entry.tweets)
    s.cache.setTweetsBytes(entry.key, buf.Bytes())
}
```

#### **监控与降级**

**1. 缓存命中率监控**
```go
// 记录缓存性能指标
func (s *looseSrv) indexTweetsFromCache(req *web.TimelineReq, limit int, offset int) {
    if data, err := s.ac.Get(key); err == nil {
        // 命中计数
        metrics.CacheHit.Inc()
        return data
    }
    // 未命中计数  
    metrics.CacheMiss.Inc()
}
```

**2. 自动降级策略**
- 缓存服务异常时直接查询数据库
- 数据库压力过大时延长缓存过期时间
- 热点内容检测和专门缓存处理

**关键设计原则**: PaoPao-CE 采用**列表缓存 + 详情实时**的策略，既保证了时间线的访问性能，又确保了单条推文的数据一致性。

---

## 问题3：如何设计一个支持千万级用户的 feeds 系统数据库架构？包括分库分表策略。

### 参考答案

#### 数据库架构演进

**第一阶段：单库架构 (< 100万用户)**
```sql
-- PaoPao-CE 当前架构
CREATE TABLE p_post (
    id BIGINT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    content TEXT,
    visibility TINYINT,
    latest_replied_on BIGINT,
    INDEX idx_user_visibility_time (user_id, visibility, latest_replied_on)
);

CREATE TABLE p_following (
    user_id BIGINT,
    follow_id BIGINT,
    INDEX idx_user_follow (user_id, follow_id)
);
```

**第二阶段：读写分离 (100万-500万用户)**
```
主库 (Master) -----> 从库1 (Slave)
     |          -----> 从库2 (Slave)
     |          -----> 从库3 (Slave)
```

**第三阶段：分库分表 (500万-千万用户)**

**1. 用户维度分库**
```sql
-- 按用户ID分库 (16个库)
db_user_00: user_id % 16 = 0
db_user_01: user_id % 16 = 1
...
db_user_15: user_id % 16 = 15

-- 每个库内按时间分表
CREATE TABLE p_post_202401 (id, user_id, content, created_on);
CREATE TABLE p_post_202402 (id, user_id, content, created_on);
```

**2. 内容维度分库**
```sql
-- 热点内容独立存储
db_hot_content:     热门推文、话题
db_user_timeline:   用户时间线 
db_user_relation:   关注关系
```

#### 分片策略选择

**1. 按用户ID分片 (推荐)**
```go
func getDBShard(userID int64) string {
    return fmt.Sprintf("db_user_%02d", userID % 16)
}

func getTableShard(timestamp int64) string {
    return time.Unix(timestamp, 0).Format("200601") // YYYYMM
}
```

**2. 按内容ID分片**
```go
func getContentShard(postID int64) string {
    return fmt.Sprintf("db_content_%02d", postID % 8)
}
```

#### 跨分片查询解决方案

**1. 聚合查询服务**
```go
type FeedsAggregator struct {
    userDBs    map[string]*sql.DB
    contentDBs map[string]*sql.DB
}

func (f *FeedsAggregator) GetUserTimeline(userID int64) ([]*Post, error) {
    // 1. 获取用户关注列表
    followings := f.getUserFollowings(userID)
    
    // 2. 并行查询各分片
    var results []*Post
    for _, followUserID := range followings {
        shard := getDBShard(followUserID)
        posts := f.queryUserPosts(shard, followUserID)
        results = append(results, posts...)
    }
    
    // 3. 内存排序合并
    sort.Slice(results, func(i, j int) bool {
        return results[i].CreatedOn > results[j].CreatedOn
    })
    
    return results[:limit], nil
}
```

**2. 消息队列同步**
```
用户发布推文 -> MQ -> 粉丝时间线更新
             -> 搜索索引更新
             -> 推荐系统更新
```

---

## 问题4：实现一个热门内容排序算法，要求考虑时间衰减、用户互动等因素。

### 参考答案

#### PaoPao-CE 的热度算法实现

**1. 基础热度计算**
```sql
-- 参考 PaoPao-CE 的 rank_score 计算
INSERT INTO p_post_metric (post_id, rank_score, created_on) 
SELECT id AS post_id, 
    comment_count + upvote_count*2 + collection_count*4 AS rank_score,
    created_on
FROM p_post
WHERE is_del=0;
```

**2. 热门推文查询**
```go
func (s *tweetSrv) ListIndexHotsTweets(limit, offset int) ([]*ms.Post, int64, error) {
    db := s.db.Table(_post_).
        Joins("LEFT JOIN p_post_metric metric ON p_post.id=metric.post_id").
        Where("visibility >= ? AND p_post.is_del=0 AND metric.is_del=0", cs.TweetVisitPublic)
    
    return db.Order("is_top DESC, metric.rank_score DESC, latest_replied_on DESC").
        Find(&res).Error
}
```

#### 改进的热度算法设计

**1. 综合热度公式**
```go
type HotScore struct {
    PostID          int64
    CommentCount    int64
    LikeCount       int64
    ShareCount      int64
    CollectionCount int64
    ViewCount       int64
    CreatedAt       time.Time
    AuthorFollowers int64
}

func (h *HotScore) Calculate() float64 {
    // 基础互动分数
    interactionScore := float64(
        h.CommentCount*10 +     // 评论权重最高
        h.LikeCount*2 +         // 点赞
        h.ShareCount*5 +        // 分享
        h.CollectionCount*3 +   // 收藏
        h.ViewCount*0.1,        // 浏览
    )
    
    // 时间衰减因子
    hoursSincePost := time.Since(h.CreatedAt).Hours()
    timeDecay := math.Exp(-hoursSincePost / 24) // 24小时半衰期
    
    // 作者影响力
    authorBoost := math.Log10(float64(h.AuthorFollowers + 1))
    
    // 最终分数
    return interactionScore * timeDecay * (1 + authorBoost*0.1)
}
```

**2. 实时热度更新**
```go
type HotRankingService struct {
    redis  redis.Client
    db     *gorm.DB
    scorer *HotScoreCalculator
}

func (s *HotRankingService) UpdatePostHotScore(postID int64) error {
    // 1. 获取最新互动数据
    metrics := s.getPostMetrics(postID)
    
    // 2. 计算热度分数
    hotScore := s.scorer.Calculate(metrics)
    
    // 3. 更新数据库
    s.db.Model(&PostMetric{}).
        Where("post_id = ?", postID).
        Update("rank_score", int64(hotScore))
    
    // 4. 更新Redis排行榜
    s.redis.ZAdd("hot_posts", &redis.Z{
        Score:  hotScore,
        Member: postID,
    })
    
    return nil
}

// 用户互动时触发更新
func (s *HotRankingService) OnUserInteraction(postID int64, action string) {
    // 异步更新热度分数
    go s.UpdatePostHotScore(postID)
}
```

**3. 防刷机制**
```go
func (s *HotRankingService) validateInteraction(userID, postID int64, action string) bool {
    // 1. 频率限制
    key := fmt.Sprintf("rate_limit:%d:%s", userID, action)
    count := s.redis.Incr(key)
    if count == 1 {
        s.redis.Expire(key, time.Hour)
    }
    if count > getActionLimit(action) {
        return false
    }
    
    // 2. 重复操作检测
    interactionKey := fmt.Sprintf("interaction:%d:%d:%s", userID, postID, action)
    exists := s.redis.Exists(interactionKey)
    if exists {
        return false
    }
    
    s.redis.SetEX(interactionKey, "1", 24*time.Hour)
    return true
}
```

---

## 问题5：如何处理大V用户（百万粉丝）发布内容时的性能问题？

### 参考答案

#### 问题分析

**大V用户带来的挑战：**
1. **写扩散爆炸**: 一条内容需要写入百万个时间线
2. **系统雷击**: 瞬间大量写入请求
3. **热点数据**: 大量用户同时访问同一内容
4. **延迟增加**: 粉丝量越大，推送完成时间越长

#### 解决方案设计

**1. 混合 Push-Pull 模型**
```go
type FeedsStrategy interface {
    ShouldUsePush(userID int64) bool
    PublishContent(userID int64, content *Content) error
}

type HybridFeedsStrategy struct {
    pushThreshold int64 // 粉丝数阈值
    redis         redis.Client
    db           *gorm.DB
}

func (h *HybridFeedsStrategy) ShouldUsePush(userID int64) bool {
    followerCount := h.getUserFollowerCount(userID)
    return followerCount < h.pushThreshold // 小于10万使用Push
}

func (h *HybridFeedsStrategy) PublishContent(userID int64, content *Content) error {
    if h.ShouldUsePush(userID) {
        // 普通用户：Push模型
        return h.pushToFollowers(userID, content)
    } else {
        // 大V用户：Pull模型 + 热点缓存
        return h.cacheForPull(userID, content)
    }
}
```

**2. 异步批量写入**
```go
type AsyncPublisher struct {
    writeQueue chan *WriteTask
    batchSize  int
    workers    int
}

type WriteTask struct {
    UserID    int64
    Content   *Content
    Followers []int64
}

func (p *AsyncPublisher) Start() {
    for i := 0; i < p.workers; i++ {
        go p.worker()
    }
}

func (p *AsyncPublisher) worker() {
    batch := make([]*WriteTask, 0, p.batchSize)
    ticker := time.NewTicker(100 * time.Millisecond)
    
    for {
        select {
        case task := <-p.writeQueue:
            batch = append(batch, task)
            if len(batch) >= p.batchSize {
                p.flushBatch(batch)
                batch = batch[:0]
            }
        case <-ticker.C:
            if len(batch) > 0 {
                p.flushBatch(batch)
                batch = batch[:0]
            }
        }
    }
}

func (p *AsyncPublisher) flushBatch(tasks []*WriteTask) error {
    // 批量写入数据库
    var timelineEntries []TimelineEntry
    for _, task := range tasks {
        for _, followerID := range task.Followers {
            timelineEntries = append(timelineEntries, TimelineEntry{
                UserID:    followerID,
                ContentID: task.Content.ID,
                CreatedAt: time.Now(),
            })
        }
    }
    
    return p.db.CreateInBatches(timelineEntries, 1000).Error
}
```

**3. 大V内容特殊处理**
```go
type BigVContentHandler struct {
    redis      redis.Client
    mq         MessageQueue
    hotPostTTL time.Duration
}

func (h *BigVContentHandler) HandleBigVPost(userID int64, content *Content) error {
    // 1. 标记为热点内容
    h.redis.SetEX(
        fmt.Sprintf("hot_content:%d", content.ID),
        content.Serialize(),
        h.hotPostTTL,
    )
    
    // 2. 预热CDN缓存
    h.preWarmCDN(content)
    
    // 3. 分批推送到活跃粉丝
    activeFollowers := h.getActiveFollowers(userID, 10000) // 只推送给1万活跃粉丝
    return h.pushToActiveFollowers(content, activeFollowers)
}

func (h *BigVContentHandler) getActiveFollowers(userID int64, limit int) []int64 {
    // 根据最近活跃时间排序
    query := `
        SELECT f.follower_id 
        FROM followings f
        JOIN user_metrics m ON f.follower_id = m.user_id
        WHERE f.user_id = ? 
        ORDER BY m.last_active_time DESC
        LIMIT ?
    `
    
    var followers []int64
    h.db.Raw(query, userID, limit).Scan(&followers)
    return followers
}
```

**4. 削峰填谷策略**
```go
type TrafficShaper struct {
    limiter  *rate.Limiter
    delayQueue *DelayQueue
}

func (t *TrafficShaper) PublishWithShaping(task *WriteTask) error {
    // 限流控制
    if !t.limiter.Allow() {
        // 超出限制的任务延迟处理
        t.delayQueue.Push(task, time.Now().Add(1*time.Second))
        return nil
    }
    
    return t.executeTask(task)
}

// 分时段处理
func (t *TrafficShaper) scheduleByTimeSlot(tasks []*WriteTask) {
    for i, task := range tasks {
        // 将任务分散到不同时间片
        delay := time.Duration(i%60) * time.Second
        t.delayQueue.Push(task, time.Now().Add(delay))
    }
}
```

**5. 监控和降级**
```go
type FeedsMonitor struct {
    metrics    *prometheus.MetricsRegistry
    alerter    AlertService
    circuitBreaker *CircuitBreaker
}

func (m *FeedsMonitor) checkSystemHealth() {
    // 监控关键指标
    writeLatency := m.getWriteLatency()
    errorRate := m.getErrorRate()
    queueLength := m.getQueueLength()
    
    if writeLatency > 5*time.Second || errorRate > 0.1 {
        // 触发降级策略
        m.triggerDegradation()
    }
}

func (m *FeedsMonitor) triggerDegradation() {
    // 1. 停止低优先级推送
    m.circuitBreaker.OpenCircuit("low_priority_push")
    
    // 2. 启用纯Pull模式
    config.SetFeedsMode("pull_only")
    
    // 3. 发送告警
    m.alerter.SendAlert("feeds系统负载过高，已启用降级模式")
}
```

#### 性能优化效果

通过以上策略，可以实现：
- **写入TPS**: 从1000提升至10万+
- **推送延迟**: 大V用户内容1分钟内完成推送
- **系统稳定性**: 99.9%可用性
- **资源利用率**: 数据库负载降低60%

这种混合架构既保证了普通用户的实时性，又解决了大V用户的性能瓶颈。 