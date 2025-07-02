# paopao-ce 本地部署配置指南

## 📋 概述

本文档详细介绍paopao-ce项目在本地环境下的配置文件(`config.yaml`)各个配置项的含义、重要程度以及本地部署的最佳实践。

## 🔴 核心配置（必须正确配置）

### 1. Features（功能特性模块）

```yaml
Features:
  Default: ["Web", "Frontend:EmbedWeb", "Meili", "LocalOSS", "MySQL", "BigCacheIndex", "LoggerFile"]
```

**说明**: 控制启用哪些功能模块，直接影响应用的功能和性能。

| 模块 | 说明 | 本地部署必要性 |
|------|------|---------------|
| `Web` | Web HTTP服务 | 必须 |
| `Frontend:EmbedWeb` | 内置前端界面 | 必须 |
| `MySQL` | MySQL数据库支持 | 必须 |
| `Meili` | Meilisearch搜索引擎 | 必须 |
| `LocalOSS` | 本地文件存储 | 必须 |
| `BigCacheIndex` | 内存缓存索引 | 推荐 |
| `LoggerFile` | 文件日志记录 | 推荐 |

### 2. WebServer（Web服务配置）

```yaml
WebServer:
  HttpIp: 0.0.0.0      # 监听所有网络接口
  HttpPort: 8008       # 主要访问端口
  ReadTimeout: 60      # 读取超时时间（秒）
  WriteTimeout: 60     # 写入超时时间（秒）
```

**本地部署建议**: 
- 端口8008通常不冲突，如有冲突可改为8009等
- IP设置为0.0.0.0允许局域网访问

### 3. MySQL数据库连接

```yaml
MySQL:
  Username: paopao          # 数据库用户名
  Password: paopao          # 数据库密码
  Host: 127.0.0.1:3306      # 本地MySQL地址
  DBName: paopao            # 数据库名
  Charset: utf8mb4          # 字符集（支持emoji）
  ParseTime: True           # 解析时间类型
  MaxIdleConns: 10          # 最大空闲连接数
  MaxOpenConns: 30          # 最大打开连接数
```

**注意事项**:
- 确保MySQL服务已启动: `brew services start mysql`
- 数据库和用户需提前创建
- utf8mb4字符集支持emoji表情

### 4. Redis缓存连接

```yaml
Redis:
  InitAddress:
  - 127.0.0.1:6379         # 本地Redis地址
```

**注意事项**:
- 确保Redis服务已启动: `brew services start redis`
- 本地部署通常不需要密码

### 5. Meilisearch搜索引擎

```yaml
Meili:
  Host: 127.0.0.1:7700      # Meilisearch服务地址
  Index: paopao-data        # 搜索索引名称
  ApiKey: paopao-meilisearch # API密钥
  Secure: False             # 是否使用HTTPS
```

**启动命令**:
```bash
MEILI_MASTER_KEY=paopao-meilisearch meilisearch --db-path ./custom/data/meili --http-addr 127.0.0.1:7700 &
```

### 6. LocalOSS本地文件存储

```yaml
LocalOSS:
  SavePath: custom/data/paopao-ce/oss  # 文件存储路径
  Secure: False                        # 是否使用HTTPS
  Bucket: paopao                       # 存储桶名称
  Domain: 127.0.0.1:8008              # 文件访问域名
```

**注意事项**:
- 确保存储目录有写入权限
- Domain需要与WebServer端口一致

## 🟡 重要配置（影响用户体验）

### 7. WebProfile（前端用户体验配置）

```yaml
WebProfile:
  UseFriendship: true              # 是否启用好友系统
  EnableTrendsBar: true            # 是否启用动态趋势栏
  EnableWallet: false              # 是否启用钱包功能（本地建议关闭）
  AllowTweetAttachment: true       # 是否允许推文附件
  AllowTweetAttachmentPrice: true  # 是否允许付费附件
  AllowTweetVideo: true            # 是否允许视频推文
  AllowUserRegister: true          # 是否允许用户注册
  AllowPhoneBind: false            # 是否允许手机绑定（本地建议关闭）
  DefaultTweetMaxLength: 2000      # 推文最大字符数
  TweetWebEllipsisSize: 400        # Web端推文显示截断长度
  TweetMobileEllipsisSize: 300     # 移动端推文显示截断长度
  DefaultTweetVisibility: public   # 默认推文可见性（建议设为public便于测试）
  DefaultMsgLoopInterval: 5000     # 消息轮询间隔（毫秒）
  CopyrightTop: "本地部署 paopao-ce"
  CopyrightLeft: "本地开发"
  CopyrightRight: "paopao-ce 本地实例"
```

**本地部署建议**:
- `EnableWallet: false` - 关闭钱包功能，避免支付配置复杂性
- `AllowPhoneBind: false` - 关闭手机绑定，避免短信服务配置
- `DefaultTweetVisibility: public` - 设为公开便于测试

### 8. JWT安全认证

```yaml
JWT:
  Secret: your-local-secret-key-change-it  # JWT密钥（请更换）
  Issuer: paopao-api                       # 签发者
  Expire: 86400                            # Token过期时间（秒，24小时）
```

**安全建议**:
- 必须更换默认密钥
- 本地开发可以使用较长的过期时间

### 9. Logger日志配置

```yaml
Logger:
  Level: debug              # 日志级别（开发用debug，生产用info）

LoggerFile:
  SavePath: custom/data/paopao-ce/logs  # 日志文件路径
  FileName: app             # 日志文件名前缀
  FileExt: .log            # 日志文件扩展名
```

