# Cursor Bridge (0.7) — design notes

Status: **proxy auth shipped** (`KOTRO_BRIDGE_TOKEN` + `KOTRO_UPSTREAM_API_KEY`). Managed `cloudflared` command still planned.

## Why Bridge exists

Cursor Chat/Agent Override Base URL is executed from **Cursor’s cloud**, which SSRF-blocks `localhost`. Kotro cannot fix that. Bridge = managed temporary public HTTPS → local sidecar.

## Security model (implemented in proxy)

Cursor BYOK UI typically exposes **Base URL + API key**, not arbitrary custom headers:

1. Set `KOTRO_BRIDGE_TOKEN` (or extension `kotrolabs.bridgeToken`) — UUID recommended.
2. Set `KOTRO_UPSTREAM_API_KEY` (or `kotrolabs.upstreamApiKey`) to the real provider key.
3. Put the **bridge token** in Cursor’s OpenAI API key field.
4. Put the tunnel URL in Override Base URL (`https://….trycloudflare.com/v1`).

When the bridge token is set, Kotro:

- Rejects LLM routes (`/v1/chat/completions`, `/v1/messages`, `/v1/*` passthrough) without a matching `Authorization: Bearer`, `x-api-key`, or `x-kotro-bridge-token` (**401**).
- Injects the upstream provider key on forward so the bridge token never reaches DeepSeek/OpenAI/Anthropic.
- Returns **503** if the bridge token is set but `KOTRO_UPSTREAM_API_KEY` is missing (except `kotro-local-verify`, which never calls upstream).

`/healthz` and the telemetry socket (`:9090`) stay unauthenticated (loopback by default).

Tunnel URL remains public; **the token** stops anonymous callers who find the URL. Still warn loudly; auto-stop `cloudflared` on `deactivate()` when the managed Bridge command ships.

Do **not** ship assuming Cursor supports custom request headers.

## cloudflared dependency (still planned)

- Do not silently download/bundle without consent.
- If missing: prompt to install (`brew install cloudflare/cloudflare/cloudflared`) via an explicit user action.

## Command sketch

`kotro.enableCursorBridge` / `kotro.disableCursorBridge` — generate token, start/stop tunnel, dispose on deactivate.
