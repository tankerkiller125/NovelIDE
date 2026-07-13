// CodeMirror 6 extension: entity mention highlighting, consistency-flag
// underlines, and hover cards backed by the Codex.
import { StateEffect, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, EditorView, hoverTooltip } from '@codemirror/view'
import type { DecorationSet, Tooltip } from '@codemirror/view'
import type {
  CardData,
  CardStatusLine,
  CodexEntry,
  Flag,
  Misspelling,
  RelationDisplay,
  ScanResult,
  Span,
} from '../types'

/**
 * Go reports rune (code point) offsets; CodeMirror uses UTF-16 code units.
 * Returns a function converting rune offset -> UTF-16 offset for `text`.
 */
export function runeToUtf16(text: string): (rune: number) => number {
  // Fast path: no astral characters means offsets are identical.
  let hasAstral = false
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i)
    if (c >= 0xd800 && c <= 0xdbff) {
      hasAstral = true
      break
    }
  }
  if (!hasAstral) return (r) => r
  const map: number[] = []
  let utf16 = 0
  for (const ch of text) {
    map.push(utf16)
    utf16 += ch.length
  }
  map.push(utf16)
  return (r) => map[Math.min(r, map.length - 1)] ?? utf16
}

export interface ScanState {
  spans: Span[]
  flags: Flag[]
  /** UTF-16 converted ranges aligned with spans/flags/misspellings */
  spanRanges: { from: number; to: number; span: Span }[]
  flagRanges: { from: number; to: number; flag: Flag }[]
  spellRanges: { from: number; to: number; word: string }[]
}

const emptyScan: ScanState = {
  spans: [],
  flags: [],
  spanRanges: [],
  flagRanges: [],
  spellRanges: [],
}

export const setScanResult = StateEffect.define<ScanState>()

export function toScanState(result: ScanResult, docText: string): ScanState {
  const conv = runeToUtf16(docText)
  const len = docText.length
  const clamp = (n: number) => Math.max(0, Math.min(n, len))
  return {
    spans: result.spans,
    flags: result.flags,
    spanRanges: result.spans.map((s) => ({
      from: clamp(conv(s.start)),
      to: clamp(conv(s.end)),
      span: s,
    })),
    flagRanges: result.flags.map((f) => ({
      from: clamp(conv(f.start)),
      to: clamp(conv(f.end)),
      flag: f,
    })),
    spellRanges: (result.misspellings ?? []).map((m: Misspelling) => ({
      from: clamp(conv(m.start)),
      to: clamp(conv(m.end)),
      word: m.word,
    })),
  }
}

export const scanField = StateField.define<ScanState>({
  create: () => emptyScan,
  update(value, tr) {
    for (const e of tr.effects) {
      if (e.is(setScanResult)) return e.value
    }
    if (tr.docChanged) {
      // Shift stored ranges so highlights track edits until the next scan.
      const mapRange = <T extends { from: number; to: number }>(r: T): T | null => {
        const from = tr.changes.mapPos(r.from, 1)
        const to = tr.changes.mapPos(r.to, -1)
        return from < to ? { ...r, from, to } : null
      }
      return {
        ...value,
        spanRanges: value.spanRanges.map(mapRange).filter((r): r is NonNullable<typeof r> => !!r),
        flagRanges: value.flagRanges.map(mapRange).filter((r): r is NonNullable<typeof r> => !!r),
        spellRanges: value.spellRanges
          .map(mapRange)
          .filter((r): r is NonNullable<typeof r> => !!r),
      }
    }
    return value
  },
})

