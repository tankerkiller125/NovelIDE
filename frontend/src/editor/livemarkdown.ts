// Live Markdown rendering for the manuscript editor.
//
// Markdown syntax markers — the `**`/`_` around emphasis, the `` ` `` around
// inline code, the leading `#` of a heading, the `~~` of strikethrough — are
// hidden so the styled text reads like the finished page. The mdHighlight
// style (in ChapterEditor) already colours/sizes the content; this extension
// just drops the punctuation. When the cursor enters a formatted span the raw
// markers reappear so they can be edited, the same reveal-on-cursor behaviour
// the annotations and scene dividers use.
import { RangeSetBuilder } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin } from '@codemirror/view'
import type { DecorationSet, ViewUpdate } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'

// Mark tokens (the punctuation) we hide. Their parent is the formatting node
// whose range decides whether the cursor is "inside" and should reveal them.
const MARK_NODES = new Set([
  'EmphasisMark',
  'CodeMark',
  'HeaderMark',
  'StrikethroughMark',
])

const hide = Decoration.replace({})

function build(view: EditorView): DecorationSet {
  const state = view.state
  const sel = state.selection.ranges
  const touches = (from: number, to: number) =>
    sel.some((r) => r.from <= to && r.to >= from)

  const b = new RangeSetBuilder<Decoration>()
  // Only decorate what's on screen; the plugin rebuilds on viewport change.
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(state).iterate({
      from,
      to,
      enter: (node) => {
        if (!MARK_NODES.has(node.name)) return
        const parent = node.node.parent
        const pf = parent ? parent.from : node.from
        const pt = parent ? parent.to : node.to
        if (touches(pf, pt)) return // editing inside the span: keep markers
        let end = node.to
        // A heading's HeaderMark is just the `#`s; swallow the following space
        // too so the styled title isn't pushed in by a stray indent.
        if (node.name === 'HeaderMark') {
          while (end < state.doc.length && /[ \t]/.test(state.doc.sliceString(end, end + 1))) end++
        }
        if (end > node.from) b.add(node.from, end, hide)
      },
    })
  }
  return b.finish()
}

export function liveMarkdownExtension() {
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet
      constructor(view: EditorView) {
        this.decorations = build(view)
      }
      update(u: ViewUpdate) {
        // Rebuild on edits, cursor moves, scrolling, and when the incremental
        // parser has advanced the syntax tree (which arrives on its own).
        if (
          u.docChanged ||
          u.selectionSet ||
          u.viewportChanged ||
          syntaxTree(u.startState) !== syntaxTree(u.state)
        ) {
          this.decorations = build(u.view)
        }
      }
    },
    { decorations: (v) => v.decorations },
  )
}
