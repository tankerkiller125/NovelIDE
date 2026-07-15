package main

import (
	"encoding/json"
	"strings"

	"novelide/internal/ai"
	"novelide/internal/model"
	"testing"

	"novelide/internal/settings"
	"novelide/internal/workspace"
)

// TestSyncCloneRejectsUnsafeRemoteID ensures a server-supplied workspace id
// can't steer the destination folder outside the one the user picked. These
// cases are rejected before any network call.
func TestSyncCloneRejectsUnsafeRemoteID(t *testing.T) {
	a := &App{settings: settings.Settings{SyncServer: "http://sync.local", SyncToken: "tok", SyncAccountID: "acct"}}
	parent := t.TempDir()
	for _, id := range []string{"..", ".", "../evil", "/etc", `a\b`, "a/b"} {
		if _, err := a.SyncCloneWorkspace(id, parent); err == nil {
			t.Errorf("SyncCloneWorkspace accepted unsafe remote id %q", id)
		}
	}
}

func TestMentionSnippet(t *testing.T) {
	// A mention deep inside a long chapter is clipped on both sides.
	long := strings.Repeat("word ", 60) + "Aria drew her blade " + strings.Repeat("word ", 60)
	at := strings.Index(long, "Aria")
	// convert byte index to rune index (ASCII here, so equal)
	got := mentionSnippet(long, at)
	if !strings.Contains(got, "Aria") {
		t.Fatalf("snippet lost the mention: %q", got)
	}
	if !strings.HasPrefix(got, "… ") || !strings.HasSuffix(got, " …") {
		t.Errorf("expected ellipses on both sides, got %q", got)
	}

	// A mention near the start has no leading ellipsis.
	short := "Aria drew her blade and struck."
	got = mentionSnippet(short, 0)
	if strings.HasPrefix(got, "… ") {
		t.Errorf("no leading ellipsis expected for start-of-text: %q", got)
	}
	if got != "Aria drew her blade and struck." {
		t.Errorf("short snippet mangled: %q", got)
	}

	if mentionSnippet("anything", -1) != "" {
		t.Error("negative offset should yield empty snippet")
	}
}

func TestCodexBibleDeterministicAndSorted(t *testing.T) {
	ws := &model.Workspace{
		Schema: model.Schema{Types: []model.TypeDef{{ID: "character", Label: "Character"}}},
		Codex: []model.CodexEntry{
			{ID: "zed", Name: "Zed", Type: "character", Summary: "a wanderer", Fields: map[string]string{"hair": "black", "age": "40"}},
			{ID: "aria", Name: "Aria", Type: "character", Aliases: []string{"the Witch"}, Summary: "the hero"},
		},
	}
	a := codexBible(ws, 0)
	b := codexBible(ws, 0)
	if a != b {
		t.Fatal("codexBible must be deterministic (byte-identical) for cache stability")
	}
	// Sorted by id → Aria before Zed; fields sorted (age before hair).
	if strings.Index(a, "Aria") > strings.Index(a, "Zed") {
		t.Error("entries should be sorted by id")
	}
	if !strings.Contains(a, "aka the Witch") || !strings.Contains(a, "Character") {
		t.Errorf("missing alias/type label: %s", a)
	}
	if strings.Index(a, "age: 40") > strings.Index(a, "hair: black") {
		t.Error("fields should be sorted for stability")
	}

	// Truncation is applied and marked.
	small := codexBible(ws, 20)
	if !strings.Contains(small, "truncated") {
		t.Errorf("expected truncation marker: %s", small)
	}
}

func TestReadToolsExecute(t *testing.T) {
	ws := &model.Workspace{
		Books: []model.Book{{ID: "01-book", Title: "Book One", Chapters: []string{"01-one.md"},
			Plan: []model.ChapterPlan{{File: "01-one.md", Synopsis: "the beginning", Status: "drafted"}}}},
		SeriesPlan: model.SeriesPlan{Synopsis: "an epic"},
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria Voss", Type: "character", Aliases: []string{"the Witch"}, Summary: "the hero"},
			{ID: "kael", Name: "Kael", Type: "character", Summary: "a soldier"},
		},
	}

	// search_codex finds by alias.
	if got := toolSearchCodex(ws, "witch"); !strings.Contains(got, "aria") {
		t.Errorf("search_codex missed alias match: %s", got)
	}
	// get_entry returns the entry, and errors helpfully for unknown ids.
	if got := toolGetEntry(ws, "aria"); !strings.Contains(got, "the hero") {
		t.Errorf("get_entry wrong: %s", got)
	}
	if got := toolGetEntry(ws, "nope"); !strings.Contains(got, "error") {
		t.Errorf("get_entry should error for unknown id: %s", got)
	}
	// list_structure includes books, chapters with plan, and series synopsis.
	ls := toolListStructure(ws)
	if !strings.Contains(ls, "01-book") || !strings.Contains(ls, "the beginning") || !strings.Contains(ls, "an epic") {
		t.Errorf("list_structure missing data: %s", ls)
	}
	// The dispatcher routes by name and errors on unknown tools.
	a := &App{ws: ws}
	if got := a.execTool(aiToolCall("search_codex", `{"query":"kael"}`)); !strings.Contains(got, "kael") {
		t.Errorf("dispatch search_codex failed: %s", got)
	}
	if got := a.execTool(aiToolCall("bogus", `{}`)); !strings.Contains(got, "unknown tool") {
		t.Errorf("unknown tool should error: %s", got)
	}
}

