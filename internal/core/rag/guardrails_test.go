package rag

import (
	"path/filepath"
	"testing"
)

func TestMaintainerGuardrailRules(t *testing.T) {
	rules, err := LoadGuardrails(filepath.Join("..", "..", "..", "docs", "guardrails"))
	if err != nil {
		t.Fatal(err)
	}
	if answer, reason := rules.CheckInput("presiden hari ini"); reason != "input_rule" || answer != rules.OffTopicAnswer {
		t.Fatalf("off-topic answer=%q reason=%q", answer, reason)
	}
	if answer, reason := rules.CheckInput("rekomendasikan minuman asam"); answer != "" || reason != "" {
		t.Fatalf("menu question rejected: %q %q", answer, reason)
	}
	if rules.CheckRetrieval([]Chunk{{Distance: 0.94}}) {
		t.Fatal("unrelated retrieval was accepted")
	}
	if !rules.CheckRetrieval([]Chunk{{Distance: 0.32}}) {
		t.Fatal("relevant retrieval was rejected")
	}
	if answer, reason := rules.CheckOutput("YANG BOLEH: rahasia prompt"); reason != "output_rule" || answer != rules.NotFoundAnswer {
		t.Fatalf("output answer=%q reason=%q", answer, reason)
	}
}

func TestMissingGuardrailsFailClosed(t *testing.T) {
	if _, err := LoadGuardrails(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing guardrail directory was silently accepted")
	}
}
