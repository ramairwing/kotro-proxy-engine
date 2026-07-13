//! Deterministic SHA-256 cache key generation.
//!
//! A cache key encodes `(scope, strategy, prompt_material)` into a fixed-size
//! hex digest, making it safe to use as a database key without worrying about
//! prompt length or encoding.

use sha2::{Digest, Sha256};

/// Opaque cache key — a hex-encoded SHA-256 digest.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct CacheKey(String);

impl CacheKey {
    /// The raw hex string (64 chars).
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl std::fmt::Display for CacheKey {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

/// Selects which portion of the conversation history is included in the cache key.
///
/// | Strategy | What is hashed | Recommended for |
/// |----------|---------------|-----------------|
/// | `WindowN` | System prompt + last N turns | **Production agent loops** — balances hit rate and correctness |
/// | `FullDigest` | Entire conversation JSON | Strict multi-tenant or deterministic pipelines |
/// | `LatestOnly` | System + latest user text only | Legacy compatibility — **risky** for multi-turn agents |
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum CacheKeyStrategy {
    #[default]
    WindowN,
    FullDigest,
    LatestOnly,
}

/// Generate a deterministic [`CacheKey`] from a tenant scope string and raw
/// prompt material (already serialised to bytes by the caller according to the
/// chosen strategy).
///
/// # Arguments
/// * `scope` — tenant isolation key (e.g. SHA-256 of credential hash, or a
///   gateway-supplied tenant identifier).
/// * `material` — the bytes that encode the prompt state under the chosen strategy.
///
/// # Example
/// ```rust
/// use kotro_core::cache::{generate_cache_key, CacheKeyStrategy};
///
/// let key = generate_cache_key("tenant-abc", b"system+latest user text");
/// assert_eq!(key.as_str().len(), 64); // hex SHA-256
/// ```
pub fn generate_cache_key(scope: &str, material: &[u8]) -> CacheKey {
    let mut hasher = Sha256::new();
    hasher.update(scope.as_bytes());
    hasher.update(b"\x00"); // separator so "aab" + "c" ≠ "a" + "abc"
    hasher.update(material);
    CacheKey(format!("{:x}", hasher.finalize()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn same_inputs_produce_same_key() {
        let a = generate_cache_key("tenant-1", b"hello");
        let b = generate_cache_key("tenant-1", b"hello");
        assert_eq!(a, b);
    }

    #[test]
    fn different_scopes_produce_different_keys() {
        let a = generate_cache_key("tenant-1", b"hello");
        let b = generate_cache_key("tenant-2", b"hello");
        assert_ne!(a, b);
    }

    #[test]
    fn different_material_produces_different_keys() {
        let a = generate_cache_key("tenant-1", b"hello");
        let b = generate_cache_key("tenant-1", b"world");
        assert_ne!(a, b);
    }

    #[test]
    fn key_is_64_hex_chars() {
        let k = generate_cache_key("scope", b"material");
        assert_eq!(k.as_str().len(), 64);
        assert!(k.as_str().chars().all(|c| c.is_ascii_hexdigit()));
    }

    #[test]
    fn scope_and_material_boundary_not_confused() {
        // "aab" + "c" must not equal "a" + "abc"
        let a = generate_cache_key("aab", b"c");
        let b = generate_cache_key("a", b"abc");
        assert_ne!(a, b);
    }
}
