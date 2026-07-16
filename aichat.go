package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/message"
	"github.com/microsoft/agent-framework-go/tool"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"novelide/internal/ai"
	"novelide/internal/model"
	"novelide/internal/workspace"
)

// aiDebug enables verbose AI-stream logging to stderr when NOVELIDE_AI_DEBUG is
// set (e.g. in the GoLand run configuration's environment). It surfaces whether
// the backend is producing text/tool events, which bisects backend-vs-frontend
// problems without a debugger.
var aiDebug = os.Getenv("NOVELIDE_AI_DEBUG") != ""

func aiDebugf(format string, args ...any) {
	if aiDebug {
		log.Printf("[ai] "+format, args...)
	}
}

// System prompts are STATIC per mode so they cache well. The volatile world
// bible is appended below (still part of the cached prefix, but re-cached only
// when the Codex changes); the current chapter and conversation stay out of the
// prefix entirely.
const toolNote = `
You have tools to look things up — prefer them over guessing, and never invent world or plot facts:
- list_structure: the books and chapters (with plan synopsis/status) and series synopsis. Call it first to learn ids.
- search_codex / get_entry: find and read full Codex entries (details, fields, timelines, status, relationships).
- search_manuscript / read_chapter: search and read the actual prose (the manuscript is NOT all in your context).
The compact world bible below is always available; use get_entry for fuller detail and the manuscript tools for the prose.`

const assistantSystem = `You are a writing assistant embedded in NovelIDE, helping the author write their novel.
Be concise and concrete. Match the manuscript's voice when drafting or revising prose; prefer showing over telling.
Answer questions about the story world strictly from the Codex and manuscript — if a fact isn't there, say so rather than inventing it.
When you suggest a concrete change to the prose, propose it with propose_prose_edit (an exact find/replace the author can Apply) instead of pasting a large rewritten passage; keep your explanation short. Make one focused proposal per distinct change. Only paste prose verbatim when the author explicitly asks to see it.
When unsure, ask a brief clarifying question.` + toolNote

const planningSystem = `You are a story-planning assistant for NovelIDE, helping the author outline and structure their novel and reason about plot, characters, arcs, and consistency.
Ground everything in the Codex and manuscript; do not invent world facts. Give concrete, structured suggestions the author can act on.
You may propose edits with propose_codex_edit, propose_plan_edit, and propose_prose_edit — these queue changes for the author to approve, they do not apply immediately. Propose concrete edits when they clearly help; still explain your reasoning briefly. For prose edits, copy the exact text to change from read_chapter.` + toolNote

func systemPrompt(mode string) string {
	if mode == "planning" {
		return planningSystem
	}
	return assistantSystem
}

// codexBible renders the workspace Codex as a compact, deterministically-ordered
// world bible for grounding. Deterministic order matters: an identical string
// across turns is what keeps the prefix cache warm. Truncated to maxChars.
func codexBible(ws *model.Workspace, maxChars int) string {
	if ws == nil || len(ws.Codex) == 0 {
		return ""
	}
	typeLabel := map[string]string{}
	for _, t := range ws.Schema.Types {
		typeLabel[t.ID] = t.Label
	}
	entries := append([]model.CodexEntry(nil), ws.Codex...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	var b strings.Builder
	b.WriteString("\n\n# World bible (Codex)\nUse ONLY these facts about the story world; do not invent world details.\n")
	for _, e := range entries {
		label := typeLabel[e.Type]
		if label == "" {
			label = e.Type
		}
		fmt.Fprintf(&b, "\n### %s (%s)", e.Name, label)
		if len(e.Aliases) > 0 {
			fmt.Fprintf(&b, " — aka %s", strings.Join(e.Aliases, ", "))
		}
		b.WriteByte('\n')
		if e.Summary != "" {
			b.WriteString(e.Summary + "\n")
		}
		if len(e.Fields) > 0 {
			keys := make([]string, 0, len(e.Fields))
			for k := range e.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, k+": "+e.Fields[k])
			}
			b.WriteString(strings.Join(parts, "; ") + "\n")
		}
	}
	s := b.String()
	if maxChars > 0 && len([]rune(s)) > maxChars {
		r := []rune(s)
		s = string(r[:maxChars]) + "\n…(world bible truncated)"
	}
	return s
}

// chapterContext returns the current chapter's text for grounding (volatile —
// not part of the cached prefix). Truncated to maxChars.
func chapterContext(ws *model.Workspace, bookID, chapter string, maxChars int) string {
	if ws == nil || bookID == "" || chapter == "" {
		return ""
	}
	text, err := workspace.ReadChapter(ws.Path, bookID, chapter)
	if err != nil {
		return ""
	}
	title := workspace.ChapterTitle(text, chapter)
	truncated := false
	if maxChars > 0 && len([]rune(text)) > maxChars {
		text = string([]rune(text)[:maxChars]) + "\n…(chapter truncated for context)"
		truncated = true
	}
	// Tell the model it already has this chapter, so it doesn't waste a tool
	// round-trip re-fetching it with read_chapter/search_manuscript.
	note := "You already have its full text here — do NOT call read_chapter or search_manuscript for THIS chapter; use those tools only for other chapters"
	if truncated {
		note = fmt.Sprintf("Only the first %d characters are shown below; to read the rest of THIS chapter, call read_chapter with offset=%d and keep paging with nextOffset", maxChars, maxChars)
	}
	return fmt.Sprintf("[The current chapter \"%s\" is included below (bookId: %s, chapter: %s — use these for propose_prose_edit). %s.]\n%s\n\n[The author's message follows.]\n\n",
		title, bookID, chapter, note, text)
}

