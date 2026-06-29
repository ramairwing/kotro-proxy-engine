// Package compressor implements local context & MCP deduplication (Feature C).
package compressor

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// StateTracker remembers prior context block hashes per session to strip
// redundant unchanged blocks from subsequent agent payloads.
type StateTracker struct {
	mu     sync.RWMutex
	blocks map[string]string // hash -> content fingerprint
	last   []string          // ordered hashes from previous turn
}

// NewStateTracker creates an empty per-process context diff tracker.
func NewStateTracker() *StateTracker {
	return &StateTracker{
		blocks: make(map[string]string),
	}
}

// blockHash fingerprints a single context block for deduplication.
func blockHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8]) // 16 hex chars — sufficient for local dedup
}

// SplitBlocks divides message content into logical blocks separated by blank
// lines — typical for MCP schemas and directory tree dumps.
func SplitBlocks(content string) []string {
	parts := strings.Split(content, "\n\n")
	var blocks []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			blocks = append(blocks, trimmed)
		}
	}
	if len(blocks) == 0 && content != "" {
		return []string{content}
	}
	return blocks
}

// CompressMessage removes blocks identical to the previous turn's payload.
// Returns the pruned content and whether any blocks were stripped.
func (st *StateTracker) CompressMessage(content string) (string, bool) {
	blocks := SplitBlocks(content)
	if len(blocks) == 0 {
		return content, false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	var kept []string
	var newHashes []string
	changed := false

	for _, block := range blocks {
		hash := blockHash(block)
		newHashes = append(newHashes, hash)

		if prev, ok := st.blocks[hash]; ok && prev == block {
			// Unchanged since last turn — skip redundant block
			changed = true
			continue
		}
		kept = append(kept, block)
		st.blocks[hash] = block
	}

	st.last = newHashes
	if !changed {
		return content, false
	}
	if len(kept) == 0 {
		return "", true
	}
	return strings.Join(kept, "\n\n"), true
}

// CompressMessages applies per-message compression, primarily targeting
// system and user roles where MCP/context bloat accumulates.
func (st *StateTracker) CompressMessages(messages []struct {
	Role    string
	Content string
}) []struct {
	Role    string
	Content string
} {
	out := make([]struct {
		Role    string
		Content string
	}, len(messages))

	for i, msg := range messages {
		content := msg.Content
		if msg.Role == "system" || msg.Role == "user" {
			if pruned, ok := st.CompressMessage(content); ok {
				content = pruned
			}
		}
		out[i] = struct {
			Role    string
			Content string
		}{Role: msg.Role, Content: content}
	}
	return out
}
