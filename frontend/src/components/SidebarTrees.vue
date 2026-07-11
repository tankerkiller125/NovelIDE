<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  CloseWorkspace,
  CreateBook,
  CreateChapter,
  DeleteBook,
  DeleteChapter,
  RenameBook,
  RenameChapter,
} from '../api'
import {
  closeProject,
  closeTab,
  confirmDialog,
  openTab,
  promptInput,
  schemaTypes,
  setWorkspace,
  state,
  tabKey,
} from '../store'
import type { CodexEntry } from '../types'

const view = ref<'manuscript' | 'codex'>('manuscript')
const collapsedBooks = ref(new Set<string>())
const error = ref('')

const books = computed(() => state.workspace?.books ?? [])
const isSeries = computed(() => state.workspace?.manifest.kind === 'series')

const codexByType = computed(() => {
  const groups = new Map<string, CodexEntry[]>()
  for (const t of schemaTypes.value) groups.set(t.id, [])
  for (const e of state.workspace?.codex ?? []) {
    groups.get(e.type)?.push(e)
  }
  return groups
})

function toggleBook(id: string) {
  const s = collapsedBooks.value
  s.has(id) ? s.delete(id) : s.add(id)
}

function chapterKey(bookId: string, chapter: string) {
  return tabKey({ kind: 'chapter', bookId, chapter })
}

async function addChapter(bookId: string) {
  const title = await promptInput({ title: 'New chapter', label: 'Chapter title', placeholder: 'Chapter One' })
  if (!title) return
  try {
    const res = await CreateChapter(bookId, title)
    setWorkspace(res.workspace)
    openTab({ kind: 'chapter', bookId, chapter: res.chapter })
  } catch (e) {
    error.value = String(e)
  }
}

async function addBook() {
  const title = await promptInput({ title: 'New book', label: 'Book title', placeholder: 'Book One' })
  if (!title) return
  try {
    setWorkspace(await CreateBook(title))
  } catch (e) {
    error.value = String(e)
  }
}

function newCodexEntry() {
  openTab({ kind: 'codex', entryId: '' })
}

