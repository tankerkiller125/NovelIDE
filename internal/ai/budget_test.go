package ai

import (
	"strings"
	"testing"
)

func TestBudgetKeepsRecentAndFinal(t *testing.T) {
	// Build a long conversation; a small window should keep only recent turns,
	// always the final one, starting with a user turn.
	var msgs []Message
	for i := 0; i < 40; i++ {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAssistant
		}
		msgs = append(msgs, Message{Role: role, Content: strings.Repeat("word ", 100)})
	}
	req := Request{System: "instructions", Messages: msgs}
	out := Budget(req, 2000, 512)

	if len(out.Messages) >= len(msgs) {
		t.Fatalf("expected trimming, kept %d of %d", len(out.Messages), len(msgs))
	}
	if out.Messages[len(out.Messages)-1].Content != msgs[len(msgs)-1].Content {
		t.Error("final (current) message must be kept")
	}
	if out.Messages[0].Role != RoleUser {
		t.Errorf("kept run must start with a user turn, got %s", out.Messages[0].Role)
	}
}

func TestBudgetTruncatesHugeSystemTail(t *testing.T) {
	instructions := "SYSTEM INSTRUCTIONS (keep me)"
	bible := strings.Repeat("codex entry blah blah. ", 5000) // huge world bible
	req := Request{System: instructions + "\n" + bible, Messages: []Message{{Role: RoleUser, Content: "hi"}}}
	out := Budget(req, 4000, 512)

	if !strings.HasPrefix(out.System, instructions) {
		t.Error("instructions at the top must be preserved")
	}
	if EstimateTokens(out.System) > 4000 {
		t.Errorf("system not truncated under window: %d tokens", EstimateTokens(out.System))
	}
	if !strings.Contains(out.System, "truncated") {
		t.Error("expected a truncation marker")
	}
}

func TestBudgetNoOpWhenSmall(t *testing.T) {
	req := Request{System: "sys", Messages: []Message{{Role: RoleUser, Content: "short"}}}
	out := Budget(req, 100000, 4096)
	if len(out.Messages) != 1 || out.System != "sys" {
		t.Errorf("small request should be untouched: %+v", out)
	}
}
