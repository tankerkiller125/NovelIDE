package workspace

import "testing"

func TestSearchText(t *testing.T) {
	text := "The cat sat.\nA CAT and a cathedral.\nNo felines here."

	// Case-insensitive, substring: matches cat, CAT, and the "cat" in cathedral.
	m, err := SearchText(text, "cat", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 {
		t.Fatalf("want 3 matches, got %d: %+v", len(m), m)
	}
	if m[0].Line != 1 || m[0].Col != 5 {
		t.Errorf("first match position wrong: line %d col %d", m[0].Line, m[0].Col)
	}

	// Case-sensitive drops the uppercase CAT but keeps cat/cathedral.
	m, _ = SearchText(text, "cat", true, false)
	if len(m) != 2 {
		t.Errorf("case-sensitive want 2, got %d", len(m))
	}

	// Whole-word excludes "cathedral".
	m, _ = SearchText(text, "cat", false, true)
	if len(m) != 2 {
		t.Errorf("whole-word want 2 (cat, CAT), got %d: %+v", len(m), m)
	}

	// Empty query yields nothing rather than every position.
	if m, _ := SearchText(text, "", false, false); m != nil {
		t.Errorf("empty query should match nothing, got %d", len(m))
	}
}

func TestReplaceAllText(t *testing.T) {
	text := "Aria and ARIA met aria."

	out, n, err := ReplaceAllText(text, "aria", "Kael", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || out != "Kael and Kael met Kael." {
		t.Errorf("case-insensitive replace: n=%d out=%q", n, out)
	}

	// Whole-word + case-sensitive: only the exact lowercase standalone "aria".
	out, n, _ = ReplaceAllText(text, "aria", "Kael", true, true)
	if n != 1 || out != "Aria and ARIA met Kael." {
		t.Errorf("scoped replace: n=%d out=%q", n, out)
	}

	// A `$` in the replacement is inserted literally, not treated as a group ref.
	out, n, _ = ReplaceAllText("price: X", "X", "$5", false, false)
	if n != 1 || out != "price: $5" {
		t.Errorf("literal replacement mangled: %q", out)
	}
}

func TestSearchTextUnicodeColumn(t *testing.T) {
	// A multibyte prefix must not throw the reported rune column off.
	m, err := SearchText("café cat", "cat", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[0].Col != 6 { // c-a-f-é-space = 5 runes, cat starts at 6
		t.Errorf("unicode column wrong: %+v", m)
	}
}
