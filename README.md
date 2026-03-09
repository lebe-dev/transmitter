# Transmitter

![Transmitter screenshot](screenshot.png)
![Transmitter screenshot dark](screenshot-dark.png)

Transmitter is a modern, lightweight alternative to Transmission's stock UI. Runs with zero external dependencies. Also Telegram bot integration.

## Features

- **Torrent list** — sortable table: name, status, progress, size, speed, added date, ETA
- **Status filters** — All, Downloading, Seeding, Paused, Done
- **Search** — filter torrents by name (case-insensitive)
- **Add torrents** — magnet links or .torrent file upload
- **Management** — pause, resume, delete torrents
- **Auto-refresh** — live updates every 3–5 seconds

## Getting Started

```bash
cp .env.example .env

# edit .env for your needs

docker-compose up -d
```

Open browser: `http://localhost:8080`

### Configuration

All settings via environment variables:

| Variable | Required | Default |
|-----------|----------|---------|
| `TRANSMISSION_USER` | Yes | — |
| `TRANSMISSION_PASS` | Yes | — |
| `TRANSMISSION_URL` | No | `http://localhost:9091/transmission/rpc` |
| `LISTEN_ADDR` | No | `:8080` |
| `TELEGRAM_TOKEN` | No | (bot disabled if empty) |
| `TELEGRAM_USERS` | If using bot | — |

For all options, see [.env.example](.env.example).

## Security

See [SECURITY.md](docs/SECURITY.md).

## Roadmap

- Pin torrent on top
- UX: 
  - Suggest paths
  - Remembed paths
- Support locales: EN, RU, ES, DE, GE
- Toggle features: webui, telegram bot
- Grouping by folders / labels (labels added in Transmission 4.0)
- Pause / delete via Telegram bot (command expansion)
- Telegram notifications on torrent completion (polling + state diff)
- WebSocket instead of polling for real-time UI updates
- Web UI authentication (Basic Auth middleware) for external access via VPN
- RSS feeds for automatic torrent addition
- Detailed torrent view: file list, peers, trackers
- Support multiple Transmission instances
