// Mirrors the Go types in internal/model and the App bindings.

export type WorkspaceKind = 'novel' | 'series'

/** A codex entry type id, defined by the workspace schema. */
export type EntryType = string

export interface TypeDef {
  id: string
  label: string
  icon: string
  fields: string[] | null
}

export interface RelationDef {
  id: string
  label: string
  inverseLabel: string
  symmetric: boolean
}

export interface Schema {
  types: TypeDef[]
  relations: RelationDef[]
}

export interface Relation {
  type: string
  to: string
  from?: StoryPoint
  until?: StoryPoint
  note?: string
}

export interface Manifest {
  name: string
  kind: WorkspaceKind
  books: string[]
}

export interface Book {
  id: string
  title: string
  chapters: string[] | null
  plan: ChapterPlan[] | null
}

/** One chapter's planning card (stored in the book's plan.yaml). */
export interface ChapterPlan {
  file: string
  synopsis: string
  /** outlined | drafted | revised | final */
  status: string
  /** codex entry ids */
  pov: string
  location: string
  /** freeform in-world time; ISO-ish values sort in chronology view */
  when: string
  arcs: string[] | null
}

export const PLAN_STATUSES = ['outlined', 'drafted', 'revised', 'final'] as const

/** Derived (not stored) planning data for one chapter. */
export interface ChapterInsight {
  cast: string[]
  words: number
}

/** One book's card in the series plan. */
export interface SeriesBookPlan {
  id: string
  synopsis: string
  status: string
  arcs: string[] | null
  targetWords: number
}

export interface SeriesPlan {
  synopsis: string
  books: SeriesBookPlan[] | null
}

export interface StoryPoint {
  book?: string
  chapter?: string
}

export interface StatusChange {
  state: string
  at: StoryPoint
  note?: string
}

export interface CodexEntry {
  id: string
  name: string
  type: EntryType
  aliases: string[] | null
  summary: string
  details: string
  fields: Record<string, string> | null
  status: StatusChange[] | null
  relations: Relation[] | null
  /** 'series' or a book id */
  scope: string
}

export interface Workspace {
  path: string
  manifest: Manifest
  schema: Schema
  books: Book[] | null
  codex: CodexEntry[] | null
  seriesPlan: SeriesPlan
}

export interface Span {
  entryId: string
  /** rune offset */
  start: number
  end: number
  text: string
}

export type Severity = 'error' | 'warning' | 'info'

export interface Flag {
  entryId: string
  start: number
  end: number
  severity: Severity
  rule: string
  message: string
}

/** A fact detected in the manuscript that the codex doesn't record yet. */
export interface Suggestion {
  kind: 'status' | 'relation' | 'entity' | 'field' | 'alias'
  entryId?: string
  targetId?: string
  state?: string
  relation?: string
  /** 'entity': the unknown proper name found by NER; 'alias': the new alias */
  name?: string
  /** for kind === 'field': e.g. "hair", "gender" */
  fieldKey?: string
  fieldValue?: string
  start: number
  end: number
  message: string
  /** position-independent dedup/dismissal key */
  key: string
}

export interface Misspelling {
  word: string
  /** rune offsets */
  start: number
  end: number
}

export interface ScanResult {
  spans: Span[]
  flags: Flag[]
  suggestions: Suggestion[]
  misspellings: Misspelling[]
}

export interface CreateChapterResult {
  workspace: Workspace
  chapter: string
}

/** Persisted application settings (app-level, not per-workspace). */
export interface Settings {
  deepEnabled: boolean
  deepModel: string
  modelsDir: string
  /** editor text column width in characters; 0 = full pane width */
  editorWidth: number
  /** "serif" | "sans" | "mono" | custom font-family name */
  editorFont: string
  editorFontSize: number
  editorLineHeight: number
  editorLineNumbers: boolean
  editorSpellcheck: boolean
  /** spellcheck dictionary language, e.g. "en_US" */
  spellcheckLang: string
  recent: string[]
}

/** One relationship prepared for display. */
export interface RelationDisplay {
  /** e.g. "married to", "child of" */
  label: string
  targetId: string
  targetName: string
  /** e.g. "until The Ember Crown, the-battle-of-cinders" */
  timespan: string
  note: string
  /** story ordinal where the relation begins (0 = from the start) */
  fromPos: number
  /** story ordinal where it ends (Infinity = ongoing) */
  untilPos: number
}

export interface CardStatusLine {
  state: string
  /** e.g. "The Ember Crown, the-battle-of-cinders" or "from the start" */
  anchor: string
  note: string
  current: boolean
}

/**
 * Everything a hover card needs, already filtered to a story position:
 * only the state and relationships in effect at the chapter being edited,
 * with the rest available behind "Show more".
 */
export interface CardData {
  entry: CodexEntry
  typeLabel: string
  /** the entity's state at this point in the story, if it has a timeline */
  state: CardStatusLine | null
  activeRelations: RelationDisplay[]
  /** relations that ended before, or begin after, this point */
  inactiveRelations: RelationDisplay[]
  /** the full status timeline, for the expanded view */
  statusTimeline: CardStatusLine[]
}
