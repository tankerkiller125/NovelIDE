// Markdown formatting commands for the manuscript editor's toolbar. Each takes
// an EditorView and edits the current selection(s), keeping the toolbar and
// keyboard shortcuts in sync. They favour Obsidian-style toggling: applying a
// format a second time removes it.
import { EditorSelection } from '@codemirror/state'
import type { ChangeSpec } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'

/**
 * Wrap (or unwrap) each selection range in an inline marker like `**` or `` ` ``.
 * An empty selection inserts the pair and drops the cursor between them.
 */
export function toggleWrap(view: EditorView, marker: string) {
  const { state } = view
  const mlen = marker.length
  const changes: ChangeSpec[] = []
  const ranges = state.selection.ranges.map((range) => {
    const { from, to } = range
    const before = state.sliceDoc(Math.max(0, from - mlen), from)
    const after = state.sliceDoc(to, Math.min(state.doc.length, to + mlen))
    // Already wrapped just outside the selection → strip the markers.
    if (before === marker && after === marker) {
      changes.push({ from: from - mlen, to: from }, { from: to, to: to + mlen })
      return EditorSelection.range(from - mlen, to - mlen)
    }
    const inner = state.sliceDoc(from, to)
    // Selection includes the markers → strip from within.
    if (inner.length >= mlen * 2 && inner.startsWith(marker) && inner.endsWith(marker)) {
      changes.push(
        { from, to: from + mlen },
        { from: to - mlen, to },
      )
      return EditorSelection.range(from, to - mlen * 2)
    }
    // Otherwise wrap.
    changes.push({ from, insert: marker }, { from: to, insert: marker })
    if (from === to) return EditorSelection.cursor(from + mlen)
    return EditorSelection.range(from + mlen, to + mlen)
  })
  view.dispatch({
    changes,
    selection: EditorSelection.create(ranges, state.selection.mainIndex),
    scrollIntoView: true,
  })
  view.focus()
}

const HEADING_RE = /^(#{1,6})\s+/

/**
 * Set the heading level of every line the selection touches. Passing the level
 * a line already has removes the heading (toggle off).
 */
export function setHeading(view: EditorView, level: number) {
  const { state } = view
  const changes: ChangeSpec[] = []
  const seen = new Set<number>()
  for (const range of state.selection.ranges) {
    let lineNo = state.doc.lineAt(range.from).number
    const last = state.doc.lineAt(range.to).number
    for (; lineNo <= last; lineNo++) {
      if (seen.has(lineNo)) continue
      seen.add(lineNo)
      const line = state.doc.line(lineNo)
      const m = HEADING_RE.exec(line.text)
      const prefix = '#'.repeat(level) + ' '
      if (m && m[1].length === level) {
        changes.push({ from: line.from, to: line.from + m[0].length }) // toggle off
      } else if (m) {
        changes.push({ from: line.from, to: line.from + m[0].length, insert: prefix })
      } else {
        changes.push({ from: line.from, insert: prefix })
      }
    }
  }
  view.dispatch({ changes, scrollIntoView: true })
  view.focus()
}

/**
 * Toggle a line prefix such as `> ` (quote) or `- ` (bullet) on every line the
 * selection touches. If every line already has it, it's removed.
 */
export function toggleLinePrefix(view: EditorView, prefix: string) {
  const { state } = view
  const lines: number[] = []
  const seen = new Set<number>()
  for (const range of state.selection.ranges) {
    const last = state.doc.lineAt(range.to).number
    for (let n = state.doc.lineAt(range.from).number; n <= last; n++) {
      if (!seen.has(n)) {
        seen.add(n)
        lines.push(n)
      }
    }
  }
  const allHave = lines.every((n) => state.doc.line(n).text.startsWith(prefix))
  const changes: ChangeSpec[] = lines.map((n) => {
    const line = state.doc.line(n)
    return allHave
      ? { from: line.from, to: line.from + prefix.length }
      : { from: line.from, insert: prefix }
  })
  view.dispatch({ changes, scrollIntoView: true })
  view.focus()
}

/** Wrap the selection as a Markdown link `[text](url)` (or insert a stub). */
export function wrapLink(view: EditorView, url: string) {
  const { state } = view
  const sel = state.selection.main
  const text = state.sliceDoc(sel.from, sel.to) || 'link text'
  const insert = `[${text}](${url})`
  view.dispatch({
    changes: { from: sel.from, to: sel.to, insert },
    // Select the visible link text so it's easy to overtype.
    selection: EditorSelection.range(sel.from + 1, sel.from + 1 + text.length),
    scrollIntoView: true,
  })
  view.focus()
}
