# Human Call Gateway

这是可配置的 Human Call 发信网关（Slack/Telegram/飞书/QQ/Webhook），只保证 sent 语义。

## Contract (v1)

完整的 API 合约和配置规范：

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