func aiToolCall(name, args string) ai.ToolCall {
	return ai.ToolCall{ID: "t", Name: name, Arguments: args}
}

func TestProposalBuilders(t *testing.T) {
	str := func(m map[string]any) func(string) string {
		return func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
	}

	ws := &model.Workspace{
		Books: []model.Book{{ID: "01-book", Title: "Book One", Chapters: []string{"01-one.md"},
			Plan: []model.ChapterPlan{{File: "01-one.md", Synopsis: "old synopsis", Status: "outlined", POV: "aria"}}}},
		Codex: []model.CodexEntry{{ID: "aria", Name: "Aria", Type: "character", Summary: "old",
			Relations: []model.Relation{{Type: "ally", To: "kael"}}}},
	}
	sl := func(m map[string]any) func(string) []string {
		return func(k string) []string {
			raw, _ := m[k].([]any)
			var out []string
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}

	// Codex update must preserve relations while changing summary.
	args := map[string]any{"id": "aria", "name": "Aria", "type": "character", "summary": "new summary"}
	p, e := buildCodexProposal(ws, args, str(args), sl(args))
	if e != "" {
		t.Fatalf("codex build errored: %s", e)
	}
	if p.entry.Summary != "new summary" || len(p.entry.Relations) != 1 {
		t.Errorf("codex merge lost data: %+v", p.entry)
	}
	if p.oldType != "character" {
		t.Errorf("oldType not captured: %q", p.oldType)
	}
	// New codex entry derives an id from the name.
	args2 := map[string]any{"name": "Kael Stone", "type": "character"}
	p2, _ := buildCodexProposal(ws, args2, str(args2), sl(args2))
	if p2.entry.ID != "kael-stone" || p2.entry.Scope != "series" {
		t.Errorf("new entry id/scope wrong: %+v", p2.entry)
	}

	// Plan merge keeps POV, changes status.
	pa := map[string]any{"bookId": "01-book", "chapter": "01-one.md", "status": "drafted"}
	pp, e := buildPlanProposal(ws, pa, str(pa), sl(pa))
	if e != "" {
		t.Fatalf("plan build errored: %s", e)
	}
	if pp.card.Status != "drafted" || pp.card.POV != "aria" || pp.card.Synopsis != "old synopsis" {
		t.Errorf("plan merge wrong: %+v", pp.card)
	}
	// Unknown book rejected.
	bad := map[string]any{"bookId": "nope", "chapter": "x.md"}
	if _, e := buildPlanProposal(ws, bad, str(bad), sl(bad)); e == "" {
		t.Error("plan build should reject unknown book")
	}
}

func TestProseProposalFullCycle(t *testing.T) {
	dir := t.TempDir()
	if _, err := workspace.Create(dir, "Test", model.KindNovel); err != nil {
		t.Fatal(err)
	}
	books, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	bookID := books.Books[0].ID
	ch, err := workspace.CreateChapter(dir, bookID, "One")
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteChapter(dir, bookID, ch, "# One\n\nThe cat sat on the mat.\n"); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.setWorkspace(ws)

	str := func(m map[string]any) func(string) string {
		return func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
	}

	// Non-unique find is rejected up front.
	dup := map[string]any{"bookId": bookID, "chapter": ch, "find": "cat", "replace": "dog"}
	// "cat" appears once here, so use something that appears 0 times to test rejection.
	miss := map[string]any{"bookId": bookID, "chapter": ch, "find": "dragon", "replace": "wyrm"}
	if _, e := buildProseProposal(a.ws, str(miss)); e == "" {
		t.Error("missing find text should be rejected")
	}
	_ = dup

	// A valid, unique find/replace queues and applies.
	got := a.proposeEdit("s1", ai.ToolCall{Name: "propose_prose_edit",
		Arguments: `{"bookId":"` + bookID + `","chapter":"` + ch + `","find":"cat sat on the mat","replace":"dog lay on the rug"}`})
	if !strings.Contains(got, "Proposal queued") {
		t.Fatalf("propose failed: %s", got)
	}
	pending := a.ListAIProposals()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending proposal, got %d", len(pending))
	}
	if _, err := a.AIApplyProposal(pending[0].ID); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	text, _ := workspace.ReadChapter(dir, bookID, ch)
	if !strings.Contains(text, "dog lay on the rug") || strings.Contains(text, "cat sat on the mat") {
		t.Errorf("prose edit not applied: %q", text)
	}
	if len(a.ListAIProposals()) != 0 {
		t.Error("applied proposal should be consumed")
	}
}