func (a *App) registerStream(id string, cancel context.CancelFunc) {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	if a.aiStreams == nil {
		a.aiStreams = map[string]context.CancelFunc{}
	}
	if old := a.aiStreams[id]; old != nil {
		old() // supersede any prior stream with this id
	}
	a.aiStreams[id] = cancel
}

func (a *App) unregisterStream(id string) {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	delete(a.aiStreams, id)
}

// AICancel stops an in-flight AI stream.
func (a *App) AICancel(streamID string) {
	a.aiMu.Lock()
	cancel := a.aiStreams[streamID]
	a.aiMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// AIChat runs one grounded chat turn for the given mode, streaming text to the
// frontend via "ai:delta" events (payload {id, text}) and finishing with
// "ai:done" ({id, stopReason}) or "ai:error" ({id, error}). history holds the
// prior user/assistant turns (no injected context); bookID/chapter name the
// chapter to ground on.
func (a *App) AIChat(streamID, mode string, history []ai.Message, bookID, chapter, providerID, model string) error {
	a.mu.RLock()
	cfg := a.settings.AI
	ws := a.ws
	a.mu.RUnlock()

	if !cfg.Enabled {
		return a.aiFail(streamID, fmt.Errorf("AI is turned off — enable it in Settings"))
	}
	// The model to run is chosen in the chat picker (providerID + model); the mode
	// only selects the system prompt and tool set below.
	provider, err := cfg.ResolveModel(providerID, model)
	if err != nil {
		return a.aiFail(streamID, err)
	}

	// A locally-installed ACP coding agent takes a different path: it's launched
	// as a subprocess and reads the manuscript itself.
	if provider.Kind == ai.KindACP {
		return a.runACP(streamID, provider, ws, history, bookID, chapter)
	}

	window := ai.DefaultContextTokens
	// System instructions: static per mode + the deterministic Codex world bible.
	instructions := systemPrompt(mode) + codexBible(ws, window*2)

	// Read tools in both modes; write (propose_*) tools per mode — the assistant
	// proposes prose edits, planning also proposes Codex/plan edits. An A2A agent
	// runs remotely with its own tools, so none of ours apply.
	var tools []tool.Tool
	if provider.Kind != ai.KindA2A {
		tools = a.readTools()
		tools = append(tools, a.writeTools(streamID, mode)...)
	}

	base := a.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	a.registerStream(streamID, cancel)
	defer a.unregisterStream(streamID)
	defer cancel()

	ag, err := ai.NewAgent(ctx, provider, instructions, tools)
	if err != nil {
		return a.aiFail(streamID, err)
	}
	msgs := mafMessages(ws, history, bookID, chapter, window*2)
	aiDebugf("start stream=%s mode=%s provider=%s model=%s tools=%d msgs=%d",
		streamID, mode, provider.Kind, provider.Model, len(tools), len(msgs))

	// The Agent Framework runs the whole tool-use loop internally; we just stream
	// its updates to the frontend. Text deltas and tool calls are Content items.
	var deltaCount, deltaChars int
	var stopReason string
	for update, err := range ag.Run(ctx, msgs, agent.Stream(true)) {
		if err != nil {
			aiDebugf("stream=%s ERROR after %d deltas: %v", streamID, deltaCount, err)
			return a.aiFail(streamID, err)
		}
		for _, c := range update.Contents {
			switch cc := c.(type) {
			case *message.TextContent:
				if cc.Text == "" {
					continue
				}
				deltaCount++
				deltaChars += len(cc.Text)
				runtime.EventsEmit(a.ctx, "ai:delta", map[string]any{"id": streamID, "text": cc.Text})
			case *message.FunctionCallContent:
				aiDebugf("stream=%s tool-call %s args=%s", streamID, cc.Name, cc.Arguments)
				runtime.EventsEmit(a.ctx, "ai:tool", map[string]any{"id": streamID, "name": cc.Name, "args": cc.Arguments})
			}
		}
		if update.FinishReason != "" {
			stopReason = update.FinishReason
		}
	}
	aiDebugf("stream=%s done: %d deltas (%d chars) stop=%q", streamID, deltaCount, deltaChars, stopReason)
	runtime.EventsEmit(a.ctx, "ai:done", map[string]any{"id": streamID, "stopReason": stopReason})
	return nil
}

// mafMessages converts the frontend conversation history into Agent Framework
// messages, prepending the current chapter's text to the most recent user turn.
func mafMessages(ws *model.Workspace, history []ai.Message, bookID, chapter string, maxChars int) []*message.Message {
	ctxBlock := chapterContext(ws, bookID, chapter, maxChars)
	lastUser := -1
	for i, m := range history {
		if m.Role == ai.RoleUser {
			lastUser = i
		}
	}
	msgs := make([]*message.Message, 0, len(history))
	for i, m := range history {
		content := m.Content
		if i == lastUser && ctxBlock != "" {
			content = ctxBlock + content
		}
		role := message.RoleUser
		if m.Role == ai.RoleAssistant {
			role = message.RoleAssistant
		}
		msgs = append(msgs, &message.Message{
			Role:     role,
			Contents: message.Contents{&message.TextContent{Text: content}},
		})
	}
	return msgs
}

func (a *App) aiFail(streamID string, err error) error {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "ai:error", map[string]any{"id": streamID, "error": err.Error()})
	}
	return err
}