const decoField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(_deco, tr) {
    const scan = tr.state.field(scanField)
    type Entry = { from: number; to: number; deco: Decoration; prio: number }
    const entries: Entry[] = []
    for (const r of scan.spanRanges) {
      entries.push({
        from: r.from,
        to: r.to,
        prio: 0,
        deco: Decoration.mark({
          class: 'nv-entity',
          attributes: { 'data-entry-id': r.span.entryId },
        }),
      })
    }
    for (const r of scan.flagRanges) {
      entries.push({
        from: r.from,
        to: r.to,
        prio: 1,
        deco: Decoration.mark({ class: `nv-flag nv-flag-${r.flag.severity}` }),
      })
    }
    for (const r of scan.spellRanges) {
      entries.push({
        from: r.from,
        to: r.to,
        prio: 2,
        deco: Decoration.mark({ class: 'nv-misspell' }),
      })
    }
    entries.sort((a, b) => a.from - b.from || a.prio - b.prio || a.to - b.to)
    const builder = new RangeSetBuilder<Decoration>()
    for (const e of entries) builder.add(e.from, e.to, e.deco)
    return builder.finish()
  },
  provide: (f) => EditorView.decorations.from(f),
})

export interface EntityHoverContext {
  getEntry: (id: string) => CodexEntry | undefined
  /** story-position-filtered card data for the chapter being edited */
  getCard?: (id: string) => CardData | null
  onOpenEntry?: (id: string) => void
  /** fetch spelling suggestions for a misspelled word */
  getSpellSuggestions?: (word: string) => Promise<string[]>
  /** add a word to the personal dictionary (caller should rescan after) */
  onAddWord?: (word: string) => void | Promise<void>
}

function severityLabel(sev: string): string {
  return sev === 'error' ? 'Consistency error' : sev === 'warning' ? 'Warning' : 'Note'
}

const goneStates = new Set(['dead', 'deceased', 'destroyed', 'killed', 'missing', 'lost'])

function relationRow(parent: HTMLElement, r: RelationDisplay, showTime: boolean) {
  const row = parent.appendChild(document.createElement('div'))
  row.className = 'nv-card-relation'
  const label = row.appendChild(document.createElement('span'))
  label.className = 'nv-card-rel-label'
  label.textContent = r.label
  row.appendChild(document.createTextNode(' ' + r.targetName))
  if (showTime && r.timespan) {
    const ts = row.appendChild(document.createElement('span'))
    ts.className = 'nv-card-rel-time'
    ts.textContent = ` (${r.timespan})`
  }
  return row
}

function statusRow(parent: HTMLElement, s: CardStatusLine) {
  const row = parent.appendChild(document.createElement('div'))
  row.className = 'nv-card-status-line' + (s.current ? ' current' : '')
  const dot = row.appendChild(document.createElement('span'))
  dot.className = 'nv-card-status-dot ' + (goneStates.has(s.state.toLowerCase()) ? 'gone' : 'ok')
  dot.textContent = '●'
  row.appendChild(document.createTextNode(` ${s.state} — ${s.anchor}`))
  if (s.note) {
    const note = row.appendChild(document.createElement('span'))
    note.className = 'nv-card-rel-time'
    note.textContent = ` (${s.note})`
  }
}