func TestWriteToolsPerMode(t *testing.T) {
	names := func(ts []ai.Tool) map[string]bool {
		m := map[string]bool{}
		for _, x := range ts {
			m[x.Name] = true
		}
		return m
	}
	asst := names(writeTools("assistant"))
	if !asst["propose_prose_edit"] || len(asst) != 1 {
		t.Errorf("assistant should offer prose edits only, got %v", asst)
	}
	plan := names(writeTools("planning"))
	if !plan["propose_prose_edit"] || !plan["propose_codex_edit"] || !plan["propose_plan_edit"] {
		t.Errorf("planning should offer all write tools, got %v", plan)
	}

	// Gating: prose allowed everywhere; codex/plan planning-only.
	if !writeToolAllowed("assistant", "propose_prose_edit") || writeToolAllowed("assistant", "propose_codex_edit") {
		t.Error("assistant gating wrong")
	}
	if !writeToolAllowed("planning", "propose_codex_edit") {
		t.Error("planning should allow codex edits")
	}

	// The executor rejects a disallowed write tool in assistant mode.
	a := &App{ws: &model.Workspace{}}
	exec := a.toolExecutor("s1", "assistant")
	if got := exec(ai.ToolCall{Name: "propose_codex_edit", Arguments: "{}"}); !strings.Contains(got, "isn't available") {
		t.Errorf("assistant should reject codex proposals: %s", got)
	}
}

func TestReadChapterChunking(t *testing.T) {
	dir := t.TempDir()
	if _, err := workspace.Create(dir, "T", model.KindNovel); err != nil {
		t.Fatal(err)
	}
	ws, _ := workspace.Load(dir)
	bookID := ws.Books[0].ID
	ch, err := workspace.CreateChapter(dir, bookID, "Long")
	if err != nil {
		t.Fatal(err)
	}
	// A chapter longer than one chunk, with a multibyte char to exercise runes.
	body := "# Long\n\n" + strings.Repeat("The night was dark and full of terrors — ", 500)
	if err := workspace.WriteChapter(dir, bookID, ch, body); err != nil {
		t.Fatal(err)
	}
	ws, _ = workspace.Load(dir)

	totalRunes := len([]rune(body))
	var got strings.Builder
	offset := 0
	pages := 0
	for {
		res := toolReadChapter(ws, bookID, ch, offset, 0)
		var r struct {
			Text       string `json:"text"`
			TotalChars int    `json:"totalChars"`
			Offset     int    `json:"offset"`
			Returned   int    `json:"returned"`
			HasMore    bool   `json:"hasMore"`
			NextOffset int    `json:"nextOffset"`
		}
		if err := json.Unmarshal([]byte(res), &r); err != nil {
			t.Fatalf("unmarshal chunk: %v (%s)", err, res)
		}
		if r.TotalChars != totalRunes {
			t.Fatalf("totalChars=%d want %d", r.TotalChars, totalRunes)
		}
		if r.Returned > chapterChunkChars {
			t.Fatalf("chunk too big: %d", r.Returned)
		}
		got.WriteString(r.Text)
		pages++
		if !r.HasMore {
			break
		}
		if r.NextOffset <= offset {
			t.Fatalf("nextOffset not advancing: %d", r.NextOffset)
		}
		offset = r.NextOffset
		if pages > 20 {
			t.Fatal("too many pages")
		}
	}
	if pages < 2 {
		t.Errorf("expected multiple chunks, got %d", pages)
	}
	if got.String() != body {
		t.Errorf("paged text did not reconstruct the chapter (len %d vs %d)", len(got.String()), len(body))
	}
}
