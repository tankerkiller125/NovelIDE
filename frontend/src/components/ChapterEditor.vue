<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { EditorView, keymap, lineNumbers, highlightActiveLine, drawSelection } from '@codemirror/view'
import { Compartment, EditorState } from '@codemirror/state'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { search, searchKeymap, highlightSelectionMatches, openSearchPanel } from '@codemirror/search'
import { tags } from '@lezer/highlight'
import { setHeading, toggleLinePrefix, toggleWrap, wrapLink } from '../editor/format'
import { AddToDictionary, DeepScan, ReadChapter, SaveChapter, ScanText, SpellSuggest } from '../api'
import { cardDataAt, codexById, openTab, pinTab, promptInput, state, tabKey } from '../store'
import { entityExtension, setScanResult, toScanState } from '../editor/entityPlugin'
import { addAnnotation, annotationExtension } from '../editor/annotations'
import { insertSceneMarker, sceneExtension } from '../editor/scenes'
import { liveMarkdownExtension } from '../editor/livemarkdown'
import type { Flag } from '../types'

const props = defineProps<{ bookId: string; chapter: string }>()
const emit = defineEmits<{ flags: [flags: Flag[]] }>()

// Floating "add note" button, shown while text is selected.
const noteBtn = reactive({ show: false, x: 0, y: 0 })

const host = ref<HTMLElement | null>(null)
const view = shallowRef<EditorView | null>(null)
const loading = ref(true)
const error = ref('')

// The content we last read from / wrote to disk, used to tell our own saves
// apart from external edits when the file changes on disk.
const lastSaved = ref('')
// Set when an external change hits a chapter with unsaved edits.
const diskConflict = ref(false)
const conflictDisk = ref('')

// ---- Creature-comfort settings, all live-applying ----
// Width, font, size, and spacing bind straight into the scoped CSS via
// v-bind; the gutter and spellchecker reconfigure through a CodeMirror
// compartment so the editor keeps its state.
const contentMaxWidth = computed(() => {
  const w = state.settings?.editorWidth ?? 0
  return w > 0 ? `${w}ch` : 'none'
})
const contentMargin = computed(() =>
  (state.settings?.editorWidth ?? 0) > 0 ? '0 auto' : '0',
)
const fontFamily = computed(() => {
  const f = state.settings?.editorFont || 'serif'
  switch (f) {
    case 'serif':
      return "Georgia, 'Times New Roman', serif"
    case 'sans':
      return "'Nunito', 'Segoe UI', Roboto, sans-serif"
    case 'mono':
      return "'JetBrains Mono', 'Fira Code', 'DejaVu Sans Mono', monospace"
    default:
      return `'${f.replace(/'/g, '')}', Georgia, serif`
  }
})
const fontSize = computed(() => `${state.settings?.editorFontSize || 15}px`)
const lineHeight = computed(() => String(state.settings?.editorLineHeight || 1.7))

const comfort = new Compartment()
function comfortExtensions() {
  const ext = []
  if (state.settings?.editorLineNumbers) ext.push(lineNumbers())
  // Live Markdown rendering is on unless the author opts into raw markup.
  if (!state.settings?.editorRawMarkup) ext.push(liveMarkdownExtension())
  ext.push(
    EditorView.contentAttributes.of({
      spellcheck: state.settings?.editorSpellcheck === false ? 'false' : 'true',
    }),
  )
  return ext
}
watch(
  () => [
    state.settings?.editorLineNumbers,
    state.settings?.editorSpellcheck,
    state.settings?.editorRawMarkup,
  ],
  () => view.value?.dispatch({ effects: comfort.reconfigure(comfortExtensions()) }),
)

let scanTimer: number | undefined
let saveTimer: number | undefined
let scanSeq = 0
let destroyed = false

const dirtyKey = () => `${props.bookId}/${props.chapter}`

const mdHighlight = HighlightStyle.define([
  { tag: tags.heading1, fontSize: '1.5em', fontWeight: 'bold', color: 'var(--nv-heading)' },
  { tag: tags.heading2, fontSize: '1.3em', fontWeight: 'bold', color: 'var(--nv-heading)' },
  { tag: tags.heading3, fontSize: '1.15em', fontWeight: 'bold', color: 'var(--nv-heading)' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.strong, fontWeight: 'bold' },
  { tag: tags.quote, color: 'var(--nv-muted)', fontStyle: 'italic' },
  { tag: tags.link, color: 'var(--nv-accent)' },
  { tag: tags.monospace, fontFamily: 'monospace', color: 'var(--nv-accent)' },
])

