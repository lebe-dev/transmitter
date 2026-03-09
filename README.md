# transmitter

> Web UI for Transmission + Telegram Bot

Modern, lightweight alternative to Transmission's stock UI. Runs with zero external dependencies.

---

## Features

### Web UI

- **Torrent list** — sortable table: name, status, progress, size, speed, added date, ETA
- **Status filters** — All, Downloading, Seeding, Paused, Done
- **Search** — filter torrents by name (case-insensitive)
- **Add torrents** — magnet links or .torrent file upload
- **Management** — pause, resume, delete torrents
- **Auto-refresh** — live updates every 3–5 seconds

### Telegram Bot

- `/start` — greeting and authorization
- `/add <magnet>` — add torrent by magnet link
- `.torrent` file upload — add torrent file directly
- `/status` — view active torrents and speeds
- `/help` — command reference

---

## Architecture

Single Go binary runs in two modes:

```
Browser (web UI)  ──→  /api/rpc  ──→  Go HTTP server  ──→  Transmission RPC
Telegram         ──→  long poll  ──→  Go bot         ──→  Transmission RPC
```

- **Go server:** proxies requests to Transmission, serves embedded static UI
- **Telegram bot:** long polling (no webhooks), whitelist-based authorization
- **Frontend:** Svelte 5 SPA, all logic runs in browser, no server-side computation

For detailed architecture, see [DEV.md](DEV.md).

---

## Getting Started

### Prerequisites

- Docker & Docker Compose
- Transmission daemon running at `localhost:9091`
- Telegram bot token (for bot mode, optional)

### Local Setup

1. Clone and copy config:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` with your Transmission credentials and Telegram token (if using bot)

3. Start services:
   ```bash
   docker-compose up -d
   ```

4. Open browser: `http://localhost:8080`

### Configuration

All settings via environment variables (see `.env.example`):

| Variable | Required | Default |
|-----------|----------|---------|
| `TRANSMISSION_USER` | Yes | — |
| `TRANSMISSION_PASS` | Yes | — |
| `TRANSMISSION_URL` | No | `http://localhost:9091/transmission/rpc` |
| `LISTEN_ADDR` | No | `:8080` |
| `TELEGRAM_TOKEN` | No | (bot disabled if empty) |
| `TELEGRAM_USERS` | If using bot | — |

For all options, see `.env.example`.

---

## Security

See [SECURITY.md](SECURITY.md).

---

## Roadmap

- Dark / light theme in UI
- Toggle features: webui, telegram bot
- Grouping by folders / labels (labels added in Transmission 4.0)
- Pause / delete via Telegram bot (command expansion)
- Telegram notifications on torrent completion (polling + state diff)
- WebSocket instead of polling for real-time UI updates
- Web UI authentication (Basic Auth middleware) for external access via VPN
- RSS feeds for automatic torrent addition
- Detailed torrent view: file list, peers, trackers