function prettyChapter(name: string) {
  return name.replace(/\.md$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')
}

async function switchProject() {
  if (
    state.dirtyChapters.size > 0 &&
    !(await confirmDialog({
      title: 'Switch project?',
      message: 'A chapter is still saving. Switch anyway?',
      confirmText: 'Switch',
    }))
  )
    return
  await CloseWorkspace()
  closeProject()
}

// ---- right-click context menu ----
interface CtxMenu {
  x: number
  y: number
  kind: 'book' | 'chapter'
  bookId: string
  chapter?: string
  label: string
}
const ctx = ref<CtxMenu | null>(null)

function openBookMenu(e: MouseEvent, bookId: string, label: string) {
  ctx.value = { x: e.clientX, y: e.clientY, kind: 'book', bookId, label }
}
function openChapterMenu(e: MouseEvent, bookId: string, chapter: string) {
  ctx.value = { x: e.clientX, y: e.clientY, kind: 'chapter', bookId, chapter, label: prettyChapter(chapter) }
}
function closeMenu() {
  ctx.value = null
}
onMounted(() => window.addEventListener('click', closeMenu))
onUnmounted(() => {
  window.removeEventListener('click', closeMenu)
  endResize()
})

// ---- user-resizable width (min = default, no maximum) ----
const MIN_WIDTH = 240
const sidebarWidth = ref(clampWidth(Number(localStorage.getItem('nv-sidebar-w')) || MIN_WIDTH))
function clampWidth(w: number) {
  return Number.isFinite(w) && w > MIN_WIDTH ? Math.round(w) : MIN_WIDTH
}
let dragStartX = 0
let dragStartW = MIN_WIDTH
function startResize(e: MouseEvent) {
  dragStartX = e.clientX
  dragStartW = sidebarWidth.value
  window.addEventListener('mousemove', onResize)
  window.addEventListener('mouseup', endResize)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  e.preventDefault()
}
function onResize(e: MouseEvent) {
  sidebarWidth.value = Math.max(MIN_WIDTH, dragStartW + (e.clientX - dragStartX))
}
function endResize() {
  window.removeEventListener('mousemove', onResize)
  window.removeEventListener('mouseup', endResize)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  localStorage.setItem('nv-sidebar-w', String(sidebarWidth.value))
}

async function renameChapterAction(bookId: string, chapter: string, current: string) {
  const title = await promptInput({ title: 'Rename chapter', label: 'Chapter title', value: current })
  if (!title) return
  try {
    const res = await RenameChapter(bookId, chapter, title)
    const wasOpen = state.tabs.some((t) => tabKey(t) === chapterKey(bookId, chapter))
    setWorkspace(res.workspace)
    if (wasOpen) openTab({ kind: 'chapter', bookId, chapter: res.chapter })
  } catch (e) {
    error.value = String(e)
  }
}

async function deleteChapterAction(bookId: string, chapter: string, label: string) {
  if (
    !(await confirmDialog({
      title: 'Delete chapter',
      message: `Delete “${label}”? The manuscript file is removed and any codex timeline anchors pointing at it are cleared. This can't be undone.`,
      confirmText: 'Delete',
      danger: true,
    }))
  )
    return
  try {
    closeTab(chapterKey(bookId, chapter))
    setWorkspace(await DeleteChapter(bookId, chapter))
  } catch (e) {
    error.value = String(e)
  }
}

async function renameBookAction(bookId: string, current: string) {
  const title = await promptInput({ title: 'Rename book', label: 'Book title', value: current })
  if (!title) return
  try {
    setWorkspace(await RenameBook(bookId, title))
  } catch (e) {
    error.value = String(e)
  }
}

async function deleteBookAction(bookId: string, label: string) {
  if (
    !(await confirmDialog({
      title: 'Delete book',
      message: `Delete “${label}” and all its chapters? Codex anchors and series-plan entries pointing at this book are cleared. This can't be undone.`,
      confirmText: 'Delete book',
      danger: true,
    }))
  )
    return
  try {
    setWorkspace(await DeleteBook(bookId))
  } catch (e) {
    error.value = String(e)
  }
}

function runCtx(action: string) {
  const c = ctx.value
  if (!c) return
  closeMenu()
  if (c.kind === 'book') {
    if (action === 'rename') renameBookAction(c.bookId, c.label)
    else if (action === 'delete') deleteBookAction(c.bookId, c.label)
    else if (action === 'chapter') addChapter(c.bookId)
    else if (action === 'plan') openTab({ kind: 'plan', bookId: c.bookId })
    else if (action === 'corkboard') openTab({ kind: 'corkboard', bookId: c.bookId })
  } else if (c.chapter) {
    if (action === 'rename') renameChapterAction(c.bookId, c.chapter, c.label)
    else if (action === 'delete') deleteChapterAction(c.bookId, c.chapter, c.label)
  }
}
</script>

<template>
  <aside class="sidebar" :style="{ width: sidebarWidth + 'px', minWidth: sidebarWidth + 'px' }">
    <div class="sb-tabs">
      <button :class="{ active: view === 'manuscript' }" @click="view = 'manuscript'">
        Manuscript
      </button>
      <button :class="{ active: view === 'codex' }" @click="view = 'codex'">Codex</button>
    </div>

    <div v-if="error" class="sb-error" @click="error = ''">{{ error }}</div>

    <div v-if="view === 'manuscript'" class="sb-tree">
      <button
        v-if="isSeries || books.length > 1"
        class="btn sb-add series-plan-btn"
        @click="openTab({ kind: 'series-plan' })"
      >
        📋 Series plan
      </button>
      <button class="btn sb-add series-plan-btn" @click="openTab({ kind: 'timeline' })">
        🕰 Story timeline
      </button>
      <div v-for="b in books" :key="b.id" class="sb-book">
        <div
          class="sb-book-head"
          @click="toggleBook(b.id)"
          @contextmenu.prevent.stop="openBookMenu($event, b.id, b.title)"
        >
          <span class="chev">{{ collapsedBooks.has(b.id) ? '▸' : '▾' }}</span>
          <span class="sb-book-title">{{ b.title }}</span>
          <button
            class="btn icon"
            title="Plan this book"
            @click.stop="openTab({ kind: 'plan', bookId: b.id })"
          >
            📋
          </button>
          <button
            class="btn icon"
            title="Corkboard (scenes)"
            @click.stop="openTab({ kind: 'corkboard', bookId: b.id })"
          >
            🗂
          </button>
          <button class="btn icon" title="New chapter" @click.stop="addChapter(b.id)">＋</button>
        </div>
        <template v-if="!collapsedBooks.has(b.id)">
          <div
            v-for="c in b.chapters ?? []"
            :key="c"
            class="sb-item"
            :class="{
              active: state.activeTab === chapterKey(b.id, c),
              dirty: state.dirtyChapters.has(`${b.id}/${c}`),
            }"
            @click="openTab({ kind: 'chapter', bookId: b.id, chapter: c })"
            @contextmenu.prevent.stop="openChapterMenu($event, b.id, c)"
          >
            <span class="sb-item-label">{{ prettyChapter(c) }}</span>
            <span class="dirty-dot" />
          </div>
        </template>
      </div>
      <button class="btn sb-add" @click="addBook">
        {{ isSeries ? '+ Add book' : '+ Add book (makes this a series)' }}
      </button>
    </div>

    <div v-else class="sb-tree">
      <template v-for="t in schemaTypes" :key="t.id">
        <div v-if="codexByType.get(t.id)?.length" class="sb-group">
          <div class="sb-group-head">{{ t.icon || '✦' }} {{ t.label || t.id }}</div>
          <div
            v-for="e in codexByType.get(t.id)"
            :key="e.id"
            class="sb-item"
            :class="{ active: state.activeTab === tabKey({ kind: 'codex', entryId: e.id }) }"
            @click="openTab({ kind: 'codex', entryId: e.id })"
          >
            <span class="sb-item-label">{{ e.name }}</span>
            <span v-if="e.scope !== 'series'" class="sb-scope" title="Book-local entry">📕</span>
          </div>
        </div>
      </template>
      <button class="btn sb-add" @click="newCodexEntry">+ New entry</button>
      <button class="btn sb-add subtle" @click="openTab({ kind: 'schema' })">
        ⚙ Edit types &amp; relations
      </button>
    </div>

    <div class="sb-footer">
      <div class="sb-project" :title="state.workspace?.path">
        {{ state.workspace?.manifest.name }}
      </div>
      <div class="sb-actions">
        <button class="btn icon" title="Search &amp; replace across the project" @click="openTab({ kind: 'search' })">🔍</button>
        <button class="btn icon" title="Revision history &amp; snapshots" @click="openTab({ kind: 'history' })">🕓</button>
        <button class="btn icon" title="Relationship graph" @click="openTab({ kind: 'graph' })">🕸</button>
        <button class="btn icon" title="Export book" @click="openTab({ kind: 'export' })">⬇</button>
        <button class="btn icon" title="Settings" @click="openTab({ kind: 'settings' })">⚙</button>
        <button class="btn icon" title="Switch project" @click="switchProject">⇄</button>
      </div>
    </div>

    <div
      v-if="ctx"
      class="ctx-menu"
      :style="{ left: ctx.x + 'px', top: ctx.y + 'px' }"
      @click.stop
    >
      <template v-if="ctx.kind === 'book'">
        <button class="ctx-item" @click="runCtx('chapter')">＋ New chapter</button>
        <button class="ctx-item" @click="runCtx('plan')">📋 Plan book</button>
        <button class="ctx-item" @click="runCtx('corkboard')">🗂 Corkboard</button>
        <button class="ctx-item" @click="runCtx('rename')">Rename…</button>
        <div class="ctx-sep" />
        <button class="ctx-item danger" @click="runCtx('delete')">Delete book…</button>
      </template>
      <template v-else>
        <button class="ctx-item" @click="runCtx('rename')">Rename…</button>
        <button class="ctx-item danger" @click="runCtx('delete')">Delete chapter…</button>
      </template>
    </div>

    <div class="sb-resize" title="Drag to resize" @mousedown="startResize" />
  </aside>
