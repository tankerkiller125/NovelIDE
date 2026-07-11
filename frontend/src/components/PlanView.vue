<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { BookInsights, MoveChapter, SavePlan } from '../api'
import { cardDataAt, codexById, openTab, pinActiveTab, relationsOf, setWorkspace, state } from '../store'
import type { ChapterInsight, ChapterPlan } from '../types'
import { PLAN_STATUSES } from '../types'

const props = defineProps<{ bookId: string }>()

const book = computed(() => (state.workspace?.books ?? []).find((b) => b.id === props.bookId))
const insights = ref<Record<string, ChapterInsight>>({})
const saving = ref(false)
const message = ref('')
const chronology = ref(false)

// Editable plan rows, keyed by chapter file, seeded from the stored plan.
const rows = reactive(new Map<string, ChapterPlan>())

function blankRow(file: string): ChapterPlan {
  return { file, synopsis: '', status: 'outlined', pov: '', location: '', when: '', arcs: [] }
}

function seed() {
  rows.clear()
  const stored = new Map((book.value?.plan ?? []).map((p) => [p.file, p]))
  for (const ch of book.value?.chapters ?? []) {
    const s = stored.get(ch)
    rows.set(ch, s ? { ...s, arcs: [...(s.arcs ?? [])] } : blankRow(ch))
  }
}
// Reseed only when the chapter list itself changes (create/reorder), not on
// every workspace refresh — otherwise a save landing mid-keystroke would
// clobber newer edits.
watch(() => (book.value?.chapters ?? []).join('|'), seed, { immediate: true })

async function refreshInsights() {
  try {
    insights.value = await BookInsights(props.bookId)
  } catch (e) {
    console.error('insights failed', e)
  }
}
onMounted(refreshInsights)

const characters = computed(() =>
  (state.workspace?.codex ?? []).filter((e) => e.type === 'character'),
)
const locations = computed(() => (state.workspace?.codex ?? []).filter((e) => e.type === 'location'))
const arcs = computed(() => (state.workspace?.codex ?? []).filter((e) => e.type === 'arc'))

const orderedChapters = computed(() => {
  const chs = [...(book.value?.chapters ?? [])]
  if (!chronology.value) return chs
  return chs.sort((a, b) => {
    const wa = rows.get(a)?.when ?? ''
    const wb = rows.get(b)?.when ?? ''
    if (!wa && !wb) return 0
    if (!wa) return 1
    if (!wb) return -1
    return wa.localeCompare(wb)
  })
})

const totalWords = computed(() =>
  Object.values(insights.value).reduce((n, i) => n + i.words, 0),
)
const statusCounts = computed(() => {
  const counts: Record<string, number> = {}
  for (const ch of book.value?.chapters ?? []) {
    const s = rows.get(ch)?.status || 'outlined'
    counts[s] = (counts[s] ?? 0) + 1
  }
  return counts
})

