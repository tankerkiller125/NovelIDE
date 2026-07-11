<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { EditorView, keymap, lineNumbers, highlightActiveLine, drawSelection } from '@codemirror/view'
import { Compartment, EditorState } from '@codemirror/state'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { AddToDictionary, DeepScan, ReadChapter, SaveChapter, ScanText, SpellSuggest } from '../api'
import { cardDataAt, codexById, openTab, pinTab, state, tabKey } from '../store'
import { entityExtension, setScanResult, toScanState } from '../editor/entityPlugin'
import type { Flag } from '../types'

const props = defineProps<{ bookId: string; chapter: string }>()
const emit = defineEmits<{ flags: [flags: Flag[]] }>()

const host = ref<HTMLElement | null>(null)
const view = shallowRef<EditorView | null>(null)
const loading = ref(true)
const error = ref('')

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
  ext.push(
    EditorView.contentAttributes.of({
      spellcheck: state.settings?.editorSpellcheck === false ? 'false' : 'true',
    }),
  )
  return ext
}
watch(
  () => [state.settings?.editorLineNumbers, state.settings?.editorSpellcheck],
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
  try {
    await SaveChapter(props.bookId, props.chapter, v.state.doc.toString())
    state.dirtyChapters.delete(dirtyKey())
  } catch (e) {
    error.value = `Save failed: ${e}`
  }
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
        keymap.of([...defaultKeymap, ...historyKeymap]),
        markdown(),
        syntaxHighlighting(mdHighlight),
        EditorView.lineWrapping,
        EditorView.theme({}, { dark: true }),
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
        EditorView.updateListener.of((u) => {
          if (u.docChanged) {
            state.dirtyChapters.add(dirtyKey())
            // Editing pins the tab so it survives navigating away.
            pinTab(tabKey({ kind: 'chapter', bookId: props.bookId, chapter: props.chapter }))
            scheduleScan(u.view)
            scheduleSave(u.view)
          }
        }),
      ],
    }),
  })
  view.value = v
  loading.value = false
  runScan(v)
}

/** Move the cursor to a flag's position (called from the problems panel). */
function jumpTo(pos: number) {
  const v = view.value
  if (!v) return
  const clamped = Math.min(pos, v.state.doc.length)
  v.dispatch({ selection: { anchor: clamped }, scrollIntoView: true })
  v.focus()
}

/** Run the optional Cybertron pass and merge its suggestions. */
async function deepScan(): Promise<void> {
  const v = view.value
  if (!v) return
  const found = await DeepScan(props.bookId, props.chapter, v.state.doc.toString())
  const have = new Set(state.suggestions.map((s) => s.key))
  state.suggestions = [...state.suggestions, ...found.filter((s) => !have.has(s.key))]
}

defineExpose({ jumpTo, deepScan, rescan: () => view.value && runScan(view.value) })

onMounted(setup)
watch(() => [props.bookId, props.chapter], setup)

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
    <div ref="host" class="editor-host" />
  </div>
</template>

<style scoped>
.editor-wrap {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.editor-error {
  padding: 8px 12px;
  color: var(--nv-error);
  background: color-mix(in srgb, var(--nv-error) 12%, transparent);
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
</style>
