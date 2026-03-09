# Development Guide

## Project Structure

```
transmitter/
├── cmd/
│   ├── transmitter/
│   │   └── main.go          // entry point
├── internal/
│   ├── config/              // env vars + .env file loading
│   │   └── config.go
│   ├── transmission/         // Transmission RPC client
│   │   ├── client.go         // HTTP client, session ID management
│   │   └── types.go          // JSON-RPC request/response structs
│   ├── server/               // HTTP server and handlers
│   │   ├── server.go         // server setup
│   │   ├── proxy.go          // RPC proxy handler + whitelist
│   │   ├── cors.go           // CORS middleware
│   │   └── static.go         // SPA static asset serving
│   └── bot/                  // Telegram bot
│       ├── bot.go            // initialization, polling loop
│       └── handlers.go       // /add, /status, file handler
├── frontend/                 // Svelte 5 SPA
│   ├── src/
│   │   ├── lib/
│   │   │   ├── api.ts        // fetch wrapper for /api/rpc
│   │   │   ├── types.ts      // TypeScript torrent types
│   │   │   └── stores.ts     // Svelte stores: torrents, filters
│   │   ├── routes/
│   │   │   └── +page.svelte  // main page
│   │   └── +layout.ts        // layout with ssr=false
│   ├── static/
│   ├── package.json
│   └── svelte.config.js      // adapter-static config
├── static/                   // go:embed target (build output)
│   └── embed.go              // //go:embed all:dist
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml
├── Justfile
└── .env.example
```

## Architecture

### High-Level Diagram

Single Go binary runs in two modes:

```
Browser (web UI)  ──→  /api/rpc  ──→  Go HTTP server  ──→  Transmission RPC
Telegram         ──→  long poll  ──→  Go bot         ──→  Transmission RPC
```

### Components

- **Go HTTP server:** proxies requests to Transmission, serves embedded static UI
- **Telegram bot:** long polling (no webhooks), whitelist-based authorization
- **Frontend:** Svelte 5 SPA, all logic runs in browser, no server-side computation

### Design Principles

- Go service is the single point of access to Transmission RPC. Frontend never contacts Transmission directly.
- Go injects HTTP Basic Auth and `X-Transmission-Session-Id` into each proxied request, managing CSRF token automatically (retry on 409).
- All sorting, filtering, and search logic runs on the client (Svelte). Transmission returns the complete torrent list in one RPC call, with small payload sizes.
- Bot and server share the same internal library for Transmission RPC communication.

## Configuration

All parameters are read from environment variables. A `.env` file in the working directory is automatically loaded (`godotenv`).

| Variable | Description | Example |
|-----------|----------|--------|
| `TRANSMISSION_URL` | Transmission RPC URL | `http://localhost:9091/transmission/rpc` |
| `TRANSMISSION_USER` | Transmission login | `admin` |
| `TRANSMISSION_PASS` | Transmission password | `secret` |
| `LISTEN_ADDR` | Web server listen address | `:8080` |
| `CORS_ORIGIN` | CORS origin (no wildcard) | `http://localhost:8080` |
| `TELEGRAM_TOKEN` | Telegram bot token | `123456:ABC...` |
| `TELEGRAM_USERS` | Whitelist user IDs (comma-separated) | `12345,67890` |
| `LOG_LEVEL` | Log level | `info` |

See `.env.example` for details.

## Key Dependencies

### Go

| Package | Purpose |
|-------|-----------|
| `github.com/joho/godotenv` | Load .env file |
| `golang.org/x/sync` | Singleflight for session ID refresh |
| `gopkg.in/telebot.v4` | Telegram Bot API (long polling) |

Transmission RPC client is a custom implementation (`internal/transmission`), with no external dependencies. The protocol is simple (JSON-RPC over HTTP with CSRF token).

### Frontend

| Package | Purpose |
|-------|-----------|
| `svelte@5` | UI framework |
| `@sveltejs/adapter-static` | Build to static files for embedding |
| `shadcn-svelte` | Component library (Bits UI) |
| `tailwindcss@4.1` | Utility CSS with oklch support |
| `lucide-svelte` | Icons |
| `@tanstack/svelte-table` | Table/data grid |
| `svelte-sonner` | Toast notifications |
| `mode-watcher` | Dark/light theme support |

## Docker and Deployment

### Multi-Stage Dockerfile

Build in three stages:

1. **Stage 1** (`node:22-alpine`): `yarn install && yarn build` in `frontend/` — compiles Svelte to static files
2. **Stage 2** (`golang:1.24-alpine`): copies built static files to `static/dist/`, `go build` — compiles with embedded assets
3. **Stage 3** (`alpine:3.23.2`): final image with single binary (~20–30 MB), runs as non-root (uid 10001)

Build platform: `--platform linux/arm/v7` for Raspberry Pi 2. Use `buildx` for cross-compilation on macOS.

```bash
just docker-build  # multi-platform build for ARMv7 (Raspberry Pi 2)
```

