//! Per-session token budget enforcement.
//!
//! Tracks approximate token consumption per scope and enforces configurable
//! limits. Tokens are estimated using the standard 4-chars-per-token
//! heuristic — accurate enough for cost-awareness and budget enforcement,
//! not suitable for billing.
//!
//! ## Configuration
//!
//! | Env var | Default | Description |
//! |---------|---------|-------------|
//! | `KOTRO_SESSION_TOKEN_BUDGET` | `0` | Token limit per scope per session (0 = unlimited) |
//! | `KOTRO_BUDGET_BLOCK` | `false` | If `true`, requests that exceed the budget return HTTP 429 |
//!
//! ## Response headers
//!
//! On every cache-miss request (i.e., a request that reaches the upstream):
//!
//! | Header | Value |
//! |--------|-------|
//! | `X-Kotro-Tokens-Used` | Cumulative estimated tokens used in this session |
//! | `X-Kotro-Budget-Remaining` | Tokens remaining (omitted when unlimited) |
//!
//! ## Session scope
//!
//! A "session" is scoped to [`crate::compressor::Scope::key()`] — the same
//! key that drives cache and compressor isolation. It resets automatically
//! after the session idles for 24 hours (via the moka TTL).

use std::sync::Arc;
use std::time::Duration;

use moka::sync::Cache;

/// Tracks cumulative estimated token usage per scope.
///
/// Cheap to clone — the internal cache is `Arc`-wrapped.
#[derive(Clone)]
pub struct BudgetTracker {
    state: Arc<Cache<String, u64>>,
    /// Hard limit in tokens (0 = unlimited).
    pub limit_tokens: u64,
    /// If `true`, requests that would exceed the limit are blocked (HTTP 429).
    /// If `false`, a warning header is set but the request proceeds.
    pub block_on_exceeded: bool,
}

impl BudgetTracker {
    /// Create a tracker with the given limit and idle-reset TTL.
    ///
    /// Pass `limit_tokens = 0` for unlimited operation.
    pub fn new(limit_tokens: u64, block_on_exceeded: bool, idle_ttl: Duration) -> Self {
        Self {
            state: Arc::new(
                Cache::builder()
                    .time_to_idle(idle_ttl)
                    .build(),
            ),
            limit_tokens,
            block_on_exceeded,
        }
    }

    /// Create a no-op tracker (unlimited, never blocks).
    pub fn unlimited() -> Self {
        Self::new(0, false, Duration::from_secs(86_400))
    }

    /// Estimate token count using the 4-chars-per-token heuristic.
    ///
    /// Returns 0 for empty strings; at least 1 for any non-empty input.
    pub fn estimate_tokens(text: &str) -> u64 {
        let chars = text.chars().count() as u64;
        if chars == 0 {
            return 0;
        }
        (chars / 4).max(1)
    }

    /// Add `tokens` to the running total for `scope_key` and return the new total.
    pub fn record(&self, scope_key: &str, tokens: u64) -> u64 {
        let prev = self.state.get(scope_key).unwrap_or(0);
        let next = prev.saturating_add(tokens);
        self.state.insert(scope_key.to_string(), next);
        next
    }

    /// Return the current cumulative usage without modifying it.
    pub fn current(&self, scope_key: &str) -> u64 {
        self.state.get(scope_key).unwrap_or(0)
    }

    /// `true` when the scope has reached or exceeded `limit_tokens`.
    ///
    /// Always `false` when `limit_tokens == 0` (unlimited).
    pub fn is_exceeded(&self, scope_key: &str) -> bool {
        self.limit_tokens > 0 && self.current(scope_key) >= self.limit_tokens
    }

    /// Tokens remaining for this scope.
    ///
    /// Returns `u64::MAX` when `limit_tokens == 0` (unlimited).
    /// Saturates at 0 rather than underflowing.
    pub fn remaining(&self, scope_key: &str) -> u64 {
        if self.limit_tokens == 0 {
            return u64::MAX;
        }
        self.limit_tokens.saturating_sub(self.current(scope_key))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tracker(limit: u64) -> BudgetTracker {
        BudgetTracker::new(limit, false, Duration::from_secs(3600))
    }

    #[test]
    fn estimate_empty_returns_zero() {
        assert_eq!(BudgetTracker::estimate_tokens(""), 0);
    }

    #[test]
    fn estimate_two_chars_returns_one() {
        assert_eq!(BudgetTracker::estimate_tokens("hi"), 1);
    }

    #[test]
    fn estimate_400_chars_is_100_tokens() {
        assert_eq!(BudgetTracker::estimate_tokens(&"a".repeat(400)), 100);
    }

    #[test]
    fn tracks_cumulative_usage() {
        let t = tracker(1000);
        assert_eq!(t.record("s1", 100), 100);
        assert_eq!(t.record("s1", 50), 150);
        assert_eq!(t.current("s1"), 150);
    }

    #[test]
    fn scopes_are_independent() {
        let t = tracker(1000);
        t.record("a", 500);
        t.record("b", 200);
        assert_eq!(t.current("a"), 500);
        assert_eq!(t.current("b"), 200);
    }

    #[test]
    fn not_exceeded_below_limit() {
        let t = tracker(100);
        t.record("s", 99);
        assert!(!t.is_exceeded("s"));
    }

    #[test]
    fn exceeded_at_limit() {
        let t = tracker(100);
        t.record("s", 100);
        assert!(t.is_exceeded("s"));
    }

    #[test]
    fn unlimited_never_exceeded() {
        let t = tracker(0);
        t.record("s", u64::MAX / 2);
        assert!(!t.is_exceeded("s"));
        assert_eq!(t.remaining("s"), u64::MAX);
    }

    #[test]
    fn remaining_decreases_correctly() {
        let t = tracker(500);
        t.record("s", 200);
        assert_eq!(t.remaining("s"), 300);
    }

    #[test]
    fn remaining_saturates_at_zero() {
        let t = tracker(100);
        t.record("s", 200); // over limit
        assert_eq!(t.remaining("s"), 0);
    }

    #[test]
    fn record_does_not_overflow() {
        let t = tracker(0);
        t.record("s", u64::MAX);
        // saturating_add with u64::MAX should not panic
        t.record("s", 1);
    }
}
