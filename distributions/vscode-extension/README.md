# Kotro Proxy Engine

<!-- Marketplace README images must use absolute GitHub raw URLs: vsce rewrites
     relative paths to repo-root /media/* which does not exist. -->

<p align="center">
  <img src="https://raw.githubusercontent.com/kotro-labs/kotro-proxy-engine/main/distributions/vscode-extension/media/icon.png" alt="Kotro" width="96" height="96" />
</p>

Transparent **IDE sidecar** for the [Kotro Proxy Engine](https://github.com/kotro-labs/kotro-proxy-engine) — a local LLM gateway with injection scanning, streaming prompt-state cache, PII redaction, and context compression for **OpenAI** and **Anthropic** APIs.

Works in **VS Code**, **Cursor**, and other VS Code–compatible editors.

## What this extension does on disk (transparency)

Security products should be boringly explicit. On first run, with your **Proceed** confirmation, the extension:

1. Downloads the platform archive from [GitHub Releases](https://github.com/kotro-labs/kotro-proxy-engine/releases/latest)
2. Verifies **SHA-256** against that release’s `checksums.txt` (refuses to install on mismatch or if checksums are missing)
3. Extracts / installs the binary under the extension **global storage** path (VS Code / Cursor managed; not your project tree)
4. Starts the sidecar on `localhost:8080` (LLM) and `127.0.0.1:9090` (dashboard)

**Agent routing is not automatic.** After the proxy is up you may choose **Run Wizard** (or Command Palette → **Kotro: Setup Wizard**). Only then, and only after a second **Confirm**, may it:

- Set `cline.openaiBaseUrl` in VS Code **user** settings
- Add a Kotro model entry to `~/.continue/config.json` if that file already exists
- Show Cursor BYOK Base URL instructions (no silent Cursor DB edits)

You can decline both prompts and configure agents yourself.

## Features

- **Verified binary install** — SHA-256 checked against release `checksums.txt` before `chmod +x`
- **Opt-in Setup Wizard** — Cline / Continue / Cursor guides; never silent global reroutes on activate
- **Prompt-state SSE cache** — exact-match replay on repeat prompts; `X-Kotro-Cache: HIT` on cache hits
- **Status bar** — live cache result and dollars saved (polls every 5s)
- **Operator dashboard** — open from the status bar (`http://127.0.0.1:9090/dashboard`)
- **Isolated telemetry** — `/metrics` and `/dashboard` bind to loopback by default, separate from LLM traffic
- **Context-aware cache keys** — `window_n` strategy prevents false cache hits in multi-turn agent loops

## Screenshots

### Status bar

![Kotro status bar showing cache hit and bytes saved](https://raw.githubusercontent.com/kotro-labs/kotro-proxy-engine/main/distributions/vscode-extension/media/status-bar.png)

### Dashboard

![Kotro proxy operator dashboard](https://raw.githubusercontent.com/kotro-labs/kotro-proxy-engine/main/distributions/vscode-extension/media/dashboard.png)

## Install

1. Install from the Marketplace (**Install** above), or:
   ```bash
   code --install-extension kotrolabs.kotro-proxy-engine
   ```
2. On first activate, confirm the **SHA-256–verified** binary download (~15MB).
3. Optionally click **Run Wizard** to configure Cline / Continue, or point your AI client at `http://localhost:8080/v1` yourself.

## Quick start (Cursor / VS Code)

1. Set your provider API key in the environment or your agent config.
2. Configure the extension (optional):

   | Setting | Default | Maps to |
   |---------|---------|---------|
   | `kotrolabs.listenAddr` | `:8080` | `KOTRO_LISTEN_ADDR` |
   | `kotrolabs.metricsAddr` | `127.0.0.1:9090` | `KOTRO_METRICS_ADDR` |
   | `kotrolabs.upstreamUrl` | `https://api.openai.com` | `KOTRO_UPSTREAM_URL` |
   | `kotrolabs.enableCache` | `true` | `KOTRO_ENABLE_CACHE` |
   | `kotrolabs.enableRedaction` | `true` | `KOTRO_ENABLE_REDACTION` |
   | `kotrolabs.enableCompression` | `true` | `KOTRO_ENABLE_COMPRESSION` |
   | `kotrolabs.enableMetrics` | `true` | `KOTRO_ENABLE_METRICS` |

3. Click the **Kotro** item in the status bar to open the dashboard.

## Verify it works (2 minutes)

The extension **starts** the proxy. Your IDE must **send API traffic** to it.

| Step | Action | Success signal |
|------|--------|----------------|
| 1 | **Cmd+Shift+P** → **Kotro: Verify Cache** | Notification: `MISS` then `HIT` |
| 2 | Open dashboard (`http://127.0.0.1:9090/dashboard`) | Recent Traffic shows `miss` then `hit` on `/v1/chat/completions` |
| 3 | (Optional) Cursor **Settings → Models** → OpenAI Base URL = `http://localhost:8080/v1` | Chat traffic appears in dashboard |

**Common mistakes**

- Reading the **chat reply** — that is the model answer, not proxy logs.
- Opening `http://localhost:8080/v1/` in a browser — API only; shows `BYPASS`, not cache.
- Using **Kotro: Show Proxy Logs** for HIT/MISS — that channel shows startup lines only; use Verify Cache or the dashboard.

## Commands

| Command | Description |
|---------|-------------|
| **Kotro: Setup Wizard** | Consentful Cline / Continue / Cursor routing setup |
| **Kotro: Verify Cache** | Sends two identical test requests; confirms cache HIT |
| **Kotro: Connect Cursor** | Wizard for routing Cursor BYOK chat through the proxy |
| **Kotro: Setup Continue.dev Config** | Alias for Setup Wizard |
| **Kotro: Open Dashboard** | Opens the local operator UI |
| **Kotro: Show Proxy Logs** | Opens the extension output channel (startup / errors) |

## Architecture

```
IDE agent  →  localhost:8080/v1/*     (LLM proxy — may bind 0.0.0.0 in cluster mode)
Operator   →  127.0.0.1:9090/dashboard  (telemetry — loopback only by default)
```

## Other install channels

- **npm:** `npm install -g @kotro-labs/proxy-engine`
- **Homebrew:** `brew tap kotro-labs/tap && brew install kotro-proxy`
- **GitHub Releases:** [kotro-labs/kotro-proxy-engine](https://github.com/kotro-labs/kotro-proxy-engine/releases)

## Documentation

Full engine docs, threat model, and observability spec: [github.com/kotro-labs/kotro-proxy-engine](https://github.com/kotro-labs/kotro-proxy-engine)

## License

MIT — [Kotrosystems](https://marketplace.visualstudio.com/publishers/kotrolabs)
