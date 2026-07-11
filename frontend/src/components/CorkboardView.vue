<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { BookScenes, MoveScene, SetSceneTitle } from '../api'
import { openTab, state } from '../store'
import type { ChapterScenes } from '../types'

const props = defineProps<{ bookId: string }>()

const book = computed(() => (state.workspace?.books ?? []).find((b) => b.id === props.bookId))
const chapters = ref<ChapterScenes[]>([])
const message = ref('')
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    chapters.value = await BookScenes(props.bookId)
    message.value = ''
  } catch (e) {
    message.value = `Couldn't load scenes: ${e}`
  } finally {
    loading.value = false
  }
}
onMounted(load)

const totalScenes = computed(() =>
  // Scene 0 is the chapter opening, not a "scene" the author placed.
  chapters.value.reduce((n, c) => n + Math.max(0, c.scenes.length - 1), 0),
)
const totalWords = computed(() =>
  chapters.value.reduce((n, c) => n + c.scenes.reduce((w, s) => w + s.words, 0), 0),
)

// --- drag & drop -----------------------------------------------------------
interface DragRef {
  chapter: string
  index: number
}
const dragged = ref<DragRef | null>(null)
// Visual drop indicator: which card, and which edge.
const over = ref<{ chapter: string; index: number; pos: 'before' | 'after' } | null>(null)

function onDragStart(chapter: string, index: number, e: DragEvent) {
  dragged.value = { chapter, index }
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    // Firefox requires data to be set for a drag to begin.
    e.dataTransfer.setData('text/plain', `${chapter}#${index}`)
  }
}

function onDragOver(chapter: string, index: number, e: DragEvent) {
  if (!dragged.value) return
  e.preventDefault()
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  // The opening (index 0) can only be dropped *after* — nothing precedes it.
  const before = index > 0 && e.clientY - rect.top < rect.height / 2
  over.value = { chapter, index, pos: before ? 'before' : 'after' }
}

function onDragEnd() {
  dragged.value = null
  over.value = null
}

async function onDrop(chapter: string, index: number, e: DragEvent) {
  e.preventDefault()
  const src = dragged.value
  over.value = null
  dragged.value = null
  if (!src) return

  // Desired insertion index in the destination chapter's current scene list.
  const pos = index > 0 && e.clientY - (e.currentTarget as HTMLElement).getBoundingClientRect().top <
    (e.currentTarget as HTMLElement).getBoundingClientRect().height / 2
    ? 'before'
    : 'after'
  let ins = pos === 'before' ? index : index + 1
  if (ins < 1) ins = 1

  if (src.chapter === chapter) {
    // No-op: dropped in its own slot.
    if (ins === src.index || ins === src.index + 1) return
    // Removal shifts everything after src down by one.
    const final = src.index < ins ? ins - 1 : ins
    if (final === src.index) return
    await doMove(src.chapter, src.index, chapter, final)
  } else {
    await doMove(src.chapter, src.index, chapter, ins)
  }
}

async function doMove(srcChapter: string, srcIndex: number, dstChapter: string, dstIndex: number) {
  try {
    chapters.value = await MoveScene(props.bookId, srcChapter, srcIndex, dstChapter, dstIndex)
    message.value = ''
  } catch (err) {
    message.value = `Move failed: ${err}`
  }
}

// --- inline retitle ---------------------------------------------------------
const editing = ref<{ chapter: string; index: number } | null>(null)
const draftTitle = ref('')

function startEdit(chapter: string, index: number, title: string) {
  editing.value = { chapter, index }
  draftTitle.value = title
}

async function commitEdit() {
  const ed = editing.value
  editing.value = null
  if (!ed) return
  const chap = chapters.value.find((c) => c.chapter === ed.chapter)
  const cur = chap?.scenes[ed.index]
  if (!cur || cur.title === draftTitle.value.trim()) return
  try {
    chapters.value = await SetSceneTitle(props.bookId, ed.chapter, ed.index, draftTitle.value.trim())
  } catch (err) {
    message.value = `Rename failed: ${err}`
  }
}

function openChapter(chapter: string) {
  openTab({ kind: 'chapter', bookId: props.bookId, chapter })
}

function indicatorClass(chapter: string, index: number) {
  const o = over.value
  if (!o || o.chapter !== chapter || o.index !== index) return {}
  return { 'drop-before': o.pos === 'before', 'drop-after': o.pos === 'after' }
}
</script>

