// Package acp connects NovelIDE to locally-installed coding-agent CLIs over the
// Agent Client Protocol (ACP): it detects which agents are installed, launches
// the chosen one as a subprocess, and drives one prompt turn while streaming the
// agent's output and mediating its file access.
//
// NovelIDE is the ACP *client*: the agent may read manuscript files through us
// and its file writes are surfaced for the author's approval — nothing lands on
// disk unless the caller's WriteFile callback allows it.
package acp

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	acp "github.com/coder/acp-go-sdk"
)

// Agent is a known ACP-capable CLI agent and how to launch it in ACP mode.
type Agent struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// command is the binary probed on PATH; args launch it in ACP mode. Not
	// serialized to the frontend.
	command string   `json:"-"`
	args    []string `json:"-"`
}

// knownAgents is the curated table of agents NovelIDE can drive. Launch commands
// come from each agent's ACP adapter; add rows as more CLIs ship ACP support.
var knownAgents = []Agent{
	{ID: "claude-code", Label: "Claude Code", command: "npx", args: []string{"-y", "@zed-industries/claude-code-acp@latest"}},
	{ID: "gemini", Label: "Gemini CLI", command: "gemini", args: []string{"--experimental-acp"}},
}

// Detect returns the known agents whose launch binary is on PATH. It always
// returns a non-nil slice so it serializes to a JSON array, not null.
func Detect() []Agent {
	out := make([]Agent, 0, len(knownAgents))
	for _, a := range knownAgents {
		if _, err := exec.LookPath(a.command); err == nil {
			out = append(out, Agent{ID: a.ID, Label: a.Label})
		}
	}
	return out
}

// Callbacks bridge the ACP agent's activity back to the caller.
type Callbacks struct {
	// OnText receives streamed assistant-text chunks.
	OnText func(text string)
	// OnTool receives a tool-call's title and status for surfacing activity.
	OnTool func(title, status string)
	// ReadFile serves a file the agent requests (absolute path); the caller
	// path-jails it to the workspace.
	ReadFile func(path string) (string, error)
	// WriteFile is called when the agent wants to write a file. Return nil to let
	// the agent proceed (the caller decides when/whether it reaches disk — e.g. by
	// queuing a proposal); return an error to reject the write.
	WriteFile func(path, content string) error
}

// Session is a launched ACP agent connected for one or more prompt turns.
type Session struct {
	cmd  *exec.Cmd
	conn *acp.ClientSideConnection
	id   acp.SessionId
}

// Launch starts the agent identified by agentID as a subprocess, connects over
// ACP, negotiates capabilities, and opens a session rooted at cwd (the
// workspace). Close the returned Session to stop the agent.
func Launch(ctx context.Context, agentID, cwd string, cb Callbacks) (*Session, error) {
	var ag *Agent
	for i := range knownAgents {
		if knownAgents[i].ID == agentID {
			ag = &knownAgents[i]
			break
		}
	}
	if ag == nil {
		return nil, fmt.Errorf("unknown ACP agent %q", agentID)
	}

	cmd := exec.CommandContext(ctx, ag.command, ag.args...)
	cmd.Dir = cwd
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch %s (%s): %w", ag.Label, ag.command, err)
	}

	conn := acp.NewClientSideConnection(&client{cb: cb}, stdin, stdout)
	kill := func(e error) (*Session, error) {
		_ = cmd.Process.Kill()
		return nil, e
	}
	if _, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{ReadTextFile: true, WriteTextFile: true},
		},
	}); err != nil {
		return kill(fmt.Errorf("ACP initialize: %w", err))
	}
	ns, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		return kill(fmt.Errorf("ACP new session: %w", err))
	}
	return &Session{cmd: cmd, conn: conn, id: ns.SessionId}, nil
}

// Prompt runs one turn with the given user text, blocking until the agent
// finishes; streaming arrives via the Callbacks. It returns the stop reason.
func (s *Session) Prompt(ctx context.Context, text string) (string, error) {
	resp, err := s.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: s.id,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	})
	if err != nil {
		return "", err
	}
	return string(resp.StopReason), nil
}

// Close stops the agent subprocess.
func (s *Session) Close() {
	if s != nil && s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// client implements acp.Client, delegating to the caller's Callbacks.
type client struct{ cb Callbacks }

var _ acp.Client = (*client)(nil)

func (c *client) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
	u := params.Update
	switch {
	case u.AgentMessageChunk != nil:
		if t := u.AgentMessageChunk.Content.Text; t != nil && c.cb.OnText != nil {
			c.cb.OnText(t.Text)
		}
	case u.ToolCall != nil:
		if c.cb.OnTool != nil {
			c.cb.OnTool(u.ToolCall.Title, string(u.ToolCall.Status))
		}
	case u.ToolCallUpdate != nil:
		if c.cb.OnTool != nil && u.ToolCallUpdate.Title != nil {
			status := ""
			if u.ToolCallUpdate.Status != nil {
				status = string(*u.ToolCallUpdate.Status)
			}
			c.cb.OnTool(*u.ToolCallUpdate.Title, status)
		}
	}
	return nil
}

func (c *client) ReadTextFile(_ context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if c.cb.ReadFile == nil {
		return acp.ReadTextFileResponse{}, fmt.Errorf("file reads are not available")
	}
	content, err := c.cb.ReadFile(params.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	return acp.ReadTextFileResponse{Content: content}, nil
}

func (c *client) WriteTextFile(_ context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if c.cb.WriteFile == nil {
		return acp.WriteTextFileResponse{}, fmt.Errorf("file writes are not available")
	}
	if err := c.cb.WriteFile(params.Path, params.Content); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

// RequestPermission auto-allows: manuscript writes are gated separately through
// the WriteFile callback (the author's approval), so we let the agent proceed.
func (c *client) RequestPermission(_ context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, o := range params.Options {
		if o.Kind == acp.PermissionOptionKindAllowOnce || o.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId},
			}}, nil
		}
	}
	return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{
		Cancelled: &acp.RequestPermissionOutcomeCancelled{},
	}}, nil
}

// Terminal capability is not declared, so these should not be called; stub them.
func (c *client) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("terminal not supported")
}
func (c *client) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("terminal not supported")
}
func (c *client) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("terminal not supported")
}
func (c *client) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("terminal not supported")
}
func (c *client) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("terminal not supported")
}
