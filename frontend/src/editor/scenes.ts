// CodeMirror 6 extension that renders in-chapter scene dividers.
//
// A scene marker is a single-line HTML comment carrying an optional title:
//
//   <!-- scene: The Ash Farms -->
//
// In the editor the raw comment is replaced by a centered divider showing the
// title (matching how it will look as a scene break in the exported book).
// When the cursor is on the marker line the raw text is revealed so the title
// can be edited. The backend (internal/workspace/scene.go) treats these same
// markers as scene boundaries for the corkboard.
import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, EditorView, WidgetType } from '@codemirror/view'
import type { DecorationSet } from '@codemirror/view'

const SCENE_RE = /^[ \t]*<!--[ \t]*scene(?:[ \t]*:[ \t]*(.*?))?[ \t]*-->[ \t]*$/

interface Marker {
  from: number
  to: number
  title: string
}

function findMarkers(state: EditorState): Marker[] {
  const out: Marker[] = []
  const doc = state.doc
  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)
    const m = SCENE_RE.exec(line.text)
    if (m) out.push({ from: line.from, to: line.to, title: (m[1] ?? '').trim() })
  }
  return out
}

const markerField = StateField.define<Marker[]>({
  create: findMarkers,
  update: (val, tr) => (tr.docChanged ? findMarkers(tr.state) : val),
})

class DividerWidget extends WidgetType {
  constructor(readonly title: string) {
    super()
  }
  eq(other: DividerWidget) {
    return other.title === this.title
  }
  toDOM() {
    const wrap = document.createElement('div')
    wrap.className = 'cm-scene-divider'
    if (this.title) {
      const label = wrap.appendChild(document.createElement('span'))
      label.className = 'cm-scene-label'
      label.textContent = this.title
    } else {
      wrap.classList.add('cm-scene-plain')
      wrap.appendChild(document.createElement('span')).textContent = '✦'
    }
    return wrap
  }
  ignoreEvent() {
    return true
  }
}

/** True when a selection touches the marker line, so we reveal raw markup. */
function onLine(state: EditorState, from: number, to: number) {
  for (const r of state.selection.ranges) {
    if (r.from <= to && r.to >= from) return true
  }
  return false
}

function buildDeco(state: EditorState): DecorationSet {
  const b = new RangeSetBuilder<Decoration>()
  for (const mk of state.field(markerField)) {
    if (onLine(state, mk.from, mk.to)) {
      // Editing the title: leave the text, just tint the line.
      b.add(mk.from, mk.from, Decoration.line({ class: 'cm-scene-line' }))
    } else {
      b.add(
        mk.from,
        mk.to,
        Decoration.replace({ widget: new DividerWidget(mk.title), block: true }),
      )
    }
  }
  return b.finish()
}

const decoField = StateField.define<DecorationSet>({
  create: buildDeco,
  update: (val, tr) => (tr.docChanged || tr.selection ? buildDeco(tr.state) : val),
  provide: (f) => EditorView.decorations.from(f),
})

/**
 * Insert a scene break at the cursor. The marker is placed on its own line
 * with blank lines around it so it reads as a paragraph boundary in Markdown.
 */
export function insertSceneMarker(view: EditorView, title: string) {
  const clean = title.replace(/-->/g, '').trim()
  const marker = clean ? `<!-- scene: ${clean} -->` : '<!-- scene -->'
  const pos = view.state.selection.main.head
  const line = view.state.doc.lineAt(pos)
  // Break the current line at the cursor, dropping the marker between the two
  // halves with blank lines so it stands alone.
  const before = view.state.sliceDoc(line.from, pos).replace(/\s+$/, '')
  const atLineStart = pos === line.from || before === ''
  const insert = atLineStart ? `${marker}\n\n` : `\n\n${marker}\n\n`
  view.dispatch({
    changes: { from: pos, insert },
    selection: { anchor: pos + insert.length },
  })
  view.focus()
}

export function sceneExtension() {
  return [markerField, decoField]
}
