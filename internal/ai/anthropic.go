package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// anthropicClient speaks the Anthropic Messages API.
type anthropicClient struct {
	p    Provider
	http *http.Client
}

func (c *anthropicClient) endpoint() string {
	return strings.TrimRight(c.p.BaseURL, "/") + "/v1/messages"
}

// --- request shapes ---

// cacheControl marks an Anthropic content block as a prompt-cache breakpoint.
type cacheControl struct {
	Type string `json:"type"`
}

func ephemeral() *cacheControl { return &cacheControl{Type: "ephemeral"} }

type antBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID    string        `json:"tool_use_id,omitempty"`
	Content      string        `json:"content,omitempty"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type antMessage struct {
	Role    string     `json:"role"`
	Content []antBlock `json:"content"`
}

type antSysBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type antTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *cacheControl   `json:"cache_control,omitempty"`
}

type antRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	System      any          `json:"system,omitempty"` // string, or []antSysBlock when cached
	Messages    []antMessage `json:"messages"`
	Tools       []antTool    `json:"tools,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	Stream      bool         `json:"stream"`
}

func (c *anthropicClient) buildBody(req Request) antRequest {
	var msgs []antMessage
	var pending []antBlock // consecutive tool results collapse into one user message
	flush := func() {
		if len(pending) > 0 {
			msgs = append(msgs, antMessage{Role: "user", Content: pending})
			pending = nil
		}
	}
	// cacheLast marks the final block of a message as a cache breakpoint.
	cacheLast := func(blocks []antBlock) {
		if len(blocks) > 0 {
			blocks[len(blocks)-1].CacheControl = ephemeral()
		}
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleTool:
			pending = append(pending, antBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content})
		case RoleUser:
			flush()
			blocks := []antBlock{{Type: "text", Text: m.Content}}
			if m.Cache {
				cacheLast(blocks)
			}
			msgs = append(msgs, antMessage{Role: "user", Content: blocks})
		case RoleAssistant:
			flush()
			var blocks []antBlock
			if m.Content != "" {
				blocks = append(blocks, antBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage("{}")
				if strings.TrimSpace(tc.Arguments) != "" {
					input = json.RawMessage(tc.Arguments)
				}
				blocks = append(blocks, antBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input})
			}
			// Skip an empty assistant turn: a message with no content blocks is
			// rejected ("content: Invalid input").
			if len(blocks) == 0 {
				continue
			}
			if m.Cache {
				cacheLast(blocks)
			}
			msgs = append(msgs, antMessage{Role: "assistant", Content: blocks})
		}
	}
	flush()

	var tools []antTool
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, antTool{Name: t.Name, Description: t.Description, InputSchema: schema})
	}
	// Cache the static prefix (system + tools) at its final block.
	if req.CachePrefix && len(tools) > 0 {
		tools[len(tools)-1].CacheControl = ephemeral()
	}

	body := antRequest{
		Model: c.p.Model, MaxTokens: req.maxTokens(),
		System: c.buildSystem(req), Messages: msgs, Tools: tools, Stream: !c.p.NoStream,
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}
	return body
}

// buildSystem returns the system field: a plain string, or a cached text block
// when CachePrefix is set (so the instructions are a cache breakpoint).
func (c *anthropicClient) buildSystem(req Request) any {
	if req.System == "" {
		return nil
	}
	if req.CachePrefix {
		return []antSysBlock{{Type: "text", Text: req.System, CacheControl: ephemeral()}}
	}
	return req.System
}

func normalizeStopReason(r string) string {
	switch r {
	case "tool_use":
		return "tool_calls"
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	default:
		return r
	}
}

func (c *anthropicClient) Stream(ctx context.Context, req Request, onText func(string)) (Response, error) {
	ctx, cancel := ctxWithDefaultTimeout(ctx)
	defer cancel()

	raw, _ := json.Marshal(c.buildBody(req))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if c.p.APIKey != "" {
		// Anthropic and Anthropic-compatible proxies (incl. Cloudflare AI Gateway's
		// anthropic route) authenticate with x-api-key; a stray bearer token gets
		// forwarded to Anthropic and rejected as OAuth, so we send x-api-key only.
		httpReq.Header.Set("x-api-key", c.p.APIKey)
	}
	if c.p.NoStream {
		httpReq.Header.Set("Accept", "application/json")
	} else {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	// Session pinning for routing-based prefix caches (Cloudflare Workers AI);
	// ignored by Anthropic's own API.
	if req.SessionID != "" {
		httpReq.Header.Set("x-session-affinity", req.SessionID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Response{}, httpErr(resp)
	}

	if c.p.NoStream {
		return c.parseMessage(resp.Body, onText)
	}

	acc := newAntStream(onText)
	if err := scanSSE(resp.Body, acc.handle); err != nil {
		return Response{}, err
	}
	return acc.response(), nil
}

// antStream accumulates an Anthropic Messages streaming response. It is shared:
// the Anthropic client always uses it, and the OpenAI client falls back to it
// when an endpoint returns Anthropic-format SSE despite an OpenAI-format request
// (notably Cloudflare serving a Claude model through its OpenAI endpoint).
type antStream struct {
	text   strings.Builder
	calls  map[int]*ToolCall
	order  []int
	stop   string
	onText func(string)
}

func newAntStream(onText func(string)) *antStream {
	return &antStream{calls: map[int]*ToolCall{}, onText: onText}
}

// handle consumes one Anthropic SSE event; it matches the scanSSE handler shape.
func (a *antStream) handle(event, data string) (bool, error) {
	switch event {
	case "content_block_start":
		var e struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if json.Unmarshal([]byte(data), &e) == nil && e.ContentBlock.Type == "tool_use" {
			a.calls[e.Index] = &ToolCall{ID: e.ContentBlock.ID, Name: e.ContentBlock.Name}
			a.order = append(a.order, e.Index)
		}
	case "content_block_delta":
		var e struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &e) != nil {
			return false, nil
		}
		switch e.Delta.Type {
		case "text_delta":
			if e.Delta.Text != "" {
				a.text.WriteString(e.Delta.Text)
				if a.onText != nil {
					a.onText(e.Delta.Text)
				}
			}
		case "input_json_delta":
			if tc, ok := a.calls[e.Index]; ok {
				tc.Arguments += e.Delta.PartialJSON
			}
		}
	case "message_delta":
		var e struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &e) == nil && e.Delta.StopReason != "" {
			a.stop = normalizeStopReason(e.Delta.StopReason)
		}
	case "error":
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal([]byte(data), &e)
		return false, fmt.Errorf("anthropic stream error: %s", e.Error.Message)
	case "message_stop":
		return true, nil
	}
	return false, nil
}

// parseMessage reads a non-streaming Anthropic Messages response (one JSON
// body) and emits its whole text via onText once. Used when NoStream is set.
func (c *anthropicClient) parseMessage(body io.Reader, onText func(string)) (Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return Response{}, err
	}
	var r struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return Response{}, fmt.Errorf("provider returned an unparseable response: %s", truncateForErr(raw))
	}
	var out Response
	out.StopReason = normalizeStopReason(r.StopReason)
	var text strings.Builder
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Arguments: string(b.Input)})
		}
	}
	out.Text = text.String()
	if out.Text != "" && onText != nil {
		onText(out.Text)
	}
	return out, nil
}

func (a *antStream) response() Response {
	out := Response{Text: a.text.String(), StopReason: a.stop}
	sort.Ints(a.order)
	for _, i := range a.order {
		out.ToolCalls = append(out.ToolCalls, *a.calls[i])
	}
	return out
}
