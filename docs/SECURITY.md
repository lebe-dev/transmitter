# Security

## Design Principles

Transmitter is designed for **home network deployment** where all users are trusted. Security features are implemented to prevent accidental exposure and protect credentials.

## Authentication & Authorization

- **Web UI:** no user authentication. Accessible only on home network. If external access is needed later, add middleware with Basic Auth or cookie session.
- **Telegram bot:** whitelist by user ID. Unauthorized messages ignored (no response, logged at warn level).

## Credentials Management

- **Transmission RPC:** credentials stored in `.env` file and container env vars. Never sent to frontend — proxied server-side in Go.
- Credentials injected via `Authorization: Basic <base64>` header (HTTP Basic Auth)
- Environment variables loaded via `godotenv`, not exposed in logs

## CSRF Protection

- **Transmission RPC:** uses its own CSRF mechanism (`X-Transmission-Session-Id` header)
- Go proxy manages token lifecycle transparently:
  1. Fetches session ID on first request (10s timeout)
  2. On 409 Conflict response, extracts new ID from header
  3. Retries request with fresh token (one retry)
  4. Caches valid token in memory (`atomic.Value` with singleflight)

## Network Security

- **CORS:** explicit origin checking (no wildcard), prevents DNS rebinding attacks
- `network_mode: host` in Docker — simplifies localhost access to Transmission
- No external exposure — assumes private network

## Request Validation

- **RPC whitelist:** only 6 methods allowed:
  - `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove`, `session-get`
- **Request size limit:** 1 MB max payload (prevents DoS)
- Raw JSON-RPC forwarding (no user input in RPC calls except torrent data)

## Recommendations for External Access

If exposing Transmitter to the internet or untrusted networks:

1. **Add authentication** — Basic Auth or session middleware in Go
2. **Use HTTPS** — reverse proxy with TLS (nginx, Caddy, etc.)
3. **Restrict IP range** — firewall or reverse proxy rules
4. **VPN/tunnel** — SSH port forwarding or WireGuard for remote access
5. **Rate limiting** — implement rate limits on `/api/rpc` endpoint
6. **Change defaults** — ensure strong Transmission credentials

## Data Privacy

- All Transmission data (torrent files, credentials) handled server-side
- Frontend receives only: torrent metadata, status, progress
- Session tokens not exposed to client
- Telegram interactions logged at configured level
