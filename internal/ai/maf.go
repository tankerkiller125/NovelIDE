package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	afanthropic "github.com/microsoft/agent-framework-go/provider/anthropicprovider"
	afgemini "github.com/microsoft/agent-framework-go/provider/geminiprovider"
	afopenai "github.com/microsoft/agent-framework-go/provider/openaiprovider"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/tool"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicopt "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"
)

// wireDebug logs failed provider calls when NOVELIDE_AI_DEBUG is set — MAF/
// openai-go otherwise report only "400 Bad Request" without the actual message.
var wireDebug = os.Getenv("NOVELIDE_AI_DEBUG") != ""

// openaiCompat rewrites the outgoing chat request so an assistant tool-call turn
// carries the canonical "content": null. openai-go omits the field, which strict
// OpenAI-compatible gateways (notably Cloudflare) reject on tool round-trips with
// "Invalid value at input". Adding null is valid OpenAI, so it's a no-op for
// providers that already accept the omission. It also logs error bodies when
// NOVELIDE_AI_DEBUG is set.
func openaiCompat(req *http.Request, next openaiopt.MiddlewareNext) (*http.Response, error) {
	if req.Body != nil {
		orig, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		fixed := fixAssistantContent(orig)
		if wireDebug && !bytes.Equal(orig, fixed) {
			log.Printf("[ai/wire] openai request: added content:null to assistant tool-call turn(s)")
		}
		req.Body = io.NopCloser(bytes.NewReader(fixed))
		req.ContentLength = int64(len(fixed))
	}
	resp, err := next(req)
	if wireDebug && err == nil && resp != nil && resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		b := string(body)
		if len(b) > 2000 {
			b = b[:2000] + "…"
		}
		log.Printf("[ai/wire] openai %d %s: %s", resp.StatusCode, req.URL.Path, b)
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	return resp, err
}

// fixAssistantContent adds "content": null to any assistant message that has
// tool_calls but no content field. Returns the input unchanged on any problem.
func fixAssistantContent(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	msgs, ok := m["messages"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, mi := range msgs {
		msg, ok := mi.(map[string]any)
		if !ok || msg["role"] != "assistant" {
			continue
		}
		if _, hasTC := msg["tool_calls"]; !hasTC {
			continue
		}
		if c, hasContent := msg["content"]; !hasContent || c == "" {
			msg["content"] = nil
			changed = true
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// NewAgent builds a Microsoft Agent Framework agent for the resolved provider,
// with the given system instructions and tools. Endpoint and key are set on the
// underlying SDK client, so OpenAI-compatible gateways and self-hosted backends
// work via a custom base URL.
func NewAgent(ctx context.Context, p Provider, instructions string, tools []tool.Tool) (*agent.Agent, error) {
	cfg := agent.Config{Tools: tools}
	switch p.Kind {
	case KindOpenAI:
		var opts []openaiopt.RequestOption
		if p.BaseURL != "" {
			opts = append(opts, openaiopt.WithBaseURL(p.BaseURL))
		}
		if p.APIKey != "" {
			opts = append(opts, openaiopt.WithAPIKey(p.APIKey))
		}
		// Canonicalize tool-call requests (content:null) for strict gateways, and
		// surface error bodies under NOVELIDE_AI_DEBUG.
		opts = append(opts, openaiopt.WithMiddleware(openaiCompat))
		cl := openai.NewClient(opts...)
		return afopenai.NewChatCompletionsAgent(cl, afopenai.AgentConfig{
			Config: cfg, Model: p.Model, Instructions: instructions,
		}), nil

	case KindAnthropic:
		var opts []anthropicopt.RequestOption
		if p.BaseURL != "" {
			opts = append(opts, anthropicopt.WithBaseURL(p.BaseURL))
		}
		if p.APIKey != "" {
			opts = append(opts, anthropicopt.WithAPIKey(p.APIKey))
		}
		cl := anthropic.NewClient(opts...)
		return afanthropic.NewAgent(cl, afanthropic.AgentConfig{
			Config: cfg, Model: p.Model, Instructions: instructions,
		}), nil

	case KindGemini:
		cl, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  p.APIKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			return nil, err
		}
		return afgemini.NewAgent(cl, afgemini.AgentConfig{
			Config: cfg, Model: p.Model, Instructions: instructions,
		}), nil

	case KindA2A:
		return newA2AAgent(ctx, p, cfg)

	default:
		return nil, fmt.Errorf("unknown provider kind %q", p.Kind)
	}
}
