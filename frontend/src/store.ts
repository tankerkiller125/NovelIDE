// Central reactive state for the IDE, shared via composables (no Pinia
// needed at this size).
import { computed, reactive } from 'vue'
import {
  AIApplyProposal,
  AIDiscardProposal,
  OpenWorkspace,
  ReadImageDataURL,
  SyncLinkPull,
  SyncNow,
  SyncStatusGet,
} from './api'
import type { SyncOutcome } from './types'
import type {
  AIProposal,
  CardData,
  CardStatusLine,
  CodexEntry,
  Flag,
  RelationDef,
  RelationDisplay,
  Settings,
  StoryPoint,
  Suggestion,
  SyncResult,
  SyncStatus,
  TimelinedFieldNow,
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

export interface ExportTab {
  kind: 'export'
  pinned?: boolean
}

export interface GraphTab {
  kind: 'graph'
  pinned?: boolean
}

export interface CorkboardTab {
  kind: 'corkboard'
  bookId: string
  pinned?: boolean
}

export interface TimelineTab {
  kind: 'timeline'
  pinned?: boolean
}

export interface SearchTab {
  kind: 'search'
  pinned?: boolean
}

export interface HistoryTab {
  kind: 'history'
  pinned?: boolean
}

export type Tab =
  | ChapterTab
  | CodexTab
  | SchemaTab
  | SettingsTab
  | PlanTab
  | SeriesPlanTab
  | ExportTab
  | GraphTab
  | CorkboardTab
  | TimelineTab
  | SearchTab
  | HistoryTab

export const tabKey = (t: Tab): string =>
  t.kind === 'chapter'
    ? `chapter:${t.bookId}/${t.chapter}`
    : t.kind === 'codex'
      ? `codex:${t.entryId || '~new~'}`
      : t.kind === 'plan'
        ? `plan:${t.bookId}`
        : t.kind === 'corkboard'
          ? `corkboard:${t.bookId}`
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
  /** distraction-free mode: hide sidebar and consistency panel */
  focusMode: boolean
  /** bumped after a chapter saves, so the stats bar can refresh promptly */
  statsTick: number
  /** a requested cursor jump, consumed by the target chapter's editor */
  pendingJump: { bookId: string; chapter: string; line: number } | null
  /** bumped when chapter files change on disk (e.g. project replace), so the
   *  open editor reloads its content */
  reloadTick: number
  /** optional-sync status (null until first fetched) */
  sync: SyncStatus | null
  /** most recent batch of external filesystem changes; seq bumps each time so
   *  the open editor can react */
  externalChange: { modified: string[]; structural: string[]; seq: number }
  /** whether the AI chat side panel is open */
  aiPanelOpen: boolean
  /** AI-proposed edits awaiting approval (prose ones render inline in the editor) */
  proposals: AIProposal[]
  /** id of a prose proposal the editor should scroll to (set by "Review in editor") */
  reviewTarget: string | null
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
  focusMode: false,
  statsTick: 0,
  pendingJump: null,
  reloadTick: 0,
  sync: null,
  externalChange: { modified: [], structural: [], seq: 0 },
  aiPanelOpen: false,
  proposals: [],
  reviewTarget: null,
})

/** Queue an AI-proposed edit (from the ai:proposal event). */
export function addProposal(p: AIProposal) {
  if (!state.proposals.some((q) => q.id === p.id)) state.proposals.push(p)
}

/** Drop a proposal from the local queue. */
export function removeProposal(id: string) {
  state.proposals = state.proposals.filter((p) => p.id !== id)
}

/** Apply a proposal on the backend (used for codex/plan, and prose as a
 *  fallback), refresh the workspace, and clear it from the queue. */
export async function applyProposal(id: string) {
  const ws = await AIApplyProposal(id)
  setWorkspace(ws)
  removeProposal(id)
}

/** Discard a proposal both server-side and locally. Also used after an inline
 *  prose accept, where the editor already applied the text itself. */
export function discardProposal(id: string) {
  void AIDiscardProposal(id)
  removeProposal(id)
}

/** Open the chapter a prose proposal targets so its inline edit is visible, and
 *  ask its editor to scroll the edit into view. */
