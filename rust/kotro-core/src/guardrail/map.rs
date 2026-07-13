//! Bidirectional map from placeholder → original secret value.
//!
//! Used to restore redacted secrets in proxy responses after the upstream
//! provider returns them (e.g. an LLM echoing back part of a prompt).

use std::collections::HashMap;

/// Tracks `placeholder ↔ original` pairs for a single request/response cycle.
///
/// One [`RedactionMap`] should be created per request, used during redaction,
/// and then passed to [`Self::restore`] when processing the upstream response.
#[derive(Default)]
pub struct RedactionMap {
    // placeholder → original
    inner: HashMap<String, String>,
    counter: usize,
}

impl RedactionMap {
    /// Create an empty map.
    pub fn new() -> Self {
        Self::default()
    }

    /// Record a discovered secret and return its placeholder string.
    ///
    /// If the same secret has already been seen this cycle, returns the existing
    /// placeholder (deduplication).
    pub fn record(&mut self, secret: String) -> String {
        // Dedup: if we already have a placeholder for this exact secret, reuse it.
        for (placeholder, original) in &self.inner {
            if original == &secret {
                return placeholder.clone();
            }
        }
        let placeholder = format!("<REDACTED_SECRET_{}>", self.counter);
        self.counter += 1;
        self.inner.insert(placeholder.clone(), secret);
        placeholder
    }

    /// Replace all placeholders in `text` with their original values.
    pub fn restore(&self, text: &str) -> String {
        let mut result = text.to_string();
        for (placeholder, original) in &self.inner {
            result = result.replace(placeholder.as_str(), original.as_str());
        }
        result
    }

    /// Returns `true` if no secrets have been recorded.
    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }

    /// Number of unique secrets recorded.
    pub fn len(&self) -> usize {
        self.inner.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn record_returns_placeholder() {
        let mut m = RedactionMap::new();
        let p = m.record("mysecret".into());
        assert_eq!(p, "<REDACTED_SECRET_0>");
    }

    #[test]
    fn same_secret_returns_same_placeholder() {
        let mut m = RedactionMap::new();
        let p1 = m.record("mysecret".into());
        let p2 = m.record("mysecret".into());
        assert_eq!(p1, p2);
        assert_eq!(m.len(), 1);
    }

    #[test]
    fn different_secrets_get_different_placeholders() {
        let mut m = RedactionMap::new();
        let p1 = m.record("secret-a".into());
        let p2 = m.record("secret-b".into());
        assert_ne!(p1, p2);
        assert_eq!(m.len(), 2);
    }

    #[test]
    fn restore_round_trips() {
        let mut m = RedactionMap::new();
        let p = m.record("AKIA1234567890ABCDEF".into());
        let redacted = format!("key is {p}");
        assert_eq!(m.restore(&redacted), "key is AKIA1234567890ABCDEF");
    }
}
