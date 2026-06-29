package guardrail_test

import (
	"strings"
	"testing"

	"github.com/kortolabs/proxy-engine/internal/guardrail"
)

func TestRedactAWSKey(t *testing.T) {
	input := "Use key AKIAIOSFODNN7EXAMPLE for auth"
	out, rm := guardrail.Redact(input)
	if strings.Contains(out, "AKIA") {
		t.Fatalf("expected redacted output, got %q", out)
	}
	restored := rm.Restore(out)
	if !strings.Contains(restored, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("restore failed: %q", restored)
	}
}

func TestRedactEmail(t *testing.T) {
	input := "Contact me at dev@example.com please"
	out, rm := guardrail.Redact(input)
	if strings.Contains(out, "dev@example.com") {
		t.Fatalf("email not redacted: %q", out)
	}
	if !strings.Contains(rm.Restore(out), "dev@example.com") {
		t.Fatal("email restore failed")
	}
}

func TestMergeRedactionMaps(t *testing.T) {
	_, rm1 := guardrail.Redact("key=AKIAIOSFODNN7EXAMPLE")
	_, rm2 := guardrail.Redact("email dev@example.com")
	rm1.Merge(rm2)

	combined := rm1.Restore("[REDACTED_SECRET_1] and [REDACTED_SECRET_2]")
	if !strings.Contains(combined, "AKIA") || !strings.Contains(combined, "dev@example.com") {
		t.Fatalf("merge restore failed: %q", combined)
	}
	if strings.Contains(combined, "REDACTED") {
		t.Fatalf("placeholders should be fully restored: %q", combined)
	}
}
