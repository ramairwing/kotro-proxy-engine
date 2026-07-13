# kotro-core

[![Crates.io](https://img.shields.io/crates/v/kotro-core)](https://crates.io/crates/kotro-core)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![CI](https://github.com/kotro-labs/kotro-proxy-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/kotro-labs/kotro-proxy-engine/actions/workflows/ci.yml)

Embeddable building blocks for LLM proxy pipelines, extracted from [Kotro Proxy Engine](https://github.com/kotro-labs/kotro-proxy-engine).

## What's in here

**`kotro_core::cache`** — deterministic SHA-256 cache key generation and a cosine-similarity `VectorIndex` for fuzzy (semantic) cache lookups. No runtime dependencies beyond `sha2`.

**`kotro_core::guardrail`** — regex-based PII/secret redaction. Strips API keys, DB connection strings, passwords, and email addresses from prompts before they reach an upstream LLM provider, then restores them in responses via a `RedactionMap`. Zero network calls; runs entirely local.

**`kotro_core::compressor`** — session-scoped content deduplication. Tracks SHA-256 fingerprints of content blocks (MCP tool schemas, file snippets) across conversation turns and lets you identify duplicates for stripping before they consume context window tokens.

## Quick start

```toml
[dependencies]
kotro-core = "0.3"

# For on-device MiniLM embedding (fuzzy semantic cache):
# kotro-core = { version = "0.3", features = ["semantic"] }
```

```rust
use kotro_core::guardrail::{Redactor, RedactionMap};

let redactor = Redactor::default();
let mut map = RedactionMap::new();
let safe = redactor.redact("token=sk-proj-abc123 and email=me@corp.com", &mut map);
// "token=<REDACTED_SECRET_0> and email=<REDACTED_SECRET_1>"
let restored = map.restore(&safe);
// "token=sk-proj-abc123 and email=me@corp.com"
```

```rust
use kotro_core::cache::{generate_cache_key, VectorIndex};

// Exact-match cache key (tenant-scoped, SHA-256)
let key = generate_cache_key("tenant-abc", b"system-prompt + latest user turn");

// Semantic lookup
let mut idx = VectorIndex::new();
idx.insert(my_embedding_vec, key.to_string());
if let Some(hit_key) = idx.find(&query_embedding, 0.94) {
    // cache hit — replay stored response
}
```

## Feature flags

| Feature | What it adds |
|---------|-------------|
| *(none)* | Exact-match cache keys + redaction + compressor. No ML deps. |
| `semantic` | `kotro_core::cache::semantic::SemanticEncoder` — on-device MiniLM via `candle`. Adds ~90 MB model download on first use; ~26-28ms per embed call. |

## Relationship to `kotro-proxy`

`kotro-core` is the library half of [Kotro Proxy Engine](https://github.com/kotro-labs/kotro-proxy-engine). The `kotro-proxy` binary is a full streaming proxy (Axum/Tokio, SSE interception, redb persistence) built on top of these building blocks. If you want to embed just the caching or redaction logic into your own Rust service without running a separate process, use `kotro-core` directly.

## License

MIT — see [LICENSE](../../LICENSE).
