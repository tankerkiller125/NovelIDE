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

/** One scene within a chapter. Index 0 is the immovable opening. */
export interface Scene {
  index: number
  title: string
  words: number
  snippet: string
}

/** A chapter's scenes, for the corkboard. */
export interface ChapterScenes {
  chapter: string
  title: string
  scenes: Scene[]
}

/** Which sign-in methods a sync server offers. */
export interface AuthConfig {
  passwordEnabled: boolean
  ssoEnabled: boolean
  ssoName?: string
}

// --- optional AI configuration ---
export type AIProviderKind = 'openai' | 'anthropic' | 'gemini' | 'a2a' | 'acp'

/** A locally-installed ACP coding agent detected on this machine. */
export interface ACPAgent {
  id: string
  label: string
}

export interface AINamedProvider {
  id: string
  name: string
  kind: AIProviderKind
  /** OpenAI-compatible base URL, Anthropic host, or (for a2a) the remote agent's
   *  card URL. Not used for gemini. */
  baseUrl: string
  apiKey: string
  /** Model ids to offer in the chat model picker (openai/anthropic/gemini). */
  models?: string[]
}

export interface AIConfig {
  enabled: boolean
  providers: AINamedProvider[] | null
}

/** One chat turn sent to the AI runtime. */
export interface AIMessage {
  role: 'user' | 'assistant'
  content: string
  /** Tools the assistant invoked this turn (UI-only; ignored by the backend). */
  tools?: string[]
}

/** An AI-proposed edit awaiting the author's approval. */
export interface AIProposal {
  id: string
  kind: 'codex' | 'plan' | 'prose'
  summary: string
  target: string
  before?: string
  after?: string
  /** prose only: the chapter this edit targets, so the editor can anchor it. */
  bookId?: string
  chapter?: string
}

/** Sync configuration/state reported by the backend. */
export interface SyncStatus {
  configured: boolean
  loggedIn: boolean
  server: string
  username: string
  linked: boolean
  remoteId: string
}

/** What a sync did. */
export interface SyncResult {
  revision: number
  pushed: number
  pulled: number
  deleted: number
  conflicts: string[]
  remoteId: string
}

/** A workspace on the sync server. */
export interface RemoteWorkspace {
  id: string
  name: string
  revision: number
  updatedAt: string
}

/** Result of a sync plus the refreshed workspace to re-render. */
export interface SyncOutcome {
  result: SyncResult
  workspace: Workspace
}

/** One captured revision of the workspace. */
export interface Snapshot {
  id: string
  time: string
  label: string
  auto: boolean
  files: number
  size: number
}

/** How one file differs between a snapshot and the current workspace. */
export interface FileChange {
  rel: string
  status: 'modified' | 'added' | 'removed'
}

/** One line of a snapshot-vs-current diff. */
export interface DiffLine {
  op: 'eq' | 'add' | 'del'
  text: string
}

export interface DiffResult {
  rel: string
  lines: DiffLine[]
}

/** One occurrence of a search query within a chapter. */
export interface TextMatch {
  line: number
  col: number
  before: string
  match: string
  after: string
}

/** A chapter's matches for the project-wide search view. */
export interface SearchHit {
  bookId: string
  bookTitle: string
  chapter: string
  chapterTitle: string
  matches: TextMatch[]
}

/** The result of a project-wide search. */
export interface SearchResults {
  hits: SearchHit[]
  total: number
}

/** One chapter that mentions a codex entity, for the backlinks panel. */
export interface Backlink {
  bookId: string
  bookTitle: string
  chapter: string
  chapterTitle: string
  count: number
  snippet: string
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
  /** workspace-relative image path, or "" */
  image: string
  fields: Record<string, string> | null
  /** facts that change over story time (e.g. age), keyed by field name */
  fieldTimelines?: Record<string, TimedValue[]> | null
  status: StatusChange[] | null
  relations: Relation[] | null
  /** 'series' or a book id */
  scope: string
}

/** One value of a timelined field, effective from `at` (empty = from the start). */
export interface TimedValue {
  value: string
  at?: StoryPoint
  note?: string
}

export interface Workspace {
  path: string
  manifest: Manifest
  schema: Schema
  books: Book[] | null
  codex: CodexEntry[] | null
  seriesPlan: SeriesPlan
  /** keys of dismissed codex-gap suggestions (persisted + synced) */
  dismissed: string[] | null
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

export interface RenameChapterResult {
  workspace: Workspace
  chapter: string
}

export interface WritingStats {
  today: string
  todayWords: number
  goal: number
  streak: number
  total: number
}

export type ExportFormat = 'epub' | 'html'

export interface ExportTheme {
  id: string
  label: string
  description: string
}

export interface ExportOptions {
  format: ExportFormat
  themeId: string
  title: string
  author: string
  /** book ids to include; empty = all, in reading order */
  books: string[]
  titlePage: boolean
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
  /** true = always show raw Markdown markers (disables live preview) */
  editorRawMarkup: boolean
  /** spellcheck dictionary language, e.g. "en_US" */
  spellcheckLang: string
  /** optional sync (empty server = off) */
  syncServer: string
  syncUsername: string
  syncToken: string
  syncAccountId: string
  /** optional AI configuration */
  ai: AIConfig
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
  /** data URL for the entry's image, if loaded */
  imageUrl?: string
  /** the entity's state at this point in the story, if it has a timeline */
  state: CardStatusLine | null
  activeRelations: RelationDisplay[]
  /** relations that ended before, or begin after, this point */
  inactiveRelations: RelationDisplay[]
  /** the full status timeline, for the expanded view */
  statusTimeline: CardStatusLine[]
  /** timelined facts resolved to their value at this point in the story */
  timelinedFields: TimelinedFieldNow[]
}

/** A timelined field's value at the current reading position, plus its history. */
export interface TimelinedFieldNow {
  key: string
  value: string
  anchor: string
  history: { value: string; anchor: string; current: boolean }[]
}
