package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// printOf captures what printEstimate writes.
func printOf(t *testing.T, body string) string {
	t.Helper()
	var e estimateView
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	printEstimate("auth/login", e)
	os.Stdout = old
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// TestPrintEstimateUnmeasured: an unmeasured entity prints "unmeasured (at
// least n tokens)" in place of the actual and the delta (SPEC-020 FR-7.2).
func TestPrintEstimateUnmeasured(t *testing.T) {
	out := printOf(t, `{"ref_type":"feature","tokens":1000,"tier":"rough","estimated":true,"complete":true,
		"actual_tokens":null,"delta":null,"unmeasured":true,"measured_part":1200}`)
	if !strings.Contains(out, "unmeasured (at least 1200 tokens)") || strings.Contains(out, "delta") {
		t.Errorf("output = %q", out)
	}
	out = printOf(t, `{"ref_type":"feature","tokens":1000,"tier":"rough","estimated":true,"complete":true,
		"actual_tokens":1500,"delta":500,"unmeasured":false}`)
	if !strings.Contains(out, "actual: 1500 tokens (delta +500)") {
		t.Errorf("output = %q", out)
	}
}
