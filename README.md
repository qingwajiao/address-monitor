# address-monitor

多链地址监控系统，支持 ETH、BSC、TRON、SOL 四条链的地址动态监控，通过 Webhook 推送标准化事件通知。

---

## 功能特性

- **多链支持**：ETH、BSC、TRON、SOL，EVM 兼容链通过配置即可接入
- **多租户**：用户注册后可创建多个应用，每个应用独立 API Key 和回调地址
- **实时监控**：ETH/BSC/TRON/SOL HTTP 轮询
- **批量导入**：支持单次最多 1 万个地址批量添加
- **可靠推送**：RabbitMQ 死信队列实现指数退避重试（1m/5m），最多重试 3 次
- **签名验证**：HMAC-SHA256 签名，用户可验证推送来源
- **高可用**：Worker 主备选举（Redis 分布式锁），RPC 主备自动切换

---

## 系统架构

```
用户/业务系统
      │ JWT / API Key
      ▼
 API Service          → 用户注册登录、应用管理、监控地址管理
      │
   MySQL + Redis
      │ Redis Pub/Sub（地址变更通知）
      ▼
   Worker             → 链上数据采集 → 地址匹配 → 发 RabbitMQ
      │
  RabbitMQ
  matched.events → dispatch.tasks → 延迟重试队列 → dispatch.dead
      │
 Dispatcher          → 消费队列 → HMAC 签名 → HTTP POST 推送
      │
用户回调 Endpoint
```

### 地址匹配三层漏斗

```
链上地址
  → Bloom Filter（进程内，纳秒级，排除 99%+ 无关地址）
  → Redis 热集合（毫秒级，命中热地址）
  → MySQL 冷地址（极少触发，命中后异步升热）
```

---

## 技术栈

| 模块 | 选型 |
|---|---|
| 语言 | Go 1.22 |
| HTTP 框架 | Gin |
| 数据库 | MySQL 8.0 |
| 缓存 | Redis 7 |
| 消息队列 | RabbitMQ 3.12 |
| 数据库迁移 | golang-migrate |
| EVM 交互 | go-ethereum |
| 配置管理 | Viper |
| 日志 | Zap |

---

## 目录结构

```
address-monitor/
├── cmd/
│   ├── api/main.go           # API Service 入口
│   ├── worker/main.go        # Worker 入口
│   └── dispatcher/main.go    # Dispatcher 入口
├── internal/
│   ├── api/
│   │   ├── dto/              # 请求/响应结构体
│   │   ├── handler/          # HTTP handler（解析请求、返回响应）
│   │   ├── service/          # 业务逻辑
│   │   └── middleware/       # 鉴权、限流、日志中间件
│   ├── chain/                # 链监听层（Listener、Supervisor、BlockTracker）
│   │   ├── evm/              # ETH/BSC HTTP 轮询
│   │   ├── tron/             # TRON HTTP 轮询
│   │   └── sol/              # SOL WebSocket 订阅
│   ├── matcher/              # 地址匹配三层漏斗 + BF 管理
│   ├── parser/               # 链上事件解析（标准化 NormalizedEvent）
│   ├── dispatcher/           # 推送执行、重试、死信处理
│   ├── mq/                   # RabbitMQ 封装（Publisher Confirms）
│   ├── store/                # 数据库操作层
│   └── config/               # 配置加载、MySQL/Redis 初始化
├── pkg/
│   ├── jwt/                  # JWT 生成/验证，Refresh Token 旋转
│   ├── email/                # 邮件发送（支持 dev 模式）
│   ├── bloom/                # Bloom Filter 封装（序列化/并发安全）
│   ├── distlock/             # Redis 分布式锁
│   ├── httputil/             # HTTP 客户端封装
│   └── signature/            # HMAC-SHA256 签名
├── migrations/               # 数据库迁移文件（golang-migrate）
└── configs/
    ├── config.dev.yaml       # 开发环境配置
    └── config.pro.yaml       # 生产环境配置（敏感信息通过环境变量注入）
```

---

## 快速开始

### 1. 启动依赖服务

```bash
docker-compose up -d
```

启动 MySQL 8.0、Redis 7、RabbitMQ 3.12。

RabbitMQ 管理界面：http://localhost:15672 （admin/admin）

### 2. 配置

复制并修改开发配置：

```bash
# configs/config.dev.yaml 已包含本地开发默认值，可直接使用
# 需要修改的配置项：
#   chains.eth.rpc_url      填入 ETH RPC 地址
#   chains.sol.rpc_url      填入 SOL WebSocket 地址
```

### 3. 启动服务

分别在三个终端启动：

```bash
# API Service（包含自动数据库迁移）
APP_ENV=dev go run cmd/api/main.go

# Worker（指定监听的链）
APP_ENV=dev ENABLED_CHAINS=eth go run cmd/worker/main.go

# Dispatcher
APP_ENV=dev go run cmd/dispatcher/main.go
```

---

## API 接口

