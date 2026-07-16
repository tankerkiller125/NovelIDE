package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/microsoft/agent-framework-go/tool"
	"github.com/microsoft/agent-framework-go/tool/functool"

	"novelide/internal/ai"
	"novelide/internal/model"
	"novelide/internal/workspace"
)

// maxToolResultChars caps a tool's returned text so a big chapter or search
// can't blow the model's context window inside the agent loop.
const maxToolResultChars = 6000

// Tool argument shapes. functool generates each tool's JSON schema from these,
// and they marshal back to the JSON that execTool already knows how to dispatch.
type (
	searchCodexArgs struct {
		Query string `json:"query"`
	}
	getEntryArgs struct {
		ID string `json:"id"`
	}
	searchManuscriptArgs struct {
		Query         string `json:"query"`
		WholeWord     bool   `json:"wholeWord,omitempty"`
		CaseSensitive bool   `json:"caseSensitive,omitempty"`
	}
	readChapterArgs struct {
		BookID  string `json:"bookId"`
		Chapter string `json:"chapter"`
		Offset  int    `json:"offset,omitempty"`
		Limit   int    `json:"limit,omitempty"`
	}
	listStructureArgs struct{}
)

// toolCall re-marshals a typed tool argument struct and dispatches it through
// execTool, reusing the existing (tested) dispatch and workspace access.
func (a *App) toolCall(name string, in any) (string, error) {
	raw, _ := json.Marshal(in)
	res := a.execTool(ai.ToolCall{Name: name, Arguments: string(raw)})
	aiDebugf("exec %s -> %.120s", name, res)
	return res, nil
}

// readTools are the safe, auto-run tools that let the AI look things up on
// demand — cheaper and more precise than stuffing the whole world into every
// prompt.
func (a *App) readTools() []tool.Tool {
	return []tool.Tool{
		functool.MustNew(functool.Config{
			Name:        "search_codex",
			Description: "Search the Codex (the story's world bible) for entries by name, alias, type, or summary text. Returns matching entries and their ids.",
		}, func(_ context.Context, in searchCodexArgs) (string, error) { return a.toolCall("search_codex", in) }),

		functool.MustNew(functool.Config{
			Name:        "get_entry",
			Description: "Fetch one full Codex entry by id: summary, details, fields, timelined facts, status timeline, and relationships.",
		}, func(_ context.Context, in getEntryArgs) (string, error) { return a.toolCall("get_entry", in) }),

		functool.MustNew(functool.Config{
			Name:        "search_manuscript",
			Description: "Search the manuscript prose across all chapters. Returns matches with the book, chapter, line, and a snippet.",
		}, func(_ context.Context, in searchManuscriptArgs) (string, error) {
			return a.toolCall("search_manuscript", in)
		}),

		functool.MustNew(functool.Config{
			Name: "read_chapter",
			Description: "Read a chapter's text, paged for long chapters. Returns up to ~6000 characters starting at 'offset' (default 0) plus totalChars, returned, hasMore, and nextOffset. " +
				"If hasMore is true, call again with offset=nextOffset to read the next chunk. Use the bookId and chapter filename from list_structure (or the current chapter given in context).",
		}, func(_ context.Context, in readChapterArgs) (string, error) { return a.toolCall("read_chapter", in) }),

		functool.MustNew(functool.Config{
			Name:        "list_structure",
			Description: "List the books and their chapters (with plan synopsis and status) plus the series synopsis. Call this first to learn ids.",
		}, func(_ context.Context, in listStructureArgs) (string, error) { return a.toolCall("list_structure", in) }),
	}
}

// execTool dispatches a tool call against the open workspace, returning result
// text for the model (errors are returned as text so the model can recover).
func (a *App) execTool(call ai.ToolCall) string {
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
	boolArg := func(k string) bool {
		b, _ := args[k].(bool)
		return b
	}
	intArg := func(k string) int {
		f, _ := args[k].(float64) // JSON numbers decode to float64
		return int(f)
	}

	switch call.Name {
	case "search_codex":
		return toolSearchCodex(ws, str("query"))
	case "get_entry":
		return toolGetEntry(ws, str("id"))
	case "search_manuscript":
		return toolSearchManuscript(ws, str("query"), boolArg("caseSensitive"), boolArg("wholeWord"))
	case "read_chapter":
		return toolReadChapter(ws, str("bookId"), str("chapter"), intArg("offset"), intArg("limit"))
	case "list_structure":
		return toolListStructure(ws)
	default:
		return "error: unknown tool " + call.Name
	}
}

func toolResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "error: " + err.Error()
	}
	s := string(b)
	if len(s) > maxToolResultChars {
		s = s[:maxToolResultChars] + " …(truncated)"
	}
	return s
}

func toolSearchCodex(ws *model.Workspace, query string) string {
	q := strings.ToLower(query)
	type hit struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Summary string `json:"summary,omitempty"`
	}
	var hits []hit
	for _, e := range ws.Codex {
		hay := strings.ToLower(e.Name + " " + strings.Join(e.Aliases, " ") + " " + e.Type + " " + e.Summary)
		if q == "" || strings.Contains(hay, q) {
			hits = append(hits, hit{e.ID, e.Name, e.Type, e.Summary})
			if len(hits) >= 40 {
				break
			}
		}
	}
	return toolResult(map[string]any{"count": len(hits), "entries": hits})
}

func toolGetEntry(ws *model.Workspace, id string) string {
	for i := range ws.Codex {
		if ws.Codex[i].ID == id {
			return toolResult(ws.Codex[i])
		}
	}
	return "error: no Codex entry with id " + id + " (use search_codex or list_structure to find ids)"
}

func toolSearchManuscript(ws *model.Workspace, query string, caseSensitive, wholeWord bool) string {
	if query == "" {
		return "error: query is required"
	}
	type hit struct {
		Book    string `json:"book"`
		Chapter string `json:"chapter"`
		Line    int    `json:"line"`
		Snippet string `json:"snippet"`
	}
	var hits []hit
	for _, book := range ws.Books {
		for _, ch := range book.Chapters {
			text, err := workspace.ReadChapter(ws.Path, book.ID, ch)
			if err != nil {
				continue
			}
			matches, err := workspace.SearchText(text, query, caseSensitive, wholeWord)
			if err != nil {
				return "error: " + err.Error()
			}
			for _, m := range matches {
				hits = append(hits, hit{book.ID, ch, m.Line, m.Before + "[" + m.Match + "]" + m.After})
				if len(hits) >= 30 {
					return toolResult(map[string]any{"count": len(hits), "matches": hits, "truncated": true})
				}
			}
		}
	}
	return toolResult(map[string]any{"count": len(hits), "matches": hits})
}

// chapterChunkChars is the max characters of chapter text returned per
// read_chapter call. Offsets and lengths are in characters (runes), so long
// chapters are paged: call again with offset=nextOffset until hasMore is false.
const chapterChunkChars = 6000

func toolReadChapter(ws *model.Workspace, bookID, chapter string, offset, limit int) string {
	text, err := workspace.ReadChapter(ws.Path, bookID, chapter)
	if err != nil {
		return "error: could not read chapter (" + err.Error() + ")"
	}
	runes := []rune(text)
	total := len(runes)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 || limit > chapterChunkChars {
		limit = chapterChunkChars
	}
	end := offset + limit
	if end > total {
		end = total
	}
	res := map[string]any{
		"book":       bookID,
		"chapter":    chapter,
		"title":      workspace.ChapterTitle(text, chapter),
		"totalChars": total,
		"offset":     offset,
		"returned":   end - offset,
		"hasMore":    end < total,
		"text":       string(runes[offset:end]),
	}
	if end < total {
		res["nextOffset"] = end
	}
	// read_chapter self-bounds its text to chapterChunkChars, so marshal directly
	// rather than through the smaller generic tool-result cap (which would corrupt
	// the returned slice).
	b, err := json.Marshal(res)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}

func toolListStructure(ws *model.Workspace) string {
	type chapterInfo struct {
		Chapter  string `json:"chapter"`
		Title    string `json:"title,omitempty"`
		Synopsis string `json:"synopsis,omitempty"`
		Status   string `json:"status,omitempty"`
	}
	type bookInfo struct {
		ID       string        `json:"id"`
		Title    string        `json:"title"`
		Chapters []chapterInfo `json:"chapters"`
	}
	var books []bookInfo
	for _, b := range ws.Books {
		plan := map[string]model.ChapterPlan{}
		for _, p := range b.Plan {
			plan[p.File] = p
		}
		bi := bookInfo{ID: b.ID, Title: b.Title}
		for _, ch := range b.Chapters {
			p := plan[ch]
			bi.Chapters = append(bi.Chapters, chapterInfo{
				Chapter: ch, Synopsis: p.Synopsis, Status: p.Status,
			})
		}
		books = append(books, bi)
	}
	sort.Slice(books, func(i, j int) bool { return books[i].ID < books[j].ID })
	return toolResult(map[string]any{
		"seriesSynopsis": ws.SeriesPlan.Synopsis,
		"books":          books,
	})
}
