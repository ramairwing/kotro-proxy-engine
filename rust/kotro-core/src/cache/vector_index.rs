//! In-memory cosine-similarity index over embedding vectors.
//!
//! Used by the semantic cache layer to find the nearest cached prompt when an
//! exact SHA-256 match doesn't exist.

/// An in-memory store of `(embedding_vector, opaque_key)` pairs.
///
/// Lookup returns the stored key whose embedding has the highest cosine
/// similarity to the query vector, provided it clears the configured threshold.
///
/// This is a linear scan — suitable for thousands of cached entries on a single
/// developer machine. For multi-tenant or shared deployments with millions of
/// entries, swap this out for an ANN index (e.g. `usearch`, `hnswlib` via FFI).
#[derive(Default)]
pub struct VectorIndex {
    entries: Vec<(Vec<f32>, String)>,
}

impl VectorIndex {
    /// Create an empty index.
    pub fn new() -> Self {
        Self::default()
    }

    /// Insert an `(embedding, cache_key)` pair.
    pub fn insert(&mut self, embedding: Vec<f32>, key: String) {
        self.entries.push((embedding, key));
    }

    /// Find the stored key whose embedding is closest to `query`, returning it
    /// only if its cosine similarity exceeds `threshold`.
    ///
    /// Returns `None` if the index is empty or no entry clears the threshold.
    pub fn find(&self, query: &[f32], threshold: f32) -> Option<&str> {
        self.entries
            .iter()
            .map(|(emb, key)| (cosine_similarity(query, emb), key.as_str()))
            .filter(|(score, _)| *score >= threshold)
            .max_by(|(a, _), (b, _)| a.partial_cmp(b).unwrap_or(std::cmp::Ordering::Equal))
            .map(|(_, key)| key)
    }

    /// Number of entries currently stored.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Returns `true` if the index is empty.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

/// Cosine similarity in [−1, 1]. Returns 0.0 if either vector has zero norm.
pub fn cosine_similarity(a: &[f32], b: &[f32]) -> f32 {
    debug_assert_eq!(a.len(), b.len(), "embedding dimension mismatch");
    let dot: f32 = a.iter().zip(b).map(|(x, y)| x * y).sum();
    let norm_a: f32 = a.iter().map(|x| x * x).sum::<f32>().sqrt();
    let norm_b: f32 = b.iter().map(|x| x * x).sum::<f32>().sqrt();
    if norm_a == 0.0 || norm_b == 0.0 {
        return 0.0;
    }
    dot / (norm_a * norm_b)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn unit_vec(dim: usize, hot: usize) -> Vec<f32> {
        let mut v = vec![0.0f32; dim];
        v[hot] = 1.0;
        v
    }

    #[test]
    fn identical_vectors_score_1() {
        let v = vec![0.6, 0.8];
        assert!((cosine_similarity(&v, &v) - 1.0).abs() < 1e-6);
    }

    #[test]
    fn orthogonal_vectors_score_0() {
        let a = unit_vec(4, 0);
        let b = unit_vec(4, 1);
        assert!((cosine_similarity(&a, &b)).abs() < 1e-6);
    }

    #[test]
    fn find_returns_none_on_empty_index() {
        let idx = VectorIndex::new();
        assert!(idx.find(&[1.0, 0.0], 0.9).is_none());
    }

    #[test]
    fn find_returns_best_match_above_threshold() {
        let mut idx = VectorIndex::new();
        idx.insert(unit_vec(3, 0), "key-0".into());
        idx.insert(unit_vec(3, 1), "key-1".into());

        // Query very close to key-0
        let query = vec![0.999, 0.045, 0.0];
        let hit = idx.find(&query, 0.9);
        assert_eq!(hit, Some("key-0"));
    }

    #[test]
    fn find_returns_none_when_nothing_clears_threshold() {
        let mut idx = VectorIndex::new();
        idx.insert(unit_vec(3, 0), "key-0".into());
        // Query is perpendicular — similarity = 0
        let query = unit_vec(3, 1);
        assert!(idx.find(&query, 0.9).is_none());
    }
}
