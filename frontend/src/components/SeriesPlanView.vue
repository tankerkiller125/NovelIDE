<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { BookInsights, MoveBook, SaveSeriesPlan } from '../api'
import { codexById, openTab, pinActiveTab, setWorkspace, state } from '../store'
import type { ChapterInsight, SeriesBookPlan } from '../types'
import { PLAN_STATUSES } from '../types'

const books = computed(() => state.workspace?.books ?? [])
const arcs = computed(() => (state.workspace?.codex ?? []).filter((e) => e.type === 'arc'))

const synopsis = ref('')
const cards = reactive(new Map<string, SeriesBookPlan>())
const insights = ref<Record<string, Record<string, ChapterInsight>>>({})
const saving = ref(false)
const message = ref('')

function blankCard(id: string): SeriesBookPlan {
  return { id, synopsis: '', status: 'outlined', arcs: [], targetWords: 0 }
}

function seed() {
  synopsis.value = state.workspace?.seriesPlan?.synopsis ?? ''
  cards.clear()
  const stored = new Map((state.workspace?.seriesPlan?.books ?? []).map((b) => [b.id, b]))
  for (const b of books.value) {
    const s = stored.get(b.id)
    cards.set(b.id, s ? { ...s, arcs: [...(s.arcs ?? [])] } : blankCard(b.id))
  }
}
// Reseed only when the book list changes, so in-flight saves can't clobber
// newer keystrokes.
watch(() => books.value.map((b) => b.id).join('|'), seed, { immediate: true })

async function refreshInsights() {
  const out: Record<string, Record<string, ChapterInsight>> = {}
  for (const b of books.value) {
    try {
      out[b.id] = await BookInsights(b.id)
    } catch {
      out[b.id] = {}
    }
  }
  insights.value = out
}
onMounted(refreshInsights)

function bookWords(id: string): number {
  return Object.values(insights.value[id] ?? {}).reduce((n, i) => n + i.words, 0)
}
const totalWords = computed(() => books.value.reduce((n, b) => n + bookWords(b.id), 0))

/** Arc ids rolled up from a book's chapter-level plan tags. */
function chapterArcs(bookId: string): Set<string> {
  const out = new Set<string>()
  const book = books.value.find((b) => b.id === bookId)
  for (const cp of book?.plan ?? []) {
    for (const a of cp.arcs ?? []) out.add(a)
  }
  return out
}

type Cell = 'planned' | 'chapters' | 'both' | ''
function matrixCell(arcId: string, bookId: string): Cell {
  const planned = cards.get(bookId)?.arcs?.includes(arcId) ?? false
  const inChapters = chapterArcs(bookId).has(arcId)
  if (planned && inChapters) return 'both'
  if (planned) return 'planned'
  if (inChapters) return 'chapters'
  return ''
}

function toggleArc(bookId: string, arcId: string) {
  const card = cards.get(bookId)
  if (!card) return
  const list = card.arcs ?? (card.arcs = [])
  const i = list.indexOf(arcId)
  if (i >= 0) list.splice(i, 1)
  else list.push(arcId)
  scheduleSave()
}

function nameOf(id: string) {
  return codexById.value.get(id)?.name ?? id
}

/** Per-book chapter status summary, e.g. "1 drafted · 1 revised". */
function statusSummary(bookId: string): string {
  const book = books.value.find((b) => b.id === bookId)
  const byFile = new Map((book?.plan ?? []).map((p) => [p.file, p.status || 'outlined']))
  const counts: Record<string, number> = {}
  for (const ch of book?.chapters ?? []) {
    const s = byFile.get(ch) ?? 'outlined'
    counts[s] = (counts[s] ?? 0) + 1
  }
  return PLAN_STATUSES.filter((s) => counts[s])
    .map((s) => `${counts[s]} ${s}`)
    .join(' · ')
}

