package model

// Schema defines the shape of a workspace's codex: which entry types exist
// and which relationship types entries can have. It lives in
// codex-schema.yaml at the workspace root and is fully user-editable — a
// space-opera series can add "planet", "bloodline", and "house" types with
// relations like "heir-of" without touching code.
type Schema struct {
	Types     []TypeDef     `yaml:"types" json:"types"`
	Relations []RelationDef `yaml:"relations" json:"relations"`
}

// TypeDef is one codex entry type. Entries of this type are stored under
// codex/<id>/.
type TypeDef struct {
	ID    string `yaml:"id" json:"id"`
	Label string `yaml:"label" json:"label"`
	Icon  string `yaml:"icon,omitempty" json:"icon"`
	// Fields suggests fact keys shown as placeholders in the editor
	// (purely advisory — entries can carry any fields).
	Fields []string `yaml:"fields,omitempty" json:"fields"`
}

// RelationDef is one relationship type. Relations are directional: the
// entry holding the relation is the subject ("parent-of" → subject is the
// parent). InverseLabel is how the target side is described ("child of").
// Symmetric relations read the same from both ends ("sibling-of").
type RelationDef struct {
	ID           string `yaml:"id" json:"id"`
	Label        string `yaml:"label" json:"label"`
	InverseLabel string `yaml:"inverseLabel,omitempty" json:"inverseLabel"`
	Symmetric    bool   `yaml:"symmetric,omitempty" json:"symmetric"`
}

// Relation is one edge from the entry holding it to another entry. From and
// Until optionally bound the relation in story time, so "allied-with" in
// book one can flip to "enemy-of" in book three without deleting history.
type Relation struct {
	Type  string      `yaml:"type" json:"type"`
	To    string      `yaml:"to" json:"to"` // target entry id
	From  *StoryPoint `yaml:"from,omitempty" json:"from,omitempty"`
	Until *StoryPoint `yaml:"until,omitempty" json:"until,omitempty"`
	Note  string      `yaml:"note,omitempty" json:"note,omitempty"`
}

// DefaultSchema is the starter schema written into new workspaces. It is a
// starting point, not a constraint — every part of it can be edited.
func DefaultSchema() Schema {
	return Schema{
		Types: []TypeDef{
			{ID: "character", Label: "Character", Icon: "👤", Fields: []string{"age", "appearance", "goal", "flaw"}},
			{ID: "location", Label: "Location", Icon: "🗺", Fields: []string{"region", "population", "ruler"}},
			{ID: "item", Label: "Item", Icon: "⚔", Fields: []string{"origin", "powers"}},
			{ID: "faction", Label: "Faction", Icon: "🏰", Fields: []string{"leader", "seat", "goal"}},
			{ID: "arc", Label: "Arc / Thread", Icon: "🧵", Fields: []string{"premise", "payoff"}},
			{ID: "concept", Label: "Concept", Icon: "✦"},
		},
		Relations: []RelationDef{
			{ID: "parent-of", Label: "parent of", InverseLabel: "child of"},
			{ID: "sibling-of", Label: "sibling of", Symmetric: true},
			{ID: "married-to", Label: "married to", Symmetric: true},
			{ID: "loves", Label: "loves", InverseLabel: "loved by"},
			{ID: "mentor-of", Label: "mentor of", InverseLabel: "student of"},
			{ID: "allied-with", Label: "allied with", Symmetric: true},
			{ID: "enemy-of", Label: "enemy of", Symmetric: true},
			{ID: "member-of", Label: "member of", InverseLabel: "has member"},
			{ID: "leads", Label: "leads", InverseLabel: "led by"},
			{ID: "serves", Label: "serves", InverseLabel: "served by"},
			{ID: "owns", Label: "owns", InverseLabel: "owned by"},
			{ID: "created", Label: "created", InverseLabel: "created by"},
			{ID: "killed", Label: "killed", InverseLabel: "killed by"},
			{ID: "located-in", Label: "located in", InverseLabel: "contains"},
		},
	}
}

// TypeIDs returns the ids of all defined types.
func (s Schema) TypeIDs() []string {
	out := make([]string, len(s.Types))
	for i, t := range s.Types {
		out[i] = t.ID
	}
	return out
}