<template>
  <div class="cork">
    <div class="ck-toolbar">
      <h2>Corkboard — {{ book?.title }}</h2>
      <span class="ck-stats">{{ totalScenes }} scenes · {{ totalWords.toLocaleString() }} words</span>
      <span class="ck-msg">{{ message }}</span>
    </div>

    <p class="ck-hint">
      Drag scene cards to reorder them or move them into another chapter. Add a scene while writing
      with a <code>&lt;!-- scene: Title --&gt;</code> divider (or the editor's “＋ Scene” button).
    </p>

    <div v-if="loading" class="ck-empty">Loading scenes…</div>
    <div v-else-if="!chapters.length" class="ck-empty">This book has no chapters yet.</div>

    <div v-for="chap in chapters" v-else :key="chap.chapter" class="ck-chapter">
      <div class="ck-chapter-head">
        <span class="ck-chapter-title" @click="openChapter(chap.chapter)">{{ chap.title }}</span>
        <span class="ck-chapter-meta">
          {{ Math.max(0, chap.scenes.length - 1) }} scene(s)
        </span>
      </div>

      <div class="ck-cards">
        <div
          v-for="scene in chap.scenes"
          :key="scene.index"
          class="ck-card"
          :class="{ opening: scene.index === 0, dragging: dragged?.chapter === chap.chapter && dragged?.index === scene.index, ...indicatorClass(chap.chapter, scene.index) }"
          :draggable="scene.index > 0"
          @dragstart="onDragStart(chap.chapter, scene.index, $event)"
          @dragover="onDragOver(chap.chapter, scene.index, $event)"
          @drop="onDrop(chap.chapter, scene.index, $event)"
          @dragend="onDragEnd"
        >
          <div class="ck-card-head">
            <span v-if="scene.index === 0" class="ck-badge">opening</span>
            <span v-else class="ck-grip" title="Drag to reorder">⠿</span>

            <template v-if="scene.index > 0">
              <input
                v-if="editing?.chapter === chap.chapter && editing?.index === scene.index"
                class="ck-title-input"
                :value="draftTitle"
                autofocus
                @input="draftTitle = ($event.target as HTMLInputElement).value"
                @keydown.enter.prevent="commitEdit"
                @keydown.esc.prevent="editing = null"
                @blur="commitEdit"
              />
              <span
                v-else
                class="ck-title"
                :class="{ untitled: !scene.title }"
                @click="startEdit(chap.chapter, scene.index, scene.title)"
                >{{ scene.title || 'Untitled scene' }}</span
              >
            </template>
            <span v-else class="ck-title opening-title" @click="openChapter(chap.chapter)">
              {{ chap.title }}
            </span>

            <span class="ck-words">{{ scene.words.toLocaleString() }}w</span>
          </div>

          <p class="ck-snippet" @click="openChapter(chap.chapter)">
            {{ scene.snippet || '—' }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cork {
  height: 100%;
  overflow-y: auto;
  padding: 18px 24px 60px;
}
.ck-toolbar {
  display: flex;
  align-items: baseline;
  gap: 14px;
  margin-bottom: 4px;
}
.ck-toolbar h2 {
  margin: 0;
  font-size: 18px;
}
.ck-stats {
  color: var(--nv-muted);
  font-size: 12px;
}
.ck-msg {
  color: var(--nv-warning);
  font-size: 12px;
  margin-left: auto;
}
.ck-hint {
  font-size: 12px;
  color: var(--nv-faint);
  margin: 0 0 16px;
}
.ck-hint code {
  background: var(--nv-panel);
  padding: 1px 4px;
  border-radius: 4px;
}
.ck-empty {
  color: var(--nv-muted);
  padding: 40px 0;
  text-align: center;
}
.ck-chapter {
  margin-bottom: 22px;
}
.ck-chapter-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 8px;
  border-bottom: 1px solid var(--nv-border);
  padding-bottom: 4px;
}
.ck-chapter-title {
  font-weight: 600;
  text-transform: capitalize;
  cursor: pointer;
}
.ck-chapter-title:hover {
  color: var(--nv-accent);
}
.ck-chapter-meta {
  font-size: 11px;
  color: var(--nv-faint);
}
.ck-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 12px;
}
.ck-card {
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 10px;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 96px;
  cursor: grab;
  position: relative;
  transition: box-shadow 0.1s;
}
.ck-card.opening {
  cursor: default;
  border-style: dashed;
  background: color-mix(in srgb, var(--nv-panel) 60%, transparent);
}
.ck-card.dragging {
  opacity: 0.4;
}
/* Drop indicators — a bright edge showing where the card will land. */
.ck-card.drop-before::before,
.ck-card.drop-after::after {
  content: '';
  position: absolute;
  left: -6px;
  right: -6px;
  height: 3px;
  background: var(--nv-accent);
  border-radius: 2px;
}
.ck-card.drop-before::before {
  top: -7px;
}
.ck-card.drop-after::after {
  bottom: -7px;
}
.ck-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.ck-badge {
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--nv-faint);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
  padding: 0 5px;
}
.ck-grip {
  color: var(--nv-faint);
  cursor: grab;
  font-size: 13px;
  line-height: 1;
}
.ck-title {
  flex: 1;
  font-weight: 600;
  font-size: 13px;
  cursor: text;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ck-title.opening-title {
  cursor: pointer;
  text-transform: capitalize;
  color: var(--nv-muted);
}
.ck-title.untitled {
  color: var(--nv-faint);
  font-style: italic;
  font-weight: 500;
}
.ck-title-input {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  padding: 2px 4px;
}
.ck-words {
  font-size: 10px;
  color: var(--nv-faint);
  flex-shrink: 0;
}
.ck-snippet {
  margin: 0;
  font-size: 12px;
  line-height: 1.4;
  color: var(--nv-muted);
  cursor: pointer;
  display: -webkit-box;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
