// Central reactive state for the IDE, shared via composables (no Pinia
// needed at this size).
import { computed, reactive } from 'vue'
import type {
  CardData,
  CardStatusLine,
  CodexEntry,
  Flag,
  RelationDef,
  RelationDisplay,
  Settings,
  StoryPoint,
  Suggestion,
  TypeDef,
  Workspace,
} from './types'

// Tabs open in a transient "preview" state (pinned falsy). Navigating away
// from a preview tab closes it; editing it pins it so it stays. This keeps
// browsing from piling up tabs — you only accumulate what you've touched.
export interface ChapterTab {
  kind: 'chapter'
  bookId: string
  chapter: string
  pinned?: boolean
}

export interface CodexTab {
  kind: 'codex'
  /** entry id, or '' for a brand-new entry */
  entryId: string
  /** set when creating a new entry */
  draft?: CodexEntry
  pinned?: boolean
}

export interface SchemaTab {
  kind: 'schema'
  pinned?: boolean
}

export interface SettingsTab {
  kind: 'settings'
  pinned?: boolean
}

export interface PlanTab {
  kind: 'plan'
  bookId: string
  pinned?: boolean
}

export interface SeriesPlanTab {
  kind: 'series-plan'
  pinned?: boolean
}

export type Tab = ChapterTab | CodexTab | SchemaTab | SettingsTab | PlanTab | SeriesPlanTab

export const tabKey = (t: Tab): string =>
  t.kind === 'chapter'
    ? `chapter:${t.bookId}/${t.chapter}`
    : t.kind === 'codex'
      ? `codex:${t.entryId || '~new~'}`
      : t.kind === 'plan'
        ? `plan:${t.bookId}`
        : t.kind

interface State {
  workspace: Workspace | null
  settings: Settings | null
  tabs: Tab[]
  activeTab: string | null
  /** flags from the most recent scan of the active chapter */
  flags: Flag[]
  /** codex-gap suggestions from the most recent scan */
  suggestions: Suggestion[]
  /** suggestion keys the user has dismissed this session */
  dismissedSuggestions: Set<string>
  dirtyChapters: Set<string>
}

export const state = reactive<State>({
  workspace: null,
  settings: null,
  tabs: [],
  activeTab: null,
  flags: [],
  suggestions: [],
  dismissedSuggestions: new Set(),
  dirtyChapters: new Set(),
})

export const codexById = computed(() => {
  const map = new Map<string, CodexEntry>()
  for (const e of state.workspace?.codex ?? []) map.set(e.id, e)
  return map
})

/** Schema types plus any types found on entries but missing from the schema. */
export const schemaTypes = computed<TypeDef[]>(() => {
  const declared = state.workspace?.schema?.types ?? []
  const seen = new Set(declared.map((t) => t.id))
  const extra: TypeDef[] = []
  for (const e of state.workspace?.codex ?? []) {
    if (e.type && !seen.has(e.type)) {
      seen.add(e.type)
      extra.push({ id: e.type, label: e.type, icon: '✦', fields: null })
    }
  }
  return [...declared, ...extra]
})

export const typeDefById = computed(() => {
  const map = new Map<string, TypeDef>()
  for (const t of schemaTypes.value) map.set(t.id, t)
  return map
})

export const relationDefById = computed(() => {
  const map = new Map<string, RelationDef>()
  for (const r of state.workspace?.schema?.relations ?? []) map.set(r.id, r)
  return map
})

function storyPointLabel(p?: StoryPoint): string {
  if (!p?.book) return ''
  const book = (state.workspace?.books ?? []).find((b) => b.id === p.book)
  const bookLabel = book?.title ?? p.book
  const ch = p.chapter ? `, ${p.chapter.replace(/\.md$/, '').replace(/^\d+-/, '')}` : ''
  return `${bookLabel}${ch}`
}

/**
 * Global story ordering, mirroring the Go detect.Timeline: every chapter of
 * every book gets an increasing ordinal; "book/" marks a book's start.
 */
const storyOrder = computed(() => {
  const order = new Map<string, number>()
  let pos = 0
  for (const b of state.workspace?.books ?? []) {
    pos++ // book start gets its own ordinal (matches Go detect.NewTimeline)
    order.set(`${b.id}/`, pos)
    for (const ch of b.chapters ?? []) {
      pos++
      order.set(`${b.id}/${ch}`, pos)
    }
  }
  return order
})

/** Ordinal of a story point: 0 = from the start, -1 = unresolvable. */
function positionOf(p?: StoryPoint): number {
  if (!p?.book) return 0
  const order = storyOrder.value
  if (!p.chapter) return order.get(`${p.book}/`) ?? -1
  return order.get(`${p.book}/${p.chapter}`) ?? -1
}

/**
 * All relationships involving an entry: its own outgoing edges plus incoming
 * edges from every other entry, described with the relation's inverse label.
 */
