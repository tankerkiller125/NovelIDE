package ai

import (
	"strings"
	"testing"
)

// The openai request rewriter adds the canonical content:null to an assistant
// tool-call turn (which Cloudflare's gateway rejects when the field is omitted),
// and leaves normal messages alone.
func TestFixAssistantContent(t *testing.T) {
	in := `{"model":"x","messages":[` +
		`{"role":"user","content":"hi"},` +
		`{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"t","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"r"}]}`
	out := string(fixAssistantContent([]byte(in)))
	if !strings.Contains(out, `"content":null`) {
		t.Fatalf("expected content:null on the assistant tool-call turn: %s", out)
	}

	// A normal assistant message (with content, no tool_calls) is untouched.
	plain := `{"messages":[{"role":"assistant","content":"hello"}]}`
	if strings.Contains(string(fixAssistantContent([]byte(plain))), "null") {
		t.Error("should not rewrite a normal assistant message")
	}

	// Non-JSON is returned unchanged.
	bad := []byte("not json")
	if string(fixAssistantContent(bad)) != "not json" {
		t.Error("non-JSON body should pass through unchanged")
	}
}
