package main

import (
	"encoding/json"
	"sort"
	"strings"

	"novelide/internal/ai"
	"novelide/internal/model"
	"novelide/internal/workspace"
)

// maxToolResultChars caps a tool's returned text so a big chapter or search
// can't blow the model's context window inside the agent loop.
const maxToolResultChars = 6000

// readTools are the safe, auto-run tools that let the AI look things up on
// demand — cheaper and more precise than stuffing the whole world into every
// prompt. Their definitions are static, so they stay in the cached prefix.
func readTools() []ai.Tool {
	obj := func(props, required string) json.RawMessage {
		return json.RawMessage(`{"type":"object","properties":{` + props + `}` + required + `}`)
	}
	return []ai.Tool{
		{
			Name:        "search_codex",
			Description: "Search the Codex (the story's world bible) for entries by name, alias, type, or summary text. Returns matching entries and their ids.",
			Schema:      obj(`"query":{"type":"string","description":"text to search for"}`, `,"required":["query"]`),
		},
		{
			Name:        "get_entry",
			Description: "Fetch one full Codex entry by id: summary, details, fields, timelined facts, status timeline, and relationships.",
			Schema:      obj(`"id":{"type":"string"}`, `,"required":["id"]`),
		},
		{
			Name:        "search_manuscript",
			Description: "Search the manuscript prose across all chapters. Returns matches with the book, chapter, line, and a snippet.",
			Schema:      obj(`"query":{"type":"string"},"wholeWord":{"type":"boolean"},"caseSensitive":{"type":"boolean"}`, `,"required":["query"]`),
		},
		{
			Name: "read_chapter",
			Description: "Read a chapter's text, paged for long chapters. Returns up to ~6000 characters starting at 'offset' (default 0) plus totalChars, returned, hasMore, and nextOffset. " +
				"If hasMore is true, call again with offset=nextOffset to read the next chunk. Use the bookId and chapter filename from list_structure (or the current chapter given in context).",
			Schema: obj(`"bookId":{"type":"string"},"chapter":{"type":"string","description":"chapter file name"},`+
				`"offset":{"type":"integer","description":"character offset to start from (default 0)"},`+
				`"limit":{"type":"integer","description":"max characters to return (default and max ~6000)"}`, `,"required":["bookId","chapter"]`),
		},
		{
			Name:        "list_structure",
			Description: "List the books and their chapters (with plan synopsis and status) plus the series synopsis. Call this first to learn ids.",
			Schema:      obj("", ""),
		},
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