### 认证接口（无需鉴权）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /auth/register | 注册（发送验证邮件） |
| POST | /auth/login | 登录（返回 Access + Refresh Token） |
| POST | /auth/refresh | 刷新 Token |
| POST | /auth/logout | 登出 |
| GET | /auth/verify-email | 验证邮箱 |
| POST | /auth/resend-verify | 重发验证邮件 |

### 应用管理（JWT 鉴权）

请求头：`Authorization: Bearer {access_token}`

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /v1/apps | 创建应用 |
| GET | /v1/apps | 查询应用列表 |
| GET | /v1/apps/:id | 查询单个应用 |
| PUT | /v1/apps/:id | 修改应用 |
| DELETE | /v1/apps/:id | 删除应用 |
| POST | /v1/apps/:id/reset-key | 重置 API Key |
| POST | /v1/apps/:id/reset-secret | 重置签名密钥 |

### 监控地址管理（API Key 鉴权）

请求头：`X-API-Key: {api_key}`

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /v1/addresses | 新增监控地址 |
| POST | /v1/addresses/batch | 批量新增（最多1万个） |
| GET | /v1/addresses | 查询监控地址列表 |
| GET | /v1/addresses/:id | 查询单条 |
| DELETE | /v1/addresses/:id | 删除 |
| GET | /v1/webhook/url | 查询回调地址 |
| POST | /v1/webhook/url | 设置回调地址 |
| GET | /v1/webhook/logs | 查询推送记录 |
| POST | /v1/webhook/logs/:id/resend | 手动重推 |

---

## Webhook 推送格式

每次事件触发后，系统向应用配置的回调地址发送 HTTP POST 请求：

```
POST {callback_url}
Content-Type: application/json
X-Signature: sha256={HMAC-SHA256}
X-Event-ID: {event_id}
```

### 请求体示例

```json
{
  "event_id": "a1b2c3d4...",
  "chain": "ETH",
  "tx_hash": "0xabcd...",
  "block_number": 19000000,
  "block_time": 1710000000,
  "event_type": "TOKEN_TRANSFER",
  "watched_address": "0xabc...",
  "direction": "IN",
  "from": "0xdef...",
  "to": "0xabc...",
  "asset": {
    "symbol": "USDT",
    "contract_address": "0xdac17f...",
    "amount": "1000000",
    "decimals": 6
  }
}
```

### event_type 枚举

| 值 | 说明 |
|---|---|
| NATIVE_TRANSFER | 原生币转账（ETH/BNB/TRX/SOL）|
| TOKEN_TRANSFER | ERC20/TRC20/SPL Token 转账 |
| SWAP | DEX 兑换 |
| STAKE | 质押 |
| UNSTAKE | 解除质押 |
| CONTRACT_CALL | 其他合约调用 |

### 签名验证示例（Go）

```go
func Verify(body []byte, secret, sigHeader string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(sigHeader))
}
```

### 响应约定

- 返回 `2xx` → 推送成功，不再重试
- 其他状态码或超时（10s）→ 进入重试队列

---

## 推送重试策略

| 重试次数 | 等待时间 | 累计等待 |
|---|---|---|
| 第 1 次 | 1 分钟 | 1 分钟 |
| 第 2 次 | 5 分钟 | 6 分钟 |
| 第 3 次 | 30 分钟 | 36 分钟 |
| 第 5 次 | 放弃，打告警 | — |

---

## 数据库表说明

| 表名 | 说明 | 分表/分区 |
|---|---|---|
| users | 用户 | — |
| email_verifications | 邮箱验证 | — |
| refresh_tokens | 刷新令牌 | — |
| apps | 应用 | — |
| watched_addresses | 监控地址 | — |
| webhook_logs | 推送记录 | 按月分区 |
| chain_raw_events_eth | ETH 原始事件（7天保留）| 按链分表 |
| chain_raw_events_bsc | BSC 原始事件 | 按链分表 |
| chain_raw_events_tron | TRON 原始事件 | 按链分表 |
| chain_raw_events_sol | SOL 原始事件 | 按链分表 |
| chain_sync_status | Worker 同步状态 | — |

---

## 环境变量

| 变量 | 说明 | 示例 |
|---|---|---|
| APP_ENV | 运行环境，加载对应配置文件 | `dev` / `pro` |
| ENABLED_CHAINS | Worker 启用的链（逗号分隔）| `eth,bsc` |
| POD_NAME | K8s Pod 名称，用作 Worker 实例 ID | `worker-eth-0` |
| MYSQL_DSN | 覆盖配置文件中的数据库连接串 | `root:xxx@tcp(...)` |
| REDIS_ADDR | 覆盖 Redis 地址 | `redis:6379` |
| RABBITMQ_URL | 覆盖 RabbitMQ 连接串 | `amqp://...` |

---

## 测试网 RPC 地址

| 链 | RPC |
|---|---|
| ETH Sepolia | https://ethereum-sepolia-rpc.publicnode.com |
| BSC Testnet | https://bsc-testnet-rpc.publicnode.com |
| TRON Shasta | https://api.shasta.trongrid.io |
| SOL Devnet | https://api.devnet.solana.com |

---

## License

MIT
