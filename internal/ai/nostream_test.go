package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With NoStream, the OpenAI client sends stream:false and reads the whole
// completion body once — the fix for Cloudflare dropping streamed content.
func TestOpenAINonStreaming(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"Hello there, full reply."},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c, _ := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, Model: "anthropic/claude-opus-4.6", NoStream: true})
	var got string
	resp, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		func(s string) { got += s })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello there, full reply." || got != "Hello there, full reply." {
		t.Errorf("text=%q emitted=%q", resp.Text, got)
	}
	if resp.StopReason != "stop" {
		t.Errorf("stop=%q", resp.StopReason)
	}
	if !strings.Contains(body, `"stream":false`) {
		t.Errorf("request should set stream:false, got: %s", body)
	}
}

// With NoStream, the Anthropic client parses a single Messages JSON body.
func TestAnthropicNonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"Full Claude reply."}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	c, _ := New(Provider{Kind: KindAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "claude", NoStream: true})
	var got string
	resp, err := c.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}},
		func(s string) { got += s })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Full Claude reply." || got != "Full Claude reply." || resp.StopReason != "stop" {
		t.Errorf("text=%q emitted=%q stop=%q", resp.Text, got, resp.StopReason)
	}
}
