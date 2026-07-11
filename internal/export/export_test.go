package export_test

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"novelide/internal/export"
	"novelide/internal/workspace"
)

func TestExportEPUBStructure(t *testing.T) {
	dir, _ := filepath.Abs("../../examples/demo-series")
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	nChapters := 0
	for _, b := range ws.Books {
		nChapters += len(b.Chapters)
	}
	if nChapters == 0 {
		t.Fatal("demo-series has no manuscript chapters to export")
	}

	res, err := export.Export(ws, export.Options{
		Format: export.FormatEPUB, ThemeID: "classic",
		Title: "Test Book", Author: "A. Writer", TitlePage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Filename != "test-book.epub" || res.MIME != "application/epub+zip" {
		t.Errorf("unexpected result meta: %+v", res)
	}

	zr, err := zip.NewReader(bytes.NewReader(res.Bytes), int64(len(res.Bytes)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	// mimetype must be first and stored uncompressed.
	if zr.File[0].Name != "mimetype" {
		t.Errorf("first entry is %q, want mimetype", zr.File[0].Name)
	}
	if zr.File[0].Method != zip.Store {
		t.Error("mimetype must be stored (uncompressed)")
	}

	got := map[string]string{}
	chapterFiles := 0
	for _, f := range zr.File {
		rc, _ := f.Open()
		var b bytes.Buffer
		b.ReadFrom(rc)
		rc.Close()
		got[f.Name] = b.String()
		if strings.HasPrefix(f.Name, "OEBPS/text/chap") {
			chapterFiles++
		}
	}
	if s := got["mimetype"]; s != "application/epub+zip" {
		t.Errorf("mimetype content = %q", s)
	}
	for _, must := range []string{"META-INF/container.xml", "OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/style.css", "OEBPS/title.xhtml"} {
		if _, ok := got[must]; !ok {
			t.Errorf("missing archive entry %q", must)
		}
	}
	if chapterFiles != nChapters {
		t.Errorf("got %d chapter files, want %d", chapterFiles, nChapters)
	}
	opf := got["OEBPS/content.opf"]
	for _, must := range []string{"<dc:title>Test Book</dc:title>", "<dc:creator>A. Writer</dc:creator>", `properties="nav"`, "<spine>"} {
		if !strings.Contains(opf, must) {
			t.Errorf("opf missing %q", must)
		}
	}
	// Every chapter must be listed in the spine.
	if strings.Count(opf, "<itemref") < nChapters {
		t.Error("spine is missing chapter itemrefs")
	}
	// The multi-book demo should produce nested nav with a book title.
	if !strings.Contains(got["OEBPS/nav.xhtml"], "The Ember Crown") {
		t.Error("nav should contain the book title for a multi-book export")
	}
}

func TestExportHTML(t *testing.T) {
	dir, _ := filepath.Abs("../../examples/demo-series")
	ws, _ := workspace.Load(dir)

	res, err := export.Export(ws, export.Options{
		Format: export.FormatHTML, ThemeID: "manuscript", Title: "HTML Book", TitlePage: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := string(res.Bytes)
	if res.Filename != "html-book.html" {
		t.Errorf("filename = %q", res.Filename)
	}
	nChapters := 0
	for _, b := range ws.Books {
		nChapters += len(b.Chapters)
	}
	if c := strings.Count(h, `<section class="chapter">`); c != nChapters {
		t.Errorf("got %d chapter sections, want %d", c, nChapters)
	}
	for _, must := range []string{"<!doctype html>", "@page", "Times New Roman", `class="title-page"`, "HTML Book"} {
		if !strings.Contains(h, must) {
			t.Errorf("html missing %q", must)
		}
	}
}

func TestExportBookSubset(t *testing.T) {
	dir, _ := filepath.Abs("../../examples/demo-series")
	ws, _ := workspace.Load(dir)
	if len(ws.Books) < 2 {
		t.Skip("need a multi-book workspace")
	}
	only := ws.Books[1].ID
	res, err := export.Export(ws, export.Options{
		Format: export.FormatHTML, Books: []string{only}, Title: "One Book",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c := strings.Count(string(res.Bytes), `<section class="chapter">`); c != len(ws.Books[1].Chapters) {
		t.Errorf("subset export chapter count = %d, want %d", c, len(ws.Books[1].Chapters))
	}
	// A single-book export inserts no book-divider title page.
	if strings.Contains(string(res.Bytes), `<section class="book">`) {
		t.Error("single-book export should not have a book divider")
	}
}
