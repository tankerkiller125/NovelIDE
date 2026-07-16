package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/functool"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"novelide/internal/ai"
	"novelide/internal/history"
	"novelide/internal/model"
	"novelide/internal/workspace"
)

// proposal is an AI-suggested edit held server-side until the author approves
// it. The model queues these via the propose_* tools (planning mode only); it
// never writes to disk directly. The apply payload is kind-specific.
type proposal struct {
	ID      string
	Kind    string // "codex" | "plan" | "prose" | "file"
	Summary string
	Target  string // human label: entry name, or "Book › chapter"
	Before  string // preview: prior text (prose/plan/file) — for the diff view
	After   string // preview: proposed text

	// codex: the fully-merged entry plus its prior identity (for relocation).
	entry    *model.CodexEntry
	oldType  string
	oldScope string

	// plan: one chapter card to merge into its book's plan.
	bookID string
	card   model.ChapterPlan

	// prose: an exact-match find/replace within a chapter.
	chapter    string
	find, repl string

	// file: an ACP agent's whole-file write (absolute path, jailed to workspace).
	path string
}

// proposalView is the UI-safe projection sent to the frontend (no apply guts).
// For prose, BookID/Chapter and Before(=find)/After(=replace) let the editor
// anchor and render the edit inline.
type proposalView struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Target  string `json:"target"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	BookID  string `json:"bookId,omitempty"`
	Chapter string `json:"chapter,omitempty"`
}

func (p *proposal) view() proposalView {
	return proposalView{p.ID, p.Kind, p.Summary, p.Target, p.Before, p.After, p.bookID, p.chapter}
}

// Write-tool argument shapes. Optional fields use pointers/omitempty so that, on
// re-marshal, fields the model didn't set are omitted — preserving the
// "only the fields you set change" merge semantics in the build* functions.
type (
	proseEditArgs struct {
		BookID  string `json:"bookId"`
		Chapter string `json:"chapter"`
		Find    string `json:"find"`
		Replace string `json:"replace"`
	}
	codexEditArgs struct {
		ID      string            `json:"id,omitempty"`
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		Summary *string           `json:"summary,omitempty"`
		Details *string           `json:"details,omitempty"`
		Aliases []string          `json:"aliases,omitempty"`
		Fields  map[string]string `json:"fields,omitempty"`
	}
	planEditArgs struct {
		BookID   string   `json:"bookId"`
		Chapter  string   `json:"chapter"`
		Synopsis *string  `json:"synopsis,omitempty"`
		Status   *string  `json:"status,omitempty"`
		POV      *string  `json:"pov,omitempty"`
		Location *string  `json:"location,omitempty"`
		When     *string  `json:"when,omitempty"`
		Arcs     []string `json:"arcs,omitempty"`
	}
)

// writeTools are the propose_* tools for a mode. Their handlers queue an edit for
// the author to approve (they never touch files directly), tagged with streamID
// so a queued proposal surfaces live. The writing assistant gets prose edits;
// planning also gets Codex and plan structure edits.
func (a *App) writeTools(streamID, mode string) []tool.Tool {
	propose := func(name string, in any) (string, error) {
		raw, _ := json.Marshal(in)
		res := a.proposeEdit(streamID, ai.ToolCall{Name: name, Arguments: string(raw)})
		aiDebugf("stream=%s exec %s -> %.200s", streamID, name, res)
		return res, nil
	}
	prose := functool.MustNew(functool.Config{
		Name: "propose_prose_edit",
		Description: "Propose a precise edit to a chapter's prose for the author to approve — prefer this over pasting a large rewritten passage. " +
			"'find' must be an exact, unique substring of the current chapter text; it will be replaced with 'replace'. " +
			"Keep 'find' short but unique. The current chapter's bookId and chapter are given in the context; use read_chapter to copy the exact text.",
	}, func(_ context.Context, in proseEditArgs) (string, error) { return propose("propose_prose_edit", in) })

	if mode != "planning" {
		return []tool.Tool{prose} // writing assistant: prose edits only
	}

	codex := functool.MustNew(functool.Config{
		Name: "propose_codex_edit",
		Description: "Propose creating or updating a Codex entry for the author to approve. " +
			"Omit id to create a new entry; pass an existing id to update it (only the fields you set change; the rest are kept).",
	}, func(_ context.Context, in codexEditArgs) (string, error) { return propose("propose_codex_edit", in) })

	plan := functool.MustNew(functool.Config{
		Name: "propose_plan_edit",
		Description: "Propose changes to a chapter's planning card (synopsis, status, pov, location, when, arcs) for the author to approve. " +
			"Only the fields you set change.",
	}, func(_ context.Context, in planEditArgs) (string, error) { return propose("propose_plan_edit", in) })

	return []tool.Tool{codex, plan, prose}
}

// proposeEdit builds a proposal from a write tool call, stores it, emits it to
// the UI, and returns a confirmation for the model. Validation failures come
// back as text so the model can correct itself.
func (a *App) proposeEdit(streamID string, call ai.ToolCall) string {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return "error: no workspace is open"
	}
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Arguments), &args)
	str := func(k string) string {
		s, _ := args[k].(string)
		return strings.TrimSpace(s)
	}
	strList := func(k string) []string {
		raw, _ := args[k].([]any)
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}

	var p *proposal
	var errText string
	switch call.Name {
	case "propose_codex_edit":
		p, errText = buildCodexProposal(ws, args, str, strList)
	case "propose_plan_edit":
		p, errText = buildPlanProposal(ws, args, str, strList)
	case "propose_prose_edit":
		p, errText = buildProseProposal(ws, str)
	}
	if errText != "" {
		return "error: " + errText
	}

	a.aiMu.Lock()
	if a.aiProposals == nil {
		a.aiProposals = map[string]*proposal{}
	}
	p.ID = fmt.Sprintf("%s-%d", streamID, len(a.aiProposals)+1)
	a.aiProposals[p.ID] = p
	a.aiMu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "ai:proposal", p.view())
	}
	return "Proposal queued for the author to review and approve: " + p.Summary
}

func buildCodexProposal(ws *model.Workspace, args map[string]any, str func(string) string, strList func(string) []string) (*proposal, string) {
	name, typ := str("name"), str("type")
	if name == "" || typ == "" {
		return nil, "name and type are required"
	}
	id := str("id")
	var entry model.CodexEntry
	var oldType, oldScope string
	if id != "" {
		found := false
		for i := range ws.Codex {
			if ws.Codex[i].ID == id {
				entry = ws.Codex[i] // start from the existing entry; keep relations/timelines/status
				oldType, oldScope = entry.Type, entry.Scope
				found = true
				break
			}
		}
		if !found {
			return nil, "no Codex entry with id " + id + " (omit id to create a new entry)"
		}
	} else {
		entry = model.CodexEntry{ID: workspace.Slugify(name), Scope: "series"}
	}
	entry.Name = name
	entry.Type = typ
	if _, ok := args["summary"]; ok {
		entry.Summary = str("summary")
	}
	if _, ok := args["details"]; ok {
		entry.Details = str("details")
	}
	if _, ok := args["aliases"]; ok {
		entry.Aliases = strList("aliases")
	}
	if raw, ok := args["fields"].(map[string]any); ok {
		fields := map[string]string{}
		for k, v := range raw {
			if s, ok := v.(string); ok {
				fields[k] = s
			}
		}
		entry.Fields = fields
	}

	verb := "Update"
	if id == "" {
		verb = "Create"
	}
	return &proposal{
		Kind:     "codex",
		Summary:  fmt.Sprintf("%s codex entry “%s” (%s)", verb, entry.Name, entry.Type),
		Target:   entry.Name,
		After:    codexPreview(&entry),
		entry:    &entry,
		oldType:  oldType,
		oldScope: oldScope,
	}, ""
}

func codexPreview(e *model.CodexEntry) string {
	var b strings.Builder
	if e.Summary != "" {
		b.WriteString(e.Summary + "\n")
	}
	if len(e.Aliases) > 0 {
		b.WriteString("aka: " + strings.Join(e.Aliases, ", ") + "\n")
	}
	if e.Details != "" {
		b.WriteString("\n" + e.Details)
	}
	return strings.TrimSpace(b.String())
}

func buildPlanProposal(ws *model.Workspace, args map[string]any, str func(string) string, strList func(string) []string) (*proposal, string) {
	bookID, chapter := str("bookId"), str("chapter")
	if bookID == "" || chapter == "" {
		return nil, "bookId and chapter are required"
	}
	var book *model.Book
	for i := range ws.Books {
		if ws.Books[i].ID == bookID {
			book = &ws.Books[i]
			break
		}
	}
	if book == nil {
		return nil, "no book with id " + bookID
	}
	found := false
	for _, ch := range book.Chapters {
		if ch == chapter {
			found = true
			break
		}
	}
	if !found {
		return nil, "book " + bookID + " has no chapter " + chapter
	}

	card := model.ChapterPlan{File: chapter}
	before := ""
	for _, pc := range book.Plan {
		if pc.File == chapter {
			card = pc // start from the existing card
			before = planPreview(pc)
			break
		}
	}
	for _, k := range []string{"synopsis", "status", "pov", "location", "when"} {
		if _, ok := args[k]; !ok {
			continue
		}
		switch k {
		case "synopsis":
			card.Synopsis = str(k)
		case "status":
			card.Status = str(k)
		case "pov":
			card.POV = str(k)
		case "location":
			card.Location = str(k)
		case "when":
			card.When = str(k)
		}
	}
	if _, ok := args["arcs"]; ok {
		card.Arcs = strList("arcs")
	}

	return &proposal{
		Kind:    "plan",
		Summary: fmt.Sprintf("Update plan for %s › %s", book.Title, chapter),
		Target:  book.Title + " › " + chapter,
		Before:  before,
		After:   planPreview(card),
		bookID:  bookID,
		card:    card,
	}, ""
}

func planPreview(c model.ChapterPlan) string {
	var parts []string
	if c.Synopsis != "" {
		parts = append(parts, c.Synopsis)
	}
	kv := func(k, v string) {
		if v != "" {
			parts = append(parts, k+": "+v)
		}
	}
	kv("status", c.Status)
	kv("pov", c.POV)
	kv("location", c.Location)
	kv("when", c.When)
	if len(c.Arcs) > 0 {
		parts = append(parts, "arcs: "+strings.Join(c.Arcs, ", "))
	}
	return strings.Join(parts, "\n")
}

func buildProseProposal(ws *model.Workspace, str func(string) string) (*proposal, string) {
	bookID, chapter := str("bookId"), str("chapter")
	find, repl := str("find"), str("replace")
	if bookID == "" || chapter == "" || find == "" {
		return nil, "bookId, chapter, and find are required"
	}
	text, err := workspace.ReadChapter(ws.Path, bookID, chapter)
	if err != nil {
		return nil, "could not read chapter (" + err.Error() + ")"
	}
	switch strings.Count(text, find) {
	case 0:
		return nil, "the 'find' text does not appear in the chapter — copy it exactly from read_chapter"
	case 1:
	default:
		return nil, "the 'find' text appears more than once — include more surrounding text to make it unique"
	}
	title := workspace.ChapterTitle(text, chapter)
	return &proposal{
		Kind:    "prose",
		Summary: fmt.Sprintf("Edit prose in %s", title),
		Target:  title,
		Before:  find,
		After:   repl,
		bookID:  bookID,
		chapter: chapter,
		find:    find,
		repl:    repl,
	}, ""
}

// ListAIProposals returns the pending proposals (e.g. to repopulate the panel).
func (a *App) ListAIProposals() []proposalView {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	out := make([]proposalView, 0, len(a.aiProposals))
	for _, p := range a.aiProposals {
		out = append(out, p.view())
	}
	return out
}

// AIDiscardProposal drops a pending proposal without applying it.
func (a *App) AIDiscardProposal(id string) {
	a.aiMu.Lock()
	delete(a.aiProposals, id)
	a.aiMu.Unlock()
}

// AIApplyProposal applies a pending proposal to disk and returns the refreshed
// workspace. The proposal is consumed whether it succeeds or fails validation.
func (a *App) AIApplyProposal(id string) (*model.Workspace, error) {
	a.aiMu.Lock()
	p := a.aiProposals[id]
	delete(a.aiProposals, id)
	a.aiMu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("proposal is no longer available")
	}

	switch p.Kind {
	case "codex":
		return a.SaveCodexEntry(*p.entry, p.oldType, p.oldScope)
	case "plan":
		return a.applyPlanProposal(p)
	case "prose":
		return a.applyProseProposal(p)
	case "file":
		return a.applyFileProposal(p)
	default:
		return nil, fmt.Errorf("unknown proposal kind %q", p.Kind)
	}
}

// applyFileProposal writes an ACP agent's whole-file edit to disk after the
// author approves it, taking a safety snapshot first. The path was jailed to the
// workspace when the proposal was created; re-verify before writing.
func (a *App) applyFileProposal(p *proposal) (*model.Workspace, error) {
	root, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	abs, err := jailPath(root, p.path)
	if err != nil {
		return nil, err
	}
	_, _, _ = history.Create(root, "before AI file edit", false, time.Now())
	if err := os.WriteFile(abs, []byte(p.After), 0o644); err != nil {
		return nil, err
	}
	return a.reload()
}

func (a *App) applyPlanProposal(p *proposal) (*model.Workspace, error) {
	a.mu.RLock()
	ws := a.ws
	a.mu.RUnlock()
	if ws == nil {
		return nil, fmt.Errorf("no workspace is open")
	}
	var plan []model.ChapterPlan
	for i := range ws.Books {
		if ws.Books[i].ID == p.bookID {
			plan = append([]model.ChapterPlan(nil), ws.Books[i].Plan...)
			break
		}
	}
	replaced := false
	for i := range plan {
		if plan[i].File == p.card.File {
			plan[i] = p.card
			replaced = true
			break
		}
	}
	if !replaced {
		plan = append(plan, p.card)
	}
	return a.SavePlan(p.bookID, plan)
}

func (a *App) applyProseProposal(p *proposal) (*model.Workspace, error) {
	path, err := a.workspacePath()
	if err != nil {
		return nil, err
	}
	text, err := workspace.ReadChapter(path, p.bookID, p.chapter)
	if err != nil {
		return nil, err
	}
	// Re-validate against the current text — the chapter may have changed since
	// the proposal was made.
	if strings.Count(text, p.find) != 1 {
		return nil, fmt.Errorf("the chapter changed and the target text is no longer uniquely present — discard and re-propose")
	}
	// Safety snapshot before mutating prose, so the edit is always reversible.
	_, _, _ = history.Create(path, "before AI prose edit", false, time.Now())
	if err := a.SaveChapter(p.bookID, p.chapter, strings.Replace(text, p.find, p.repl, 1)); err != nil {
		return nil, err
	}
	return a.reload()
}
