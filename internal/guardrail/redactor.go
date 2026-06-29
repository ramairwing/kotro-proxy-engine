// Package guardrail implements the local privacy guardrail (Feature B).
// Outbound payloads are scanned for sensitive entities and masked with stable
// placeholders; inbound SSE chunks are restored before reaching the IDE.
package guardrail

import (
	"regexp"
	"strings"
	"sync"
)

// RedactionMap holds placeholder -> original value mappings for a single request.
type RedactionMap struct {
	mu    sync.RWMutex
	forward map[string]string // placeholder -> secret
	reverse map[string]string // secret -> placeholder (unused, kept for clarity)
	seq   int
}

// NewRedactionMap creates an empty per-request redaction registry.
func NewRedactionMap() *RedactionMap {
	return &RedactionMap{
		forward: make(map[string]string),
		reverse: make(map[string]string),
	}
}

// sensitivePatterns are ordered from most specific to broadest to minimize
// false positives while catching common secret formats.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                              // AWS access key
	regexp.MustCompile(`(?i)(?:password|passwd|pwd)\s*[:=]\s*['"]?[^\s'"]{4,}['"]?`),   // password assignments
	regexp.MustCompile(`(?i)(?:api[_-]?key|secret[_-]?key|token)\s*[:=]\s*['"]?[^\s'"]{8,}['"]?`),
	regexp.MustCompile(`postgres(?:ql)?://[^\s]+`),                                       // DB connection strings
	regexp.MustCompile(`mysql://[^\s]+`),
	regexp.MustCompile(`mongodb(?:\+srv)?://[^\s]+`),
	regexp.MustCompile(`redis://[^\s]+`),
	regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),            // emails
	regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),                                            // OpenAI-style keys
	regexp.MustCompile(`sk-ant-[a-zA-Z0-9\-]{20,}`),                                      // Anthropic-style keys
}

// Redact scans text and replaces discovered secrets with stable placeholders.
// Returns the redacted text and the map used for inbound restoration.
func Redact(text string) (string, *RedactionMap) {
	rm := NewRedactionMap()
	result := text

	for _, pat := range sensitivePatterns {
		result = pat.ReplaceAllStringFunc(result, func(match string) string {
			rm.mu.Lock()
			defer rm.mu.Unlock()

			for placeholder, original := range rm.forward {
				if original == match {
					return placeholder
				}
			}

			rm.seq++
			placeholder := "[REDACTED_SECRET_" + itoa(rm.seq) + "]"
			rm.forward[placeholder] = match
			rm.reverse[match] = placeholder
			return placeholder
		})
	}

	return result, rm
}

// Merge copies all entries from other into rm, re-numbering placeholders on collision.
func (rm *RedactionMap) Merge(other *RedactionMap) {
	if other == nil {
		return
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, orig := range other.forward {
		// Skip if this secret is already mapped in rm.
		for _, existing := range rm.forward {
			if existing == orig {
				goto next
			}
		}
		rm.seq++
		placeholder := "[REDACTED_SECRET_" + itoa(rm.seq) + "]"
		rm.forward[placeholder] = orig
		rm.reverse[orig] = placeholder
	next:
	}
}

// Restore reverses placeholder masking on inbound streaming text.
func (rm *RedactionMap) Restore(text string) string {
	if rm == nil || len(rm.forward) == 0 {
		return text
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := text
	for placeholder, original := range rm.forward {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
