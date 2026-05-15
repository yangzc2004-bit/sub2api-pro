# Sub2API Pro

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

**在原版 Sub2API 基础上增强的多平台 AI API 网关**

中文 | [English](README.md)

</div>

> 本仓库是 `sub2api-pro` 分支/发行版，基于开源项目 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 二次开发，保留原项目的 API Key 分发、计费、调度、支付、管理后台等能力，并增加更多上游平台、OAuth/Token 接入、测试工具和前端管理能力。

## 项目定位

Sub2API Pro 适合需要统一管理多种 AI 订阅/API 资源的场景：

- 将不同平台账号统一转换为 OpenAI / Anthropic 兼容接口；
- 通过分组、账号池、API Key、限流和计费规则分发额度；
- 在后台完成账号接入、可用性测试、模型白名单、模型映射、错误透传和用量统计；
- 支持自托管部署，前后端一体化构建。

## Pro 相比原版新增/增强功能

### 1. 新增更多上游平台

原版主要围绕 Anthropic、OpenAI、Gemini、Antigravity 等平台。Pro 版新增/扩展：

| 平台 | 接入方式 | 主要能力 |
|---|---|---|
| **Kimi Code** | OAuth / access token | OpenAI Chat Completions 兼容转发、Kimi Code 专用请求头、流式/非流式返回、工具调用兼容处理 |
| **Qwen Web** | Session Token + Cookie | 将 Qwen 网页版能力封装为 OpenAI Chat Completions；支持 Anthropic `/v1/messages` 转换到 Chat Completions；支持模型映射、附件/多模态输入辅助处理、工具调用提示与解析 |
| **MiMo Token Plan** | API Key | 同时支持 OpenAI-compatible `/v1/chat/completions` 与 Anthropic-compatible `/v1/messages`；支持多区域/自定义 Base URL |
| **DeepSeek** | API Key | 作为独立平台接入 OpenAI-compatible API，默认模型映射到 `deepseek-v4-flash` / `deepseek-v4-pro` |

### 2. Kimi OAuth 接入流程

Pro 版新增 Kimi 设备授权流程：

- 后台生成 Kimi OAuth 授权 URL / User Code；
- 轮询换取 access token / refresh token；
- 支持刷新单个 Kimi 账号 token；
- 支持按代理发起授权和刷新请求；
- 前端提供 Kimi OAuth 操作入口和账号创建表单。

相关接口：

```text
POST /api/v1/admin/kimi/oauth/auth-url
POST /api/v1/admin/kimi/oauth/exchange-code
POST /api/v1/admin/kimi/oauth/refresh-token
POST /api/v1/admin/kimi/oauth/accounts/:id/refresh
```

### 3. 网关协议增强

- `/v1/chat/completions` 可按账号平台自动路由到 Kimi、Qwen、MiMo、DeepSeek 等上游；
- `/v1/messages` 对 MiMo 走 Anthropic-compatible 原生接口，对 Qwen 自动转换为 Chat Completions；
- `/v1/models` 为 Qwen、MiMo 返回平台专属模型列表；
- 对不支持的接口（如 Qwen/MiMo 的 Responses API、Token Counting）返回明确的 OpenAI/Anthropic 风格错误；
- Kimi 工具调用场景自动关闭不兼容 thinking 参数，提升 Claude Code / Codex 类客户端兼容性。

### 4. 模型映射与白名单扩展

内置新增默认映射：

- Kimi：`kimi-k2.6`、`kimi-k2.6-thinking`、`kimi-k2.6-vision`、`kimi-k2.6-tools` 等别名统一映射到 Kimi Code 上游模型；
- Qwen：`qwen3.6-plus`、`qwen3-coder-plus`、`qwen3-coder`、`qwq-plus`、`qwen3.5-vl-plus` 等；
- MiMo：`mimo-v2.5-pro`、`mimo-v2.5`、`mimo-v2-pro`、`mimo-v2-omni`、`mimo-v2-flash`、TTS 相关模型；
- DeepSeek：`deepseek-v4-flash`、`deepseek-v4-pro`。

前端账号创建、编辑、筛选、平台图标、平台颜色、用 Key 弹窗和图表均已适配这些平台。

### 5. 账号连通性测试增强

后台账号测试能力新增：

- Kimi OAuth 账号流式 Chat Completions 测试；
- Qwen Web Token/Cookie 测试；
- MiMo Token Plan API Key 测试；
- DeepSeek API Key Chat Completions 测试；
- 测试过程通过 SSE 实时返回事件，便于后台查看上游响应和错误。

### 6. Gemini Google One 429 策略修复

针对 Gemini CLI / Google One Pro 反代账号，已修复 `google_one` / `google_ai_pro` OAuth 账号在上游返回 `MODEL_CAPACITY_EXHAUSTED` 临时 429 时，被误判为 AI Studio/API Key 日配额并锁到次日的问题。