async function runScan(v: EditorView) {
  const seq = ++scanSeq
  const text = v.state.doc.toString()
  try {
    const result = await ScanText(props.bookId, props.chapter, text)
    if (destroyed || seq !== scanSeq || view.value !== v) return
    // Only apply if the doc hasn't changed since we sent it.
    if (v.state.doc.toString() !== text) return
    v.dispatch({ effects: setScanResult.of(toScanState(result, text)) })
    state.flags = result.flags
    state.suggestions = result.suggestions
    emit('flags', result.flags)
  } catch (e) {
    console.error('scan failed', e)
  }
}

async function save(v: EditorView) {
  const content = v.state.doc.toString()
  try {
    await SaveChapter(props.bookId, props.chapter, content)
    lastSaved.value = content
    state.dirtyChapters.delete(dirtyKey())
    state.statsTick++ // nudge the writing-stats bar to refresh
  } catch (e) {
    error.value = `Save failed: ${e}`
  }
}

// Show a floating "add note" button above the current selection.
function updateNoteButton(v: EditorView) {
  const sel = v.state.selection.main
  if (sel.empty) {
    noteBtn.show = false
    return
  }
  const c = v.coordsAtPos(sel.from)
  if (!c) {
    noteBtn.show = false
    return
  }
  noteBtn.x = c.left
  noteBtn.y = c.top - 34
  noteBtn.show = true
}

// Prompt for a note and annotate the current selection (or insert a
// standalone note at the cursor).
async function addNote() {
  const v = view.value
  if (!v) return
  const sel = v.state.selection.main
  const range = { from: sel.from, to: sel.to }
  const note = await promptInput({
    title: sel.empty ? 'Add note' : 'Annotate selection',
    label: 'Note',
    placeholder: 'e.g. check the timeline here',
  })
  if (!note) return
  v.dispatch({ selection: { anchor: range.from, head: range.to } })
  addAnnotation(v, note)
  noteBtn.show = false
}

// Prompt for an optional title and drop a scene break at the cursor.
async function addScene() {
  const v = view.value
  if (!v) return
  const title = await promptInput({
    title: 'New scene',
    label: 'Scene title (optional)',
    placeholder: 'e.g. The Ash Farms',
  })
  // The prompt returns null on cancel (and on empty input). Bail then; an
  // untitled break can still be made by typing the bare marker.
  if (title === null) return
  insertSceneMarker(v, title)
}

// --- formatting toolbar actions ---
function fmtWrap(marker: string) {
  if (view.value) toggleWrap(view.value, marker)
}
function fmtHeading(level: number) {
  if (view.value) setHeading(view.value, level)
}
function fmtPrefix(prefix: string) {
  if (view.value) toggleLinePrefix(view.value, prefix)
}
async function fmtLink() {
  const v = view.value
  if (!v) return
  const url = await promptInput({
    title: 'Insert link',
    label: 'URL',
    placeholder: 'https://…',
  })
  if (url === null) return
  wrapLink(v, url)
}
function openFind() {
  if (view.value) openSearchPanel(view.value)
}

function scheduleScan(v: EditorView, delay = 350) {
  window.clearTimeout(scanTimer)
  scanTimer = window.setTimeout(() => runScan(v), delay)
}

function scheduleSave(v: EditorView) {
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => save(v), 900)
}

