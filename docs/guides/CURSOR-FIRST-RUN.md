# Cursor first-run guide (extension)

## Read this first

**Kotro’s sidecar is local.** Anything that calls from *your* machine (Verify Cache, Continue, Cline, Claude Code, curl) can use `http://127.0.0.1:8080/v1`.

**Cursor Chat / Agent cannot.** Override OpenAI Base URL is invoked from **Cursor’s cloud**, which **blocks private networks**. Plain `http://localhost:8080/v1` fails with *Access to private networks is forbidden*. That is Cursor’s SSRF policy — not a Kotro bug. Even with a tunnel, prompts already pass through Cursor’s servers before they hit Kotro.

| Goal | Do this |
|------|---------|
| Prove Kotro works | **Kotro: Verify Cache** (no tunnel) |
| Local routing in-editor | **Continue.dev** or **Cline** (Setup Wizard) |
| Claude in terminal | `ANTHROPIC_BASE_URL=http://localhost:8080` |
| Cursor built-in Chat | Tunnel + **bridge auth** (`kotrolabs.bridgeToken` / `upstreamApiKey`) — Bridge command in 0.7 |

Full client matrix: repository [README](../../README.md#which-client-works-how).

---

## 1. Install and prove the sidecar

1. Install **Kotro Proxy Engine** from the Marketplace.
2. Confirm **Proceed** for the SHA-256–verified binary download.
3. Wait until status ≠ `Kotro: offline` (**Kotro: Show Proxy Logs** → `listening`).
4. **Cmd+Shift+P** → **Kotro: Verify Cache** → MISS then HIT.
5. Optional: **Kotro: Open Dashboard** → `http://127.0.0.1:9090/dashboard`.

If you see `Address already in use`, see [§5](#5-port-already-in-use-kotro-offline).

Cline “not a registered configuration” in the wizard log is harmless if Cline isn’t installed.

---

## 2. Cursor Chat via HTTPS tunnel (manual today)

### Security (not small print)

A `*.trycloudflare.com` URL is **public until you stop the tunnel**. Anyone who learns the URL can *reach* your local Kotro — but with **bridge auth** they cannot use it without the token:

1. Set extension settings (or env): `kotrolabs.bridgeToken` + `kotrolabs.upstreamApiKey`
2. Reload so the sidecar picks them up (`KOTRO_BRIDGE_TOKEN` / `KOTRO_UPSTREAM_API_KEY`)
3. Put the **bridge token** in Cursor’s OpenAI API key field (not the provider key)
4. Provider key stays only in `kotrolabs.upstreamApiKey` — Kotro injects it upstream

Without the matching token, LLM routes return **401**. Still `Ctrl+C` the tunnel when idle; don’t paste the URL in public channels.

Planned **Kotro: Enable Cursor Bridge** (0.7) will: check/install `cloudflared` with consent, start/stop the tunnel, generate a bridge token, and auto-kill on window close.  
Auth model details: [CURSOR-BRIDGE.md](./CURSOR-BRIDGE.md).

### Steps (manual)

**0.** Kotro running (`curl http://127.0.0.1:8080/healthz` → ok).

**0b.** Bridge auth (recommended for any tunnel):

Generate a token (do **not** reuse your provider API key):

```bash
uuidgen                 # macOS — e.g. 57324CEC-67A7-4DCE-AEDF-20762224E03A
# or
openssl rand -hex 32    # portable 64-char hex
```

| Setting | Value |
|---------|--------|
| `kotrolabs.bridgeToken` | paste the generated token |
| `kotrolabs.upstreamApiKey` | your DeepSeek / OpenAI / Anthropic key |
| `kotrolabs.upstreamUrl` | e.g. `https://api.deepseek.com` |

Reload the window after saving.

**1.** Install once:

```bash
brew install cloudflare/cloudflare/cloudflared
```

**2.** Leave this Terminal open:

```bash
cloudflared tunnel --url http://127.0.0.1:8080
```

Copy `https://….trycloudflare.com`.

**3.** Cursor Settings → **Models**:

| Setting | Value |
|---------|--------|
| Override OpenAI Base URL | **ON** |
| Base URL | `https://….trycloudflare.com/v1` |
| OpenAI API key | your **bridge token** (same as `kotrolabs.bridgeToken`) |
| Add Model | e.g. `deepseek-v4-flash` |

Never set Base URL to `http://localhost:8080/v1` for Cursor Chat.  
Never put the real provider key in Cursor’s API key field when bridge auth is on.

**4.** Chat with that model (not Auto). Send twice; dashboard should show miss/hit.

**5.** `Ctrl+C` the tunnel; turn Override Base URL **OFF**.

### Upstream URL (extension setting)

`kotrolabs.upstreamUrl` is **not** in Cursor General settings.

`Cmd+Shift+P` → **Preferences: Open User Settings (JSON)**:

```json
"kotrolabs.upstreamUrl": "https://api.deepseek.com"
```

Then **Developer: Reload Window**.

---

## 3. Status bar

| Status | Meaning |
|--------|---------|
| `offline` | Sidecar not running — logs / port / Reload |
| `ready` / `idle` / `running` | Up; little LLM traffic yet — Verify Cache |
| `MISS` / `HIT` | Live cache label |
| `disconnected` | Up but idle ~5m — often Auto or no bridge |

---

## 4. Verify Cache failures

Prefer keyless `kotro-local-verify`. See extension logs if HIT fails.

| Symptom | Fix |
|---------|-----|
| `AddrInUse` / exit code 1 | [§5](#5-port-already-in-use-kotro-offline) |
| Content-Type 415 with a **valid** provider key | Upgrade proxy (upstream forward must send `Content-Type: application/json`) |
| Cursor *private networks forbidden* | Use tunnel/bridge — not Verify Cache’s problem |

---

## 5. Port already in use (`Kotro: offline`)

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
kill <PID>
```

Then **Developer: Reload Window**. Or set `kotrolabs.listenAddr` to another port.

---

## 6. Checklist

- [ ] Extension installed; Checksum OK; not offline
- [ ] **Verify Cache** MISS → HIT
- [ ] (Optional Chat) Tunnel URL + `/v1`; never localhost in Cursor Base URL
- [ ] Named model Add Model’d; not Auto
- [ ] `kotrolabs.upstreamUrl` matches provider
- [ ] Tunnel stopped after testing

---

## 7. Commands

| Command | Use |
|---------|-----|
| **Kotro: Verify Cache** | Prove sidecar (localhost) |
| **Kotro: Show Proxy Logs** | Startup / errors |
| **Kotro: Open Dashboard** | Traffic UI |
| **Kotro: Setup Wizard** | Continue / Cline |
| **Kotro: Connect Cursor** | Reminder + guide link |
| **Developer: Reload Window** | After port / upstream changes |
