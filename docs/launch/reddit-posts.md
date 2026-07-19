# Reddit Launch Posts — Kotro v0.3.0

## r/cursor

**Title:** Local Kotro proxy for Cursor: Verify Cache works in-IDE; Cursor Chat needs an HTTPS bridge (Cursor blocks localhost)

**Body:**
I run a local Rust sidecar (Kotro) for injection scan + cache between coding agents and the LLM provider. Marketplace extension installs a SHA-256–verified binary and starts it on localhost.

**What works with zero tunnel:**
- Command Palette → **Kotro: Verify Cache** (MISS then HIT) — extension talks to localhost
- **Continue.dev / Cline** (opt-in Setup Wizard) — they call from the IDE process
- Dashboard at http://127.0.0.1:9090/dashboard

**Cursor Chat / Agent cannot use `http://localhost:8080`.** Override Base URL is called from Cursor’s cloud, which blocks private IPs (SSRF). You need a temporary HTTPS tunnel (Cloudflare quick tunnel today; one-command Bridge planned). Set `kotrolabs.bridgeToken` + `kotrolabs.upstreamApiKey` so a public tunnel URL alone cannot use your proxy — put the bridge token in Cursor’s API key field. Even then, prompts still pass through Cursor’s servers first — Kotro still helps on the provider hop (cache / scan / redact).

Install: search **Kotro Proxy Engine** in the marketplace, or `brew install kotro-labs/tap/kotro-proxy`.

Setup guide: https://github.com/kotro-labs/kotro-proxy-engine/blob/main/docs/guides/CURSOR-FIRST-RUN.md  
Client matrix: https://github.com/kotro-labs/kotro-proxy-engine (README)

Happy to answer questions about the HTTP-path injection scanner vs MCP-stdio firewalls.

---

## r/LocalLLaMA

**Title:** Open-source Rust proxy for AI coding agents: 68% token reduction + MCP prompt injection blocking. VS Code extension now auto-configures everything.

**Body:**
Built a local proxy that sits between your AI coding IDE and the LLM provider. It's a single Rust binary (~15MB idle RAM), no external dependencies.

**What it does:**

- SHA-256 exact-match cache (redb) — repeated prompts return in ~0.3ms, never touch the API
- On-device semantic cache via MiniLM — catches rephrased variants of cached prompts without an external embedding call
- MCP tool response cache — file listings, status checks, search results cached with per-category TTLs (30s / 5m / 1h)
- Agent loop circuit breaker — detects 3+ identical payloads in a window, aborts the death loop
- Reasoning budget controller — caps `thinking.budget_tokens` / `max_completion_tokens` per request
- MCP prompt injection scanner — 14 regex patterns across every tool response before it hits the model
- Secret/PII redaction — strips API keys, DB URLs, SSH keys outbound; restores them on the response stream

**Numbers from a real session:**
- ~68% of API calls were duplicates that now replay from cache
- 2 secrets intercepted before reaching OpenAI
- 2 prompt injection payloads caught in MCP tool responses

The VS Code extension (v0.3.0) now auto-downloads the binary and auto-configures Cline + Continue.dev on first install. No manual setup.

GitHub (MIT): https://github.com/kotro-labs/kotro-proxy-engine

Works with OpenAI, Anthropic, Ollama, and any OpenAI-compatible endpoint.

---

## r/rust

**Title:** Show r/rust: single-binary Rust proxy for LLM API traffic — redb cache, MiniLM embeddings via candle, MCP injection scanner

**Body:**
Built a local reverse proxy for AI coding agents in Rust. Thought the technical stack might be interesting here.

**Stack:**

- **Axum** for the HTTP proxy with SSE streaming passthrough
- **redb** as the embedded KV store for the SHA-256 exact-match cache (no SQLite, no Postgres)
- **candle** + HuggingFace `all-MiniLM-L6-v2` for on-device semantic embeddings (~3ms per request on CPU)
- **Tokio** throughout; `block_in_place` for the redb blocking reads inside async handlers
- Single static binary via `cargo build --release` — ~15MB idle RAM

**Features built in Rust:**

- SHA-256 exact-match cache with LRU eviction
- Cosine similarity semantic cache (384-dim MiniLM embeddings, threshold 0.94)
- MCP tool response cache with per-category TTLs
- Circuit breaker for agent death loops (3+ identical payloads)
- Regex-based MCP prompt injection scanner (14 patterns)
- Secret/PII redaction with in-place stream restoration
- OpenTelemetry OTLP tracing via `SdkTracerProvider`
- WASM plugin engine via Extism

157 tests. Go reference implementation frozen at v0.1.0-go.

GitHub (MIT): https://github.com/kotro-labs/kotro-proxy-engine

One thing I'd genuinely like feedback on: is bundling MiniLM via candle the right call long-term, or is there a lighter embedding approach worth exploring for a proxy that needs to stay sub-millisecond on cache misses?