function renderCard(ctx: EntityHoverContext, entryId: string, flags: Flag[]): HTMLElement {
  const card = document.createElement('div')
  card.className = 'nv-card'
  const data = ctx.getCard?.(entryId)
  const entry = data?.entry ?? ctx.getEntry(entryId)
  if (!entry) {
    card.textContent = 'Unknown codex entry'
    return card
  }
  const head = card.appendChild(document.createElement('div'))
  head.className = 'nv-card-head'
  const name = head.appendChild(document.createElement('span'))
  name.className = 'nv-card-name'
  name.textContent = entry.name
  const type = head.appendChild(document.createElement('span'))
  type.className = `nv-card-type nv-type-${entry.type}`
  type.textContent = data?.typeLabel ?? entry.type
  if (entry.scope && entry.scope !== 'series') {
    const scope = head.appendChild(document.createElement('span'))
    scope.className = 'nv-card-scope'
    scope.textContent = `book: ${entry.scope}`
  }
  if (entry.aliases?.length) {
    const aka = card.appendChild(document.createElement('div'))
    aka.className = 'nv-card-aliases'
    aka.textContent = `a.k.a. ${entry.aliases.join(', ')}`
  }

  if (data?.imageUrl) {
    const img = card.appendChild(document.createElement('img'))
    img.className = 'nv-card-image'
    img.src = data.imageUrl
    img.alt = entry.name
  }

  // Current state at this point in the story — never future knowledge.
  if (data?.state) {
    const st = card.appendChild(document.createElement('div'))
    st.className =
      'nv-card-state ' + (goneStates.has(data.state.state.toLowerCase()) ? 'gone' : 'ok')
    st.textContent = data.state.state
  }

  if (entry.summary) {
    const sum = card.appendChild(document.createElement('div'))
    sum.className = 'nv-card-summary'
    sum.textContent = entry.summary
  }
  const fields = Object.entries(entry.fields ?? {})
  const timelined = (data?.timelinedFields ?? []).filter((f) => f.value !== '')
  if (fields.length || timelined.length) {
    const dl = card.appendChild(document.createElement('div'))
    dl.className = 'nv-card-fields'
    for (const [k, v] of fields.slice(0, 6)) {
      const row = dl.appendChild(document.createElement('div'))
      row.className = 'nv-card-field'
      const key = row.appendChild(document.createElement('span'))
      key.className = 'nv-card-field-key'
      key.textContent = k
      row.appendChild(document.createTextNode(v))
    }
    // Timelined facts, resolved to their value at this point in the story.
    for (const f of timelined.slice(0, 6)) {
      const row = dl.appendChild(document.createElement('div'))
      row.className = 'nv-card-field'
      const key = row.appendChild(document.createElement('span'))
      key.className = 'nv-card-field-key'
      key.textContent = f.key
      row.appendChild(document.createTextNode(f.value))
      const clock = row.appendChild(document.createElement('span'))
      clock.className = 'nv-card-field-clock'
      clock.textContent = ' 🕐'
      clock.title = `as of ${f.anchor}`
    }
  }

  // Only relationships in effect at this chapter.
  const active = data?.activeRelations ?? []
  if (active.length) {
    const rl = card.appendChild(document.createElement('div'))
    rl.className = 'nv-card-relations'
    for (const r of active.slice(0, 6)) relationRow(rl, r, false)
    if (active.length > 6) {
      const more = rl.appendChild(document.createElement('div'))
      more.className = 'nv-card-rel-more'
      more.textContent = `+ ${active.length - 6} more`
    }
  }

  for (const f of flags) {
    const fl = card.appendChild(document.createElement('div'))
    fl.className = `nv-card-flag nv-card-flag-${f.severity}`
    fl.textContent = `${severityLabel(f.severity)}: ${f.message}`
  }

  // Everything time-filtered away lives behind "Show more": the full status
  // timeline, field histories, and past/future relationships. Beware of
  // spoiling yourself.
  const hiddenStatus = (data?.statusTimeline ?? []).length
  const hiddenRels = data?.inactiveRelations ?? []
  const fieldHistories = (data?.timelinedFields ?? []).filter((f) => f.history.length > 1)
  if (hiddenStatus > 0 || hiddenRels.length > 0 || fieldHistories.length > 0) {
    const moreWrap = card.appendChild(document.createElement('div'))
    moreWrap.className = 'nv-card-more'
    moreWrap.style.display = 'none'
    if (hiddenStatus > 0) {
      const h = moreWrap.appendChild(document.createElement('div'))
      h.className = 'nv-card-more-head'
      h.textContent = 'Full timeline'
      for (const s of data!.statusTimeline) statusRow(moreWrap, s)
    }
    for (const f of fieldHistories) {
      const h = moreWrap.appendChild(document.createElement('div'))
      h.className = 'nv-card-more-head'
      h.textContent = f.key
      for (const v of f.history) {
        const row = moreWrap.appendChild(document.createElement('div'))
        row.className = 'nv-card-status-line' + (v.current ? ' current' : '')
        row.appendChild(document.createTextNode(`${v.value} — ${v.anchor}`))
      }
    }
    if (hiddenRels.length > 0) {
      const h = moreWrap.appendChild(document.createElement('div'))
      h.className = 'nv-card-more-head'
      h.textContent = 'Past & future relationships'
      for (const r of hiddenRels) relationRow(moreWrap, r, true).classList.add('inactive')
    }
    const toggle = card.appendChild(document.createElement('button'))
    toggle.className = 'nv-card-open'
    toggle.textContent = 'Show more ▾'
    toggle.addEventListener('mousedown', (ev) => {
      ev.preventDefault()
      const open = moreWrap.style.display !== 'none'
      moreWrap.style.display = open ? 'none' : 'block'
      toggle.textContent = open ? 'Show more ▾' : 'Show less ▴'
    })
    card.appendChild(moreWrap) // keep the expander above the codex link
  }

  if (ctx.onOpenEntry) {
    const link = card.appendChild(document.createElement('button'))
    link.className = 'nv-card-open'
    link.textContent = 'Open in Codex →'
    link.addEventListener('mousedown', (ev) => {
      ev.preventDefault()
      ctx.onOpenEntry?.(entry.id)
    })
  }
  return card
}