async function setup() {
  loading.value = true
  error.value = ''
  view.value?.destroy()
  view.value = null
  let content = ''
  try {
    content = await ReadChapter(props.bookId, props.chapter)
  } catch (e) {
    error.value = `Could not open chapter: ${e}`
    loading.value = false
    return
  }
  lastSaved.value = content
  diskConflict.value = false
  if (destroyed || !host.value) return
  const v = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: content,
      extensions: [
        comfort.of(comfortExtensions()),
        history(),
        drawSelection(),
        highlightActiveLine(),
        keymap.of([
          { key: 'Mod-Alt-m', run: () => (addNote(), true) },
          { key: 'Mod-Alt-s', run: () => (addScene(), true) },
          { key: 'Mod-b', run: (v) => (toggleWrap(v, '**'), true) },
          { key: 'Mod-i', run: (v) => (toggleWrap(v, '*'), true) },
          ...searchKeymap,
          ...defaultKeymap,
          ...historyKeymap,
        ]),
        search({ top: true }),
        highlightSelectionMatches(),
        markdown(),
        syntaxHighlighting(mdHighlight),
        EditorView.lineWrapping,
        EditorView.theme({}, { dark: true }),
        sceneExtension(),
        entityExtension({
          getEntry: (id) => codexById.value.get(id),
          getCard: (id) => cardDataAt(id, props.bookId, props.chapter),
          onOpenEntry: (id) => openTab({ kind: 'codex', entryId: id }),
          getSpellSuggestions: (word) => SpellSuggest(word),
          onAddWord: async (word) => {
            await AddToDictionary(word)
            if (view.value) runScan(view.value)
          },
        }),
        annotationExtension(),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            state.dirtyChapters.add(dirtyKey())
            // Editing pins the tab so it survives navigating away.
            pinTab(tabKey({ kind: 'chapter', bookId: props.bookId, chapter: props.chapter }))
            scheduleScan(u.view)
            scheduleSave(u.view)
          }
          if (u.docChanged || u.selectionSet) updateNoteButton(u.view)
        }),
      ],
    }),
  })
  view.value = v
  loading.value = false
  runScan(v)
  applyPendingJump()
}

/** Move the cursor to a flag's position (called from the problems panel). */
function jumpTo(pos: number) {
  const v = view.value
  if (!v) return
  const clamped = Math.min(pos, v.state.doc.length)
  v.dispatch({ selection: { anchor: clamped }, scrollIntoView: true })
  v.focus()
}

/** Consume a pending search jump aimed at this chapter, if any. */
function applyPendingJump() {
  const j = state.pendingJump
  const v = view.value
  if (!v || !j || j.bookId !== props.bookId || j.chapter !== props.chapter) return
  const line = Math.max(1, Math.min(j.line, v.state.doc.lines))
  const pos = v.state.doc.line(line).from
  v.dispatch({ selection: { anchor: pos }, scrollIntoView: true })
  v.focus()
  state.pendingJump = null
}
// React to a jump requested while this chapter is already open.
watch(() => state.pendingJump, applyPendingJump)

// This chapter's path relative to the workspace, matching the watcher's output.
const chapterRel = computed(() => `books/${props.bookId}/manuscript/${props.chapter}`)

/** When this chapter changed on disk (external edit), reconcile the buffer. */
async function reconcileFromDisk() {
  const v = view.value
  if (!v) return
  let disk: string
  try {
    disk = await ReadChapter(props.bookId, props.chapter)
  } catch {
    return // e.g. the chapter was removed; the tab will be reconciled away
  }
  const buf = v.state.doc.toString()
  if (disk === buf) return // already in sync
  if (disk === lastSaved.value) return // our own save; buffer may hold newer edits
  if (!state.dirtyChapters.has(dirtyKey())) {
    adoptDisk(disk) // no local edits at risk — just take the new version
  } else {
    conflictDisk.value = disk // unsaved edits: let the author choose
    diskConflict.value = true
  }
}

/** Replace the buffer with the on-disk version, keeping the cursor in range. */
function adoptDisk(disk: string) {
  const v = view.value
  if (!v) return
  const sel = v.state.selection.main
  const clamp = (n: number) => Math.min(n, disk.length)
  v.dispatch({
    changes: { from: 0, to: v.state.doc.length, insert: disk },
    selection: { anchor: clamp(sel.anchor), head: clamp(sel.head) },
  })
  lastSaved.value = disk
  state.dirtyChapters.delete(dirtyKey())
  diskConflict.value = false
  runScan(v)
}

function keepMine() {
  // Keep the buffer; the next save overwrites disk. The banner clears; the
  // same external change won't re-prompt (the watcher only fires on new edits).
  diskConflict.value = false
}

// React when the file watcher reports this chapter changed on disk.
watch(
  () => state.externalChange.seq,
  () => {
    if (state.externalChange.modified.includes(chapterRel.value)) void reconcileFromDisk()
  },
)

