package detect

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"novelide/internal/match"
	"novelide/internal/model"
	"novelide/internal/nlp"
)

// This file mines character *features* from prose: gender (by pronoun
// statistics), appearance facts (hair, eyes, height, build, age), kinship
// and social relationships, aliases, and ownership/membership. Extracted
// facts the codex lacks become suggestions; facts that contradict recorded
// codex fields become warning flags — the machinery behind "her brown eyes"
// lighting up when the codex says green.
//
// Everything is local: prose v3 supplies POS tags and sentence boundaries,
// golem supplies lemmas, and pronouns resolve by recency with a strict
// ambiguity guard (a pronoun only counts when exactly one character is in
// scope — the right failure mode is silence, not a wrong guess).

// bodyParts maps attribute nouns to the codex field key they populate.
var bodyParts = map[string]string{
	"hair": "hair", "eyes": "eyes", "eye": "eyes", "skin": "skin",
	"beard": "beard", "voice": "voice",
}

var colorWords = map[string]bool{
	"black": true, "brown": true, "blond": true, "blonde": true,
	"golden": true, "gold": true, "silver": true, "white": true,
	"grey": true, "gray": true, "red": true, "copper": true,
	"auburn": true, "green": true, "blue": true, "hazel": true,
	"amber": true, "violet": true, "dark": true, "pale": true,
	"raven": true, "chestnut": true, "crimson": true, "emerald": true,
}

var heightWords = map[string]bool{"tall": true, "short": true, "towering": true, "petite": true}

var buildWords = map[string]bool{
	"lanky": true, "broad": true, "stocky": true, "slight": true,
	"wiry": true, "burly": true, "gaunt": true, "plump": true,
	"muscular": true, "slender": true, "thin": true, "lean": true,
	"heavyset": true, "willowy": true,
}

var maleWords = map[string]bool{
	"he": true, "him": true, "his": true, "himself": true,
}
var femaleWords = map[string]bool{
	"she": true, "her": true, "hers": true, "herself": true,
}
var maleHonorifics = map[string]bool{
	"mr": true, "mister": true, "sir": true, "lord": true, "king": true,
	"prince": true, "duke": true, "brother": false, // brother-as-title is ambiguous; excluded
}
var femaleHonorifics = map[string]bool{
	"mrs": true, "ms": true, "miss": true, "lady": true, "queen": true,
	"princess": true, "duchess": true, "madam": true, "madame": true,
}

// kinDef maps a kinship noun to a schema relation. reverse means the
// *object* of the pattern is the relation's subject ("A was B's son" →
// B parent-of A).
type kinDef struct {
	rel     string
	reverse bool
}

var kinship = map[string]kinDef{
	"brother": {rel: "sibling-of"}, "sister": {rel: "sibling-of"},
	"mother": {rel: "parent-of"}, "father": {rel: "parent-of"},
	"son": {rel: "parent-of", reverse: true}, "daughter": {rel: "parent-of", reverse: true},
	"husband": {rel: "married-to"}, "wife": {rel: "married-to"},
	"mentor": {rel: "mentor-of"}, "teacher": {rel: "mentor-of"},
	"apprentice": {rel: "mentor-of", reverse: true}, "student": {rel: "mentor-of", reverse: true},
	"servant": {rel: "serves"}, "friend": {rel: "allied-with"},
	"enemy": {rel: "enemy-of"}, "rival": {rel: "enemy-of"},
}

// ownLemmas/joinLemmas: verbs whose object becomes an owns / member-of edge.
var ownLemmas = map[string]bool{"wield": true, "wear": true, "bear": true, "own": true}
var joinLemmas = map[string]bool{"join": true}

var extractCopulas = map[string]bool{"is": true, "was": true, "were": true, "are": true}

// Extract mines feature observations from one chapter. It returns codex-gap
// suggestions and contradiction flags in a single pass over the parsed doc.
func Extract(ws *model.Workspace, bookID, chapter string, spans []match.Span, doc *nlp.Doc) ([]Suggestion, []Flag) {
	if doc == nil {
		return nil, nil
	}
	x := &extractor{
		ws:      ws,
		doc:     doc,
		bookID:  bookID,
		chapter: chapter,
		tl:      NewTimeline(ws),
		byID:    map[string]*model.CodexEntry{},
		seen:    map[string]bool{},
	}
	for i := range ws.Codex {
		x.byID[ws.Codex[i].ID] = &ws.Codex[i]
	}
	for _, r := range ws.Schema.Relations {
		x.relDefs = append(x.relDefs, r)
	}
	x.spans = spans
	x.run()
	return x.suggestions, x.flags
}

