//! SHA-256 fingerprinting for conversation content blocks.

use sha2::{Digest, Sha256};

/// A SHA-256 fingerprint of a content block (MCP schema, file snippet, etc.).
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct ContentFingerprint([u8; 32]);

impl ContentFingerprint {
    /// Compute a fingerprint from arbitrary bytes.
    pub fn of(bytes: &[u8]) -> Self {
        let mut h = Sha256::new();
        h.update(bytes);
        Self(h.finalize().into())
    }

    /// Compute a fingerprint from a UTF-8 string.
    pub fn of_str(s: &str) -> Self {
        Self::of(s.as_bytes())
    }
}