### docker-compose.yml

```yaml
version: "3.8"
services:
  web:
    image: transmitter:latest
    ports: ["8080:8080"]
    environment:
      - TRANSMISSION_USER=admin
      - TRANSMISSION_PASS=secret
    env_file: .env
    restart: unless-stopped
    network_mode: host        # access to localhost:9091

  bot:
    image: transmitter:latest
    environment:
      - TRANSMISSION_USER=admin
      - TRANSMISSION_PASS=secret
    env_file: .env
    restart: unless-stopped
    network_mode: host
```

> With `network_mode: host`, port forwarding isn't needed. The container uses the host network directly, simplifying access to Transmission at `localhost:9091`.

## Commands

Use `just` for all common tasks:

```sh
just build          # build-frontend + go build → ./transmitter binary
just build-frontend # yarn install + yarn build, copies output to static/dist/
just run-backend    # go run ./cmd/transmitter
just run-frontend   # cd frontend && yarn dev
just test-backend   # go test ./...
just test name=Foo  # run a single test matching Foo
just lint-backend   # go vet ./...
just lint-frontend  # yarn check (svelte-check)
just format         # go fmt ./...
just docker-build   # multi-platform build for ARMv7 (Raspberry Pi 2)
```

## Architectural Decisions

| Decision | Rationale |
|---------|------------|
| `go:embed` for assets | Single binary = single Docker image. No volume mount dependency. Simplifies Pi deployment |
| Single binary, two modes | Shared codebase (`transmission client`, `config`). Two containers from one image. Can run locally without Docker |
| Long polling for bot | No external URL needed (webhooks require HTTPS domain). Pi is home network — no public IP |
| Client-side sorting | Transmission returns all torrents in one call (<1 KB per torrent). Even 500 torrents = <500 KB. Server pagination unnecessary |
| Custom RPC client | Transmission JSON-RPC is simple (6 whitelisted methods). External library adds dependency with little benefit. Full control over retry, timeout, logging, singleflight |
| `network_mode: host` | Transmission runs at localhost on Pi. Host networking removes extra NAT layer |
| stdlib `net/http` | Lightweight, no framework overhead. Sufficient for simple proxy service |
| `telebot.v4` | Minimal, clean long polling implementation |

## Go Server Implementation Details

### Routes

| Method | Path | Description |
|-------|------|----------|
| `POST` | `/api/rpc` | Proxies JSON-RPC call to Transmission, adding Basic Auth and Session ID |
| `GET` | `/api/health` | Healthcheck: verifies Transmission RPC availability |
| `GET` | `/*` | Serves embedded Svelte SPA assets (`index.html` as fallback) |

### Transmission RPC Proxy

The proxy layer performs these tasks:

1. Receives JSON-RPC request from Svelte (`POST /api/rpc`)
2. Adds headers `Authorization: Basic <base64>` and `X-Transmission-Session-Id`
3. On 409 Conflict — extracts new Session ID from response header and retries (one retry)
4. Caches Session ID in memory (`atomic.Value` with singleflight)
5. Returns Transmission response to client unmodified

**Allowed RPC methods (whitelist):** `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove`, `session-get`

**Torrent fields returned by `torrent-get`:** `id, name, status, percentDone, totalSize, rateDownload, rateUpload, addedDate, eta, hashString, downloadDir, error, errorString`

### Session ID Management

- Session ID cached in `atomic.Value`, refreshed via `singleflight`
- On first request, fetches session ID with 10s timeout
- On 409 response, extracts new ID from `X-Transmission-Session-Id` header and retries once
- Concurrent requests wait for first one to complete (singleflight) to avoid race conditions

## Frontend Implementation Details

### Svelte 5 SPA

Built as static files and embedded in the Go binary via `go:embed`. All Transmission interaction goes through `/api/rpc`.

**Key settings:**
- `SSR` disabled globally (`src/routes/+layout.ts`: `ssr = false`)
- `adapter-static` with `fallback: 'index.html'` for client-side routing

### Transmission RPC Methods Used

Svelte calls `POST /api/rpc` with JSON-RPC payload. Used Transmission methods:

| Method | Purpose and requested fields |
|-------|-------------------------------|
| `torrent-get` | Get list. Fields: `id, name, status, percentDone, totalSize, rateDownload, rateUpload, addedDate, eta, hashString, downloadDir, error, errorString` |
| `torrent-add` | Add torrent. Arguments: `filename` (magnet/URL) or `metainfo` (base64 .torrent) |
| `torrent-start` | Resume. Argument: `ids` |
| `torrent-stop` | Pause. Argument: `ids` |
| `torrent-remove` | Delete. Arguments: `ids`, `delete-local-data` |
| `session-get` | Session info (`download-dir`, `version`) for healthcheck and UI |

## Security

See [SECURITY.md](SECURITY.md) for detailed security information, including:
- Authentication & authorization strategy
- Credentials management
- CSRF protection mechanism
- Network security considerations
- Recommendations for external access