- `google_one` 临时容量型 429 改为按 Gemini tier cooldown 处理，`google_ai_pro` 默认短冷却为 5 分钟；
- AI Studio / API Key 的日配额类 429 仍保留 PST 午夜重置兜底；
- 覆盖了 `RESOURCE_EXHAUSTED` + `MODEL_CAPACITY_EXHAUSTED` 回归测试，避免短时间少量请求后被错误锁到第二天。

### 7. 计费与定价增强

- Kimi 模型新增静态兜底定价，远程定价源缺失时仍可自动填充；
- MiMo 模型新增零价/自定义兜底定价入口，方便作为内部额度或订阅套餐资源管理；
- 支持本地 `model_pricing.json` / `model_pricing.sha256` 快照作为定价数据补充。

### 8. 前端管理体验增强

- 新增 Kimi、MiMo、Qwen、DeepSeek 平台选择入口；
- 新增平台说明卡片、默认 Base URL、专属凭证字段；
- API Key 使用弹窗补充 OpenAI-compatible、Anthropic-compatible、平台专属示例；
- 管理后台图表和筛选器识别新增平台；
- Vite 开发代理和公开设置注入优化；
- 前端构建后自动复制到后端嵌入目录，方便一体化打包。

## 原版 Sub2API 已保留能力

- 多账号池管理：OAuth、API Key、Setup Token、Upstream、Bedrock、Service Account 等；
- API Key 分发和用户额度管理；
- Token 级用量统计和成本计算；
- 智能调度、粘性会话、并发控制、速率限制；
- 内置支付系统：EasyPay、支付宝、微信支付、Stripe；
- 管理后台：用户、账号、分组、渠道、订阅、订单、监控、设置；
- PostgreSQL + Redis 后端架构；
- Docker / Docker Compose / 二进制部署。

## 技术栈

| 组件 | 技术 |
|---|---|
| 后端 | Go 1.26+、Gin、Ent |
| 前端 | Vue 3、Vite 5、TailwindCSS、Pinia |
| 数据库 | PostgreSQL 15+ |
| 缓存/队列 | Redis 7+ |
| 构建 | pnpm / npm、Go build、Docker |

## 快速开始

### 方式一：Docker Compose

```bash
git clone https://github.com/yangzc2004-bit/sub2api-pro.git
cd sub2api-pro
cp deploy/config.example.yaml deploy/config.yaml  # 如存在示例配置，请按需调整

docker compose up -d
```

启动后访问：

```text
http://服务器IP:8080
```

首次启动按安装向导配置 PostgreSQL、Redis 和管理员账号。

### 方式二：本地开发

后端：

```bash
cd backend
go mod download
go run ./cmd/server
```

前端：

```bash
cd frontend
pnpm install
pnpm dev
```

默认前端开发端口为 `3000`，可通过环境变量调整：

```bash
VITE_DEV_PROXY_TARGET=http://localhost:8080
VITE_DEV_PORT=3000
```

### 构建前端并嵌入后端

```bash
cd frontend
pnpm build
```

Pro 版的 `postbuild` 会将 `frontend/dist` 复制到：

```text
backend/internal/web/dist
```

便于后端二进制直接嵌入前端静态资源。

## Nginx 反向代理提示

如果通过 Nginx 代理并搭配 Codex CLI / Claude Code 等客户端使用，请在 Nginx `http` 配置块中启用：

```nginx
underscores_in_headers on;
```

否则 Nginx 默认会丢弃带下划线的请求头，可能影响粘性会话和部分客户端兼容逻辑。

## 常用接口

OpenAI-compatible：

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses        # 仅支持对应平台
```

Anthropic-compatible：

```text
POST /v1/messages
POST /v1/messages/count_tokens  # 仅支持对应平台
```

管理后台：

```text
/api/v1/admin/...
```

## 重要说明

- Kimi、Qwen、MiMo、DeepSeek 的上游模型、鉴权字段和可用接口可能随平台变化，需要以实际账号权限和上游返回为准；
- Qwen Web 接入依赖浏览器会话 Token/Cookie，过期后需重新更新；
- Gemini `google_one` OAuth 账号遇到临时容量型 429 时会走短冷却，不再按日配额锁到次日；AI Studio/API Key 的日配额兜底逻辑保持不变；
- 建议为不同平台单独建分组，分别配置模型白名单、倍率、限流和错误透传规则；
- 生产环境请务必配置 HTTPS、强密码、数据库备份、Redis 持久化和反向代理超时。

## 致谢与许可

本项目基于 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 开源项目二次开发，感谢原项目作者和社区贡献者。

许可证请参见仓库中的 [LICENSE](LICENSE)。
