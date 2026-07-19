# Cursor Bridge (0.7) — design notes

Status: **planned**. Manual Cloudflare tunnel is documented in `CURSOR-FIRST-RUN.md` today.

## Why Bridge exists

Cursor Chat/Agent Override Base URL is executed from **Cursor’s cloud**, which SSRF-blocks `localhost`. Kotro cannot fix that. Bridge = managed temporary public HTTPS → local sidecar.

## Security model (preferred)

Cursor BYOK UI typically exposes **Base URL + API key**, not arbitrary custom headers. Prefer:

1. Generate `KOTRO_BRIDGE_TOKEN` (UUID) when Bridge starts.
2. Restart/configure sidecar with `KOTRO_BRIDGE_TOKEN` set → reject requests that lack `Authorization: Bearer <token>` (or equal bridge header) when token is set.
3. Put the **bridge token** in Cursor’s OpenAI API key field.
4. Keep the **real provider key** in extension settings / env (`KOTRO_UPSTREAM_API_KEY` or existing upstream config) so Kotro injects it on forward to DeepSeek/OpenAI.

Do **not** ship assuming Cursor supports custom request headers.

Tunnel URL remains public; the token stops anonymous callers who find the URL. Still warn loudly; auto-stop `cloudflared` on `deactivate()`.

## cloudflared dependency

- Do not silently download/bundle without consent.
- If missing: prompt to install (`brew install cloudflare/cloudflare/cloudflared`) via an explicit user action.

## Command sketch

`kotro.enableCursorBridge` / `kotro.disableCursorBridge` — see product notes in chat 2026-07-19. Child process must be disposed on deactivate.