</template>

<style scoped>
.ctx-menu {
  position: fixed;
  z-index: 900;
  min-width: 160px;
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 4px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}
.ctx-item {
  background: none;
  border: none;
  color: var(--nv-text);
  text-align: left;
  padding: 6px 10px;
  font: inherit;
  font-size: 13px;
  border-radius: 5px;
  cursor: pointer;
}
.ctx-item:hover {
  background: var(--nv-hover);
}
.ctx-item.danger {
  color: var(--nv-error);
}
.ctx-sep {
  height: 1px;
  background: var(--nv-border);
  margin: 4px 2px;
}
.sidebar {
  width: 240px;
  min-width: 240px;
  background: var(--nv-panel);
  border-right: 1px solid var(--nv-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
  position: relative;
}
/* Drag handle sitting over the right border. */
.sb-resize {
  position: absolute;
  top: 0;
  right: -3px;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 5;
}
.sb-resize:hover {
  background: color-mix(in srgb, var(--nv-accent) 40%, transparent);
}
.sb-tabs {
  display: flex;
  border-bottom: 1px solid var(--nv-border);
}
.sb-tabs button {
  flex: 1;
  padding: 8px 0;
  background: none;
  border: none;
  color: var(--nv-muted);
  cursor: pointer;
  font-size: 12px;
  border-bottom: 2px solid transparent;
}
.sb-tabs button.active {
  color: var(--nv-text);
  border-bottom-color: var(--nv-accent);
}
.sb-error {
  padding: 6px 10px;
  font-size: 11px;
  color: var(--nv-error);
  cursor: pointer;
}
.sb-tree {
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}
.sb-book-head {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  cursor: pointer;
  font-weight: 600;
  font-size: 13px;
}
.sb-book-head:hover {
  background: var(--nv-hover);
}
.sb-book-head .btn.icon {
  visibility: hidden;
}
.sb-book-head .btn.icon:first-of-type {
  margin-left: auto;
}
.sb-book-head:hover .btn.icon {
  visibility: visible;
}
.chev {
  width: 12px;
  color: var(--nv-faint);
}
.sb-book-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sb-group-head {
  padding: 6px 10px 2px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--nv-faint);
}
.sb-item {
  display: flex;
  align-items: center;
  padding: 3px 10px 3px 24px;
  cursor: pointer;
  font-size: 13px;
  color: var(--nv-muted);
}
.sb-item:hover {
  background: var(--nv-hover);
}
.sb-item.active {
  background: color-mix(in srgb, var(--nv-accent) 18%, transparent);
  color: var(--nv-text);
}
.sb-item-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-transform: capitalize;
}
.dirty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  visibility: hidden;
}
.sb-item.dirty .dirty-dot {
  visibility: visible;
  background: var(--nv-accent);
}
.sb-scope {
  font-size: 10px;
}
.sb-add {
  margin: 10px 10px 0;
  width: calc(100% - 20px);
}
.sb-add.subtle {
  margin-top: 6px;
  border-style: dashed;
  color: var(--nv-muted);
  font-size: 12px;
}
.series-plan-btn {
  margin: 0 10px 8px;
  font-size: 12px;
}
.sb-footer {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  padding: 8px 10px;
  border-top: 1px solid var(--nv-border);
}
.sb-project {
  font-size: 12px;
  font-weight: 600;
  color: var(--nv-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sb-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
.sb-footer .btn.icon {
  font-size: 13px;
  padding: 3px 7px;
}
</style>
