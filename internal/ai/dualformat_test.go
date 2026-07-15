package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A faithful slice of the real Cloudflare→Claude stream: Anthropic Messages
// framing with whitespace-padded data lines and event: ping keep-alives,
// returned in response to an OpenAI-format request.
const realAnthropicStream = "event: message_start\n" +
	`data: {"type":"message_start","message":{"model":"claude-opus-4-6","id":"msg_x","role":"assistant","content":[],"usage":{"input_tokens":12033,"output_tokens":2}}            }` + "\n\n" +
	"event: content_block_start\n" +
	`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
	"event: ping\n" +
	`data: {"type": "ping"}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"\n\nHere"}    }` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" are some thoughts"}     }` + "\n\n" +
	"event: content_block_stop\n" +
	`data: {"type":"content_block_stop","index":0           }` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1451}         }` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"        }` + "\n\n"

func serveStream(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(body))
	}))
}

// The Anthropic client parses its own real-world stream (padding, pings).
func TestAnthropicRealStream(t *testing.T) {
	srv := serveStream(t, realAnthropicStream)
	defer srv.Close()
	c, _ := New(Provider{Kind: KindAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "claude-opus-4-6"})
	var deltas []string
	resp, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp.Text != "\n\nHere are some thoughts" || len(deltas) != 2 || resp.StopReason != "stop" {
		t.Errorf("text=%q deltas=%d stop=%q", resp.Text, len(deltas), resp.StopReason)
	}
}

// The OpenAI client transparently parses an Anthropic-format response (Cloudflare
// serving Claude via its OpenAI endpoint) — the fix for the blank-panel bug.
func TestOpenAIParsesAnthropicResponse(t *testing.T) {
	srv := serveStream(t, realAnthropicStream)
	defer srv.Close()
	c, _ := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, Model: "claude"})
	var deltas []string
	resp, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp.Text != "\n\nHere are some thoughts" || len(deltas) != 2 || resp.StopReason != "stop" {
		t.Errorf("openai client failed to parse anthropic response: text=%q deltas=%d stop=%q", resp.Text, len(deltas), resp.StopReason)
	}
}

// An empty assistant turn (e.g. a failed prior response) must not be sent —
// providers reject a message with neither content nor tool calls.
func TestEmptyAssistantMessageSkipped(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: ""}, // failed/blank turn
		{Role: RoleUser, Content: "still there?"},
	}
	oa := (&openaiClient{p: Provider{Model: "m"}}).buildBody(Request{Messages: msgs}, oaFallbacks{})
	for i, m := range oa.Messages {
		if m.Role == "assistant" && (m.Content == nil || *m.Content == "") && len(m.ToolCalls) == 0 {
			t.Errorf("openai: empty assistant message not skipped at %d", i)
		}
	}
	if len(oa.Messages) != 2 {
		t.Errorf("openai: expected 2 messages, got %d", len(oa.Messages))
	}

	ant := (&anthropicClient{p: Provider{Model: "m"}}).buildBody(Request{Messages: msgs})
	for i, m := range ant.Messages {
		if len(m.Content) == 0 {
			t.Errorf("anthropic: empty message at %d", i)
		}
	}
}

// An assistant tool-call turn must serialize the canonical "content": null
// (strict OpenAI-compatible validators reject the field being absent).
func TestAssistantToolCallHasNullContent(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "read_chapter", Arguments: `{"bookId":"b","chapter":"c.md"}`}}},
		{Role: RoleTool, ToolCallID: "c1", Content: "result"},
	}
	body := (&openaiClient{p: Provider{Model: "gpt"}}).buildBody(Request{Messages: msgs}, oaFallbacks{})
	raw, _ := json.Marshal(body)
	s := string(raw)
	if !strings.Contains(s, `"tool_calls"`) || !strings.Contains(s, `"content":null`) {
		t.Errorf("assistant tool-call turn should carry content:null: %s", s)
	}
}