/** Run the optional Cybertron pass and merge its suggestions. */
async function deepScan(): Promise<void> {
  const v = view.value
  if (!v) return
  const found = await DeepScan(props.bookId, props.chapter, v.state.doc.toString())
  const have = new Set(state.suggestions.map((s) => s.key))
  state.suggestions = [...state.suggestions, ...found.filter((s) => !have.has(s.key))]
}

defineExpose({ jumpTo, deepScan, addScene, rescan: () => view.value && runScan(view.value) })

onMounted(setup)
watch(() => [props.bookId, props.chapter], setup)
// A project-wide replace rewrote files on disk; reload this chapter's text.
// The search view only allows replacing when nothing is dirty, so re-reading
// can't discard unsaved edits here.
watch(() => state.reloadTick, () => setup())

onBeforeUnmount(() => {
  destroyed = true
  window.clearTimeout(scanTimer)
  const v = view.value
  if (v && state.dirtyChapters.has(dirtyKey())) {
    window.clearTimeout(saveTimer)
    save(v)
  }
  v?.destroy()
})
</script>

<template>
  <div class="editor-wrap">
    <div v-if="error" class="editor-error">{{ error }}</div>
    <div v-if="diskConflict" class="disk-conflict">
      <span>This chapter changed on disk while you had unsaved edits.</span>
      <button class="btn mini" @click="adoptDisk(conflictDisk)">Load disk version</button>
      <button class="btn mini" @click="keepMine">Keep mine</button>
    </div>
    <div v-if="!state.focusMode" class="fmt-toolbar">
      <button class="fmt-btn" title="Bold (Ctrl+B)" @click="fmtWrap('**')"><b>B</b></button>
      <button class="fmt-btn" title="Italic (Ctrl+I)" @click="fmtWrap('*')"><i>I</i></button>
      <button class="fmt-btn" title="Strikethrough" @click="fmtWrap('~~')"><s>S</s></button>
      <button class="fmt-btn mono" title="Inline code" @click="fmtWrap('`')">&lt;/&gt;</button>
      <span class="fmt-sep" />
      <button class="fmt-btn" title="Heading 1" @click="fmtHeading(1)">H1</button>
      <button class="fmt-btn" title="Heading 2" @click="fmtHeading(2)">H2</button>
      <button class="fmt-btn" title="Heading 3" @click="fmtHeading(3)">H3</button>
      <span class="fmt-sep" />
      <button class="fmt-btn" title="Quote" @click="fmtPrefix('> ')">❝</button>
      <button class="fmt-btn" title="Bulleted list" @click="fmtPrefix('- ')">•</button>
      <button class="fmt-btn" title="Link" @click="fmtLink">🔗</button>
      <span class="fmt-sep" />
      <button class="fmt-btn" title="Add a note (Ctrl+Alt+M)" @click="addNote">💬</button>
      <button class="fmt-btn wide" title="Insert a scene break (Ctrl+Alt+S)" @click="addScene">＋ Scene</button>
      <span class="fmt-spacer" />
      <button class="fmt-btn" title="Find / replace (Ctrl+F)" @click="openFind">🔍</button>
    </div>
    <div ref="host" class="editor-host" />
    <button
      v-if="noteBtn.show"
      class="note-fab"
      :style="{ left: noteBtn.x + 'px', top: noteBtn.y + 'px' }"
      title="Add a note (Ctrl+Alt+M)"
      @mousedown.prevent="addNote"
    >
      💬 Note
    </button>
  </div>
</template>

