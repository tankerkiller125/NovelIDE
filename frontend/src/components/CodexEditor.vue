<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Backlinks, ClearEntryImage, DeleteCodexEntry, PickEntryImage, SaveCodexEntry } from '../api'
import {
  closeTab,
  codexById,
  imageURL,
  openTab,
  pinActiveTab,
  pinTab,
  relationDefById,
  schemaTypes,
  setWorkspace,
  state,
  tabKey,
} from '../store'
import type { Backlink, CodexEntry, Relation, StatusChange, TimedValue } from '../types'

const props = defineProps<{ entryId: string; draft?: CodexEntry }>()

function blankEntry(): CodexEntry {
  return {
    id: '',
    name: '',
    type: schemaTypes.value[0]?.id ?? 'character',
    aliases: [],
    summary: '',
    details: '',
    image: '',
    fields: {},
    fieldTimelines: {},
    status: [],
    relations: [],
    scope: 'series',
  }
}

const original = computed<CodexEntry | null>(() =>
  props.entryId ? (codexById.value.get(props.entryId) ?? null) : (props.draft ?? null),
)

interface FieldRow {
  key: string
  value: string
}

interface RelationRow {
  type: string
  to: string
  fromBook: string
  fromChapter: string
  untilBook: string
  untilChapter: string
  note: string
}

// A fact that changes over story time: one key with several values, each
// anchored to a story point ("from the start" when no book is chosen).
interface TimelineValueRow {
  value: string
  book: string
  chapter: string
}
interface TimelineField {
  key: string
  values: TimelineValueRow[]
}

const form = reactive({
  name: '',
  type: 'character',
  aliasText: '',
  summary: '',
  details: '',
  scope: 'series',
  fields: [] as FieldRow[],
  fieldTimelines: [] as TimelineField[],
  status: [] as StatusChange[],
  relations: [] as RelationRow[],
})

const saving = ref(false)
const message = ref('')
const isNew = computed(() => !props.entryId)

const imageBusy = ref(false)
const currentImage = computed(() =>
  original.value?.image ? imageURL(original.value.image) : undefined,
)
async function pickImage() {
  if (!original.value) return
  imageBusy.value = true
  try {
    setWorkspace(await PickEntryImage(original.value))
  } catch (e) {
    message.value = `Image failed: ${e}`
  } finally {
    imageBusy.value = false
  }
}
async function removeImage() {
  if (!original.value) return
  try {
    setWorkspace(await ClearEntryImage(original.value))
  } catch (e) {
    message.value = `Image failed: ${e}`
  }
}

function loadForm() {
  const e = original.value ?? blankEntry()
  form.name = e.name
  form.type = e.type
  form.aliasText = (e.aliases ?? []).join(', ')
  form.summary = e.summary ?? ''
  form.details = e.details ?? ''
  form.scope = e.scope || 'series'
  form.fields = Object.entries(e.fields ?? {}).map(([key, value]) => ({ key, value }))
  form.fieldTimelines = Object.entries(e.fieldTimelines ?? {}).map(([key, values]) => ({
    key,
    values: (values ?? []).map((v) => ({
      value: v.value,
      book: v.at?.book ?? '',
      chapter: v.at?.chapter ?? '',
    })),
  }))
  form.status = (e.status ?? []).map((s) => ({
    state: s.state,
    at: { book: s.at?.book ?? '', chapter: s.at?.chapter ?? '' },
    note: s.note ?? '',
  }))
  form.relations = (e.relations ?? []).map((r) => ({
    type: r.type,
    to: r.to,
    fromBook: r.from?.book ?? '',
    fromChapter: r.from?.chapter ?? '',
    untilBook: r.until?.book ?? '',
    untilChapter: r.until?.chapter ?? '',
    note: r.note ?? '',
  }))
}
watch(() => [props.entryId, original.value], loadForm, { immediate: true })

const books = computed(() => state.workspace?.books ?? [])
const chaptersOf = (bookId: string) => books.value.find((b) => b.id === bookId)?.chapters ?? []

const typeDef = computed(() => schemaTypes.value.find((t) => t.id === form.type))
const suggestedFields = computed(() => {
  const used = new Set(form.fields.map((f) => f.key))
  return (typeDef.value?.fields ?? []).filter((f) => !used.has(f))
})

const relationDefs = computed(() => state.workspace?.schema?.relations ?? [])
const otherEntries = computed(() =>
  (state.workspace?.codex ?? []).filter((e) => e.id !== original.value?.id),
)

