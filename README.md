# Sub2API Pro

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

**A Pro multi-platform AI API gateway based on Sub2API**

[中文](README_CN.md) | English

</div>

> This repository is `sub2api-pro`, a customized edition based on the open-source [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api). It keeps the original account pool, API key distribution, billing, scheduling, payment, and admin dashboard features, while adding more upstream platforms and management tools.

## What's New in Pro

- **New upstream platforms**: Kimi Code, Qwen Web, MiMo Token Plan, and DeepSeek.
- **Kimi OAuth flow**: device authorization, token exchange, token refresh, proxy-aware authorization, and admin UI integration.
- **Qwen Web gateway**: Session Token + Cookie based access, OpenAI Chat Completions compatible output, Anthropic `/v1/messages` conversion, attachment handling, and tool-call prompt/parse helpers.
- **MiMo Token Plan support**: OpenAI-compatible `/v1/chat/completions` and Anthropic-compatible `/v1/messages` with custom OpenAI/Anthropic base URLs.
- **DeepSeek support**: independent platform for OpenAI-compatible API key accounts.
- **Model mappings**: built-in aliases for Kimi K2.6, Qwen 3.x/Coder/VL, MiMo V2/V2.5/TTS, and DeepSeek V4 models.
- **Account testing**: SSE-based live connectivity tests for Kimi, Qwen, MiMo, and DeepSeek accounts.
- **Gemini Google One 429 fallback**: transient `MODEL_CAPACITY_EXHAUSTED` 429s from `google_one` / Google AI Pro OAuth accounts now use the Gemini tier cooldown instead of the AI Studio/API-key daily reset fallback.
- **Pricing fallback**: static fallback pricing for Kimi and MiMo models, plus local pricing snapshot support.
- **Admin UI updates**: new platform selectors, platform icons/colors, credential forms, filters, charts, and API key usage examples.
- **Build workflow**: frontend build output can be copied into the backend embedded static directory automatically.

## Supported Pro Platforms

| Platform | Auth | Compatibility |
|---|---|---|
| Kimi Code | OAuth / access token | OpenAI Chat Completions |
| Qwen Web | Session Token + Cookie | OpenAI Chat Completions; Anthropic Messages conversion |
| MiMo Token Plan | API Key | OpenAI Chat Completions and Anthropic Messages |
| DeepSeek | API Key | OpenAI-compatible API |

## Quick Start

```bash
git clone https://github.com/yangzc2004-bit/sub2api-pro.git
cd sub2api-pro
```

Backend development:

```bash
cd backend
go mod download
go run ./cmd/server
```

Frontend development:

```bash
cd frontend
pnpm install
pnpm dev
```

Build frontend and embed it into backend static assets:

```bash
cd frontend
pnpm build
```

## Main Endpoints

OpenAI-compatible:

```text
GET  /v1/models
POST /v1/chat/completions
POST /v1/responses
```

Anthropic-compatible:

```text
POST /v1/messages
POST /v1/messages/count_tokens
```

Kimi admin OAuth endpoints:

```text
POST /api/v1/admin/kimi/oauth/auth-url
POST /api/v1/admin/kimi/oauth/exchange-code
POST /api/v1/admin/kimi/oauth/refresh-token
POST /api/v1/admin/kimi/oauth/accounts/:id/refresh
```

## Notes

- Upstream model availability and authentication fields depend on the actual upstream account status.
- Qwen Web accounts rely on browser Session Token/Cookie values and may need manual refresh after expiration.
- Gemini `google_one` OAuth accounts keep short cooldown behavior for temporary model-capacity 429s; AI Studio/API-key accounts still keep the daily reset fallback for quota-style 429s.
- For production deployments, configure HTTPS, strong admin credentials, database backups, Redis persistence, and proper reverse proxy timeouts.
- If using Nginx with Codex CLI / Claude Code style clients, enable:

```nginx
underscores_in_headers on;
```

## Credits and License

Sub2API Pro is based on [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api). Thanks to the original author and community contributors.

See [LICENSE](LICENSE) for license details.
