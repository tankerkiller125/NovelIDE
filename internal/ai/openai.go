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

// openaiClient speaks the OpenAI Chat Completions API (also spoken by Ollama,
// OpenRouter, LM Studio, vLLM, Azure OpenAI, …).
type openaiClient struct {
	p    Provider
	http *http.Client
}

func (c *openaiClient) endpoint() string {
	return strings.TrimRight(c.p.BaseURL, "/") + "/chat/completions"
}

// --- request shapes ---

type oaMessage struct {
	Role string `json:"role"`
	// Content is a *string so an assistant tool-call turn serializes as the
	// canonical "content": null (some strict OpenAI-compatible validators reject
	// the field being absent) while normal messages carry their text.
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

func strptr(s string) *string { return &s }

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Index    int    `json:"index,omitempty"`
	Function oaFunc `json:"function"`
}

type oaFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type oaTool struct {
	Type     string     `json:"type"`
	Function oaToolSpec `json:"function"`
}

type oaToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type oaRequest struct {
	Model    string      `json:"model"`
	Messages []oaMessage `json:"messages"`
	Tools    []oaTool    `json:"tools,omitempty"`
	// Newer OpenAI models (o-series, gpt-5, some proxies like Cloudflare) reject
	// max_tokens and require max_completion_tokens; older models and most
	// OpenAI-compatible servers only accept max_tokens. We send max_tokens by
	// default and fall back to max_completion_tokens on a 400.
	Temperature         *float64 `json:"temperature,omitempty"`
	MaxTokens           *int     `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int     `json:"max_completion_tokens,omitempty"`
	Stream              bool     `json:"stream"`
}

// oaFallbacks holds compatibility adjustments applied after a provider rejects
// our default request shape with a 400 (see Stream).
type oaFallbacks struct {
	completionTokens bool // use max_completion_tokens instead of max_tokens
	foldSystem       bool // fold the system prompt into the first user message
	dropTools        bool // omit tools entirely
}

// buildBody assembles the request, applying any compatibility fallbacks.
func (c *openaiClient) buildBody(req Request, fb oaFallbacks) oaRequest {
	msgs := make([]oaMessage, 0, len(req.Messages)+1)
	if req.System != "" && !fb.foldSystem {
		msgs = append(msgs, oaMessage{Role: "system", Content: strptr(req.System)})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case RoleTool:
			msgs = append(msgs, oaMessage{Role: "tool", ToolCallID: m.ToolCallID, Content: strptr(m.Content)})
		case RoleAssistant:
			// Skip an empty assistant turn (e.g. a failed/blank prior response):
			// providers reject a message with neither content nor tool calls.
			if m.Content == "" && len(m.ToolCalls) == 0 {
				continue
			}
			om := oaMessage{Role: "assistant"}
			if m.Content != "" {
				om.Content = strptr(m.Content)
			} // else leave nil → "content": null for a tool-call-only turn
			for _, tc := range m.ToolCalls {
				om.ToolCalls = append(om.ToolCalls, oaToolCall{
					ID: tc.ID, Type: "function",
					Function: oaFunc{Name: tc.Name, Arguments: tc.Arguments},
				})
			}
			msgs = append(msgs, om)
		default:
			msgs = append(msgs, oaMessage{Role: "user", Content: strptr(m.Content)})
		}
	}
	if req.System != "" && fb.foldSystem {
		// Prepend to the first user message (or add one) so the prompt survives
		// without a system role. The first user turn is stable across turns, so
		// the cached prefix stays intact.
		placed := false
		for i := range msgs {
			if msgs[i].Role == "user" {
				prev := ""
				if msgs[i].Content != nil {
					prev = *msgs[i].Content
				}
				msgs[i].Content = strptr(req.System + "\n\n" + prev)
				placed = true
				break
			}
		}
		if !placed {
			msgs = append([]oaMessage{{Role: "user", Content: strptr(req.System)}}, msgs...)
		}
	}
	var tools []oaTool
	if !fb.dropTools {
		for _, t := range req.Tools {
			tools = append(tools, oaTool{Type: "function", Function: oaToolSpec{
				Name: t.Name, Description: t.Description, Parameters: t.Schema,
			}})
		}
	}
	body := oaRequest{Model: c.p.Model, Messages: msgs, Tools: tools, Stream: !c.p.NoStream}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		if fb.completionTokens {
			body.MaxCompletionTokens = &mt
		} else {
			body.MaxTokens = &mt
		}
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}
	return body
}

// rejectsSystemRole reports whether a 400 body indicates the provider won't
// accept a "system" role in messages (so we should fold it into a user turn).
func rejectsSystemRole(data []byte) bool {
	s := strings.ToLower(string(data))
	// Cloudflare: `Invalid value at messages[0].role: ... expected one of "user"|"assistant"`.
	if strings.Contains(s, "messages[0].role") {
		return true
	}
	// Generic variants: "system role is not supported/allowed", "invalid role: system".
	return strings.Contains(s, "system") && strings.Contains(s, "role") &&
		(strings.Contains(s, "not supported") || strings.Contains(s, "not allowed") ||
			strings.Contains(s, "unsupported") || strings.Contains(s, "invalid"))
}

// parseCompletion reads a non-streaming Chat Completions response (one JSON
// body) and emits its whole text via onText once. Used when NoStream is set for
// endpoints whose SSE streaming is broken.
func (c *openaiClient) parseCompletion(body io.Reader, onText func(string)) (Response, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return Response{}, err
	}
	if wireDebug {
		b := string(raw)
		if len(b) > 4000 {
			b = b[:4000] + "…"
		}
		dbgf("openai non-stream body: %s", b)
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content   string       `json:"content"`
				ToolCalls []oaToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return Response{}, fmt.Errorf("provider returned an unparseable response: %s", truncateForErr(raw))
	}
	if len(r.Choices) == 0 {
		return Response{}, fmt.Errorf("provider returned no choices: %s", truncateForErr(raw))
	}
	ch := r.Choices[0]
	out := Response{Text: ch.Message.Content, StopReason: ch.FinishReason}
	if out.Text != "" && onText != nil {
		onText(out.Text)
	}
	for _, tc := range ch.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return out, nil
}

func truncateForErr(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// rejectsToolFormat reports whether a 400 body indicates the provider won't
// accept our OpenAI function-style tool definitions (notably Cloudflare passing
// them unconverted to a Claude model, which expects Anthropic-style tools).
func rejectsToolFormat(data []byte) bool {
	s := strings.ToLower(string(data))
	if !strings.Contains(s, "tool") {
		return false
	}
	return strings.Contains(s, "input tag") || strings.Contains(s, "expected tag") ||
		strings.Contains(s, "does not match any of the expected") ||
		(strings.Contains(s, "'function'") && strings.Contains(s, "type"))
}

// post issues one streaming request with the given body.
func (c *openaiClient) post(ctx context.Context, body oaRequest, sessionID string) (*http.Response, error) {
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint(), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.p.APIKey)
	}
	if c.p.NoStream {
		httpReq.Header.Set("Accept", "application/json")
	} else {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	// Pins the session to the node holding its cached prefix (Cloudflare Workers
	// AI prompt caching); an ignored custom header elsewhere.
	if sessionID != "" {
		httpReq.Header.Set("x-session-affinity", sessionID)
	}
	return c.http.Do(httpReq)
}

// --- streaming response shapes ---

type oaChunk struct {
	Choices []struct {
		Delta struct {
			Content   string       `json:"content"`
			ToolCalls []oaToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (c *openaiClient) Stream(ctx context.Context, req Request, onText func(string)) (Response, error) {
	ctx, cancel := ctxWithDefaultTimeout(ctx)
	defer cancel()

	// Some OpenAI-compatible providers reject request shapes we send by default:
	// newer models want max_completion_tokens over max_tokens; a few (e.g. certain
	// Cloudflare Workers AI models) accept only user/assistant roles, not a system
	// message; and Cloudflare's OpenAI shim forwards our function-style tools
	// unconverted to Claude, which rejects them. Detect those specific 400s and
	// retry with the relevant adjustment(s) rather than failing outright.
	var fb oaFallbacks
	var resp *http.Response
	var err error
	for attempt := 0; ; attempt++ {
		resp, err = c.post(ctx, c.buildBody(req, fb), req.SessionID)
		if err != nil {
			return Response{}, err
		}
		if resp.StatusCode != http.StatusBadRequest || attempt >= 3 {
			break
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		changed := false
		if !fb.completionTokens && bytes.Contains(data, []byte("max_completion_tokens")) {
			fb.completionTokens, changed = true, true
		}
		if !fb.foldSystem && rejectsSystemRole(data) {
			fb.foldSystem, changed = true, true
		}
		if !fb.dropTools && len(req.Tools) > 0 && rejectsToolFormat(data) {
			fb.dropTools, changed = true, true
		}
		if !changed {
			return Response{}, fmt.Errorf("provider returned 400: %s", string(data))
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Response{}, httpErr(resp)
	}
	dbgf("openai resp status=%d content-type=%q content-encoding=%q transfer-encoding=%v",
		resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"), resp.TransferEncoding)

	if c.p.NoStream {
		return c.parseCompletion(resp.Body, onText)
	}

	var out Response
	var text strings.Builder
	calls := map[int]*ToolCall{}
	var order []int
	var evNum int
	// Some OpenAI-compatible endpoints answer an OpenAI-format request with an
	// Anthropic-format stream (Cloudflare serving a Claude model through its
	// OpenAI endpoint). OpenAI SSE never uses "event:" lines and Anthropic always
	// does, so a non-empty event reliably means we should parse it as Anthropic.
	var ant *antStream

	err = scanSSE(resp.Body, func(event, data string) (bool, error) {
		evNum++
		if wireDebug && evNum <= 40 {
			d := data
			if len(d) > 200 {
				d = d[:200] + "…"
			}
			dbgf("evt#%d event=%q data=%s", evNum, event, d)
		}
		if event != "" || ant != nil {
			if ant == nil {
				ant = newAntStream(onText)
			}
			return ant.handle(event, data)
		}
		if data == "[DONE]" {
			return true, nil
		}
		var ch oaChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return false, nil // ignore keep-alives / malformed chunks
		}
		if len(ch.Choices) == 0 {
			return false, nil
		}
		ci := ch.Choices[0]
		if ci.Delta.Content != "" {
			text.WriteString(ci.Delta.Content)
			if onText != nil {
				onText(ci.Delta.Content)
			}
		}
		for _, tc := range ci.Delta.ToolCalls {
			cur, ok := calls[tc.Index]
			if !ok {
				cur = &ToolCall{}
				calls[tc.Index] = cur
				order = append(order, tc.Index)
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function.Name != "" {
				cur.Name = tc.Function.Name
			}
			cur.Arguments += tc.Function.Arguments
		}
		if ci.FinishReason != "" {
			out.StopReason = ci.FinishReason
		}
		return false, nil
	})
	if err != nil {
		return Response{}, err
	}
	if ant != nil { // the endpoint replied in Anthropic format
		return ant.response(), nil
	}

	out.Text = text.String()
	sort.Ints(order)
	for _, i := range order {
		out.ToolCalls = append(out.ToolCalls, *calls[i])
	}
	return out, nil
}
