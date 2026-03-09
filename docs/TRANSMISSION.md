# Transmission Configuration Guide

This guide explains how to securely configure Transmission so that Transmitter can connect to it safely.

## How Transmitter Connects

Transmitter talks to Transmission exclusively from the **server side** — credentials never reach the browser. The Go backend authenticates via `Authorization: Basic <base64>` on every RPC request and manages the `X-Transmission-Session-Id` token transparently.

```
Browser → POST /api/rpc → Transmitter service (auth + whitelist) → Transmission RPC
```

The proxy enforces a **method whitelist**: only `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove`, and `session-get` are forwarded. All other methods are rejected with `403`.

## Required: Enable RPC Authentication

Transmitter **requires** `TRANSMISSION_USER` and `TRANSMISSION_PASS` to be set. The corresponding Transmission setting must match.

In `/etc/transmission-daemon/settings.json` (stop the daemon before editing):

```json
{
  "rpc-authentication-required": true,
  "rpc-username": "your-username",
  "rpc-password": "your-password"
}
```

Transmission hashes the password on first start — the plain text you write gets replaced with a hash like `{5d1a45db...`. This is expected behavior.

Set matching credentials in `.env`:

```env
TRANSMISSION_USER=your-username
TRANSMISSION_PASS=your-password
```

## Optional: IP Whitelist

`rpc-bind-address: "0.0.0.0"` is intentional if you also use the built-in Transmission Web UI from other machines on your local network. Authentication (above) is the primary protection in this case.

As an additional layer, you can restrict which IPs are allowed to connect at all:

```json
{
  "rpc-whitelist-enabled": true,
  "rpc-whitelist": "127.0.0.1,192.168.88.*"
}
```

Wildcards are supported per-octet. Adjust the subnet to match your LAN.

## Docker note

Transmitter's Docker Compose uses `network_mode: host`, so the container shares the host network stack. `TRANSMISSION_URL=http://localhost:9091/transmission/rpc` works without changes regardless of `rpc-bind-address`.

## Recommended settings.json Diff

Starting from the example config, the minimum required change:

| Setting | Current | Recommended |
|---|---|---|
| `rpc-authentication-required` | `false` | `true` |
| `rpc-username` | `""` | your username |
| `rpc-password` | `""` | your password (plain, gets hashed on restart) |

## Verifying the Connection

After applying changes, restart Transmission and check the Transmitter health endpoint:

```sh
curl http://localhost:8080/api/health
# {"status":"ok"}
```

A `502` response means Transmitter cannot reach Transmission — check `TRANSMISSION_URL`, credentials, and that Transmission is running. A `401` from Transmission means credentials mismatch.

## Do Not Expose Transmission RPC Externally

Port `9091` should never be open to the internet. Transmitter is the only intended client — it proxies, validates, and authenticates all requests. If you need remote access, expose Transmitter (port `8080`) behind a reverse proxy with TLS, not Transmission directly.
