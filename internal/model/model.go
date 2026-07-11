// Package model defines the core domain types for a NovelIDE workspace.
//
// On-disk layout:
//
//	my-series/
//	  novelide.yaml            workspace manifest
//	  codex-schema.yaml        user-defined entry types + relation types
//	  codex/                   series-level (shared) codex entries
//	    character/aria-voss.yaml     (one directory per schema type id)
//	    location/emberfall.yaml
//	  books/
//	    01-the-ember-crown/
//	      book.yaml
//	      codex/               book-local codex entries (optional)
//	      manuscript/
//	        01-chapter-one.md
//
// A standalone novel is simply a workspace with a single book.
package model

// WorkspaceKind distinguishes a standalone novel from a multi-book series.
type WorkspaceKind string

const (
	KindNovel  WorkspaceKind = "novel"
	KindSeries WorkspaceKind = "series"
)

// Manifest is the content of novelide.yaml at the workspace root.
type Manifest struct {
	Name  string        `yaml:"name" json:"name"`
	Kind  WorkspaceKind `yaml:"kind" json:"kind"`
	Books []string      `yaml:"books" json:"books"` // directory names under books/, in reading order
}

// Book is the content of book.yaml inside a book directory.
type Book struct {
	ID       string   `yaml:"-" json:"id"` // directory name
	Title    string   `yaml:"title" json:"title"`
	Chapters []string `json:"chapters" yaml:"-"` // manuscript file names, in order
	// Plan is per-chapter planning metadata from plan.yaml (sidecar file,
	// so manuscripts stay pure prose). Keyed by chapter file name.
	Plan []ChapterPlan `yaml:"-" json:"plan"`
}

// ChapterPlan is one chapter's planning card.
type ChapterPlan struct {
	File     string `yaml:"file" json:"file"`
	Synopsis string `yaml:"synopsis,omitempty" json:"synopsis"`
	// Status: outlined | drafted | revised | final
	Status string `yaml:"status,omitempty" json:"status"`
	// POV and Location are codex entry ids.
	POV      string `yaml:"pov,omitempty" json:"pov"`
	Location string `yaml:"location,omitempty" json:"location"`
	// When is freeform in-world time ("Year 3127, spring"). ISO-ish values
	// ("3127-04") sort correctly in the chronology view.
	When string `yaml:"when,omitempty" json:"when"`
	// Arcs holds codex entry ids (typically of the "arc" type) whose
	// threads run through this chapter.
	Arcs []string `yaml:"arcs,omitempty" json:"arcs"`
}

// BookPlan is the content of plan.yaml.
type BookPlan struct {
	Chapters []ChapterPlan `yaml:"chapters" json:"chapters"`
}

// SeriesBookPlan is one book's card in the series plan.
type SeriesBookPlan struct {
	ID       string `yaml:"id" json:"id"` // book directory name
	Synopsis string `yaml:"synopsis,omitempty" json:"synopsis"`
	// Status: outlined | drafted | revised | final
	Status string `yaml:"status,omitempty" json:"status"`
	// Arcs holds codex entry ids of threads running through this book.
	Arcs []string `yaml:"arcs,omitempty" json:"arcs"`
	// TargetWords is the word-count goal for the book (0 = none).
	TargetWords int `yaml:"targetWords,omitempty" json:"targetWords"`
}

// SeriesPlan is the content of series-plan.yaml at the workspace root.
type SeriesPlan struct {
	Synopsis string           `yaml:"synopsis,omitempty" json:"synopsis"`
	Books    []SeriesBookPlan `yaml:"books,omitempty" json:"books"`
}

// StoryPoint anchors an event to a position in the story: a book and a
// chapter within it. Empty fields mean "from the beginning".
type StoryPoint struct {
	Book    string `yaml:"book,omitempty" json:"book,omitempty"`
	Chapter string `yaml:"chapter,omitempty" json:"chapter,omitempty"`
}

// StatusChange records a change in an entity's life/existence state at a
// point in the story. The detection engine uses the ordered list of these to
// decide what state an entity is in at any given chapter.
type StatusChange struct {
	State string     `yaml:"state" json:"state"` // e.g. "alive", "dead", "destroyed", "missing"
	At    StoryPoint `yaml:"at,omitempty" json:"at"`
	Note  string     `yaml:"note,omitempty" json:"note,omitempty"`
}

// CodexEntry is one entity in the codex, stored as a YAML file. Type is a
// type id from the workspace schema; entries live under codex/<type>/.
type CodexEntry struct {
	ID      string   `yaml:"id" json:"id"`
	Name    string   `yaml:"name" json:"name"`
	Type    string   `yaml:"type" json:"type"`
	Aliases []string `yaml:"aliases,omitempty" json:"aliases"`
	Summary string   `yaml:"summary,omitempty" json:"summary"`
	Details string   `yaml:"details,omitempty" json:"details"` // markdown body
	// Image is a workspace-relative path to a portrait/map (e.g.
	// "assets/aria-voss.png"), copied into the workspace when set.
	Image     string            `yaml:"image,omitempty" json:"image"`
	Fields    map[string]string `yaml:"fields,omitempty" json:"fields"`
	Status    []StatusChange    `yaml:"status,omitempty" json:"status"`
	Relations []Relation        `yaml:"relations,omitempty" json:"relations"`

	// Scope is derived from where the file lives, not stored in YAML.
	// "series" for workspace-level codex/, otherwise the book ID.
	Scope string `yaml:"-" json:"scope"`
}

// Names returns the entry's canonical name plus all aliases.
func (e *CodexEntry) Names() []string {
	out := make([]string, 0, len(e.Aliases)+1)
	if e.Name != "" {
		out = append(out, e.Name)
	}
	out = append(out, e.Aliases...)
	return out
}

// Workspace is the fully loaded project handed to the frontend.
type Workspace struct {
	Path       string       `json:"path"`
	Manifest   Manifest     `json:"manifest"`
	Schema     Schema       `json:"schema"`
	Books      []Book       `json:"books"`
	Codex      []CodexEntry `json:"codex"`
	SeriesPlan SeriesPlan   `json:"seriesPlan"`
}
