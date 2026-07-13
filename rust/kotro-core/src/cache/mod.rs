//! Cache key generation and vector index for LLM prompt caching.
//!
//! Two layers:
//! 1. **Exact-match** — deterministic SHA-256 key from prompt state (always available).
//! 2. **Semantic** — cosine-similarity index over MiniLM embeddings (requires `semantic` feature).

mod key;
mod vector_index;

pub use key::{CacheKey, generate_cache_key, CacheKeyStrategy};
pub use vector_index::VectorIndex;

#[cfg(feature = "semantic")]
pub mod semantic;