<style scoped>
.editor-wrap {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
  position: relative;
}
.note-fab {
  position: fixed;
  transform: translateX(-50%);
  z-index: 50;
  background: var(--nv-accent);
  color: #1b1408;
  border: none;
  border-radius: 6px;
  padding: 3px 10px;
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.4);
}
.editor-error {
  padding: 8px 12px;
  color: var(--nv-error);
  background: color-mix(in srgb, var(--nv-error) 12%, transparent);
}
.disk-conflict {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 12px;
  font-size: 12px;
  color: var(--nv-text);
  background: color-mix(in srgb, var(--nv-warning) 18%, transparent);
  border-bottom: 1px solid var(--nv-border);
}
.disk-conflict span {
  flex: 1;
}
.disk-conflict .btn.mini {
  padding: 2px 9px;
  font-size: 11px;
}
.fmt-toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px 10px;
  background: var(--nv-panel);
  border-bottom: 1px solid var(--nv-border);
  flex-shrink: 0;
  flex-wrap: wrap;
}
.fmt-btn {
  min-width: 26px;
  height: 24px;
  padding: 0 6px;
  background: none;
  border: 1px solid transparent;
  border-radius: 5px;
  color: var(--nv-muted);
  cursor: pointer;
  font-size: 12.5px;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.fmt-btn:hover {
  background: var(--nv-hover);
  color: var(--nv-text);
  border-color: var(--nv-border);
}
.fmt-btn.mono {
  font-family: monospace;
  font-size: 11px;
}
.fmt-btn.wide {
  font-size: 12px;
  gap: 3px;
}
.fmt-sep {
  width: 1px;
  height: 16px;
  background: var(--nv-border);
  margin: 0 5px;
}
.fmt-spacer {
  flex: 1;
}
.editor-host {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.editor-host :deep(.cm-editor) {
  height: 100%;
  font-size: v-bind(fontSize);
  background: var(--nv-bg);
}
.editor-host :deep(.cm-scroller) {
  font-family: v-bind(fontFamily);
  line-height: v-bind(lineHeight);
  overflow: auto;
}
.editor-host :deep(.cm-content) {
  max-width: v-bind(contentMaxWidth);
  margin: v-bind(contentMargin);
  padding: 24px 32px;
  caret-color: var(--nv-accent);
}
.editor-host :deep(.cm-gutters) {
  background: var(--nv-bg);
  color: var(--nv-faint);
  border-right: 1px solid var(--nv-border);
}
.editor-host :deep(.cm-activeLine) {
  background: color-mix(in srgb, var(--nv-accent) 5%, transparent);
}
/* Find/replace panel — restyle CodeMirror's default to fit the dark theme. */
.editor-host :deep(.cm-panels) {
  background: var(--nv-panel);
  color: var(--nv-text);
  border-bottom: 1px solid var(--nv-border);
}
.editor-host :deep(.cm-panel.cm-search) {
  padding: 6px 8px;
  font-size: 12px;
}
.editor-host :deep(.cm-panel.cm-search input),
.editor-host :deep(.cm-panel.cm-search button),
.editor-host :deep(.cm-textfield) {
  background: var(--nv-bg);
  color: var(--nv-text);
  border: 1px solid var(--nv-border);
  border-radius: 4px;
}
.editor-host :deep(.cm-panel.cm-search button:hover),
.editor-host :deep(.cm-button:hover) {
  background: var(--nv-hover);
}
.editor-host :deep(.cm-panel.cm-search label) {
  color: var(--nv-muted);
  font-size: 11px;
}
.editor-host :deep(.cm-searchMatch) {
  background: color-mix(in srgb, var(--nv-accent) 28%, transparent);
}
.editor-host :deep(.cm-searchMatch-selected) {
  background: color-mix(in srgb, var(--nv-accent) 55%, transparent);
}
.editor-host :deep(.cm-selectionMatch) {
  background: color-mix(in srgb, var(--nv-accent) 16%, transparent);
}
/* Rendered scene divider (replaces the raw <!-- scene: … --> marker). */
.editor-host :deep(.cm-scene-divider) {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin: 6px 0;
  color: var(--nv-muted);
  user-select: none;
}
.editor-host :deep(.cm-scene-divider)::before,
.editor-host :deep(.cm-scene-divider)::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--nv-border);
  max-width: 120px;
}
.editor-host :deep(.cm-scene-label) {
  font-size: 0.85em;
  font-variant: small-caps;
  letter-spacing: 0.08em;
  color: var(--nv-faint);
}
.editor-host :deep(.cm-scene-plain) {
  color: var(--nv-faint);
}
/* Marker line while it's being edited: a faint tint so it's obviously special. */
.editor-host :deep(.cm-scene-line) {
  background: color-mix(in srgb, var(--nv-accent) 6%, transparent);
  color: var(--nv-muted);
  font-style: italic;
}
</style>
