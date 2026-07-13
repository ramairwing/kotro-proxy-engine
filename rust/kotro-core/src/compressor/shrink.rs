//! Session-scoped context compressor.
//!
//! Maintains a set of seen content fingerprints per conversation session and
//! exposes a simple API to test whether a block is new or a duplicate.

use std::collections::HashSet;
use super::fingerprint::ContentFingerprint;

/// A stateless factory for [`CompressorSession`]s.
///
/// Construct once; call [`Compressor::session`] at the start of each
/// conversation to get a fresh per-session state.
#[derive(Debug, Default, Clone)]
pub struct Compressor {
    /// Minimum byte length below which blocks are never compressed (too short
    /// to be worth the complexity).
    pub min_block_bytes: usize,
}

impl Compressor {
    /// Create a [`Compressor`] with the default minimum block size (64 bytes).
    pub fn new() -> Self {
        Self { min_block_bytes: 64 }
    }

    /// Start a new per-conversation [`CompressorSession`].
    pub fn session(&self) -> CompressorSession {
        CompressorSession {
            seen: HashSet::new(),
            min_block_bytes: self.min_block_bytes,
            blocks_saved: 0,
        }
    }
}

/// Per-conversation state: tracks which content blocks have already been sent.
pub struct CompressorSession {
    seen: HashSet<ContentFingerprint>,
    min_block_bytes: usize,
    blocks_saved: usize,
}

impl CompressorSession {
    /// Returns `true` if `block` is a duplicate of something already seen this
    /// session *and* the caller should strip it from the outgoing request.
    ///
    /// Always returns `false` for blocks shorter than `min_block_bytes` —
    /// small blocks aren't worth the deduplication complexity.
    ///
    /// Registers `block` as seen on the first call, so subsequent identical
    /// calls return `true`.
    pub fn is_duplicate(&mut self, block: &str) -> bool {
        if block.len() < self.min_block_bytes {
            return false;
        }
        let fp = ContentFingerprint::of_str(block);
        if !self.seen.insert(fp) {
            self.blocks_saved += 1;
            return true;
        }
        false
    }

    /// Number of blocks that have been identified as duplicates this session.
    pub fn blocks_saved(&self) -> usize {
        self.blocks_saved
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn long_enough(s: &str) -> String {
        // Pad to ensure it exceeds the 64-byte minimum.
        format!("{:0>80}", s)
    }

    #[test]
    fn first_occurrence_is_not_duplicate() {
        let c = Compressor::new();
        let mut s = c.session();
        assert!(!s.is_duplicate(&long_enough("schema-a")));
    }

    #[test]
    fn second_occurrence_is_duplicate() {
        let c = Compressor::new();
        let mut s = c.session();
        let block = long_enough("schema-a");
        s.is_duplicate(&block);
        assert!(s.is_duplicate(&block));
        assert_eq!(s.blocks_saved(), 1);
    }

    #[test]
    fn different_blocks_are_not_duplicates_of_each_other() {
        let c = Compressor::new();
        let mut s = c.session();
        assert!(!s.is_duplicate(&long_enough("schema-a")));
        assert!(!s.is_duplicate(&long_enough("schema-b")));
    }

    #[test]
    fn short_blocks_never_marked_duplicate() {
        let c = Compressor::new();
        let mut s = c.session();
        let short = "tiny";
        s.is_duplicate(short);
        // Same short block seen again — still not marked duplicate.
        assert!(!s.is_duplicate(short));
    }
}