function entityHover(ctx: EntityHoverContext) {
  return hoverTooltip(
    (view, pos): Tooltip | null => {
      const scan = view.state.field(scanField)
      const hit = scan.spanRanges.find((r) => pos >= r.from && pos <= r.to)
      if (!hit) return null
      const flags = scan.flagRanges
        .filter((r) => r.from < hit.to && r.to > hit.from)
        .map((r) => r.flag)
      return {
        pos: hit.from,
        end: hit.to,
        above: true,
        create: () => ({ dom: renderCard(ctx, hit.span.entryId, flags) }),
      }
    },
    { hoverTime: 220 },
  )
}

function spellHover(ctx: EntityHoverContext) {
  return hoverTooltip(
    (view, pos): Tooltip | null => {
      if (!ctx.getSpellSuggestions) return null
      const scan = view.state.field(scanField)
      const hit = scan.spellRanges.find((r) => pos >= r.from && pos <= r.to)
      if (!hit) return null
      return {
        pos: hit.from,
        end: hit.to,
        above: true,
        create: (tipView) => {
          const dom = document.createElement('div')
          dom.className = 'nv-card nv-spell-card'
          const head = dom.appendChild(document.createElement('div'))
          head.className = 'nv-spell-word'
          head.textContent = `"${hit.word}"`
          const list = dom.appendChild(document.createElement('div'))
          list.className = 'nv-spell-suggestions'
          list.textContent = 'Checking…'
          ctx
            .getSpellSuggestions!(hit.word)
            .then((suggestions) => {
              list.textContent = ''
              if (!suggestions.length) {
                list.textContent = 'No suggestions'
                list.className += ' none'
              }
              for (const s of suggestions) {
                const b = list.appendChild(document.createElement('button'))
                b.className = 'nv-spell-suggestion'
                b.textContent = s
                b.addEventListener('mousedown', (ev) => {
                  ev.preventDefault()
                  tipView.dispatch({ changes: { from: hit.from, to: hit.to, insert: s } })
                  tipView.focus()
                })
              }
            })
            .catch(() => {
              list.textContent = 'No suggestions'
            })
          if (ctx.onAddWord) {
            const add = dom.appendChild(document.createElement('button'))
            add.className = 'nv-card-open'
            add.textContent = 'Add to dictionary'
            add.addEventListener('mousedown', (ev) => {
              ev.preventDefault()
              ctx.onAddWord?.(hit.word)
            })
          }
          return { dom }
        },
      }
    },
    { hoverTime: 220 },
  )
}

export function entityExtension(ctx: EntityHoverContext) {
  return [scanField, decoField, entityHover(ctx), spellHover(ctx)]
}
