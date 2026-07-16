package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"novelide/internal/acp"
	"novelide/internal/ai"
	"novelide/internal/model"
)

// DetectACPAgents lists locally-installed ACP coding agents (for the Settings
// provider picker).
func (a *App) DetectACPAgents() []acp.Agent {
	return acp.Detect()
}

// runACP drives a locally-installed ACP coding agent for one turn: it launches
// the agent rooted at the workspace, sends the author's latest message, streams
// the reply to the frontend, serves file reads, and turns file writes into
// proposals the author approves. A fresh agent session is used per turn.
func (a *App) runACP(streamID string, provider ai.Provider, ws *model.Workspace, history []ai.Message, bookID, chapter string) error {
	if ws == nil {
		return a.aiFail(streamID, fmt.Errorf("open a workspace first"))
	}
	// The agent maintains its own context and reads the manuscript, so we send the
	// latest user message (prior chat turns aren't replayed into the agent).
	prompt := ""
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == ai.RoleUser {
			prompt = history[i].Content
			break
		}
	}
	if strings.TrimSpace(prompt) == "" {
		return a.aiFail(streamID, fmt.Errorf("no message to send"))
	}
	grounding := "You are a writing assistant working on the novel manuscript in this folder: " +
		"chapters are plain Markdown under books/*/manuscript/, and the world bible is YAML under codex/. " +
		"Read files as needed. When you change prose, edit the relevant chapter file."
	if chapter != "" {
		grounding += fmt.Sprintf(" The author is currently editing books/%s/manuscript/%s.", bookID, chapter)
	}
	fullPrompt := grounding + "\n\nAuthor's request:\n" + prompt

	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	a.registerStream(streamID, cancel)
	defer a.unregisterStream(streamID)
	defer cancel()

	// overlay holds the agent's not-yet-approved writes so its own reads stay
	// consistent within the turn, while disk changes only on approval.
	var mu sync.Mutex
	overlay := map[string]string{}

	cb := acp.Callbacks{
		OnText: func(t string) {
			runtime.EventsEmit(a.ctx, "ai:delta", map[string]any{"id": streamID, "text": t})
		},
		OnTool: func(title, status string) {
			aiDebugf("stream=%s acp-tool %s (%s)", streamID, title, status)
			runtime.EventsEmit(a.ctx, "ai:tool", map[string]any{"id": streamID, "name": title, "args": status})
		},
		ReadFile: func(path string) (string, error) {
			abs, err := jailPath(ws.Path, path)
			if err != nil {
				return "", err
			}
			mu.Lock()
			v, ok := overlay[abs]
			mu.Unlock()
			if ok {
				return v, nil
			}
			b, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		WriteFile: func(path, content string) error {
			abs, err := jailPath(ws.Path, path)
			if err != nil {
				return err
			}
			mu.Lock()
			overlay[abs] = content
			mu.Unlock()
			a.proposeFileWrite(streamID, ws.Path, abs, content)
			return nil // accept: the change reaches disk only when the author approves the proposal
		},
	}

	aiDebugf("start stream=%s ACP agent=%s cwd=%s", streamID, provider.BaseURL, ws.Path)
	sess, err := acp.Launch(ctx, provider.BaseURL, ws.Path, cb)
	if err != nil {
		return a.aiFail(streamID, err)
	}
	defer sess.Close()

	stop, err := sess.Prompt(ctx, fullPrompt)
	if err != nil {
		aiDebugf("stream=%s ACP error: %v", streamID, err)
		return a.aiFail(streamID, err)
	}
	aiDebugf("stream=%s ACP done stop=%q", streamID, stop)
	runtime.EventsEmit(a.ctx, "ai:done", map[string]any{"id": streamID, "stopReason": stop})
	return nil
}

// proposeFileWrite queues an agent's whole-file edit as a proposal for approval.
func (a *App) proposeFileWrite(streamID, root, absPath, content string) {
	old := ""
	if b, err := os.ReadFile(absPath); err == nil {
		old = string(b)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		rel = absPath
	}
	p := &proposal{
		Kind:    "file",
		Summary: "Agent edit to " + rel,
		Target:  rel,
		Before:  old,
		After:   content,
		path:    absPath,
	}
	a.aiMu.Lock()
	if a.aiProposals == nil {
		a.aiProposals = map[string]*proposal{}
	}
	p.ID = fmt.Sprintf("%s-file-%d", streamID, len(a.aiProposals)+1)
	a.aiProposals[p.ID] = p
	a.aiMu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "ai:proposal", p.view())
	}
}

// jailPath resolves p and confirms it stays inside root, returning the absolute
// path. It blocks an agent from reading or writing outside the workspace.
func jailPath(root, p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace: %s", p)
	}
	return abs, nil
}