export function relationsOf(entryId: string): RelationDisplay[] {
  const ws = state.workspace
  if (!ws) return []
  const defs = relationDefById.value
  const byId = codexById.value
  const out: RelationDisplay[] = []
  const timespan = (from?: StoryPoint, until?: StoryPoint) => {
    const f = storyPointLabel(from)
    const u = storyPointLabel(until)
    if (f && u) return `${f} → ${u}`
    if (f) return `from ${f}`
    if (u) return `until ${u}`
    return ''
  }
  const bounds = (from?: StoryPoint, until?: StoryPoint) => {
    const fp = positionOf(from)
    const up = until ? positionOf(until) : Infinity
    return { fromPos: fp < 0 ? 0 : fp, untilPos: up < 0 ? Infinity : up }
  }
  for (const e of ws.codex ?? []) {
    for (const r of e.relations ?? []) {
      const def = defs.get(r.type)
      if (e.id === entryId) {
        out.push({
          label: def?.label ?? r.type,
          targetId: r.to,
          targetName: byId.get(r.to)?.name ?? `⚠ unknown: ${r.to}`,
          timespan: timespan(r.from, r.until),
          note: r.note ?? '',
          ...bounds(r.from, r.until),
        })
      } else if (r.to === entryId) {
        out.push({
          label: def?.symmetric
            ? (def?.label ?? r.type)
            : def?.inverseLabel || `← ${def?.label ?? r.type}`,
          targetId: e.id,
          targetName: e.name,
          timespan: timespan(r.from, r.until),
          note: r.note ?? '',
          ...bounds(r.from, r.until),
        })
      }
    }
  }
  return out
}

/**
 * Hover-card data filtered to the story position of the chapter being
 * edited: the state the entity is in *right here*, and only the
 * relationships currently in effect — everything else goes behind
 * "Show more". Mirrors the Go engine's StateAt semantics (changes anchored
 * to the very chapter being edited don't apply yet — the death scene itself
 * shouldn't read "dead").
 */
export function cardDataAt(entryId: string, bookId: string, chapter: string): CardData | null {
  const entry = codexById.value.get(entryId)
  if (!entry) return null
  const here = storyOrder.value.get(`${bookId}/${chapter}`) ?? -1

  const statusTimeline: CardStatusLine[] = (entry.status ?? []).map((sc) => ({
    state: sc.state,
    anchor: storyPointLabel(sc.at) || 'from the start',
    note: sc.note ?? '',
    current: false,
  }))
  let currentIdx = -1
  if (here >= 0) {
    let best = -1
    ;(entry.status ?? []).forEach((sc, i) => {
      const p = positionOf(sc.at)
      if (p < 0 || p > here) return
      if (p === here && sc.at?.chapter) return
      if (p >= best) {
        best = p
        currentIdx = i
      }
    })
  }
  if (currentIdx >= 0) statusTimeline[currentIdx].current = true

  const all = relationsOf(entryId)
  const activeRelations: RelationDisplay[] = []
  const inactiveRelations: RelationDisplay[] = []
  for (const r of all) {
    const active = here < 0 || (r.fromPos <= here && here <= r.untilPos)
    ;(active ? activeRelations : inactiveRelations).push(r)
  }

  return {
    entry,
    typeLabel: typeDefById.value.get(entry.type)?.label ?? entry.type,
    state: currentIdx >= 0 ? statusTimeline[currentIdx] : null,
    activeRelations,
    inactiveRelations,
    statusTimeline,
  }
}

export function setWorkspace(ws: Workspace) {
  state.workspace = ws
  // Drop tabs pointing at things that no longer exist.
  const entryIds = new Set((ws.codex ?? []).map((e) => e.id))
  state.tabs = state.tabs.filter((t) => {
    if (t.kind === 'chapter')
      return (ws.books ?? []).some(
        (b) => b.id === t.bookId && (b.chapters ?? []).includes(t.chapter),
      )
    if (t.kind === 'codex') return t.entryId === '' || entryIds.has(t.entryId)
    if (t.kind === 'plan') return (ws.books ?? []).some((b) => b.id === t.bookId)
    return true
  })
  if (state.activeTab && !state.tabs.some((t) => tabKey(t) === state.activeTab)) {
    state.activeTab = state.tabs.length ? tabKey(state.tabs[0]) : null
  }
}

/**
 * Close the current preview tab when leaving it for a different tab. A
 * preview tab is one the user hasn't edited (pinned falsy). Pinned tabs and
 * the tab being navigated to are left alone.
 */
function closePreviewLeaving(nextKey: string) {
  const cur = state.activeTab
  if (!cur || cur === nextKey) return
  const i = state.tabs.findIndex((t) => tabKey(t) === cur)
  if (i !== -1 && !state.tabs[i].pinned) state.tabs.splice(i, 1)
}

/** Switch to an already-open tab, closing the previous preview tab. */
export function activateTab(key: string) {
  if (state.activeTab === key) return
  closePreviewLeaving(key)
  state.activeTab = key
}

export function openTab(tab: Tab) {
  const key = tabKey(tab)
  if (state.tabs.some((t) => tabKey(t) === key)) {
    activateTab(key)
    return
  }
  // Opening something new: the tab we're leaving, if still a preview,
  // closes; the new one opens as a preview (pinned falsy).
  closePreviewLeaving(key)
  state.tabs.push(tab)
  state.activeTab = key
}

/** Pin a tab so it stops being a self-closing preview. */
export function pinTab(key: string) {
  const t = state.tabs.find((x) => tabKey(x) === key)
  if (t) t.pinned = true
}

/** Pin whatever tab is currently active — called by editors on first edit. */
export function pinActiveTab() {
  if (state.activeTab) pinTab(state.activeTab)
}

export function closeTab(key: string) {
  const i = state.tabs.findIndex((t) => tabKey(t) === key)
  if (i === -1) return
  state.tabs.splice(i, 1)
  if (state.activeTab === key) {
    state.activeTab = state.tabs.length ? tabKey(state.tabs[Math.max(0, i - 1)]) : null
  }
}

export const activeTabObj = computed(
  () => state.tabs.find((t) => tabKey(t) === state.activeTab) ?? null,
)

/** Return to the welcome screen, clearing all workspace-bound state. */
export function closeProject() {
  state.workspace = null
  state.tabs = []
  state.activeTab = null
  state.flags = []
  state.suggestions = []
  state.dismissedSuggestions = new Set()
  state.dirtyChapters = new Set()
}
