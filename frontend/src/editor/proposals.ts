// CodeMirror 6 extension that renders AI prose-edit proposals inline, at the
// exact spot in the manuscript they change: the target text is struck through
// and the proposed replacement is shown after it, in green, with Accept (✓) and
// Reject (✗) buttons. Accepting applies the replacement directly in the editor
// (so it flows through the normal save/undo path); both actions then clear the
// server-side proposal via onResolve.
//
// A proposal is only shown when its 'find' text occurs exactly once in the
// current document — otherwise it can't be anchored unambiguously and stays in
// the AI panel instead.
import { StateEffect, StateField, RangeSetBuilder } from '@codemirror/state'
import type { EditorState, Text } from '@codemirror/state'
import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import type { DecorationSet } from '@codemirror/view'

export interface ProseProposal {
  id: string
  find: string
  replace: string
}

/** Replace the set of prose proposals the editor should display. */
export const setProseProposals = StateEffect.define<ProseProposal[]>()

const propsField = StateField.define<ProseProposal[]>({
  create: () => [],
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setProseProposals)) return e.value
    return val
  },
})

/** Locate the single occurrence of `find`; null if absent or not unique. */
function locate(doc: Text, find: string): { from: number; to: number } | null {
  if (!find) return null
  const text = doc.toString()
  const i = text.indexOf(find)
  if (i < 0 || text.indexOf(find, i + 1) >= 0) return null
  return { from: i, to: i + find.length }
}

class ProposalWidget extends WidgetType {
  constructor(readonly p: ProseProposal) {
    super()
  }
  eq(other: ProposalWidget) {
    return other.p.id === this.p.id && other.p.replace === this.p.replace
  }
  toDOM() {
    const wrap = document.createElement('span')
    wrap.className = 'cm-prop'
    const nw = wrap.appendChild(document.createElement('span'))
    nw.className = 'cm-prop-new'
    nw.textContent = this.p.replace || '(delete)'
    const accept = wrap.appendChild(document.createElement('button'))
    accept.className = 'cm-prop-btn cm-prop-accept'
    accept.textContent = '✓'
    accept.title = 'Accept edit'
    accept.setAttribute('data-prop', `accept:${this.p.id}`)
    const reject = wrap.appendChild(document.createElement('button'))
    reject.className = 'cm-prop-btn cm-prop-reject'
    reject.textContent = '✗'
    reject.title = 'Reject edit'
    reject.setAttribute('data-prop', `reject:${this.p.id}`)
    return wrap
  }
  ignoreEvent() {
    return false
  }
}

const oldMark = Decoration.mark({ class: 'cm-prop-old' })

function buildDeco(state: EditorState): DecorationSet {
  const items: Array<{ p: ProseProposal; from: number; to: number }> = []
  for (const p of state.field(propsField)) {
    const loc = locate(state.doc, p.find)
    if (loc) items.push({ p, from: loc.from, to: loc.to })
  }
  items.sort((a, b) => a.from - b.from || a.to - b.to)
  const b = new RangeSetBuilder<Decoration>()
  for (const it of items) {
    if (it.to > it.from) b.add(it.from, it.to, oldMark)
    b.add(it.to, it.to, Decoration.widget({ widget: new ProposalWidget(it.p), side: 1 }))
  }
  return b.finish()
}

const decoField = StateField.define<DecorationSet>({
  create: (s) => buildDeco(s),
  update(val, tr) {
    if (tr.docChanged || tr.effects.some((e) => e.is(setProseProposals))) return buildDeco(tr.state)
    return val
  },
  provide: (f) => EditorView.decorations.from(f),
})

/**
 * Inline prose-proposal decorations. onResolve(id) is called after a proposal is
 * accepted (text already applied to the doc) or rejected, so the caller can
 * clear the server-side proposal.
 */
export function proposalsExtension(onResolve: (id: string) => void) {
  const handlers = EditorView.domEventHandlers({
    mousedown(event, view) {
      const el = (event.target as HTMLElement | null)?.closest('[data-prop]')
      if (!el) return false
      event.preventDefault()
      const [action, id] = (el.getAttribute('data-prop') || '').split(':')
      const p = view.state.field(propsField).find((x) => x.id === id)
      if (!p) return true
      if (action === 'accept') {
        const loc = locate(view.state.doc, p.find)
        if (loc) view.dispatch({ changes: { from: loc.from, to: loc.to, insert: p.replace } })
      }
      onResolve(id)
      return true
    },
  })
  return [propsField, decoField, handlers]
}
