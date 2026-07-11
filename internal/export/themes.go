package export

import (
	"fmt"
	"strings"
)

// Theme describes the look of an exported book. Themes are data — the CSS
// is generated from these knobs, so adding a look is a struct literal.
type Theme struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`

	BodyFont   string `json:"-"` // CSS font-family stack
	BodySize   string `json:"-"` // e.g. "12pt"
	LineHeight string `json:"-"` // e.g. "2" (double) or "1.5"
	Justify    bool   `json:"-"`
	Indent     string `json:"-"` // first-line indent, e.g. "1.6em" or "0"

	// SceneGlyph replaces a Markdown thematic break (***). Empty = blank gap.
	SceneGlyph string `json:"-"`

	// Print/page geometry (HTML export only; EPUB readers paginate).
	PageSize       string `json:"-"` // "6in 9in", "letter", "a4"
	Margin         string `json:"-"` // "1in", "0.75in 0.75in"
	ChapterNewPage bool   `json:"-"`
}

// BuiltinThemes are the shipped looks, in display order.
func BuiltinThemes() []Theme {
	return []Theme{
		{
			ID:             "manuscript",
			Label:          "Standard Manuscript",
			Description:    "Double-spaced Times, 1\" margins, new page per chapter — the format agents and editors expect. Print → Save as PDF to submit.",
			BodyFont:       `"Times New Roman", Times, serif`,
			BodySize:       "12pt",
			LineHeight:     "2",
			Justify:        false,
			Indent:         "0.5in",
			SceneGlyph:     "#",
			PageSize:       "letter",
			Margin:         "1in",
			ChapterNewPage: true,
		},
		{
			ID:             "classic",
			Label:          "Classic Book",
			Description:    "A traditional serif paperback: justified Georgia, ornamental scene breaks, 6×9 trim.",
			BodyFont:       `Georgia, "Iowan Old Style", "Palatino Linotype", Palatino, serif`,
			BodySize:       "11.5pt",
			LineHeight:     "1.45",
			Justify:        true,
			Indent:         "1.4em",
			SceneGlyph:     "⁂",
			PageSize:       "6in 9in",
			Margin:         "0.75in 0.7in",
			ChapterNewPage: true,
		},
		{
			ID:             "clean",
			Label:          "Clean Sans",
			Description:    "A bright, modern reading look: sans-serif, generous leading, ragged-right, asterisk scene breaks.",
			BodyFont:       `"Helvetica Neue", Arial, "Segoe UI", system-ui, sans-serif`,
			BodySize:       "12pt",
			LineHeight:     "1.6",
			Justify:        false,
			Indent:         "0",
			SceneGlyph:     "* * *",
			PageSize:       "a4",
			Margin:         "0.9in",
			ChapterNewPage: true,
		},
	}
}

// ThemeByID returns the theme with the given id, falling back to the first
// built-in theme when unknown/empty.
func ThemeByID(id string) Theme {
	themes := BuiltinThemes()
	for _, t := range themes {
		if t.ID == id {
			return t
		}
	}
	return themes[0]
}

// contentCSS is the typography used by BOTH the EPUB and the HTML export —
// paragraphs, headings, scene breaks. No page geometry (EPUB readers own
// pagination).
func contentCSS(t Theme) string {
	align := "left"
	if t.Justify {
		align = "justify"
	}
	glyph := t.SceneGlyph
	// A blank scene break: keep vertical space, no mark.
	sceneRule := `hr { border: 0; height: 1.6em; }`
	if glyph != "" {
		sceneRule = fmt.Sprintf(
			`hr { border: 0; margin: 1.4em 0; text-align: center; }
hr::before { content: %q; letter-spacing: 0.3em; color: #333; }`,
			glyph)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `body {
  font-family: %s;
  font-size: %s;
  line-height: %s;
  text-align: %s;
  color: #111;
  margin: 0;
}
p { margin: 0; text-indent: %s; orphans: 2; widows: 2; }
/* First paragraph of a section is not indented (book convention). */
h1 + p, h2 + p, hr + p, .chapter > p:first-child, .book-title + p { text-indent: 0; }
h1.chapter-title {
  font-size: 1.5em;
  font-weight: bold;
  text-align: center;
  text-indent: 0;
  margin: 2em 0 1.2em;
  line-height: 1.2;
}
h1.book-title {
  font-size: 2em;
  text-align: center;
  text-indent: 0;
  margin: 3em 0 0.5em;
}
h1, h2, h3 { text-indent: 0; }
em { font-style: italic; }
strong { font-weight: bold; }
blockquote { margin: 1em 2em; font-style: italic; }
%s
`, t.BodyFont, t.BodySize, t.LineHeight, align, t.Indent, sceneRule)
	return b.String()
}

// htmlChromeCSS is HTML-export-only: a screen "paper" look plus the print
// @page geometry and per-chapter page breaks.
func htmlChromeCSS(t Theme) string {
	pageBreak := ""
	if t.ChapterNewPage {
		pageBreak = ".chapter { page-break-before: always; }"
	}
	return fmt.Sprintf(`
/* Screen: show the manuscript as a centered sheet of paper. */
@media screen {
  html { background: #6b6f76; }
  body { max-width: 6.5in; margin: 24px auto; padding: 0.9in 1in;
         background: #fff; box-shadow: 0 2px 18px rgba(0,0,0,0.35); }
}
/* Print / PDF geometry. */
@page { size: %s; margin: %s; }
@media print {
  html, body { background: #fff; }
  body { max-width: none; margin: 0; padding: 0; box-shadow: none; }
  .title-page { page-break-after: always; }
  %s
}
`, t.PageSize, t.Margin, pageBreak)
}