function pretty(file: string) {
  return file.replace(/\.md$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')
}
function nameOf(id: string) {
  return codexById.value.get(id)?.name ?? id
}

let saveTimer: number | undefined
function scheduleSave() {
  pinActiveTab() // any plan edit (incl. button-driven arc toggles) keeps the tab
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(save, 700)
}
async function save() {
  saving.value = true
  message.value = ''
  try {
    const plan = (book.value?.chapters ?? [])
      .map((ch) => rows.get(ch))
      .filter((p): p is ChapterPlan => !!p)
      .filter(
        (p) => p.synopsis || p.pov || p.location || p.when || (p.arcs?.length ?? 0) > 0 || p.status !== 'outlined',
      )
    setWorkspace(await SavePlan(props.bookId, plan))
    message.value = 'Saved.'
  } catch (e) {
    message.value = `Save failed: ${e}`
  } finally {
    saving.value = false
  }
}

async function move(ch: string, delta: number) {
  pinActiveTab()
  window.clearTimeout(saveTimer)
  try {
    setWorkspace(await MoveChapter(props.bookId, ch, delta))
    await refreshInsights()
  } catch (e) {
    message.value = `Move failed: ${e}`
  }
}

function toggleArc(ch: string, arcId: string) {
  const row = rows.get(ch)
  if (!row) return
  const arcs = row.arcs ?? (row.arcs = [])
  const i = arcs.indexOf(arcId)
  if (i >= 0) arcs.splice(i, 1)
  else arcs.push(arcId)
  scheduleSave()
}

const goneStates = new Set(['dead', 'deceased', 'destroyed', 'killed'])

/** Plan-vs-manuscript checks, computed live per card. */
function warningsFor(ch: string): string[] {
  const row = rows.get(ch)
  const insight = insights.value[ch]
  const out: string[] = []
  if (!row) return out
  if (row.pov) {
    const st = cardDataAt(row.pov, props.bookId, ch)?.state
    if (st && goneStates.has(st.state.toLowerCase())) {
      out.push(`POV ${nameOf(row.pov)} is ${st.state} at this point in the story`)
    }
    if (insight && insight.cast.length > 0 && !insight.cast.includes(row.pov)) {
      out.push(`POV ${nameOf(row.pov)} never appears in the chapter text`)
    }
  }
  if (insight) {
    for (const arcId of row.arcs ?? []) {
      const involved = new Set(relationsOf(arcId).map((r) => r.targetId))
      involved.add(arcId)
      if (![...involved].some((id) => insight.cast.includes(id))) {
        out.push(`No trace of "${nameOf(arcId)}" — none of its entities appear here`)
      }
    }
  }
  return out
}
</script>

<template>
  <div class="plan-view" @input="pinActiveTab" @change="pinActiveTab">
    <div class="pv-toolbar">
      <h2>Plan — {{ book?.title }}</h2>
      <span class="pv-msg">{{ saving ? 'Saving…' : message }}</span>
      <label class="pv-chrono">
        <input type="checkbox" v-model="chronology" />
        Sort by story time
      </label>
    </div>
    <div class="pv-stats">
      <span class="pv-stat">{{ totalWords.toLocaleString() }} words</span>
      <span v-for="s in PLAN_STATUSES" :key="s" class="pv-stat" :class="'st-' + s">
        {{ statusCounts[s] ?? 0 }} {{ s }}
      </span>
      <span v-if="!arcs.length" class="pv-hint">
        Tip: add entries of type “Arc / Thread” to the Codex to tag chapters with plot threads.
      </span>
    </div>

    <div class="pv-grid">
      <div v-for="(ch, i) in orderedChapters" :key="ch" class="pv-card">
        <div class="pv-card-head">
          <span class="pv-num">{{ i + 1 }}</span>
          <span class="pv-title" @click="openTab({ kind: 'chapter', bookId, chapter: ch })">
            {{ pretty(ch) }}
          </span>
          <span class="pv-words">{{ insights[ch]?.words?.toLocaleString() ?? '…' }}w</span>
          <span class="pv-move" v-if="!chronology">
            <button class="btn icon" :disabled="i === 0" @click="move(ch, -1)">↑</button>
            <button
              class="btn icon"
              :disabled="i === orderedChapters.length - 1"
              @click="move(ch, 1)"
            >
              ↓
            </button>
          </span>
        </div>

        <textarea
          class="pv-synopsis"
          rows="3"
          placeholder="What happens in this chapter?"
          :value="rows.get(ch)?.synopsis"
          @input="(rows.get(ch)!.synopsis = ($event.target as HTMLTextAreaElement).value), scheduleSave()"
        />

        <div class="pv-row">
          <select
            :value="rows.get(ch)?.status || 'outlined'"
            class="pv-status"
            :class="'st-' + (rows.get(ch)?.status || 'outlined')"
            @change="(rows.get(ch)!.status = ($event.target as HTMLSelectElement).value), scheduleSave()"
          >
            <option v-for="s in PLAN_STATUSES" :key="s" :value="s">{{ s }}</option>
          </select>
          <select
            :value="rows.get(ch)?.pov ?? ''"
            @change="(rows.get(ch)!.pov = ($event.target as HTMLSelectElement).value), scheduleSave()"
          >
            <option value="">— POV —</option>
            <option v-for="c in characters" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
          <select
            :value="rows.get(ch)?.location ?? ''"
            @change="(rows.get(ch)!.location = ($event.target as HTMLSelectElement).value), scheduleSave()"
          >
            <option value="">— location —</option>
            <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.name }}</option>
          </select>
          <input
            class="pv-when"
            placeholder="story time (e.g. 3127-04)"
            :value="rows.get(ch)?.when"
            @input="(rows.get(ch)!.when = ($event.target as HTMLInputElement).value), scheduleSave()"
          />
        </div>

        <div v-if="arcs.length" class="pv-arcs">
          <button
            v-for="a in arcs"
            :key="a.id"
            class="pv-arc"
            :class="{ on: rows.get(ch)?.arcs?.includes(a.id) }"
            @click="toggleArc(ch, a.id)"
          >
            🧵 {{ a.name }}
          </button>
        </div>

        <div v-if="insights[ch]?.cast?.length" class="pv-cast">
          <span class="pv-cast-label">In the text:</span>
          <a
            v-for="id in insights[ch].cast"
            :key="id"
            class="pv-cast-chip"
            @click="openTab({ kind: 'codex', entryId: id })"
          >
            {{ nameOf(id) }}
          </a>
        </div>

        <div v-for="(w, wi) in warningsFor(ch)" :key="wi" class="pv-warning">⚠ {{ w }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.plan-view {
  height: 100%;
  overflow-y: auto;
  padding: 18px 24px 60px;
}
.pv-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 6px;
}
.pv-toolbar h2 {
  flex: 1;
  margin: 0;
  font-size: 18px;
}
.pv-msg {
  color: var(--nv-muted);
  font-size: 12px;
}
.pv-chrono {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--nv-muted);
  cursor: pointer;
}
.pv-stats {
  display: flex;
  gap: 14px;
  align-items: baseline;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.pv-stat {
  font-size: 12px;
  color: var(--nv-muted);
}
.pv-hint {
  font-size: 11px;
  color: var(--nv-faint);
}
.pv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 14px;
}
.pv-card {
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 10px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.pv-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.pv-num {
  color: var(--nv-faint);
  font-size: 12px;
  min-width: 18px;
}
.pv-title {
  flex: 1;
  font-weight: 600;
  text-transform: capitalize;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pv-title:hover {
  color: var(--nv-accent);
}
.pv-words {
  font-size: 11px;
  color: var(--nv-faint);
}
.pv-move {
  display: flex;
  gap: 2px;
}
.pv-synopsis {
  width: 100%;
  resize: vertical;
  font-size: 13px;
}
.pv-row {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.pv-row select,
.pv-when {
  font-size: 12px;
  padding: 4px 6px;
}
.pv-when {
  flex: 1;
  min-width: 110px;
}
.pv-status.st-outlined {
  color: var(--nv-muted);
}
.pv-status.st-drafted {
  color: var(--nv-accent);
}
.pv-status.st-revised {
  color: #6fa8dc;
}
.pv-status.st-final {
  color: #6fcf97;
}
.st-outlined {
  color: var(--nv-muted);
}
.st-drafted {
  color: var(--nv-accent);
}
.st-revised {
  color: #6fa8dc;
}
.st-final {
  color: #6fcf97;
}
.pv-arcs {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.pv-arc {
  background: none;
  border: 1px dashed var(--nv-border);
  border-radius: 10px;
  color: var(--nv-faint);
  padding: 1px 9px;
  font-size: 11px;
  cursor: pointer;
}
.pv-arc.on {
  border-style: solid;
  border-color: var(--nv-accent);
  color: var(--nv-accent);
}
.pv-cast {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: baseline;
}
.pv-cast-label {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
}
.pv-cast-chip {
  font-size: 11.5px;
  color: var(--nv-muted);
  cursor: pointer;
}
.pv-cast-chip:hover {
  color: var(--nv-accent);
}
.pv-warning {
  font-size: 12px;
  color: var(--nv-warning);
}
</style>