/** Incoming edges (defined on other entries), shown read-only. */
const incoming = computed(() => {
  const id = original.value?.id
  if (!id) return []
  const defs = relationDefById.value
  const out = []
  for (const e of state.workspace?.codex ?? []) {
    if (e.id === id) continue
    for (const r of e.relations ?? []) {
      if (r.to !== id) continue
      const def = defs.get(r.type)
      out.push({
        label: def?.symmetric ? (def?.label ?? r.type) : def?.inverseLabel || `← ${def?.label ?? r.type}`,
        targetId: e.id,
        targetName: e.name,
        timespan: '',
        note: r.note ?? '',
      })
    }
  }
  return out
})

// Manuscript backlinks: chapters that actually mention this entity. Loaded
// lazily and re-fetched when the entry changes or its name/aliases are saved.
const backlinks = ref<Backlink[]>([])
const backlinksLoading = ref(false)
async function loadBacklinks() {
  const id = original.value?.id
  if (!id) {
    backlinks.value = []
    return
  }
  backlinksLoading.value = true
  try {
    backlinks.value = await Backlinks(id)
  } catch (e) {
    console.error('backlinks failed', e)
    backlinks.value = []
  } finally {
    backlinksLoading.value = false
  }
}
watch(() => [props.entryId, original.value?.aliases?.join(','), original.value?.name], loadBacklinks, {
  immediate: true,
})
const backlinkTotal = computed(() => backlinks.value.reduce((n, b) => n + b.count, 0))

function addField(key = '') {
  form.fields.push({ key, value: '' })
}
function addTimelineField() {
  form.fieldTimelines.push({ key: '', values: [{ value: '', book: '', chapter: '' }] })
}
function addTimelineValue(tf: TimelineField) {
  tf.values.push({ value: '', book: '', chapter: '' })
}
function addStatus() {
  form.status.push({
    state: form.status.length ? 'dead' : 'alive',
    at: { book: '', chapter: '' },
    note: '',
  })
}
function addRelation() {
  form.relations.push({
    type: relationDefs.value[0]?.id ?? '',
    to: otherEntries.value[0]?.id ?? '',
    fromBook: '',
    fromChapter: '',
    untilBook: '',
    untilChapter: '',
    note: '',
  })
}

async function save() {
  if (!form.name.trim()) {
    message.value = 'Name is required.'
    return
  }
  saving.value = true
  message.value = ''
  const old = original.value
  const entry: CodexEntry = {
    id: old?.id ?? '',
    name: form.name.trim(),
    type: form.type,
    aliases: form.aliasText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
    summary: form.summary,
    details: form.details,
    image: old?.image ?? '', // preserve the portrait across edits
    fields: Object.fromEntries(
      form.fields.filter((f) => f.key.trim()).map((f) => [f.key.trim(), f.value]),
    ),
    fieldTimelines: Object.fromEntries(
      form.fieldTimelines
        .filter((tf) => tf.key.trim() && tf.values.some((v) => v.value.trim()))
        .map((tf): [string, TimedValue[]] => [
          tf.key.trim(),
          tf.values
            .filter((v) => v.value.trim())
            .map((v) => ({
              value: v.value,
              at: v.book ? { book: v.book, chapter: v.chapter || undefined } : undefined,
              note: undefined,
            })),
        ]),
    ),
    status: form.status
      .filter((s) => s.state.trim())
      .map((s) => ({
        state: s.state.trim().toLowerCase(),
        at: {
          book: s.at.book || undefined,
          chapter: s.at.book ? s.at.chapter || undefined : undefined,
        },
        note: s.note || undefined,
      })),
    relations: form.relations
      .filter((r) => r.type && r.to)
      .map((r): Relation => ({
        type: r.type,
        to: r.to,
        from: r.fromBook
          ? { book: r.fromBook, chapter: r.fromChapter || undefined }
          : undefined,
        until: r.untilBook
          ? { book: r.untilBook, chapter: r.untilChapter || undefined }
          : undefined,
        note: r.note || undefined,
      })),
    scope: form.scope,
  }
  try {
    const ws = await SaveCodexEntry(entry, old?.type ?? '', old?.scope ?? '')
    setWorkspace(ws)
    message.value = 'Saved.'
    if (isNew.value) {
      const saved = (ws.codex ?? []).find((e) => e.name === entry.name)
      closeTab(tabKey({ kind: 'codex', entryId: '' }))
      if (saved) {
        const key = tabKey({ kind: 'codex', entryId: saved.id })
        openTab({ kind: 'codex', entryId: saved.id })
        pinTab(key) // a just-created entry stays open, not a preview
      }
    }
  } catch (e) {
    message.value = `Save failed: ${e}`
  } finally {
    saving.value = false
  }
}

