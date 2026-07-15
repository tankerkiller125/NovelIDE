package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer returns a test server that records the request and streams body.
func sseServer(t *testing.T, body string, capture *http.Request, captureBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			*capture = *r
		}
		if captureBody != nil {
			b, _ := io.ReadAll(r.Body)
			*captureBody = string(b)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, body)
	}))
}

const openaiStream = `data: {"choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_entry","arguments":"{\"id\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"aria\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`

func TestOpenAIStream(t *testing.T) {
	var req http.Request
	var body string
	srv := sseServer(t, openaiStream, &req, &body)
	defer srv.Close()

	c, err := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, APIKey: "sk-test", Model: "gpt-x"})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	resp, err := c.Stream(context.Background(), Request{
		System:    "be helpful",
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		Tools:     []Tool{{Name: "get_entry", Description: "d", Schema: json.RawMessage(`{"type":"object"}`)}},
		SessionID: "ses_abc",
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello world" {
		t.Errorf("text = %q", resp.Text)
	}
	if strings.Join(deltas, "|") != "Hello| world" {
		t.Errorf("deltas = %v", deltas)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_entry" ||
		resp.ToolCalls[0].Arguments != `{"id":"aria"}` {
		t.Fatalf("tool call = %+v", resp.ToolCalls)
	}
	if resp.StopReason != "tool_calls" {
		t.Errorf("stop = %q", resp.StopReason)
	}
	// request shape
	if req.URL.Path != "/chat/completions" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if req.Header.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("auth header = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-session-affinity") != "ses_abc" {
		t.Errorf("session-affinity header = %q", req.Header.Get("x-session-affinity"))
	}
	if !strings.Contains(body, `"stream":true`) || !strings.Contains(body, `"model":"gpt-x"`) ||
		!strings.Contains(body, `"role":"system"`) {
		t.Errorf("body missing fields: %s", body)
	}
}

const anthropicStream = `event: message_start
data: {"type":"message_start","message":{}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}

event: content_block_delta
data: {"index":0,"delta":{"type":"text_delta","text":" there"}}

event: content_block_start
data: {"index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_entry"}}

event: content_block_delta
data: {"index":1,"delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}

event: content_block_delta
data: {"index":1,"delta":{"type":"input_json_delta","partial_json":"\"aria\"}"}}

event: message_delta
data: {"delta":{"stop_reason":"tool_use"}}

event: message_stop
data: {"type":"message_stop"}

`

func TestAnthropicStream(t *testing.T) {
	var req http.Request
	var body string
	srv := sseServer(t, anthropicStream, &req, &body)
	defer srv.Close()

	c, err := New(Provider{Kind: KindAnthropic, BaseURL: srv.URL, APIKey: "ant-test", Model: "claude-x"})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	resp, err := c.Stream(context.Background(), Request{
		System:    "be helpful",
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		Tools:     []Tool{{Name: "get_entry"}},
		SessionID: "ses_xyz",
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hi there" {
		t.Errorf("text = %q", resp.Text)
	}
	if strings.Join(deltas, "|") != "Hi| there" {
		t.Errorf("deltas = %v", deltas)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_entry" ||
		resp.ToolCalls[0].Arguments != `{"id":"aria"}` || resp.ToolCalls[0].ID != "toolu_1" {
		t.Fatalf("tool call = %+v", resp.ToolCalls)
	}
	if resp.StopReason != "tool_calls" { // normalized from "tool_use"
		t.Errorf("stop = %q", resp.StopReason)
	}
	if req.URL.Path != "/v1/messages" {
		t.Errorf("path = %q", req.URL.Path)
	}
	if req.Header.Get("x-api-key") != "ant-test" || req.Header.Get("anthropic-version") == "" {
		t.Errorf("anthropic headers wrong: %v", req.Header)
	}
	// Anthropic auth is x-api-key only — no bearer token (it'd hit the OAuth path).
	if req.Header.Get("Authorization") != "" {
		t.Errorf("anthropic should not send an Authorization header, got %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-session-affinity") != "ses_xyz" {
		t.Errorf("session-affinity header = %q", req.Header.Get("x-session-affinity"))
	}
	if !strings.Contains(body, `"max_tokens"`) || !strings.Contains(body, `"system":"be helpful"`) {
		t.Errorf("body missing fields: %s", body)
	}
	// tool result + assistant tool_use round-trip conversion produces valid JSON.
	var parsed antRequest
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Errorf("request body not valid antRequest: %v", err)
	}
}

func TestToolResultConversion(t *testing.T) {
	// An assistant tool call followed by a tool result must serialize to the
	// right shapes for each provider.
	msgs := []Message{
		{Role: RoleUser, Content: "who is aria?"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c1", Name: "get_entry", Arguments: `{"id":"aria"}`}}},
		{Role: RoleTool, ToolCallID: "c1", Content: "Aria Voss, protagonist"},
	}

	oa := (&openaiClient{p: Provider{Model: "m"}}).buildBody(Request{Messages: msgs}, oaFallbacks{})
	if oa.Messages[2].Role != "tool" || oa.Messages[2].ToolCallID != "c1" {
		t.Errorf("openai tool result wrong: %+v", oa.Messages[2])
	}
	if len(oa.Messages[1].ToolCalls) != 1 {
		t.Errorf("openai assistant tool_calls missing: %+v", oa.Messages[1])
	}

	ant := (&anthropicClient{p: Provider{Model: "m"}}).buildBody(Request{Messages: msgs})
	// user, assistant(tool_use), user(tool_result)
	if len(ant.Messages) != 3 || ant.Messages[2].Role != "user" ||
		ant.Messages[2].Content[0].Type != "tool_result" || ant.Messages[2].Content[0].ToolUseID != "c1" {
		t.Errorf("anthropic tool result wrong: %+v", ant.Messages)
	}
	if ant.Messages[1].Content[0].Type != "tool_use" {
		t.Errorf("anthropic assistant tool_use missing: %+v", ant.Messages[1])
	}
}

func TestUnknownProvider(t *testing.T) {
	if _, err := New(Provider{Kind: "gemini"}); err == nil {
		t.Error("unknown provider kind should error")
	}
}

func TestAnthropicCacheMarkers(t *testing.T) {
	req := Request{
		System:      "instructions",
		Tools:       []Tool{{Name: "get_entry", Schema: json.RawMessage(`{"type":"object"}`)}},
		Messages:    []Message{{Role: RoleUser, Content: "big world context", Cache: true}, {Role: RoleUser, Content: "question"}},
		CachePrefix: true,
	}
	body, _ := json.Marshal((&anthropicClient{p: Provider{Model: "claude"}}).buildBody(req))
	s := string(body)
	// system-as-block, last tool, and the cached context message → 3 breakpoints.
	if got := strings.Count(s, `"cache_control"`); got != 3 {
		t.Fatalf("expected 3 cache_control markers, got %d in %s", got, s)
	}
	// System must be serialized as a text block (array), not a bare string.
	if !strings.Contains(s, `"system":[{"type":"text","text":"instructions"`) {
		t.Errorf("system not emitted as a cached block: %s", s)
	}

	// Without caching, no markers and system is a plain string.
	body2, _ := json.Marshal((&anthropicClient{p: Provider{Model: "claude"}}).buildBody(Request{System: "x", Messages: req.Messages[1:]}))
	if strings.Contains(string(body2), "cache_control") {
		t.Errorf("no cache markers expected when CachePrefix is off: %s", body2)
	}
}

func TestOpenAIIgnoresCacheMarkers(t *testing.T) {
	req := Request{
		System:      "instructions",
		Tools:       []Tool{{Name: "get_entry", Schema: json.RawMessage(`{"type":"object"}`)}},
		Messages:    []Message{{Role: RoleUser, Content: "ctx", Cache: true}},
		CachePrefix: true,
	}
	body, _ := json.Marshal((&openaiClient{p: Provider{Model: "gpt"}}).buildBody(req, oaFallbacks{}))
	// OpenAI caches stable prefixes automatically — we must not emit markup.
	if strings.Contains(string(body), "cache_control") {
		t.Errorf("openai body should not contain cache_control: %s", body)
	}
}

func TestOpenAISystemRoleFallback(t *testing.T) {
	var calls int
	var secondBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		if calls == 1 {
			// Reject the system role the way Cloudflare Workers AI does.
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"errors":[{"message":"Invalid value at messages[0].role: Invalid option: expected one of \"user\"|\"assistant\"","code":7003}]}`)
			return
		}
		secondBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	c, _ := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, Model: "gpt-5.4"})
	resp, err := c.Stream(context.Background(), Request{
		System:   "be a helpful writing assistant",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q", resp.Text)
	}
	if calls != 2 {
		t.Errorf("expected a retry (2 calls), got %d", calls)
	}
	// The retry must carry no system role and fold the prompt into the user turn.
	var parsed oaRequest
	if err := json.Unmarshal([]byte(secondBody), &parsed); err != nil {
		t.Fatalf("retry body invalid: %v", err)
	}
	if len(parsed.Messages) != 1 || parsed.Messages[0].Role != "user" {
		t.Fatalf("expected a single user message, got %+v", parsed.Messages)
	}
	folded := ""
	if parsed.Messages[0].Content != nil {
		folded = *parsed.Messages[0].Content
	}
	if !strings.Contains(folded, "helpful writing assistant") || !strings.Contains(folded, "hi") {
		t.Errorf("system prompt not folded into user turn: %q", folded)
	}
}

func TestOpenAIToolFormatFallback(t *testing.T) {
	var calls int
	var secondBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		if calls == 1 {
			// Cloudflare forwarding OpenAI function tools to Claude, which rejects them.
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"errors":[{"message":"tools.0: Input tag 'function' found using 'type' does not match any of the expected tags: 'custom'","code":7003}]}`)
			return
		}
		secondBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	c, _ := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, Model: "claude-via-cf"})
	resp, err := c.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
		Tools:    []Tool{{Name: "search_codex", Schema: json.RawMessage(`{"type":"object"}`)}},
	}, nil)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if resp.Text != "hi" || calls != 2 {
		t.Errorf("text=%q calls=%d", resp.Text, calls)
	}
	// The retry must carry no tools at all.
	if strings.Contains(secondBody, `"tools"`) {
		t.Errorf("retry should omit tools: %s", secondBody)
	}
}

