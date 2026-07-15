package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	Kind    string // "codex" | "plan" | "prose"
	Summary string
	Target  string // human label: entry name, or "Book › chapter"
	Before  string // preview: prior text (prose/plan) — for the diff view
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

// writeTools are the propose_* tools for a mode. They queue an edit for the
// author to approve rather than applying it, so they are safe to auto-run; their
// definitions are static, keeping them in the cached prefix. The writing
// assistant gets prose edits (propose a concrete change instead of pasting a big
// rewrite); planning also gets Codex and plan structure edits.
func writeTools(mode string) []ai.Tool {
	obj := func(props, required string) json.RawMessage {
		return json.RawMessage(`{"type":"object","properties":{` + props + `}` + required + `}`)
	}
	prose := ai.Tool{
		Name: "propose_prose_edit",
		Description: "Propose a precise edit to a chapter's prose for the author to approve — prefer this over pasting a large rewritten passage. " +
			"'find' must be an exact, unique substring of the current chapter text; it will be replaced with 'replace'. " +
			"Keep 'find' short but unique. The current chapter's bookId and chapter are given in the context; use read_chapter to copy the exact text.",
		Schema: obj(`"bookId":{"type":"string"},"chapter":{"type":"string","description":"chapter file name"},`+
			`"find":{"type":"string"},"replace":{"type":"string"}`, `,"required":["bookId","chapter","find","replace"]`),
	}
	if mode != "planning" {
		return []ai.Tool{prose} // writing assistant: prose edits only
	}
	return []ai.Tool{
		{
			Name: "propose_codex_edit",
			Description: "Propose creating or updating a Codex entry for the author to approve. " +
				"Omit id to create a new entry; pass an existing id to update it (only the fields you set change; the rest are kept).",
			Schema: obj(`"id":{"type":"string","description":"existing entry id to update; omit to create"},`+
				`"name":{"type":"string"},"type":{"type":"string","description":"schema type id, e.g. character"},`+
				`"summary":{"type":"string"},"details":{"type":"string","description":"markdown body"},`+
				`"aliases":{"type":"array","items":{"type":"string"}},`+
				`"fields":{"type":"object","additionalProperties":{"type":"string"}}`, `,"required":["name","type"]`),
		},
		{
			Name: "propose_plan_edit",
			Description: "Propose changes to a chapter's planning card (synopsis, status, pov, location, when, arcs) for the author to approve. " +
				"Only the fields you set change.",
			Schema: obj(`"bookId":{"type":"string"},"chapter":{"type":"string","description":"chapter file name"},`+
				`"synopsis":{"type":"string"},"status":{"type":"string","enum":["outlined","drafted","revised","final"]},`+
				`"pov":{"type":"string","description":"codex entry id"},"location":{"type":"string","description":"codex entry id"},`+
				`"when":{"type":"string"},"arcs":{"type":"array","items":{"type":"string"}}`, `,"required":["bookId","chapter"]`),
		},
		prose,
	}
}

// writeToolAllowed reports whether a write tool may run in the given mode. Prose
// edits work in both modes; Codex/plan structure edits are planning-only.
func writeToolAllowed(mode, name string) bool {
	switch name {
	case "propose_prose_edit":
		return true
	case "propose_codex_edit", "propose_plan_edit":
		return mode == "planning"
	}
	return false
}

// toolExecutor returns the exec callback for one chat turn. Read tools always
// run; write tools queue a proposal tagged with this stream, gated to the modes
// that offer them. Binding streamID here lets a queued proposal surface live.
func (a *App) toolExecutor(streamID, mode string) func(ai.ToolCall) string {
	return func(call ai.ToolCall) string {
		switch call.Name {
		case "propose_codex_edit", "propose_plan_edit", "propose_prose_edit":
			if !writeToolAllowed(mode, call.Name) {
				return "error: the " + call.Name + " tool isn't available in this mode"
			}
			return a.proposeEdit(streamID, call)
		default:
			return a.execTool(call)
		}
	}
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
	default:
		return nil, fmt.Errorf("unknown proposal kind %q", p.Kind)
	}
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