export function reviewProposal(p: AIProposal) {
  if (p.kind !== 'prose' || !p.bookId || !p.chapter) return
  const tab: Tab = { kind: 'chapter', bookId: p.bookId, chapter: p.chapter }
  openTab(tab)
  pinTab(tabKey(tab))
  state.reviewTarget = p.id
}

export const codexById = computed(() => {
  const map = new Map<string, CodexEntry>()
  for (const e of state.workspace?.codex ?? []) map.set(e.id, e)
  return map
})

// Codex entry images, lazily fetched as data URLs and cached (reactive so
// views update when a fetch completes).
export const imageCache = reactive(new Map<string, string>())

export async function ensureImage(rel: string) {
  if (!rel || imageCache.has(rel)) return
  imageCache.set(rel, '') // in-flight marker prevents duplicate fetches
  try {
    imageCache.set(rel, await ReadImageDataURL(rel))
  } catch {
    imageCache.delete(rel) // allow a later retry
  }
}

/** Cached data URL for an image path, or undefined if not (yet) loaded. */
export function imageURL(rel: string): string | undefined {
  return (rel ? imageCache.get(rel) : '') || undefined
}

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

export function storyPointLabel(p?: StoryPoint): string {
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
export const storyOrder = computed(() => {
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
export function positionOf(p?: StoryPoint): number {
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

  const timelinedFields = resolveTimelinedFields(entry, here)

  if (entry.image) void ensureImage(entry.image)
  return {
    entry,
    typeLabel: typeDefById.value.get(entry.type)?.label ?? entry.type,
    imageUrl: imageURL(entry.image),
    state: currentIdx >= 0 ? statusTimeline[currentIdx] : null,
    activeRelations,
    inactiveRelations,
    statusTimeline,
    timelinedFields,
  }
}

// resolveTimelinedFields picks each timelined field's value in effect at the
// reading position `here` (the most recent value anchored at/before it), and
// keeps the full history for the expanded view. When the position is unknown
// (here < 0) it falls back to the latest value so the card isn't blank.
function resolveTimelinedFields(entry: CodexEntry, here: number): TimelinedFieldNow[] {
  const out: TimelinedFieldNow[] = []
  const timelines = entry.fieldTimelines ?? {}
  for (const key of Object.keys(timelines)) {
    const vals = timelines[key] ?? []
    if (!vals.length) continue
    let bestPos = -Infinity
    let bestIdx = -1
    vals.forEach((v, i) => {
      const p = positionOf(v.at)
      if (p < 0) return
      if (here >= 0 && p > here) return // future — don't spoil
      if (p >= bestPos) {
        bestPos = p
        bestIdx = i
      }
    })
    const history = vals.map((v, i) => ({
      value: v.value,
      anchor: storyPointLabel(v.at) || 'from the start',
      current: i === bestIdx,
    }))
    out.push({
      key,
      value: bestIdx >= 0 ? vals[bestIdx].value : '',
      anchor: bestIdx >= 0 ? storyPointLabel(vals[bestIdx].at) || 'from the start' : '',
      history,
    })
  }
  return out
}

export function setWorkspace(ws: Workspace) {
  state.workspace = ws
  // Seed dismissed suggestions from the (persisted, synced) workspace. Union
  // with any set this session so an in-flight local dismissal isn't dropped by
  // a concurrent reload before it has persisted.
  state.dismissedSuggestions = new Set([
    ...(ws.dismissed ?? []),
    ...state.dismissedSuggestions,
  ])
  // Preload codex images so hover cards and the codex editor show them.
  for (const e of ws.codex ?? []) if (e.image) void ensureImage(e.image)
  // Drop tabs pointing at things that no longer exist.
  const entryIds = new Set((ws.codex ?? []).map((e) => e.id))
  state.tabs = state.tabs.filter((t) => {
    if (t.kind === 'chapter')
      return (ws.books ?? []).some(
        (b) => b.id === t.bookId && (b.chapters ?? []).includes(t.chapter),
      )
    if (t.kind === 'codex') return t.entryId === '' || entryIds.has(t.entryId)
    if (t.kind === 'plan' || t.kind === 'corkboard')
      return (ws.books ?? []).some((b) => b.id === t.bookId)
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

/**
 * Open a chapter and request the cursor jump to a line (1-based). The chapter's
 * editor consumes `pendingJump` when it mounts or, if already open, reacts to
 * the change. Pinned so a search result the author is inspecting stays open.
 */
export function openChapterAtLine(bookId: string, chapter: string, line: number) {
  state.pendingJump = { bookId, chapter, line }
  const tab: Tab = { kind: 'chapter', bookId, chapter }
  openTab(tab)
  pinTab(tabKey(tab))
}

/**
 * Handle a batch of external filesystem changes (from the fs:changed event).
 * Reloads workspace metadata when the structure or a codex/plan/schema file
 * changed, and signals the open chapter editor to reconcile any changed
 * manuscript file it holds.
 */
export async function handleExternalChange(modified: string[], structural: string[]) {
  state.externalChange = { modified, structural, seq: state.externalChange.seq + 1 }
  const yamlChanged = [...modified, ...structural].some(
    (p) => p.endsWith('.yaml') || p.endsWith('.yml'),
  )
  if ((structural.length > 0 || yamlChanged) && state.workspace) {
    try {
      setWorkspace(await OpenWorkspace(state.workspace.path))
    } catch (e) {
      console.error('reload after external change failed', e)
    }
  }
}

/** Refresh the optional-sync status (safe no-op if unconfigured). */
export async function refreshSyncStatus() {
  try {
    state.sync = await SyncStatusGet()
  } catch {
    state.sync = null
  }
}

// Apply a sync outcome: adopt the refreshed workspace, nudge the open editor to
// re-read pulled files, and refresh status. Throws on failure so callers can
// surface the message.
async function applySyncOutcome(p: Promise<SyncOutcome>): Promise<SyncResult> {
  const outcome = await p
  setWorkspace(outcome.workspace)
  state.reloadTick++
  await refreshSyncStatus()
  return outcome.result
}

/** Sync the open workspace (auto-links it on first run). */
export const syncNow = (): Promise<SyncResult> => applySyncOutcome(SyncNow())

/** Link the open workspace to an existing remote and pull it. */
export const syncLinkPull = (remoteId: string): Promise<SyncResult> =>
  applySyncOutcome(SyncLinkPull(remoteId))

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

// ---- Modal dialogs (real replacements for window.prompt / confirm) ----
interface ModalState {
  kind: 'prompt' | 'confirm' | null
  title: string
  message: string
  label: string
  value: string
  placeholder: string
  confirmText: string
  danger: boolean
  resolve: ((v: unknown) => void) | null
}

export const modal = reactive<ModalState>({
  kind: null,
  title: '',
  message: '',
  label: '',
  value: '',
  placeholder: '',
  confirmText: 'OK',
  danger: false,
  resolve: null,
})

/** Ask for a line of text. Resolves to the trimmed value, or null if cancelled. */
export function promptInput(o: {
  title: string
  label?: string
  value?: string
  placeholder?: string
  confirmText?: string
}): Promise<string | null> {
  return new Promise((resolve) => {
    Object.assign(modal, {
      kind: 'prompt',
      title: o.title,
      message: '',
      label: o.label ?? '',
      value: o.value ?? '',
      placeholder: o.placeholder ?? '',
      confirmText: o.confirmText ?? 'OK',
      danger: false,
      resolve: resolve as (v: unknown) => void,
    })
  })
}

/** Ask a yes/no question. Resolves true (confirmed) or false (cancelled). */
export function confirmDialog(o: {
  title: string
  message: string
  confirmText?: string
  danger?: boolean
}): Promise<boolean> {
  return new Promise((resolve) => {
    Object.assign(modal, {
      kind: 'confirm',
      title: o.title,
      message: o.message,
      confirmText: o.confirmText ?? 'OK',
      danger: o.danger ?? false,
      resolve: resolve as (v: unknown) => void,
    })
  })
}

export function resolveModal(result: unknown) {
  const r = modal.resolve
  modal.kind = null
  modal.resolve = null
  if (r) r(result)
}

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
