package export

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"fmt"
	"html"
	"strings"
	"time"
)

// exportEPUB assembles a valid EPUB 3 archive by hand: the mimetype entry
// stored (uncompressed) and first, META-INF/container.xml, an OPF package
// document, an EPUB 3 nav, a stylesheet, and one XHTML file per chapter
// (plus optional title and per-book divider pages).
func exportEPUB(opts Options, theme Theme, chs []chapter) (*Result, error) {
	// spineItem drives the manifest, spine, and nav.
	type spineItem struct {
		id       string
		file     string // path inside OEBPS/
		title    string
		inNav    bool
		isBook   bool   // top-level nav entry (book divider)
		navUnder string // id of the book this chapter belongs under (nested nav)
	}
	var items []spineItem
	var files = map[string]string{} // OEBPS-relative path -> contents

	css := contentCSS(theme)
	files["style.css"] = css

	multi := multiBook(chs)

	if opts.TitlePage {
		body := fmt.Sprintf(
			`<section epub:type="titlepage" style="text-align:center;margin-top:25%%;">
<h1 class="book-title">%s</h1>%s
</section>`,
			html.EscapeString(opts.Title),
			func() string {
				if opts.Author != "" {
					return "\n<p style=\"text-indent:0;margin-top:1em;\">" + html.EscapeString(opts.Author) + "</p>"
				}
				return ""
			}())
		files["title.xhtml"] = xhtmlDoc(opts.Title, "style.css", body)
		items = append(items, spineItem{id: "title", file: "title.xhtml", title: opts.Title})
	}

	bookNav := "" // current book's nav id (for nesting chapters)
	bookN, chapN := 0, 0
	for _, c := range chs {
		if multi && c.firstOf {
			bookN++
			id := fmt.Sprintf("book%02d", bookN)
			file := fmt.Sprintf("text/%s.xhtml", id)
			body := fmt.Sprintf(`<section epub:type="bodymatter"><h1 class="book-title">%s</h1></section>`,
				html.EscapeString(c.bookName))
			files[file] = xhtmlDoc(c.bookName, "../style.css", body)
			items = append(items, spineItem{id: id, file: file, title: c.bookName, inNav: true, isBook: true})
			bookNav = id
		}
		chapN++
		id := fmt.Sprintf("chap%03d", chapN)
		file := fmt.Sprintf("text/%s.xhtml", id)
		// Promote the chapter's leading H1 to a themed chapter title.
		body := `<section epub:type="chapter">` + "\n" + promoteChapterHeading(c.bodyHTML) + "\n</section>"
		files[file] = xhtmlDoc(c.title, "../style.css", body)
		items = append(items, spineItem{id: id, file: file, title: c.title, inNav: true, navUnder: bookNav})
	}

	// ---- content.opf ----
	uid := "urn:uuid:" + newUUID()
	modified := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	var opf strings.Builder
	opf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
`)
	fmt.Fprintf(&opf, "    <dc:identifier id=\"bookid\">%s</dc:identifier>\n", uid)
	fmt.Fprintf(&opf, "    <dc:title>%s</dc:title>\n", html.EscapeString(opts.Title))
	fmt.Fprintf(&opf, "    <dc:language>en</dc:language>\n")
	if opts.Author != "" {
		fmt.Fprintf(&opf, "    <dc:creator>%s</dc:creator>\n", html.EscapeString(opts.Author))
	}
	fmt.Fprintf(&opf, "    <meta property=\"dcterms:modified\">%s</meta>\n", modified)
	opf.WriteString("  </metadata>\n  <manifest>\n")
	opf.WriteString(`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	opf.WriteString(`    <item id="css" href="style.css" media-type="text/css"/>` + "\n")
	for _, it := range items {
		fmt.Fprintf(&opf, `    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n", it.id, it.file)
	}
	opf.WriteString("  </manifest>\n  <spine>\n")
	for _, it := range items {
		fmt.Fprintf(&opf, `    <itemref idref="%s"/>`+"\n", it.id)
	}
	opf.WriteString("  </spine>\n</package>\n")
	files["content.opf"] = opf.String()

	// ---- nav.xhtml (nested when multi-book) ----
	var nav strings.Builder
	nav.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head><meta charset="utf-8"/><title>Contents</title></head>
<body>
<nav epub:type="toc" id="toc">
<h1>Contents</h1>
<ol>
`)
	openBook := false
	for _, it := range items {
		if !it.inNav {
			continue
		}
		if it.isBook {
			if openBook {
				nav.WriteString("</ol></li>\n")
			}
			fmt.Fprintf(&nav, `<li><a href="%s">%s</a><ol>`+"\n", it.file, html.EscapeString(it.title))
			openBook = true
			continue
		}
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`+"\n", it.file, html.EscapeString(it.title))
	}
	if openBook {
		nav.WriteString("</ol></li>\n")
	}
	nav.WriteString("</ol>\n</nav>\n</body>\n</html>\n")
	files["nav.xhtml"] = nav.String()

	// ---- container.xml ----
	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

	// ---- zip it ----
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// mimetype MUST be first and stored (uncompressed).
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mw.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}
	writeZip := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := writeZip("META-INF/container.xml", container); err != nil {
		return nil, err
	}
	for path, content := range files {
		if err := writeZip("OEBPS/"+path, content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return &Result{
		Bytes:    buf.Bytes(),
		Filename: slugFilename(opts.Title) + ".epub",
		MIME:     "application/epub+zip",
	}, nil
}

// promoteChapterHeading turns a rendered chapter's leading <h1> into a
// themed chapter title (class="chapter-title"). goldmark emits a plain
// <h1>...</h1> for the leading `# heading`.
func promoteChapterHeading(body string) string {
	trimmed := strings.TrimLeft(body, " \t\r\n")
	if strings.HasPrefix(trimmed, "<h1>") {
		return strings.Replace(trimmed, "<h1>", `<h1 class="chapter-title">`, 1)
	}
	return body
}

// newUUID returns a random RFC 4122 v4 UUID string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