**日志级别说明**:
- `debug`: 详细调试信息（开发环境）
- `info`: 一般信息（生产环境）
- `warn`: 警告信息
- `error`: 错误信息

## 🟢 可选配置（高级功能）

### 10. 缓存优化配置

```yaml
BigCacheIndex:
  MaxIndexPage: 512         # 最大缓存页数（可根据内存调整）
  Verbose: False           # 是否显示缓存操作日志
  ExpireInSecond: 300      # 缓存过期时间（秒）

CacheIndex:
  MaxUpdateQPS: 100        # 最大更新QPS限制
```

**性能调优**:
- `MaxIndexPage`: 内存充足时可增大，提升缓存命中率
- `ExpireInSecond`: 根据内容更新频率调整

### 11. 数据库性能配置

```yaml
Database:
  LogLevel: error          # 数据库日志级别
  TablePrefix: p_          # 表名前缀
```

### 12. 搜索性能配置

```yaml
TweetSearch:
  MaxUpdateQPS: 100        # 搜索索引更新QPS限制
  MinWorker: 10           # 最小后台工作者数量
```

## 🚫 本地部署可忽略的配置

### 短信服务（留空即可）
```yaml
SmsJuhe:
  Gateway: https://v.juhe.cn/sms/send
  Key:                     # 留空表示不启用
  TplID:                   # 留空表示不启用
```

### 支付服务（留空即可）
```yaml
Alipay:
  AppID:                   # 留空表示不启用
  InProduction: False      # 本地测试环境
```

### 云存储服务（使用LocalOSS即可）
```yaml
# 以下云存储配置在本地部署时可忽略
AliOSS: { }              # 阿里云OSS
COS: { }                 # 腾讯云COS  
HuaweiOBS: { }           # 华为云OBS
MinIO: { }               # MinIO对象存储
S3: { }                  # Amazon S3
```

## 📝 本地部署最佳实践配置模板

```yaml
# 精简的本地部署配置
App:
  RunMode: debug
  DefaultPageSize: 10
  MaxPageSize: 100

Features:
  Default: ["Web", "Frontend:EmbedWeb", "Meili", "LocalOSS", "MySQL", "BigCacheIndex", "LoggerFile"]

WebServer:
  HttpIp: 0.0.0.0
  HttpPort: 8008
  ReadTimeout: 60
  WriteTimeout: 60

MySQL:
  Username: paopao
  Password: paopao
  Host: 127.0.0.1:3306
  DBName: paopao
  Charset: utf8mb4
  ParseTime: True
  MaxIdleConns: 10
  MaxOpenConns: 30

Redis:
  InitAddress:
  - 127.0.0.1:6379

Meili:
  Host: 127.0.0.1:7700
  Index: paopao-data
  ApiKey: paopao-meilisearch
  Secure: False

LocalOSS:
  SavePath: custom/data/paopao-ce/oss
  Secure: False
  Bucket: paopao
  Domain: 127.0.0.1:8008

Logger:
  Level: debug
LoggerFile:
  SavePath: custom/data/paopao-ce/logs
  FileName: app
  FileExt: .log

JWT:
  Secret: your-local-secret-key-change-it
  Issuer: paopao-api
  Expire: 86400

WebProfile:
  UseFriendship: true
  EnableTrendsBar: true
  EnableWallet: false
  AllowTweetAttachment: true
  AllowTweetVideo: true
  AllowUserRegister: true
  AllowPhoneBind: false
  DefaultTweetMaxLength: 2000
  DefaultTweetVisibility: public
  DefaultMsgLoopInterval: 5000

BigCacheIndex:
  MaxIndexPage: 512
  Verbose: False
  ExpireInSecond: 300

Database:
  LogLevel: error
  TablePrefix: p_
```

## 🛠️ 常见配置问题

### 1. YAML格式错误
**错误**: `found a tab character that violates indentation`
**解决**: 使用空格而非Tab键缩进
```bash
sed -i '' 's/\t/  /g' config.yaml
```

### 2. 端口冲突
**错误**: `bind: address already in use`
**解决**: 修改WebServer.HttpPort为其他端口（如8009）

### 3. 数据库连接失败
**错误**: `connection refused`
**解决**: 确保MySQL服务已启动
```bash
brew services start mysql
```

### 4. Meilisearch连接失败
**错误**: `connection refused`
**解决**: 确保Meilisearch服务已启动
```bash
MEILI_MASTER_KEY=paopao-meilisearch meilisearch --db-path ./custom/data/meili --http-addr 127.0.0.1:7700 &
```

### 5. 文件存储权限错误
**错误**: `permission denied`
**解决**: 确保存储目录有写权限
```bash
mkdir -p custom/data/paopao-ce/oss
chmod 755 custom/data/paopao-ce/oss
```

## 📚 相关文档

- [本地开发依赖环境部署](001-本地开发依赖环境部署.md)
- [paopao-ce后端项目概览](后端项目概览.md)
- [go-mir框架绑定渲染模式](go-mir-binding-render-patterns.md)
- [Go Context和Select模式](go-context-and-select-patterns.md)

## 🔗 外部资源

- [Meilisearch官方文档](https://docs.meilisearch.com/)
- [MySQL配置优化指南](https://dev.mysql.com/doc/refman/8.0/en/optimization.html)
- [Redis配置参考](https://redis.io/topics/config) 