let saveTimer: number | undefined
function scheduleSave() {
  pinActiveTab() // any series-plan edit (incl. matrix/arc clicks) keeps the tab
  window.clearTimeout(saveTimer)
  saveTimer = window.setTimeout(save, 700)
}
async function save() {
  saving.value = true
  message.value = ''
  try {
    const bookCards = books.value
      .map((b) => cards.get(b.id))
      .filter((c): c is SeriesBookPlan => !!c)
      .filter((c) => c.synopsis || (c.arcs?.length ?? 0) > 0 || c.targetWords > 0 || c.status !== 'outlined')
    setWorkspace(await SaveSeriesPlan({ synopsis: synopsis.value, books: bookCards }))
    message.value = 'Saved.'
  } catch (e) {
    message.value = `Save failed: ${e}`
  } finally {
    saving.value = false
  }
}

async function move(bookId: string, delta: number) {
  pinActiveTab()
  window.clearTimeout(saveTimer)
  try {
    setWorkspace(await MoveBook(bookId, delta))
    await refreshInsights()
  } catch (e) {
    message.value = `Move failed: ${e}`
  }
}
</script>

<template>
  <div class="sp-view" @input="pinActiveTab" @change="pinActiveTab">
    <div class="sp-toolbar">
      <h2>Series Plan — {{ state.workspace?.manifest.name }}</h2>
      <span class="sp-msg">{{ saving ? 'Saving…' : message }}</span>
      <span class="sp-total">{{ totalWords.toLocaleString() }} words across the series</span>
    </div>

    <textarea
      class="sp-synopsis"
      rows="3"
      placeholder="What is this series about? The promise made in book one, kept in the last."
      v-model="synopsis"
      @input="scheduleSave"
    />

    <div class="sp-books">
      <div v-for="(b, i) in books" :key="b.id" class="sp-card">
        <div class="sp-card-head">
          <span class="sp-num">{{ i + 1 }}</span>
          <span class="sp-title" @click="openTab({ kind: 'plan', bookId: b.id })">
            {{ b.title }}
          </span>
          <span class="sp-move">
            <button class="btn icon" :disabled="i === 0" @click="move(b.id, -1)">↑</button>
            <button class="btn icon" :disabled="i === books.length - 1" @click="move(b.id, 1)">
              ↓
            </button>
          </span>
        </div>
        <div class="sp-meta">
          <select
            :value="cards.get(b.id)?.status || 'outlined'"
            :class="'st-' + (cards.get(b.id)?.status || 'outlined')"
            @change="(cards.get(b.id)!.status = ($event.target as HTMLSelectElement).value), scheduleSave()"
          >
            <option v-for="s in PLAN_STATUSES" :key="s" :value="s">{{ s }}</option>
          </select>
          <span class="sp-words">
            {{ bookWords(b.id).toLocaleString() }}
            <template v-if="cards.get(b.id)?.targetWords">
              / {{ cards.get(b.id)!.targetWords.toLocaleString() }}
            </template>
            words
          </span>
          <input
            class="sp-target"
            type="number"
            min="0"
            step="1000"
            placeholder="target"
            :value="cards.get(b.id)?.targetWords || ''"
            @input="(cards.get(b.id)!.targetWords = Number(($event.target as HTMLInputElement).value) || 0), scheduleSave()"
          />
        </div>
        <div
          v-if="cards.get(b.id)?.targetWords"
          class="sp-progress"
          :title="`${Math.min(100, Math.round((bookWords(b.id) / cards.get(b.id)!.targetWords) * 100))}%`"
        >
          <div
            class="sp-progress-fill"
            :style="{
              width: Math.min(100, (bookWords(b.id) / cards.get(b.id)!.targetWords) * 100) + '%',
            }"
          />
        </div>
        <textarea
          class="sp-book-synopsis"
          rows="2"
          placeholder="This book's promise…"
          :value="cards.get(b.id)?.synopsis"
          @input="(cards.get(b.id)!.synopsis = ($event.target as HTMLTextAreaElement).value), scheduleSave()"
        />
        <div class="sp-chapter-status">{{ statusSummary(b.id) }}</div>
      </div>
    </div>

    <section v-if="arcs.length" class="sp-matrix-section">
      <h3>Threads across the series</h3>
      <p class="sp-hint">
        ● planned for the book · ◐ tagged on its chapters · ⬤ both. Click a cell to toggle the
        book-level plan.
      </p>
      <div class="sp-matrix-scroll">
        <table class="sp-matrix">
          <thead>
            <tr>
              <th></th>
              <th v-for="b in books" :key="b.id">{{ b.title }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="a in arcs" :key="a.id">
              <td class="sp-arc-name" @click="openTab({ kind: 'codex', entryId: a.id })">
                🧵 {{ nameOf(a.id) }}
              </td>
              <td
                v-for="b in books"
                :key="b.id"
                class="sp-cell"
                :class="matrixCell(a.id, b.id)"
                @click="toggleArc(b.id, a.id)"
              >
                <span v-if="matrixCell(a.id, b.id) === 'both'">⬤</span>
                <span v-else-if="matrixCell(a.id, b.id) === 'planned'">●</span>
                <span v-else-if="matrixCell(a.id, b.id) === 'chapters'">◐</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
    <p v-else class="sp-hint">
      Add Codex entries of type “Arc / Thread” to track plot threads across books.
    </p>
  </div>
</template>

<style scoped>
.sp-view {
  height: 100%;
  overflow-y: auto;
  padding: 18px 24px 60px;
}
.sp-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 10px;
}
.sp-toolbar h2 {
  flex: 1;
  margin: 0;
  font-size: 18px;
}
.sp-msg {
  color: var(--nv-muted);
  font-size: 12px;
}
.sp-total {
  color: var(--nv-faint);
  font-size: 12px;
}
.sp-synopsis {
  width: 100%;
  margin-bottom: 16px;
}
.sp-books {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
  margin-bottom: 24px;
}
.sp-card {
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 10px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.sp-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sp-num {
  color: var(--nv-faint);
  font-size: 12px;
}
.sp-title {
  flex: 1;
  font-weight: 600;
  cursor: pointer;
}
.sp-title:hover {
  color: var(--nv-accent);
}
.sp-move {
  display: flex;
  gap: 2px;
}
.sp-meta {
  display: flex;
  gap: 8px;
  align-items: center;
}
.sp-meta select {
  font-size: 12px;
  padding: 3px 6px;
}
.sp-words {
  font-size: 12px;
  color: var(--nv-muted);
  flex: 1;
}
.sp-target {
  width: 90px;
  font-size: 12px;
  padding: 3px 6px;
}
.sp-progress {
  height: 4px;
  border-radius: 2px;
  background: var(--nv-hover);
  overflow: hidden;
}
.sp-progress-fill {
  height: 100%;
  background: var(--nv-accent);
}
.sp-book-synopsis {
  width: 100%;
  resize: vertical;
  font-size: 13px;
}
.sp-chapter-status {
  font-size: 11px;
  color: var(--nv-faint);
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
.sp-matrix-section h3 {
  margin: 0 0 4px;
  font-size: 14px;
}
.sp-hint {
  color: var(--nv-faint);
  font-size: 12px;
  margin: 0 0 10px;
}
.sp-matrix-scroll {
  overflow-x: auto;
}
.sp-matrix {
  border-collapse: collapse;
  font-size: 12px;
}
.sp-matrix th {
  text-align: left;
  padding: 6px 14px;
  color: var(--nv-muted);
  font-weight: 600;
}
.sp-arc-name {
  padding: 6px 14px 6px 0;
  white-space: nowrap;
  cursor: pointer;
  color: var(--nv-text);
}
.sp-arc-name:hover {
  color: var(--nv-accent);
}
.sp-cell {
  text-align: center;
  padding: 6px 14px;
  cursor: pointer;
  color: var(--nv-accent);
  border: 1px solid var(--nv-border);
  min-width: 90px;
}
.sp-cell.chapters {
  color: var(--nv-muted);
}
.sp-cell:hover {
  background: var(--nv-hover);
}
</style>
