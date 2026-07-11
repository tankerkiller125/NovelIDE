// CodeMirror 6 extension for editorial annotations written in CriticMarkup:
//
//   {==highlighted text==}{>>a note<<}   note attached to a span
//   {>>a standalone note<<}              note with no span
//
// Annotated spans are highlighted; the markup delimiters and the note text
// are hidden behind a small 💬 marker; hovering shows the note with a Remove
// action; and when the cursor is inside an annotation the raw markup is
// revealed so it can be edited. Annotations live in the manuscript file and
// are stripped by the exporter, so they never reach the finished book.
import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, EditorView, WidgetType, hoverTooltip } from '@codemirror/view'
import type { DecorationSet, Tooltip } from '@codemirror/view'

export interface Annotation {
  kind: 'span' | 'note'
  from: number
  to: number
  note: string
  // span only:
  innerFrom?: number
  innerTo?: number
  commentFrom?: number
}

// Highlight+comment, or a standalone comment.
const RE = /\{==([\s\S]*?)==\}\{>>([\s\S]*?)<<\}|\{>>([\s\S]*?)<<\}/g

export function parseAnnotations(text: string): Annotation[] {
  const out: Annotation[] = []
  RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = RE.exec(text))) {
    const from = m.index
    if (m[1] !== undefined) {
      const inner = m[1]
      const innerFrom = from + 3 // after "{=="
      const innerTo = innerFrom + inner.length
      const commentFrom = innerTo + 3 // after "==}"
      out.push({
        kind: 'span',
        from,
        to: from + m[0].length,
        note: m[2],
        innerFrom,
        innerTo,
        commentFrom,
      })
    } else {
      out.push({ kind: 'note', from, to: from + m[0].length, note: m[3] ?? '' })
    }
  }
  return out
}

const annField = StateField.define<Annotation[]>({
  create: (state) => parseAnnotations(state.doc.toString()),
  update: (val, tr) => (tr.docChanged ? parseAnnotations(tr.newDoc.toString()) : val),
})

const hideMark = Decoration.replace({})
const spanMark = Decoration.mark({ class: 'cm-annot' })

class NoteWidget extends WidgetType {
  constructor(readonly note: string) {
    super()
  }
  eq(other: NoteWidget) {
    return other.note === this.note
  }
  toDOM() {
    const s = document.createElement('span')
    s.className = 'cm-note-marker'
    s.textContent = '💬'
    s.title = this.note
    return s
  }
  ignoreEvent() {
    return false
  }
}

/** True when the cursor/selection touches [from,to] — then show raw markup. */
function revealed(state: EditorState, from: number, to: number) {
  for (const r of state.selection.ranges) {
    if (r.from <= to && r.to >= from) return true
  }
  return false
}

function buildDeco(state: EditorState): DecorationSet {
  const anns = state.field(annField)
  const b = new RangeSetBuilder<Decoration>()
  for (const a of anns) {
    if (revealed(state, a.from, a.to)) continue
    if (a.kind === 'span') {
      b.add(a.from, a.innerFrom!, hideMark) // "{=="
      b.add(a.innerFrom!, a.innerTo!, spanMark) // highlighted text
      b.add(a.innerTo!, a.commentFrom!, hideMark) // "==}"
      b.add(a.commentFrom!, a.to, Decoration.replace({ widget: new NoteWidget(a.note) }))
    } else {
      b.add(a.from, a.to, Decoration.replace({ widget: new NoteWidget(a.note) }))
    }
  }
  return b.finish()
}

const decoField = StateField.define<DecorationSet>({
  create: (state) => buildDeco(state),
  update: (val, tr) => (tr.docChanged || tr.selection ? buildDeco(tr.state) : val),
  provide: (f) => EditorView.decorations.from(f),
})

/** Remove an annotation, keeping the highlighted prose. */
function removeAnnotation(view: EditorView, a: Annotation) {
  const changes =
    a.kind === 'span'
      ? [
          { from: a.from, to: a.innerFrom! }, // drop "{=="
          { from: a.innerTo!, to: a.to }, // drop "==}{>>...<<}"
        ]
      : [{ from: a.from, to: a.to }]
  view.dispatch({ changes })
}

function annotationHover() {
  return hoverTooltip((view, pos): Tooltip | null => {
    const anns = view.state.field(annField)
    const hit = anns.find((a) => {
      if (a.kind === 'span') return pos >= a.innerFrom! && pos <= a.to
      return pos >= a.from && pos <= a.to
    })
    if (!hit) return null
    return {
      pos: hit.kind === 'span' ? hit.innerFrom! : hit.from,
      end: hit.to,
      above: true,
      create: () => {
        const dom = document.createElement('div')
        dom.className = 'nv-card nv-note-card'
        const label = dom.appendChild(document.createElement('div'))
        label.className = 'nv-note-label'
        label.textContent = 'Note'
        const body = dom.appendChild(document.createElement('div'))
        body.className = 'nv-note-text'
        body.textContent = hit.note || '(empty)'
        const rm = dom.appendChild(document.createElement('button'))
        rm.className = 'nv-card-open'
        rm.textContent = 'Remove note'
        rm.addEventListener('mousedown', (ev) => {
          ev.preventDefault()
          // re-find by position in case the doc shifted since hover opened
          const cur = view.state.field(annField).find((a) => a.from === hit.from) ?? hit
          removeAnnotation(view, cur)
        })
        return { dom }
      },
    }
  })
}

/**
 * Wrap the current selection in an annotation (or insert a standalone note
 * at the cursor). `note` is sanitized so it can't break the markup.
 */
export function addAnnotation(view: EditorView, note: string) {
  const clean = note.replace(/<<\}/g, '').replace(/\{>>/g, '').trim()
  if (!clean) return
  const sel = view.state.selection.main
  if (sel.empty) {
    const insert = `{>>${clean}<<}`
    view.dispatch({
      changes: { from: sel.from, insert },
      selection: { anchor: sel.from + insert.length },
    })
  } else {
    const text = view.state.sliceDoc(sel.from, sel.to).replace(/==\}/g, '')
    view.dispatch({
      changes: { from: sel.from, to: sel.to, insert: `{==${text}==}{>>${clean}<<}` },
    })
  }
  view.focus()
}

export function annotationExtension() {
  return [annField, decoField, annotationHover()]
}
