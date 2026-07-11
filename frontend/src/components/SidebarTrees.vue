<script setup lang="ts">
import { computed, ref } from 'vue'
import { CloseWorkspace, CreateBook, CreateChapter } from '../api'
import { closeProject, openTab, schemaTypes, setWorkspace, state, tabKey } from '../store'
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
  const title = prompt('Chapter title:')
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
  const title = prompt('Book title:')
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
  if (state.dirtyChapters.size > 0 && !confirm('A chapter is still saving — switch anyway?')) {
    return
  }
  await CloseWorkspace()
  closeProject()
}
</script>

<template>
  <aside class="sidebar">
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
      <div v-for="b in books" :key="b.id" class="sb-book">
        <div class="sb-book-head" @click="toggleBook(b.id)">
          <span class="chev">{{ collapsedBooks.has(b.id) ? '▸' : '▾' }}</span>
          <span class="sb-book-title">{{ b.title }}</span>
          <button
            class="btn icon"
            title="Plan this book"
            @click.stop="openTab({ kind: 'plan', bookId: b.id })"
          >
            📋
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
      <button class="btn icon" title="Settings" @click="openTab({ kind: 'settings' })">⚙</button>
      <button class="btn icon" title="Switch project" @click="switchProject">⇄</button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 240px;
  min-width: 240px;
  background: var(--nv-panel);
  border-right: 1px solid var(--nv-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
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
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border-top: 1px solid var(--nv-border);
}
.sb-project {
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  color: var(--nv-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sb-footer .btn.icon {
  font-size: 13px;
  padding: 3px 7px;
}
</style>
