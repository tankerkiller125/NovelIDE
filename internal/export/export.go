// Package export compiles a NovelIDE workspace into a finished book —
// stitching the manuscript chapters together in reading order, rendering
// Markdown to XHTML, applying a theme, and emitting an EPUB or a
// print-ready HTML file (which the user can Print → Save as PDF). Fully
// local; no external tools.
package export

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	"novelide/internal/model"
	"novelide/internal/workspace"
)

// Format is an output format.
type Format string

const (
	FormatEPUB Format = "epub"
	FormatHTML Format = "html"
)

// Options controls a single export.
type Options struct {
	Format    Format   `json:"format"`
	ThemeID   string   `json:"themeId"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	Books     []string `json:"books"`     // book ids to include; empty = all, in manifest order
	TitlePage bool     `json:"titlePage"` // include a title page
}

// Result is a rendered book ready to write to disk.
type Result struct {
	Bytes    []byte
	Filename string // suggested filename incl. extension
	MIME     string
}

// chapter is one compiled manuscript chapter.
type chapter struct {
	bookID   string
	bookName string
	title    string // chapter heading
	bodyHTML string // rendered <body> inner HTML (XHTML-safe)
	firstOf  bool   // first chapter of its book
}

var md = goldmark.New(
	// XHTML makes void elements self-close (<br/>, <hr/>), which EPUB's XML
	// parser requires. Raw HTML is left disabled so output stays well-formed.
	goldmark.WithRendererOptions(gmhtml.WithXHTML()),
)

var h1Line = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

func renderMarkdown(src string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// chapterTitle finds a chapter's display title: its first `# heading`, else
// the prettified filename.
func chapterTitle(mdSrc, file string) string {
	if m := h1Line.FindStringSubmatch(mdSrc); m != nil {
		return strings.TrimSpace(m[1])
	}
	name := strings.TrimSuffix(file, ".md")
	name = regexp.MustCompile(`^\d+-`).ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "-", " ")
	return strings.Title(name)
}

// compile gathers the selected chapters in reading order.
func compile(ws *model.Workspace, opts Options) ([]chapter, error) {
	include := map[string]bool{}
	for _, b := range opts.Books {
		include[b] = true
	}
	var out []chapter
	for _, book := range ws.Books {
		if len(include) > 0 && !include[book.ID] {
			continue
		}
		for i, ch := range book.Chapters {
			src, err := workspace.ReadChapter(ws.Path, book.ID, ch)
			if err != nil {
				return nil, fmt.Errorf("reading %s/%s: %w", book.ID, ch, err)
			}
			// Editorial annotations are private — resolve them away, and
			// turn in-chapter scene dividers into themed scene breaks,
			// before the manuscript becomes a book.
			body, err := renderMarkdown(convertSceneBreaks(stripCriticMarkup(src)))
			if err != nil {
				return nil, err
			}
			out = append(out, chapter{
				bookID:   book.ID,
				bookName: book.Title,
				title:    chapterTitle(src, ch),
				bodyHTML: body,
				firstOf:  i == 0,
			})
		}
	}
	return out, nil
}

// multiBook reports whether the export spans more than one book (so book
// divider pages/headings are worth inserting).
func multiBook(chs []chapter) bool {
	seen := map[string]bool{}
	for _, c := range chs {
		seen[c.bookID] = true
	}
	return len(seen) > 1
}

// Export renders the workspace to the requested format.
func Export(ws *model.Workspace, opts Options) (*Result, error) {
	theme := ThemeByID(opts.ThemeID)
	if opts.Title == "" {
		opts.Title = ws.Manifest.Name
	}
	chs, err := compile(ws, opts)
	if err != nil {
		return nil, err
	}
	if len(chs) == 0 {
		return nil, fmt.Errorf("nothing to export: no chapters in the selected books")
	}
	switch opts.Format {
	case FormatEPUB:
		return exportEPUB(opts, theme, chs)
	default:
		return exportHTML(opts, theme, chs), nil
	}
}

// slugFilename builds a filesystem-friendly base name from a title.
func slugFilename(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "book"
	}
	return s
}

// exportHTML assembles a single self-contained, print-ready HTML document.
func exportHTML(opts Options, theme Theme, chs []chapter) *Result {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\"/>\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>\n%s%s</style>\n</head>\n<body>\n",
		html.EscapeString(opts.Title), contentCSS(theme), htmlChromeCSS(theme))

	if opts.TitlePage {
		b.WriteString(`<section class="title-page" style="text-align:center;padding-top:3in;">` + "\n")
		fmt.Fprintf(&b, `<h1 class="book-title">%s</h1>`+"\n", html.EscapeString(opts.Title))
		if opts.Author != "" {
			fmt.Fprintf(&b, `<p style="text-indent:0;margin-top:1em;font-size:1.1em;">%s</p>`+"\n",
				html.EscapeString(opts.Author))
		}
		b.WriteString("</section>\n")
	}

	multi := multiBook(chs)
	for _, c := range chs {
		if multi && c.firstOf {
			fmt.Fprintf(&b, `<section class="book"><h1 class="book-title">%s</h1></section>`+"\n",
				html.EscapeString(c.bookName))
		}
		b.WriteString(`<section class="chapter">` + "\n")
		b.WriteString(promoteChapterHeading(c.bodyHTML))
		b.WriteString("\n</section>\n")
	}
	b.WriteString("</body>\n</html>\n")

	return &Result{
		Bytes:    []byte(b.String()),
		Filename: slugFilename(opts.Title) + ".html",
		MIME:     "text/html",
	}
}

// xhtmlDoc wraps rendered body HTML in a well-formed XHTML document for the
// EPUB. cssHref is relative to the document's location in the archive.
func xhtmlDoc(title, cssHref, body string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
<head>
<meta charset="utf-8"/>
<title>%s</title>
<link rel="stylesheet" type="text/css" href="%s"/>
</head>
<body>
%s
</body>
</html>
`, html.EscapeString(title), cssHref, body)
}
