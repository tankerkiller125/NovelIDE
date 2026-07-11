<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { OpenWorkspace, ReplaceAllProject, SearchProject } from '../api'
import { confirmDialog, openChapterAtLine, setWorkspace, state } from '../store'
import type { SearchResults } from '../types'

const query = ref('')
const replacement = ref('')
const caseSensitive = ref(false)
const wholeWord = ref(false)

const results = ref<SearchResults | null>(null)
const searching = ref(false)
const message = ref('')
const showReplace = ref(false)

async function runSearch() {
  const q = query.value
  if (!q) {
    results.value = null
    return
  }
  searching.value = true
  message.value = ''
  try {
    results.value = await SearchProject(q, caseSensitive.value, wholeWord.value)
  } catch (e) {
    message.value = `Search failed: ${e}`
    results.value = null
  } finally {
    searching.value = false
  }
}

const total = computed(() => results.value?.total ?? 0)
const chapterCount = computed(() => results.value?.hits.length ?? 0)

async function replaceAll() {
  const q = query.value
  if (!q) return
  // Replacing rewrites files on disk; a chapter with unsaved edits open in the
  // editor would race its autosave against the rewrite. Make the author save
  // first so nothing is lost.
  if (state.dirtyChapters.size > 0) {
    message.value = 'Save your open chapters first — there are unsaved edits.'
    return
  }
  const ok = await confirmDialog({
    title: 'Replace across the whole project?',
    message: `Replace all ${total.value} occurrence(s) of "${q}" with "${replacement.value}" in ${chapterCount.value} chapter(s)? This rewrites the files and can't be undone in one step.`,
    confirmText: 'Replace all',
    danger: true,
  })
  if (!ok) return
  searching.value = true
  message.value = ''
  try {
    const n = await ReplaceAllProject(q, replacement.value, caseSensitive.value, wholeWord.value)
    // Reload the workspace, then nudge the open chapter editor to re-read disk.
    if (state.workspace) setWorkspace(await OpenWorkspace(state.workspace.path))
    state.reloadTick++
    message.value = `Replaced ${n} occurrence(s).`
    await nextTick()
    await runSearch()
  } catch (e) {
    message.value = `Replace failed: ${e}`
  } finally {
    searching.value = false
  }
}

const multiBook = computed(() => (state.workspace?.books?.length ?? 0) > 1)
</script>

<template>
  <div class="sv">
    <div class="sv-head">
      <h2>Search</h2>
      <span class="sv-msg">{{ searching ? 'Working…' : message }}</span>
    </div>

    <div class="sv-controls">
      <div class="sv-inputs">
        <input
          v-model="query"
          class="sv-query"
          placeholder="Find in all chapters…"
          @keydown.enter="runSearch"
        />
        <input
          v-if="showReplace"
          v-model="replacement"
          class="sv-query"
          placeholder="Replace with…"
          @keydown.enter="replaceAll"
        />
      </div>
      <div class="sv-buttons">
        <button class="btn primary" :disabled="!query || searching" @click="runSearch">Search</button>
        <button class="btn" @click="showReplace = !showReplace">
          {{ showReplace ? 'Hide replace' : 'Replace…' }}
        </button>
        <button
          v-if="showReplace"
          class="btn danger"
          :disabled="!query || !total || searching"
          @click="replaceAll"
        >
          Replace all{{ total ? ` (${total})` : '' }}
        </button>
      </div>
    </div>

    <div class="sv-opts">
      <label><input type="checkbox" v-model="caseSensitive" @change="runSearch" /> Match case</label>
      <label><input type="checkbox" v-model="wholeWord" @change="runSearch" /> Whole word</label>
    </div>

    <div v-if="results" class="sv-summary">
      <template v-if="total">{{ total }} match(es) in {{ chapterCount }} chapter(s)</template>
      <template v-else>No matches.</template>
    </div>

    <div v-if="results && total" class="sv-results">
      <div v-for="hit in results.hits" :key="hit.bookId + '/' + hit.chapter" class="sv-hit">
        <div class="sv-hit-head">
          <span v-if="multiBook" class="sv-book">{{ hit.bookTitle }} · </span>
          <span class="sv-chapter">{{ hit.chapterTitle }}</span>
          <span class="sv-hit-count">{{ hit.matches.length }}</span>
        </div>
        <div
          v-for="(m, i) in hit.matches"
          :key="i"
          class="sv-match"
          @click="openChapterAtLine(hit.bookId, hit.chapter, m.line)"
        >
          <span class="sv-line">{{ m.line }}</span>
          <span class="sv-text">
            <span class="sv-ctx">{{ m.before }}</span
            ><mark>{{ m.match }}</mark
            ><span class="sv-ctx">{{ m.after }}</span>
          </span>
        </div>
      </div>
    </div>

    <p class="sv-hint">
      Searches manuscript prose across every book. Replace rewrites the plain-text chapter files —
      each stays individually editable, so you can review changes in version control afterwards.
    </p>
  </div>
</template>

<style scoped>
.sv {
  height: 100%;
  overflow-y: auto;
  padding: 18px 24px 60px;
  max-width: 900px;
}
.sv-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 12px;
}
.sv-head h2 {
  margin: 0;
  font-size: 18px;
}
.sv-msg {
  color: var(--nv-muted);
  font-size: 12px;
}
.sv-controls {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}
.sv-inputs {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.sv-query {
  width: 100%;
  padding: 7px 10px;
  font-size: 13px;
}
.sv-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.sv-opts {
  display: flex;
  gap: 16px;
  margin: 8px 0 4px;
  font-size: 12px;
  color: var(--nv-muted);
}
.sv-opts label {
  display: flex;
  gap: 5px;
  align-items: center;
  cursor: pointer;
}
.sv-summary {
  font-size: 12px;
  color: var(--nv-faint);
  margin: 6px 0;
}
.sv-results {
  margin-top: 6px;
}
.sv-hit {
  margin-bottom: 14px;
}
.sv-hit-head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--nv-border);
  margin-bottom: 4px;
}
.sv-book {
  color: var(--nv-muted);
  font-size: 12px;
}
.sv-chapter {
  font-weight: 600;
  text-transform: capitalize;
  flex: 1;
}
.sv-hit-count {
  font-size: 11px;
  color: var(--nv-faint);
  background: var(--nv-panel);
  border-radius: 8px;
  padding: 0 7px;
}
.sv-match {
  display: flex;
  gap: 10px;
  padding: 3px 6px;
  border-radius: 5px;
  cursor: pointer;
  font-size: 12.5px;
  align-items: baseline;
}
.sv-match:hover {
  background: var(--nv-hover);
}
.sv-line {
  color: var(--nv-faint);
  font-variant-numeric: tabular-nums;
  min-width: 30px;
  text-align: right;
  flex-shrink: 0;
}
.sv-text {
  font-family: var(--nv-mono, monospace);
  white-space: pre-wrap;
  word-break: break-word;
}
.sv-ctx {
  color: var(--nv-muted);
}
.sv-text mark {
  background: color-mix(in srgb, var(--nv-accent) 35%, transparent);
  color: var(--nv-text);
  border-radius: 2px;
}
.sv-hint {
  margin-top: 22px;
  font-size: 12px;
  color: var(--nv-faint);
}
</style>
