// Package ai is NovelIDE's optional AI layer. It presents one neutral,
// streaming, tool-calling interface over two provider wire formats — OpenAI
// Chat Completions and Anthropic Messages — so any provider (OpenAI, Anthropic,
// Ollama, OpenRouter, Azure, self-hosted…) works with a custom base URL, key,
// and model. The same client serves both the writing-assistant and planning
// (agent) modes; only the system prompt, tools, and loop differ.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProviderKind selects the wire format.
type ProviderKind string

const (
	KindOpenAI    ProviderKind = "openai"
	KindAnthropic ProviderKind = "anthropic"
)

// Provider is a fully-specified connection to a model.
type Provider struct {
	Kind    ProviderKind `json:"kind"`
	BaseURL string       `json:"baseUrl"` // e.g. https://api.openai.com/v1, https://api.anthropic.com, http://localhost:11434/v1
	APIKey  string       `json:"apiKey"`
	Model   string       `json:"model"`
	// NoStream requests the full reply in one response (stream:false) instead of
	// SSE. Needed for endpoints whose streaming is broken — notably Cloudflare's
	// OpenAI-compat gateway, which drops the content when streaming a Claude
	// model. The reply then arrives all at once rather than token-by-token.
	NoStream bool `json:"noStream"`
}

// Role is a message author.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a model request to invoke a tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON object as a string
}

// Message is one turn in the conversation. A tool result is a message with
// Role=tool, ToolCallID set, and Content holding the result text.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`  // assistant only
	ToolCallID string     `json:"toolCallId,omitempty"` // tool result only
	// Cache marks this message as a prompt-cache breakpoint: everything up to
	// and including it is cached (Anthropic emits cache_control; OpenAI caches
	// stable prefixes automatically). Put it on the large, static world/codex
	// context message so the whole grounding prefix is reused across turns.
	Cache bool `json:"cache,omitempty"`
}

// Tool is a function the model may call. Schema is a JSON Schema object for the
// parameters.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// Request is a single model call.
//
// Cache-friendliness: keep the static prefix (System, Tools, and the leading
// world/codex context message) byte-identical across turns and put volatile
// content last. Set CachePrefix to cache System+Tools, and Message.Cache on the
// context message; on OpenAI these are no-ops but stable ordering still hits
// its automatic prefix cache.
type Request struct {
	System      string
	Messages    []Message
	Tools       []Tool
	Temperature float64
	MaxTokens   int
	// CachePrefix caches the System prompt and Tool definitions (Anthropic
	// cache_control breakpoints; ignored by OpenAI).
	CachePrefix bool
	// SessionID, when set, is sent as the x-session-affinity header. Providers
	// that do prefix caching by routing (notably Cloudflare Workers AI) use it to
	// pin a session's requests to the node holding its cached prefix; on other
	// providers it's an ignored custom header. Keep it stable across every turn
	// (and every agent-loop step) of a session to maximize cache hits.
	SessionID string
}

// Response is the assembled result of a streamed call.
type Response struct {
	Text       string     // full assistant text
	ToolCalls  []ToolCall // tool-use requests, if any
	StopReason string     // "stop" | "tool_calls" | "length" | provider-specific
}

// Client sends requests to a provider.
type Client interface {
	// Stream sends req and calls onText for each incremental text delta,
	// returning the fully assembled response (including any tool calls).
	Stream(ctx context.Context, req Request, onText func(string)) (Response, error)
}

// New returns a client for the provider.
func New(p Provider) (Client, error) {
	switch p.Kind {
	case KindOpenAI:
		return &openaiClient{p: p, http: defaultHTTP()}, nil
	case KindAnthropic:
		return &anthropicClient{p: p, http: defaultHTTP()}, nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q", p.Kind)
	}
}

func defaultHTTP() *http.Client {
	// No overall timeout — streamed responses can run for minutes; cancellation
	// is via the request context.
	return &http.Client{}
}

// defaultMaxTokens supplies a value when the caller left it zero (Anthropic
// requires max_tokens).
func (r Request) maxTokens() int {
	if r.MaxTokens > 0 {
		return r.MaxTokens
	}
	return 4096
}

// EstimateTokens approximates the token count of a string for context
// budgeting. Exact tokenization varies per model and bundling every tokenizer
// is impractical for an any-provider tool, so this uses a deliberately
// conservative ~3.5 chars/token ratio to stay safely under a model's window.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s)*10 + 34) / 35 // ceil(len/3.5)
}

// EstimateRequestTokens is a rough input-token estimate for a whole request
// (system + tools + messages), for budgeting against a model's context window.
func EstimateRequestTokens(req Request) int {
	n := EstimateTokens(req.System)
	for _, t := range req.Tools {
		n += EstimateTokens(t.Name) + EstimateTokens(t.Description) + EstimateTokens(string(t.Schema))
		n += 8 // per-tool framing overhead
	}
	for _, m := range req.Messages {
		n += EstimateTokens(m.Content) + 4 // per-message framing
		for _, tc := range m.ToolCalls {
			n += EstimateTokens(tc.Name) + EstimateTokens(tc.Arguments) + 8
		}
	}
	return n
}

// httpErr reads an error body and forms a useful message.
func httpErr(resp *http.Response) error {
	var buf [4096]byte
	n, _ := resp.Body.Read(buf[:])
	body := string(buf[:n])
	if body != "" {
		return fmt.Errorf("provider returned %d: %s", resp.StatusCode, body)
	}
	return fmt.Errorf("provider returned %d", resp.StatusCode)
}

// ctxWithDefaultTimeout gives a call a generous ceiling if the caller passed a
// context without a deadline.
func ctxWithDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 10*time.Minute)
}
