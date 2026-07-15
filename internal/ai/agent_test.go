package ai

import (
	"context"
	"strings"
	"testing"
)

// scriptedClient returns queued responses in order, recording each request.
type scriptedClient struct {
	responses []Response
	calls     []Request
}

func (c *scriptedClient) Stream(_ context.Context, req Request, onText func(string)) (Response, error) {
	c.calls = append(c.calls, req)
	r := c.responses[len(c.calls)-1]
	if onText != nil && r.Text != "" {
		onText(r.Text)
	}
	return r, nil
}

func TestRunAgentToolLoop(t *testing.T) {
	client := &scriptedClient{responses: []Response{
		{ToolCalls: []ToolCall{{ID: "c1", Name: "search_codex", Arguments: `{"query":"aria"}`}}, StopReason: "tool_calls"},
		{Text: "Aria is the protagonist.", StopReason: "stop"},
	}}

	var executed []string
	var tooled []string
	resp, err := RunAgent(context.Background(),
		client,
		Request{Messages: []Message{{Role: RoleUser, Content: "who is aria?"}}, Tools: []Tool{{Name: "search_codex"}}},
		func(tc ToolCall) string { executed = append(executed, tc.Name); return "Aria Voss — hero" },
		nil,
		func(tc ToolCall) { tooled = append(tooled, tc.Name) },
		8,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Aria is the protagonist." {
		t.Errorf("final text = %q", resp.Text)
	}
	if len(executed) != 1 || executed[0] != "search_codex" {
		t.Errorf("tool not executed: %v", executed)
	}
	if len(tooled) != 1 {
		t.Errorf("onTool not called: %v", tooled)
	}
	// Second call must include the assistant tool-call turn + the tool result.
	second := client.calls[1]
	if len(second.Messages) != 3 { // user, assistant(tool_calls), tool result
		t.Fatalf("second call messages = %d: %+v", len(second.Messages), second.Messages)
	}
	if second.Messages[2].Role != RoleTool || !strings.Contains(second.Messages[2].Content, "Aria Voss") {
		t.Errorf("tool result not fed back: %+v", second.Messages[2])
	}
}

func TestRunAgentForcesFinalAnswerAtCap(t *testing.T) {
	// A model that always asks for a tool should be forced to answer with tools
	// removed on the final step.
	loopy := Response{ToolCalls: []ToolCall{{ID: "x", Name: "search_codex", Arguments: "{}"}}}
	final := Response{Text: "forced answer"}
	client := &scriptedClient{responses: []Response{loopy, loopy, final}}

	resp, err := RunAgent(context.Background(), client,
		Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Tools: []Tool{{Name: "search_codex"}}},
		func(ToolCall) string { return "..." }, nil, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "forced answer" {
		t.Errorf("expected forced answer, got %q", resp.Text)
	}
	// The forced final turn must have tools stripped.
	if last := client.calls[len(client.calls)-1]; len(last.Tools) != 0 {
		t.Errorf("final forced turn should have no tools, got %d", len(last.Tools))
	}
}