func TestRejectsToolFormat(t *testing.T) {
	yes := `{"errors":[{"message":"tools.0: Input tag 'function' found using 'type' does not match any of the expected tags: 'custom'"}]}`
	if !rejectsToolFormat([]byte(yes)) {
		t.Error("should detect tool-format rejection")
	}
	if rejectsToolFormat([]byte(`{"error":"context length exceeded"}`)) {
		t.Error("false positive on unrelated 400")
	}
}

func TestRejectsSystemRole(t *testing.T) {
	yes := []string{
		`Invalid value at messages[0].role: expected one of "user"|"assistant"`,
		`{"message":"system role is not supported by this model"}`,
		`invalid role: system`,
	}
	for _, s := range yes {
		if !rejectsSystemRole([]byte(s)) {
			t.Errorf("should detect system-role rejection: %s", s)
		}
	}
	if rejectsSystemRole([]byte(`{"error":"rate limit exceeded"}`)) {
		t.Error("false positive on unrelated 400")
	}
}

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty string should be 0 tokens")
	}
	// ~3.5 chars/token: a 350-char string is roughly 100 tokens.
	got := EstimateTokens(strings.Repeat("a", 350))
	if got < 90 || got > 110 {
		t.Errorf("estimate off: %d", got)
	}
	req := Request{System: strings.Repeat("x", 70), Messages: []Message{{Role: RoleUser, Content: strings.Repeat("y", 70)}}}
	if EstimateRequestTokens(req) < 30 {
		t.Errorf("request estimate too low: %d", EstimateRequestTokens(req))
	}
}

func TestOpenAIMaxCompletionTokensFallback(t *testing.T) {
	var calls int
	var secondBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		if calls == 1 {
			// Reject max_tokens the way newer OpenAI/Cloudflare models do.
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"errors":[{"message":"Unsupported parameter: 'max_tokens' is not supported. Use 'max_completion_tokens' instead."}]}`)
			return
		}
		secondBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	c, _ := New(Provider{Kind: KindOpenAI, BaseURL: srv.URL, Model: "gpt-5"})
	resp, err := c.Stream(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		MaxTokens: 100,
	}, nil)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q", resp.Text)
	}
	if calls != 2 {
		t.Errorf("expected a retry (2 calls), got %d", calls)
	}
	if !strings.Contains(secondBody, `"max_completion_tokens":100`) || strings.Contains(secondBody, `"max_tokens"`) {
		t.Errorf("retry should use max_completion_tokens only: %s", secondBody)
	}
}
