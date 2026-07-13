package deep

import (
	"strings"
	"testing"

	"novelide/internal/model"
)

func TestSuggestEntitiesFiltering(t *testing.T) {
	ws := &model.Workspace{
		Codex: []model.CodexEntry{
			{ID: "aria", Name: "Aria Voss", Aliases: []string{"Aria"}},
		},
	}
	ents := []Entity{
		{Text: "Torin Vale", Label: "PERSON", Score: 0.95},   // keep
		{Text: "Aria Voss", Label: "PERSON", Score: 0.99},    // known
		{Text: "Voss", Label: "PERSON", Score: 0.9},          // overlaps known
		{Text: "Torin Vale", Label: "PERSON", Score: 0.91},   // duplicate
		{Text: "Xu", Label: "PERSON", Score: 0.95},           // too short
		{Text: "Maybe Someone", Label: "PERSON", Score: 0.5}, // low confidence
		{Text: "Emberfall", Label: "GPE", Score: 0.9},        // keep (location)
		{Text: "Old Tongue", Label: "MISC", Score: 0.9},      // MISC dropped
	}
	got := SuggestEntities(ws, ents)
	if len(got) != 2 || got[0].Text != "Torin Vale" || got[1].Text != "Emberfall" {
		t.Errorf("unexpected filtering result: %+v", got)
	}
}

func TestOffsetConverterUnicode(t *testing.T) {
	text := "«Über» Aria"
	conv := offsetConverter(text)
	// "Aria" starts at byte 10 («=2 bytes, Ü=2 bytes, »=2 bytes) = rune 7
	if conv(10) != 7 {
		t.Errorf("byte 10 should map to rune 7, got %d", conv(10))
	}
	ascii := offsetConverter("plain text")
	if ascii(5) != 5 {
		t.Errorf("ascii should be identity")
	}
}

func TestChunkTextWindows(t *testing.T) {
	// Build text well over the window size from many short sentences.
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("Aria walked to the ashen gate. ")
	}
	text := b.String()
	runes := []rune(text)

	windows := chunkText(text, 200)
	if len(windows) < 2 {
		t.Fatalf("expected the text to be split into multiple windows, got %d", len(windows))
	}

	var reassembled strings.Builder
	for _, w := range windows {
		wr := []rune(w.text)
		if len(wr) > 200 {
			t.Errorf("window exceeds the rune budget: %d", len(wr))
		}
		// runeStart must point at exactly this window's slice of the original.
		if got := string(runes[w.runeStart : w.runeStart+len(wr)]); got != w.text {
			t.Errorf("window offset wrong at runeStart %d", w.runeStart)
		}
		reassembled.WriteString(w.text)
	}
	if reassembled.String() != text {
		t.Error("windows do not reassemble into the original text (coverage gap)")
	}

	// Short text is a single window at offset 0.
	one := chunkText("A short line.", 200)
	if len(one) != 1 || one[0].runeStart != 0 {
		t.Errorf("short text should be one window at 0, got %+v", one)
	}
}

func TestChunkTextHardSplitsLongSentence(t *testing.T) {
	// A single sentence longer than the budget must still be chunked.
	long := strings.Repeat("word ", 200) // 1000 runes, no sentence break
	windows := chunkText(long, 100)
	for _, w := range windows {
		if len([]rune(w.text)) > 100 {
			t.Fatalf("hard-split window too big: %d", len([]rune(w.text)))
		}
	}
	var sb strings.Builder
	for _, w := range windows {
		sb.WriteString(w.text)
	}
	if sb.String() != long {
		t.Error("hard-split windows lost text")
	}
}

func TestRecoverName(t *testing.T) {
	// The model returns approximate/shifted spans; recoverName rebuilds the
	// clean name from the source text. Each "span" below is a real substring of
	// the text standing in for the shifted offsets the model produced.
	cases := []struct{ text, span, want string }{
		{"But then at Hagrid came running.", "t Hagr", "Hagrid"},
		{"and Dumbledore watched over them.", "d Dumbledo", "Dumbledore"},
		{"lived at four, Privet Drive, near", ", Privet Dri", "Privet Drive"},
		{"Harry Potter smiled at them.", "Harry Potter", "Harry Potter"},
		{"the Order of the Phoenix met again", "Order of the Phoenix", "Order of the Phoenix"},
		{"they walked and talked quietly", "and talked", ""}, // no proper noun -> dropped
	}
	for _, c := range cases {
		runes := []rune(c.text)
		idx := strings.Index(c.text, c.span)
		if idx < 0 {
			t.Fatalf("span %q not in %q", c.span, c.text)
		}
		start := len([]rune(c.text[:idx]))
		end := start + len([]rune(c.span))
		got, s, e := recoverName(runes, start, end)
		if got != c.want {
			t.Errorf("recoverName(%q, span %q) = %q, want %q", c.text, c.span, got, c.want)
			continue
		}
		if got != "" && string(runes[s:e]) != got {
			t.Errorf("returned offsets [%d,%d)=%q don't match name %q", s, e, string(runes[s:e]), got)
		}
	}
}
