//! # kotro-core
//!
//! Embeddable building blocks for LLM proxy pipelines:
//!
//! - **[`cache`]** — deterministic SHA-256 prompt-state cache key generation and
//!   cosine-similarity vector index for fuzzy (semantic) cache lookups.
//! - **[`guardrail`]** — regex-based PII/secret redaction that strips API keys,
//!   database connection strings, passwords, and email addresses from prompts
//!   before they reach an upstream provider, then restores placeholders in the
//!   response stream.
//! - **[`compressor`]** — AST-aware context deduplication that strips unchanged
//!   MCP tool schemas and repeated file blocks across conversation turns,
//!   reducing context-window consumption without altering semantics.
//!
//! ## Minimum example
//!
//! ```rust
//! use kotro_core::guardrail::{Redactor, RedactionMap};
//!
//! let redactor = Redactor::default();
//! let mut map = RedactionMap::new();
//! let safe_prompt = redactor.redact("my key is sk-ant-abc123", &mut map);
//! // safe_prompt: "my key is <REDACTED_API_KEY_0>"
//! // map contains the original value for restoration
//! ```
//!
//! ## Feature flags
//!
//! | Feature | What it adds | Default |
//! |---------|-------------|---------|
//! | `semantic` | MiniLM on-device embedding for fuzzy cache lookups | off |
//!
//! The `semantic` feature pulls in the full `candle` + `tokenizers` stack (~90 MB
//! model on first run). Without it, only exact-match SHA-256 cache keys are
//! available.

pub mod cache;
pub mod compressor;
pub mod guardrail;

/// Kotro-core library version. Matches the workspace package version.
pub const VERSION: &str = env!("CARGO_PKG_VERSION");
