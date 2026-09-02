# Human Call Gateway

这是可配置的 Human Call 发信网关（Slack/Telegram/飞书/QQ/Webhook），只保证 sent 语义。

## Contract (v1)

完整的 API 合约和配置规范:

- [**OpenAPI 规范**](docs/openapi.yaml) — v1 API 定义
- [**配置 Schema**](docs/config.schema.json) — 网关配置 JSON Schema
- [**配置示例**](docs/config.example.yaml) — 各通道类型配置示例

### 端点

- `POST /v1/human-call` — 发送结构化人工介入请求

### 错误码

- `INVALID_PAYLOAD` (400) — 请求体字段缺失、为空、过长、类型错误或包含未知字段
- `CONFIG_MISSING` (400) — 通道 ID 未找到、通道已禁用、默认通道未配置或必需的环境变量未设置
- `TRANSPORT_ERROR` (502) — 上游传输服务返回错误
- `TIMEOUT` (504) — 传输超时（硬编码 10 秒，配置和 UI 无法更改）

### 语义约束

- **仅发送**：API 确认消息已离开网关，不等待人类响应
- **无 human_result**：响应中永远不包含人类的回复内容
- **无入站**：网关不接收来自通道的消息
- **无队列**：不重试、不轮询、不保留消息
- **urgent 字段**：可选布尔值（默认 false），原样转发到通道映射器主体，不改变超时（仍然 10s），不重试，不重新排序

### 扩展通道类型

添加新通道类型需要：
1. 实现一个传输映射器（URL + headers + body）
2. 在 `config.schema.json` 添加类型定义
3. 在网关中注册该类型

所有 secrets 必须通过环境变量引用，不得出现在配置文件、日志或 API 响应中。

## 运行

### 配置

网关通过 YAML 配置文件进行配置。配置文件路径可通过以下方式指定（优先级从高到低）：

1. 命令行参数 `-config`
2. 环境变量 `CONFIG_PATH`
3. 默认路径 `./config.yaml`

### 环境变量

所有敏感信息（webhook URLs、tokens 等）必须通过环境变量提供。配置文件中只包含环境变量的名称。

根据配置的通道类型，需要设置以下环境变量：

- **Slack**: `SLACK_WEBHOOK_URL` (或配置中指定的其他名称)
- **Telegram**: `TELEGRAM_BOT_TOKEN` (或配置中指定的其他名称)
- **Feishu**: `FEISHU_WEBHOOK_URL` (或配置中指定的其他名称)
- **QQ**: `QQ_ACCESS_TOKEN` (可选，或配置中指定的其他名称)
- **Webhook**: 根据配置中的 `url_env` 和 `headers` 中使用的 `${ENV_VAR}` 占位符

### 配置示例

创建 `config.yaml`：

```yaml
default_channel_id: "slack-alerts"

channels:
  - id: "slack-alerts"
    type: slack
    enabled: true
    slack:
      webhook_url_env: SLACK_WEBHOOK_URL

  - id: "telegram-ops"
    type: telegram
    enabled: true
    telegram:
      bot_token_env: TELEGRAM_BOT_TOKEN
      chat_id: "-1001234567890"
      api_base_url: "https://api.telegram.org"

  - id: "feishu-team"
    type: feishu
    enabled: true
    feishu:
      webhook_url_env: FEISHU_WEBHOOK_URL

  - id: "qq-group"
    type: qq
    enabled: true
    qq:
      http_api_url: "http://127.0.0.1:5700/send_msg"
      access_token_env: QQ_ACCESS_TOKEN
      group_id: "987654321"

  - id: "generic-webhook"
    type: webhook
    enabled: true
    webhook:
      url_env: GENERIC_WEBHOOK_URL
      method: POST
      headers:
        Authorization: "Bearer ${WEBHOOK_API_TOKEN}"
        X-Custom-Header: "human-call-gateway"
      content_type: "application/json"
```

### 启动网关

```bash
# 设置环境变量
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
export TELEGRAM_BOT_TOKEN="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
export FEISHU_WEBHOOK_URL="https://open.feishu.cn/open-apis/bot/v2/hook/YOUR-WEBHOOK-TOKEN"

# 运行网关（默认监听 :8080）
go run cmd/gateway/main.go

# 或指定配置文件
go run cmd/gateway/main.go -config /path/to/config.yaml

# 或使用环境变量指定配置
export CONFIG_PATH=/path/to/config.yaml
go run cmd/gateway/main.go

# 指定端口（默认 8080）
export PORT=3000
go run cmd/gateway/main.go
```

### 编译并运行

```bash
# 编译
go build -o gateway cmd/gateway/main.go

# 运行
./gateway -config config.yaml
```

### 发送请求

```bash
curl -X POST http://localhost:8080/v1/human-call \
  -H "Content-Type: application/json" \
  -d '{
    "need": "需要前端工程师",
    "blocker": "API 返回格式变更导致页面白屏",
    "action": "请修复 /api/users 接口的响应处理",
    "fallback": "暂时使用旧版本回滚"
  }'
```

带 urgent 和 channel_id：

```bash
curl -X POST http://localhost:8080/v1/human-call \
  -H "Content-Type: application/json" \
  -d '{
    "need": "需要 DBA",
    "blocker": "生产数据库连接池耗尽",
    "action": "请紧急扩容数据库连接数",
    "fallback": "触发自动降级，关闭非核心功能",
    "channel_id": "telegram-ops",
    "urgent": true
  }'
```

成功响应（200）：

```json
{
  "sent": true,
  "id": "1234567890.123456"
}
```

错误响应（400/502/504）：

```json
{
  "sent": false,
  "error": {
    "code": "INVALID_PAYLOAD",
    "message": "Field 'need' is required and must be non-empty"
  }
}
```

## 开发

### 运行测试

```bash
go test ./...
```

### 项目结构

```
.
├── cmd/gateway/          # 主程序入口
│   └── main.go
├── internal/
│   ├── adapter/          # 通道适配器
│   │   ├── adapter.go    # 接口定义和注册
│   │   ├── httpclient.go # 共享 HTTP 客户端
│   │   ├── slack.go
│   │   ├── telegram.go
│   │   ├── feishu.go
│   │   ├── qq.go
│   │   └── webhook.go
│   ├── config/           # 配置加载和验证
│   │   └── config.go
│   └── http/             # HTTP 处理器
│       ├── handler.go
│       └── handler_test.go
├── docs/                 # API 规范和配置 schema
├── config.yaml           # 配置文件（需创建）
├── go.mod
└── README.md
```

## 安全注意事项

- **不要**在配置文件中包含任何敏感信息（tokens、webhook URLs 等）
- **不要**在日志中打印环境变量的值
- **不要**在错误消息中返回完整的 webhook URLs 或 tokens
- 所有敏感信息必须通过环境变量引用