async function remove() {
  const e = original.value
  if (!e || !confirm(`Delete "${e.name}" from the codex?`)) return
  try {
    const ws = await DeleteCodexEntry(e)
    closeTab(tabKey({ kind: 'codex', entryId: e.id }))
    setWorkspace(ws)
  } catch (err) {
    message.value = `Delete failed: ${err}`
  }
}
</script>

<template>
  <div class="codex-editor" @input="pinActiveTab" @change="pinActiveTab">
    <div class="ce-toolbar">
      <h2>{{ isNew ? 'New Codex Entry' : form.name || 'Codex Entry' }}</h2>
      <span class="ce-msg">{{ message }}</span>
      <button class="btn danger" v-if="!isNew" @click="remove">Delete</button>
      <button class="btn primary" :disabled="saving" @click="save">Save</button>
    </div>

    <div class="ce-image-row">
      <div class="ce-image" :class="{ empty: !currentImage }">
        <img v-if="currentImage" :src="currentImage" :alt="form.name" />
        <span v-else class="ce-image-ph">No image</span>
      </div>
      <div class="ce-image-actions">
        <template v-if="isNew">
          <span class="hint">Save the entry first to add an image.</span>
        </template>
        <template v-else>
          <button class="btn" :disabled="imageBusy" @click="pickImage">
            {{ imageBusy ? 'Adding…' : currentImage ? 'Replace image…' : 'Add image…' }}
          </button>
          <button v-if="currentImage" class="btn danger" @click="removeImage">Remove</button>
        </template>
      </div>
    </div>

    <div class="ce-grid">
      <label>Name <input v-model="form.name" placeholder="Aria Voss" /></label>
      <label>
        Type
        <select v-model="form.type">
          <option v-for="t in schemaTypes" :key="t.id" :value="t.id">
            {{ t.icon }} {{ t.label || t.id }}
          </option>
        </select>
      </label>
      <label>
        Scope
        <select v-model="form.scope">
          <option value="series">Series (shared across books)</option>
          <option v-for="b in books" :key="b.id" :value="b.id">Book: {{ b.title }}</option>
        </select>
      </label>
      <label class="wide">
        Aliases <span class="hint">comma-separated; also matched in the manuscript</span>
        <input v-model="form.aliasText" placeholder="Aria, the Ember Witch" />
      </label>
      <label class="wide">
        Summary
        <input v-model="form.summary" placeholder="One-line summary shown on hover cards" />
      </label>
      <label class="wide">
        Details <textarea v-model="form.details" rows="6" placeholder="Longer notes (markdown)" />
      </label>
    </div>

    <section>
      <div class="ce-sect-head">
        <h3>Facts</h3>
        <button class="btn" @click="addField()">+ Add fact</button>
      </div>
      <div v-if="suggestedFields.length" class="ce-suggest">
        Suggested:
        <button
          v-for="f in suggestedFields"
          :key="f"
          class="btn chip"
          @click="addField(f)"
        >
          {{ f }}
        </button>
      </div>
      <div v-for="(f, i) in form.fields" :key="i" class="ce-row">
        <input v-model="f.key" placeholder="age" class="ce-key" />
        <input v-model="f.value" placeholder="27" class="ce-val" />
        <button class="btn icon" @click="form.fields.splice(i, 1)">✕</button>
      </div>
    </section>

    <section>
      <div class="ce-sect-head">
        <h3>Facts that change over time</h3>
        <button class="btn" @click="addTimelineField">+ Add timelined fact</button>
      </div>
      <p class="hint">
        For a fact that changes across the story — a character's age, a title, a location.
        Give the values in order; each takes effect from its story point (leave the first
        "from the start") and hover cards show the value current at where you're reading.
      </p>
      <div v-for="(tf, ti) in form.fieldTimelines" :key="ti" class="ce-timeline">
        <div class="ce-row">
          <input v-model="tf.key" placeholder="age" class="ce-key" />
          <span class="hint">changes over time</span>
          <button class="btn icon" @click="form.fieldTimelines.splice(ti, 1)">✕</button>
        </div>
        <div v-for="(v, vi) in tf.values" :key="vi" class="ce-row ce-timeline-val">
          <input v-model="v.value" placeholder="17" class="ce-key" />
          <select v-model="v.book" class="ce-book">
            <option value="">from the start</option>
            <option v-for="b in books" :key="b.id" :value="b.id">{{ b.title }}</option>
          </select>
          <select v-if="v.book" v-model="v.chapter" class="ce-book">
            <option value="">start of book</option>
            <option v-for="c in chaptersOf(v.book)" :key="c" :value="c">{{ c }}</option>
          </select>
          <button class="btn icon" @click="tf.values.splice(vi, 1)">✕</button>
        </div>
        <button class="btn mini" @click="addTimelineValue(tf)">+ Add value</button>
      </div>
    </section>

    <section>
      <div class="ce-sect-head">
        <h3>Relationships</h3>
        <button class="btn" @click="addRelation">+ Add relationship</button>
      </div>
      <p class="hint">
        Directional and story-time aware — "serves X <em>until</em> book 1 chapter 12" is a
        different fact than "serves X". Relationship types are defined in the workspace schema
        (⚙ in the Codex sidebar).
      </p>
      <div v-for="(r, i) in form.relations" :key="i" class="ce-rel">
        <div class="ce-row">
          <select v-model="r.type" class="ce-key">
            <option v-for="d in relationDefs" :key="d.id" :value="d.id">{{ d.label }}</option>
          </select>
          <select v-model="r.to" class="ce-target">
            <option v-for="e in otherEntries" :key="e.id" :value="e.id">{{ e.name }}</option>
          </select>
          <input v-model="r.note" placeholder="note" class="ce-val" />
          <button class="btn icon" @click="form.relations.splice(i, 1)">✕</button>
        </div>
        <div class="ce-row ce-rel-time">
          <span class="hint">from</span>
          <select v-model="r.fromBook" class="ce-book">
            <option value="">the start</option>
            <option v-for="b in books" :key="b.id" :value="b.id">{{ b.title }}</option>
          </select>
          <select v-if="r.fromBook" v-model="r.fromChapter" class="ce-book">
            <option value="">start of book</option>
            <option v-for="c in chaptersOf(r.fromBook)" :key="c" :value="c">{{ c }}</option>
          </select>
          <span class="hint">until</span>
          <select v-model="r.untilBook" class="ce-book">
            <option value="">— ongoing —</option>
            <option v-for="b in books" :key="b.id" :value="b.id">{{ b.title }}</option>
          </select>
          <select v-if="r.untilBook" v-model="r.untilChapter" class="ce-book">
            <option value="">start of book</option>
            <option v-for="c in chaptersOf(r.untilBook)" :key="c" :value="c">{{ c }}</option>
          </select>
        </div>
      </div>
      <div v-if="incoming.length" class="ce-incoming">
        <h4>Referenced by</h4>
        <div v-for="(r, i) in incoming" :key="i" class="ce-in-row">
          <span class="ce-in-label">{{ r.label }}</span>
          <a class="ce-in-target" @click="openTab({ kind: 'codex', entryId: r.targetId })">
            {{ r.targetName }}
          </a>
          <span v-if="r.timespan" class="hint">({{ r.timespan }})</span>
          <span v-if="r.note" class="hint">— {{ r.note }}</span>
        </div>
      </div>
    </section>

    <section>
      <div class="ce-sect-head">
        <h3>Status timeline</h3>
        <button class="btn" @click="addStatus">+ Add status change</button>
      </div>
      <p class="hint">
        The consistency checker uses this. Example: <em>alive</em> from the start, then
        <em>dead</em> anchored to the chapter where it happens — any later scene where they act
        gets flagged.
      </p>
      <div v-for="(s, i) in form.status" :key="i" class="ce-row">
        <input v-model="s.state" placeholder="alive / dead / missing" class="ce-key" />
        <select v-model="s.at.book" class="ce-book">
          <option value="">from the start</option>
          <option v-for="b in books" :key="b.id" :value="b.id">{{ b.title }}</option>
        </select>
        <select v-if="s.at.book" v-model="s.at.chapter" class="ce-book">
          <option value="">start of book</option>
          <option v-for="c in chaptersOf(s.at.book!)" :key="c" :value="c">{{ c }}</option>
        </select>
        <input v-model="s.note" placeholder="note" class="ce-val" />
        <button class="btn icon" @click="form.status.splice(i, 1)">✕</button>
      </div>
    </section>

    <section v-if="!isNew">
      <div class="ce-sect-head">
        <h3>
          Appears in the manuscript
          <span v-if="backlinkTotal" class="ce-count">{{ backlinkTotal }}</span>
        </h3>
      </div>
      <p v-if="backlinksLoading" class="hint">Scanning chapters…</p>
      <p v-else-if="!backlinks.length" class="hint">
        No mentions found yet. The scan matches this entry's name and aliases in the prose.
      </p>
      <div v-for="(b, i) in backlinks" :key="i" class="ce-backlink" @click="openTab({ kind: 'chapter', bookId: b.bookId, chapter: b.chapter })">
        <div class="ce-bl-head">
          <span class="ce-bl-where">
            <span v-if="books.length > 1" class="ce-bl-book">{{ b.bookTitle }} · </span>{{ b.chapterTitle }}
          </span>
          <span class="ce-bl-count">{{ b.count }}×</span>
        </div>
        <p class="ce-bl-snippet">{{ b.snippet }}</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.codex-editor {
  height: 100%;
  overflow-y: auto;
  padding: 20px 28px 60px;
}
.ce-image-row {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  margin-bottom: 18px;
}
.ce-image {
  width: 120px;
  height: 120px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
  background: var(--nv-bg);
  border: 1px solid var(--nv-border);
}
.ce-image.empty {
  display: flex;
  align-items: center;
  justify-content: center;
  border-style: dashed;
}
.ce-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.ce-image-ph {
  font-size: 11px;
  color: var(--nv-faint);
}
.ce-image-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 4px;
}
.ce-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}
.ce-toolbar h2 {
  flex: 1;
  margin: 0;
  font-size: 18px;
}
.ce-msg {
  color: var(--nv-muted);
  font-size: 12px;
}
.ce-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 12px 16px;
  margin-bottom: 20px;
}
.ce-grid label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--nv-muted);
}
.ce-grid .wide {
  grid-column: 1 / -1;
}
section {
  margin-bottom: 22px;
}
.ce-sect-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.ce-sect-head h3 {
  margin: 0;
  font-size: 14px;
}
.ce-suggest {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--nv-faint);
  margin-bottom: 8px;
}
.btn.chip {
  padding: 1px 9px;
  font-size: 11px;
  border-radius: 10px;
}
.ce-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
  align-items: center;
}
.ce-key {
  width: 180px;
}
.ce-target {
  width: 220px;
}
.ce-book {
  width: 190px;
}
.ce-val {
  flex: 1;
}
.ce-rel {
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px 10px 4px;
  margin-bottom: 8px;
}
.ce-timeline {
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 8px;
}
.ce-timeline-val {
  margin-left: 18px;
}
.btn.mini {
  padding: 2px 9px;
  font-size: 11px;
}
.ce-rel-time {
  margin-bottom: 2px;
}
.ce-incoming {
  margin-top: 12px;
}
.ce-incoming h4 {
  margin: 0 0 6px;
  font-size: 12px;
  color: var(--nv-faint);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.ce-in-row {
  display: flex;
  gap: 6px;
  align-items: baseline;
  font-size: 13px;
  padding: 2px 0;
}
.ce-in-label {
  color: var(--nv-muted);
}
.ce-in-target {
  color: var(--nv-accent);
  cursor: pointer;
}
.ce-count {
  font-size: 11px;
  color: var(--nv-faint);
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 0 7px;
  margin-left: 6px;
}
.ce-backlink {
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 7px 10px;
  margin-bottom: 6px;
  cursor: pointer;
}
.ce-backlink:hover {
  border-color: var(--nv-accent);
  background: var(--nv-hover);
}
.ce-bl-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.ce-bl-where {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  text-transform: capitalize;
}
.ce-bl-book {
  color: var(--nv-muted);
  font-weight: normal;
  text-transform: none;
}
.ce-bl-count {
  font-size: 11px;
  color: var(--nv-faint);
}
.ce-bl-snippet {
  margin: 3px 0 0;
  font-size: 12px;
  color: var(--nv-muted);
  line-height: 1.4;
}
.hint {
  color: var(--nv-faint);
  font-size: 11px;
  font-weight: normal;
}
p.hint {
  margin: 0 0 10px;
  font-size: 12px;
}
</style>