type extractor struct {
	ws      *model.Workspace
	doc     *nlp.Doc
	bookID  string
	chapter string
	tl      *Timeline
	byID    map[string]*model.CodexEntry
	relDefs []model.RelationDef
	spans   []match.Span

	suggestions []Suggestion
	flags       []Flag
	seen        map[string]bool

	// gender evidence: entryID -> {male, female} vote counts
	genderVotes map[string][2]int
}

func (x *extractor) add(s Suggestion) {
	if !x.seen[s.Key] {
		x.seen[s.Key] = true
		x.suggestions = append(x.suggestions, s)
	}
}

func (x *extractor) relDef(id string) *model.RelationDef {
	for i := range x.relDefs {
		if x.relDefs[i].ID == id {
			return &x.relDefs[i]
		}
	}
	return nil
}

func (x *extractor) hasRelation(e *model.CodexEntry, relType, to string) bool {
	def := x.relDef(relType)
	for _, r := range e.Relations {
		if r.Type == relType && r.To == to {
			return true
		}
	}
	if def != nil && def.Symmetric {
		if other, ok := x.byID[to]; ok {
			for _, r := range other.Relations {
				if r.Type == relType && r.To == e.ID {
					return true
				}
			}
		}
	}
	return false
}

// fieldValue does a case-insensitive lookup in an entry's static fields.
func fieldValue(e *model.CodexEntry, key string) string {
	for k, v := range e.Fields {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// recordedField reports whether the entry already records key — as a static
// fact or a timelined one — and, when known, the value in effect at the current
// chapter. This stops the engine re-suggesting a fact the author has already
// captured on a timeline (e.g. a changing age). A timelined key with no value
// applying yet at this point counts as present with an empty value.
func (x *extractor) recordedField(e *model.CodexEntry, key string) (value string, present bool) {
	if v := fieldValue(e, key); v != "" {
		return v, true
	}
	for k, tl := range e.FieldTimelines {
		if strings.EqualFold(k, key) {
			return x.tl.valueAt(tl, x.bookID, x.chapter), true
		}
	}
	return "", false
}

// compatible avoids flagging "copper-red" against "copper": one value
// containing the other counts as agreement.
func compatible(a, b string) bool {
	al, bl := strings.ToLower(a), strings.ToLower(b)
	return strings.Contains(al, bl) || strings.Contains(bl, al)
}

// observeField turns an (entity, key, value) observation into either a gap
// suggestion or a contradiction flag.
func (x *extractor) observeField(e *model.CodexEntry, key, value string, sp match.Span) {
	if value == "" {
		return
	}
	existing, present := x.recordedField(e, key)
	if !present {
		x.add(Suggestion{
			Kind: "field", EntryID: e.ID, FieldKey: key, FieldValue: value,
			Start: sp.Start, End: sp.End,
			Message: fmt.Sprintf("The manuscript describes %s's %s as %q. Save it as a fact?", e.Name, key, value),
			Key:     "field|" + e.ID + "|" + key + "|" + strings.ToLower(value),
		})
		return
	}
	// Already recorded (static or timelined). Only flag a genuine contradiction
	// against a known current value — a timelined key with no value applying at
	// this point yet has nothing to contradict.
	if existing != "" && !compatible(existing, value) {
		x.flags = append(x.flags, Flag{
			EntryID: e.ID, Start: sp.Start, End: sp.End,
			Severity: SevWarning, Rule: "field-contradiction",
			Message: fmt.Sprintf("Here %s's %s is %q, but the Codex records %q", e.Name, key, value, existing),
		})
	}
}

func (x *extractor) suggestRelation(subj, obj *model.CodexEntry, relType string, sp match.Span, verb string) {
	def := x.relDef(relType)
	if def == nil || x.hasRelation(subj, relType, obj.ID) {
		return
	}
	x.add(Suggestion{
		Kind: "relation", EntryID: subj.ID, TargetID: obj.ID, Relation: relType,
		Start: sp.Start, End: sp.End,
		Message: fmt.Sprintf("%s appears to be %s %s (%q). Record the relationship?", subj.Name, def.Label, obj.Name, verb),
		Key:     "relation|" + subj.ID + "|" + relType + "|" + obj.ID,
	})
}

// run drives all extractors sentence by sentence, maintaining the pronoun
// resolution state across sentences.
func (x *extractor) run() {
	x.genderVotes = map[string][2]int{}
	sentRanges := x.doc.SentenceTokenRanges()
	toks := x.doc.Tokens

	// carried is the character a pronoun resolves to when the current
	// sentence introduces no characters of its own.
	var carried *model.CodexEntry

	for _, sr := range sentRanges {
		st, en := sr[0], sr[1]
		if st >= en {
			continue
		}
		sentStart, sentEnd := toks[st].Start, toks[en-1].End

		// Mentions inside this sentence, split by whether they're characters.
		var mentions []match.Span
		var chars []match.Span
		for _, sp := range x.spans {
			if sp.Start >= sentStart && sp.End <= sentEnd {
				mentions = append(mentions, sp)
				if e, ok := x.byID[sp.EntryID]; ok && e.Type == "character" {
					chars = append(chars, sp)
				}
			}
		}
		sort.Slice(mentions, func(i, j int) bool { return mentions[i].Start < mentions[j].Start })
		sort.Slice(chars, func(i, j int) bool { return chars[i].Start < chars[j].Start })

		// Pronoun scope: exactly one character in the sentence, or carry-over.
		var scope *model.CodexEntry
		var scopeSpan match.Span
		uniq := map[string]match.Span{}
		for _, c := range chars {
			uniq[c.EntryID] = c
		}
		switch len(uniq) {
		case 0:
			scope = carried
		case 1:
			for id, sp := range uniq {
				scope = x.byID[id]
				scopeSpan = sp
			}
			carried = scope
		default:
			scope = nil
			carried = nil
		}
		if scope != nil && scopeSpan.End == 0 {
			// scope carried from a previous sentence: anchor to sentence start
			scopeSpan = match.Span{EntryID: scope.ID, Start: sentStart, End: sentEnd}
		}

		x.genderPass(toks[st:en], chars, scope)
		x.appearancePass(toks[st:en], chars, scope, scopeSpan)
		x.kinshipPass(toks[st:en], chars)
		x.ownershipPass(toks[st:en], chars, mentions)
		x.aliasPass(toks[st:en], chars)
	}

	x.genderSuggestions()
}

// ---- gender ----

func (x *extractor) genderPass(toks []nlp.Token, chars []match.Span, scope *model.CodexEntry) {
	// Honorific immediately before a character mention: strong evidence.
	for _, c := range chars {
		e := x.byID[c.EntryID]
		for i, t := range toks {
			if t.End > c.Start {
				break
			}
			if i == len(toks)-1 || toks[i+1].Start < c.Start {
				continue
			}
			w := strings.ToLower(strings.TrimSuffix(t.Text, "."))
			v := x.genderVotes[e.ID]
			if maleHonorifics[w] {
				v[0] += 3
				x.genderVotes[e.ID] = v
			} else if femaleHonorifics[w] {
				v[1] += 3
				x.genderVotes[e.ID] = v
			}
		}
	}
	// Gendered pronouns vote for the sentence's sole character in scope.
	if scope == nil {
		return
	}
	for _, t := range toks {
		w := strings.ToLower(t.Text)
		v := x.genderVotes[scope.ID]
		if maleWords[w] {
			v[0]++
			x.genderVotes[scope.ID] = v
		} else if femaleWords[w] {
			v[1]++
			x.genderVotes[scope.ID] = v
		}
	}
}

func (x *extractor) genderSuggestions() {
	for id, v := range x.genderVotes {
		e := x.byID[id]
		if e == nil {
			continue
		}
		if _, present := x.recordedField(e, "gender"); present {
			continue // already recorded, statically or on a timeline
		}
		total := v[0] + v[1]
		if total < 3 {
			continue
		}
		gender, n := "male", v[0]
		pronouns := "he/him"
		if v[1] > v[0] {
			gender, n = "female", v[1]
			pronouns = "she/her"
		}
		if float64(n)/float64(total) < 0.8 {
			continue // mixed evidence — abstain
		}
		var sp match.Span
		for _, s := range x.spans {
			if s.EntryID == id {
				sp = s
				break
			}
		}
		x.add(Suggestion{
			Kind: "field", EntryID: id, FieldKey: "gender", FieldValue: gender,
			Start: sp.Start, End: sp.End,
			Message: fmt.Sprintf("%s is referred to as %s %d time(s) in this chapter. Record gender: %s?", e.Name, pronouns, n, gender),
			Key:     "field|" + id + "|gender|" + gender,
		})
	}
}

// ---- appearance ----

func isColorish(w string) bool {
	lw := strings.ToLower(w)
	if colorWords[lw] {
		return true
	}
	if strings.Contains(lw, "-") {
		for _, part := range strings.Split(lw, "-") {
			if colorWords[part] {
				return true
			}
		}
	}
	return false
}

// descValue accepts a token as an appearance value: an adjective, a color
// word, or a color compound.
func descValue(t nlp.Token) bool {
	return t.Tag == "JJ" || t.Tag == "VBN" || isColorish(t.Text)
}

// collectDesc gathers a run of descriptive tokens starting at i — "red",
// "red long", "long and silver" — returning the joined description and the
// index of the first token after the run. Adjectives chain freely in prose
// ("her red long hair"), so a single-token match is not enough.
func collectDesc(toks []nlp.Token, i int) (string, int) {
	var words []string
	j := i
	for j < len(toks) && len(words) < 4 {
		t := toks[j]
		if descValue(t) {
			words = append(words, t.Text)
			j++
			continue
		}
		// Connectors inside a run: "long and red", "long, red".
		if len(words) > 0 && j+1 < len(toks) && descValue(toks[j+1]) {
			if strings.ToLower(t.Text) == "and" {
				words = append(words, t.Text)
				j++
				continue
			}
			if t.Tag == "," {
				j++
				continue
			}
		}
		break
	}
	if len(words) == 0 {
		return "", i
	}
	return strings.Join(words, " "), j
}

func (x *extractor) appearancePass(toks []nlp.Token, chars []match.Span, scope *model.CodexEntry, scopeSpan match.Span) {
	// Possessive owner positions: "<Name>'s ..." or "her/his ..." (resolved).
	type owner struct {
		e   *model.CodexEntry
		sp  match.Span
		idx int // token index right after the possessive marker
	}
	var owners []owner
	for _, c := range chars {
		e := x.byID[c.EntryID]
		// Only the token immediately after the mention counts — a later
		// `'s` in the sentence belongs to someone else.
		for i, t := range toks {
			if t.Start < c.End {
				continue
			}
			if t.Tag == "POS" {
				owners = append(owners, owner{e: e, sp: c, idx: i + 1})
			}
			break
		}
	}
	if scope != nil {
		for i, t := range toks {
			w := strings.ToLower(t.Text)
			if t.Tag == "PRP$" && (w == "his" || w == "her" || w == "their") {
				owners = append(owners, owner{e: scope, sp: scopeSpan, idx: i + 1})
			}
		}
	}

	for _, o := range owners {
		if o.idx >= len(toks) {
			continue
		}
		// Form 1: <owner> <bodypart> was <value(s)>
		// ("Aria's hair was copper-red", "was long and silver")
		if key, ok := bodyParts[strings.ToLower(toks[o.idx].Text)]; ok {
			j := o.idx + 1
			if j < len(toks) && extractCopulas[strings.ToLower(toks[j].Text)] {
				j++
				// Skip adverbs/articles — but never a color word, which the
				// tagger sometimes mislabels as RB ("silver").
				for j < len(toks) && !isColorish(toks[j].Text) &&
					(toks[j].Tag == "RB" || toks[j].Tag == "DT") {
					j++
				}
				if value, _ := collectDesc(toks, j); value != "" {
					x.observeField(o.e, key, value, o.sp)
				}
			}
			continue
		}
		// Form 2: <owner> <value(s)> <bodypart>
		// ("her copper hair", "Aria's red long hair")
		if value, next := collectDesc(toks, o.idx); value != "" && next < len(toks) {
			if key, ok := bodyParts[strings.ToLower(toks[next].Text)]; ok {
				x.observeField(o.e, key, value, o.sp)
			}
		}
	}

	// Form 3: "... with <value(s)> <bodypart>" — needs an unambiguous subject.
	if scope != nil {
		for i, t := range toks {
			if strings.ToLower(t.Text) != "with" || i+2 >= len(toks) {
				continue
			}
			if value, next := collectDesc(toks, i+1); value != "" && next < len(toks) {
				if key, ok := bodyParts[strings.ToLower(toks[next].Text)]; ok {
					x.observeField(scope, key, value, scopeSpan)
				}
			}
		}
	}

	// Form 4: copula predicates — height, build, age.
	for _, c := range chars {
		e := x.byID[c.EntryID]
		i := 0
		for i < len(toks) && toks[i].Start < c.End {
			i++
		}
		if i >= len(toks) {
			continue
		}
		w := strings.ToLower(toks[i].Text)
		if !extractCopulas[w] && lemmaOf(w) != "turn" {
			continue
		}
		j := i + 1
		for j < len(toks) && (toks[j].Tag == "RB" || toks[j].Tag == "DT") {
			j++
		}
		if j >= len(toks) {
			continue
		}
		val := strings.ToLower(toks[j].Text)
		switch {
		case heightWords[val]:
			x.observeField(e, "height", toks[j].Text, c)
		case buildWords[val]:
			x.observeField(e, "build", toks[j].Text, c)
		case toks[j].Tag == "CD":
			// "was twenty-seven years old" / "turned twenty-seven"
			if lemmaOf(w) == "turn" {
				x.observeField(e, "age", toks[j].Text, c)
			} else if j+2 < len(toks) &&
				strings.ToLower(toks[j+1].Text) == "years" &&
				strings.ToLower(toks[j+2].Text) == "old" {
				x.observeField(e, "age", toks[j].Text, c)
			}
		}
	}
}

// ---- kinship ----

func (x *extractor) kinshipPass(toks []nlp.Token, chars []match.Span) {
	if len(chars) < 2 {
		return
	}
	for i := 0; i < len(chars); i++ {
		for j := 0; j < len(chars); j++ {
			if i == j || chars[i].EntryID == chars[j].EntryID {
				continue
			}
			a, b := x.byID[chars[i].EntryID], x.byID[chars[j].EntryID]
			if a == nil || b == nil {
				continue
			}
			// Pattern 1: A was B's <kin>   ("Torin was Aria's brother")
			if chars[i].Start < chars[j].Start {
				if kin := x.kinAfterPossessive(toks, chars[i], chars[j]); kin != "" {
					x.applyKin(a, b, kin, chars[i])
				}
			}
			// Pattern 2: B's <kin>, A   ("Aria's brother, Torin")
			if chars[j].Start < chars[i].Start {
				if kin := x.kinAppositive(toks, chars[j], chars[i]); kin != "" {
					x.applyKin(a, b, kin, chars[i])
				}
			}
		}
	}
}

// kinAfterPossessive matches: A [copula] B ['s] [kin]
func (x *extractor) kinAfterPossessive(toks []nlp.Token, aSp, bSp match.Span) string {
	// first meaningful token after A must be a copula, and B must follow it
	i := 0
	for i < len(toks) && toks[i].Start < aSp.End {
		i++
	}
	if i >= len(toks) || !extractCopulas[strings.ToLower(toks[i].Text)] {
		return ""
	}
	if toks[i].End > bSp.Start {
		return ""
	}
	// gap between copula and B must be tiny (allow DT: "was the")
	gap := 0
	for k := i + 1; k < len(toks) && toks[k].End <= bSp.Start; k++ {
		gap++
	}
	if gap > 1 {
		return ""
	}
	// after B: 's + kin
	k := 0
	for k < len(toks) && toks[k].Start < bSp.End {
		k++
	}
	if k+1 < len(toks) && toks[k].Tag == "POS" {
		if _, ok := kinship[strings.ToLower(toks[k+1].Text)]; ok {
			return strings.ToLower(toks[k+1].Text)
		}
	}
	return ""
}

// kinAppositive matches: B ['s] [kin] [,]? A   ("Aria's brother Torin")
func (x *extractor) kinAppositive(toks []nlp.Token, bSp, aSp match.Span) string {
	k := 0
	for k < len(toks) && toks[k].Start < bSp.End {
		k++
	}
	if k+1 >= len(toks) || toks[k].Tag != "POS" {
		return ""
	}
	kin := strings.ToLower(toks[k+1].Text)
	if _, ok := kinship[kin]; !ok {
		return ""
	}
	// A must start within 2 tokens after the kin noun.
	gap := 0
	for m := k + 2; m < len(toks) && toks[m].End <= aSp.Start; m++ {
		if toks[m].Tag != "," {
			gap++
		}
	}
	if gap > 0 {
		return ""
	}
	return kin
}

// applyKin records "a is the <kin> of b".
func (x *extractor) applyKin(a, b *model.CodexEntry, kin string, sp match.Span) {
	def := kinship[kin]
	subj, obj := a, b
	if def.reverse {
		subj, obj = b, a
	}
	x.suggestRelation(subj, obj, def.rel, sp, kin)
}

// ---- ownership / membership ----

func (x *extractor) ownershipPass(toks []nlp.Token, chars []match.Span, mentions []match.Span) {
	for _, c := range chars {
		i := 0
		for i < len(toks) && toks[i].Start < c.End {
			i++
		}
		for i < len(toks) && toks[i].Tag == "RB" {
			i++
		}
		if i >= len(toks) || !nlp.IsVerb(toks[i].Tag) {
			continue
		}
		lem := lemmaOf(toks[i].Text)
		var rel string
		switch {
		case ownLemmas[lem]:
			rel = "owns"
		case joinLemmas[lem]:
			rel = "member-of"
		default:
			continue
		}
		// object: next codex mention within 3 tokens of the verb
		for _, m := range mentions {
			if m.EntryID == c.EntryID || m.Start < toks[i].End {
				continue
			}
			gap := 0
			for k := i + 1; k < len(toks) && toks[k].End <= m.Start; k++ {
				gap++
			}
			if gap <= 2 {
				subj, obj := x.byID[c.EntryID], x.byID[m.EntryID]
				if subj != nil && obj != nil {
					x.suggestRelation(subj, obj, rel, c, toks[i].Text)
				}
			}
			break
		}
	}
}

// ---- aliases ----

// aliasPass finds appositive epithets: `Aria, the Ember Witch, ...` and
// "known as the Ember Witch".
func (x *extractor) aliasPass(toks []nlp.Token, chars []match.Span) {
	knownNames := map[string]bool{}
	for i := range x.ws.Codex {
		for _, n := range x.ws.Codex[i].Names() {
			knownNames[strings.ToLower(n)] = true
		}
	}
	capitalized := func(t nlp.Token) bool {
		r := []rune(t.Text)
		return len(r) > 0 && unicode.IsUpper(r[0])
	}
	tryPhrase := func(e *model.CodexEntry, from int, sp match.Span) {
		if from >= len(toks) || strings.ToLower(toks[from].Text) != "the" {
			return
		}
		var words []string
		for k := from + 1; k < len(toks) && len(words) < 3; k++ {
			if !capitalized(toks[k]) {
				break
			}
			words = append(words, toks[k].Text)
		}
		if len(words) == 0 {
			return
		}
		phrase := "the " + strings.Join(words, " ")
		if knownNames[strings.ToLower(phrase)] {
			return
		}
		x.add(Suggestion{
			Kind: "alias", EntryID: e.ID, Name: phrase,
			Start: sp.Start, End: sp.End,
			Message: fmt.Sprintf("%s is called %q here — add it as an alias so mentions of it get linked?", e.Name, phrase),
			Key:     "alias|" + e.ID + "|" + strings.ToLower(phrase),
		})
	}

	for _, c := range chars {
		e := x.byID[c.EntryID]
		i := 0
		for i < len(toks) && toks[i].Start < c.End {
			i++
		}
		// `Name, the Epithet`
		if i < len(toks) && toks[i].Tag == "," {
			tryPhrase(e, i+1, c)
		}
		// `Name, known as the Epithet` / `Name ... called the Epithet`
		for k := i; k < len(toks) && k < i+6; k++ {
			w := strings.ToLower(toks[k].Text)
			if w == "known" && k+1 < len(toks) && strings.ToLower(toks[k+1].Text) == "as" {
				tryPhrase(e, k+2, c)
			}
			if lemmaOf(w) == "call" {
				j := k + 1
				if j < len(toks) && toks[j].Tag == "PRP" {
					j++
				}
				tryPhrase(e, j, c)
			}
		}
	}
}
