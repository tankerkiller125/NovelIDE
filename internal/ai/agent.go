package ai

import "context"

// DefaultAgentSteps bounds a tool-use loop so a misbehaving model can't spin
// forever.
const DefaultAgentSteps = 8

// RunAgent drives a tool-use loop: it streams a model turn, executes any tools
// the model requests via exec, feeds the results back as tool messages, and
// repeats until the model answers without calling tools (or maxSteps is hit, at
// which point it makes one final turn with tools removed to force an answer).
//
// onText streams text deltas across all turns; onTool is called once per
// requested tool (for surfacing activity to the UI). Tools live in req.Tools
// and, being static, stay in the cached prefix across every turn of the loop.
func RunAgent(
	ctx context.Context,
	client Client,
	req Request,
	exec func(ToolCall) string,
	onText func(string),
	onTool func(ToolCall),
	maxSteps int,
) (Response, error) {
	if maxSteps <= 0 {
		maxSteps = DefaultAgentSteps
	}
	for step := 0; step < maxSteps; step++ {
		resp, err := client.Stream(ctx, req, onText)
		if err != nil {
			return resp, err
		}
		if len(resp.ToolCalls) == 0 {
			return resp, nil // the model answered
		}
		// Record the assistant's tool-call turn, then each tool's result.
		req.Messages = append(req.Messages, Message{
			Role: RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls,
		})
		for _, tc := range resp.ToolCalls {
			if onTool != nil {
				onTool(tc)
			}
			result := exec(tc)
			req.Messages = append(req.Messages, Message{
				Role: RoleTool, ToolCallID: tc.ID, Content: result,
			})
		}
	}
	// Step cap reached: force a final answer with tools removed.
	req.Tools = nil
	return client.Stream(ctx, req, onText)
}
