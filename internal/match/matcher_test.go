package match

import (
	"reflect"
	"testing"

	"novelide/internal/model"
)

func entries() []model.CodexEntry {
	return []model.CodexEntry{
		{ID: "aria-voss", Name: "Aria Voss", Aliases: []string{"Aria", "the Ember Witch"}},
		{ID: "kael", Name: "Kael"},
		{ID: "emberfall", Name: "Emberfall"},
	}
}

func TestLongestMatchWins(t *testing.T) {
	m := New(entries())
	spans := m.Scan("Aria Voss walked into Emberfall.")
	want := []Span{
		{EntryID: "aria-voss", Start: 0, End: 9, Text: "Aria Voss"},
		{EntryID: "emberfall", Start: 22, End: 31, Text: "Emberfall"},
	}
	if !reflect.DeepEqual(spans, want) {
		t.Errorf("got %+v, want %+v", spans, want)
	}
}

func TestAliasAndWordBoundary(t *testing.T) {
	m := New(entries())
	spans := m.Scan("Ariadne is not Aria. Kaelith is not Kael.")
	want := []Span{
		{EntryID: "aria-voss", Start: 15, End: 19, Text: "Aria"},
		{EntryID: "kael", Start: 36, End: 40, Text: "Kael"},
	}
	if !reflect.DeepEqual(spans, want) {
		t.Errorf("got %+v, want %+v", spans, want)
	}
}

func TestMultiWordAlias(t *testing.T) {
	m := New(entries())
	spans := m.Scan("They feared the Ember Witch above all.")
	if len(spans) != 1 || spans[0].EntryID != "aria-voss" || spans[0].Text != "the Ember Witch" {
		t.Errorf("got %+v", spans)
	}
}

func TestCaseSensitive(t *testing.T) {
	m := New(entries())
	if spans := m.Scan("an aria of sorrow"); len(spans) != 0 {
		t.Errorf("lowercase 'aria' should not match a proper noun, got %+v", spans)
	}
}

func TestUnicodeOffsets(t *testing.T) {
	m := New(entries())
	spans := m.Scan("«Über» — Aria!")
	if len(spans) != 1 || spans[0].Start != 9 || spans[0].End != 13 {
		t.Errorf("rune offsets wrong: %+v", spans)
	}
}
