<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import SidebarTrees from './SidebarTrees.vue'
import StatusBar from './StatusBar.vue'
import ChapterEditor from './ChapterEditor.vue'
import CodexEditor from './CodexEditor.vue'
import SchemaEditor from './SchemaEditor.vue'
import SettingsEditor from './SettingsEditor.vue'
import PlanView from './PlanView.vue'
import SeriesPlanView from './SeriesPlanView.vue'
import ExportView from './ExportView.vue'
import GraphView from './GraphView.vue'
import CorkboardView from './CorkboardView.vue'
import TimelineView from './TimelineView.vue'
import SearchView from './SearchView.vue'
import HistoryView from './HistoryView.vue'
import AIPanel from './AIPanel.vue'
import { DismissSuggestion, SaveCodexEntry } from '../api'
import {
  activateTab,
  activeTabObj,
  closeTab,
  codexById,
  openTab,
  pinTab,
  schemaTypes,
  setWorkspace,
  state,
  tabKey,
} from '../store'
import type { Tab } from '../store'
import type { CodexEntry, Suggestion } from '../types'

const editorRef = ref<InstanceType<typeof ChapterEditor> | null>(null)
const problemsOpen = ref(true)

const activeChapterTab = computed(() =>
  activeTabObj.value?.kind === 'chapter' ? activeTabObj.value : null,
)
const activeCodexTab = computed(() =>
  activeTabObj.value?.kind === 'codex' ? activeTabObj.value : null,
)

function tabLabel(t: Tab): string {
  if (t.kind === 'chapter') {
    return t.chapter.replace(/\.md$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')
  }
  if (t.kind === 'schema') return 'Codex Schema'
  if (t.kind === 'settings') return 'Settings'
  if (t.kind === 'plan') {
    const b = (state.workspace?.books ?? []).find((b) => b.id === t.bookId)
    return `Plan: ${b?.title ?? t.bookId}`
  }
  if (t.kind === 'series-plan') return 'Series Plan'
  if (t.kind === 'export') return 'Export'
  if (t.kind === 'graph') return 'Graph'
  if (t.kind === 'corkboard') {
    const b = (state.workspace?.books ?? []).find((b) => b.id === t.bookId)
    return `Corkboard: ${b?.title ?? t.bookId}`
  }
  if (t.kind === 'timeline') return 'Timeline'
  if (t.kind === 'search') return 'Search'
  if (t.kind === 'history') return 'History'
  if (!t.entryId) return 'New entry'
  return codexById.value.get(t.entryId)?.name ?? t.entryId
}

function tabIcon(t: Tab): string {
  if (t.kind === 'chapter') return '📄'
  if (t.kind === 'schema' || t.kind === 'settings') return '⚙'
  if (t.kind === 'plan' || t.kind === 'series-plan') return '📋'
  if (t.kind === 'export') return '⬇'
  if (t.kind === 'graph') return '🕸'
  if (t.kind === 'corkboard') return '🗂'
  if (t.kind === 'timeline') return '🕰'
  if (t.kind === 'search') return '🔍'
  if (t.kind === 'history') return '🕓'
  return '📔'
}

const problems = computed(() =>
  activeChapterTab.value ? state.flags.filter((f) => f.severity !== 'info') : [],
)
const infoCount = computed(() => state.flags.filter((f) => f.severity === 'info').length)

const suggestions = computed(() =>
  activeChapterTab.value
    ? state.suggestions.filter((s) => !state.dismissedSuggestions.has(s.key))
    : [],
)

const savingSuggestion = ref('')
const deepScanning = ref(false)
const deepError = ref('')

async function runDeepScan() {
  if (!editorRef.value || deepScanning.value) return
  deepScanning.value = true
  deepError.value = ''
  try {
    await editorRef.value.deepScan()
  } catch (e) {
    deepError.value = String(e)
  } finally {
    deepScanning.value = false
  }
}

async function acceptSuggestion(s: Suggestion) {
  const tab = activeChapterTab.value
  if (!tab) return
  if (s.kind === 'entity' && s.name) {
    // Open a prefilled draft entry — saving it is up to the author.
    openTab({
      kind: 'codex',
      entryId: '',
      draft: {
        id: '',
        name: s.name,
        type: schemaTypes.value[0]?.id ?? 'character',
        aliases: [],
        summary: '',
        details: '',
        image: '',
        fields: {},
        status: [],
        relations: [],
        scope: 'series',
      },
    })
    state.dismissedSuggestions.add(s.key)
    return
  }
  const entry = s.entryId ? codexById.value.get(s.entryId) : undefined
  if (!entry) return
  const updated: CodexEntry = JSON.parse(JSON.stringify(entry))
  const at = { book: tab.bookId, chapter: tab.chapter }
  if (s.kind === 'status' && s.state) {
    updated.status = [
      ...(updated.status ?? []),
      { state: s.state, at, note: 'Detected in the manuscript' },
    ]
  } else if (s.kind === 'relation' && s.relation && s.targetId) {
    updated.relations = [
      ...(updated.relations ?? []),
      { type: s.relation, to: s.targetId, from: at, note: 'Detected in the manuscript' },
    ]
  } else if (s.kind === 'field' && s.fieldKey && s.fieldValue) {
    updated.fields = { ...(updated.fields ?? {}), [s.fieldKey]: s.fieldValue }
  } else if (s.kind === 'alias' && s.name) {
    updated.aliases = [...(updated.aliases ?? []), s.name]
  } else {
    return
  }
  savingSuggestion.value = s.key
  try {
    const ws = await SaveCodexEntry(updated, entry.type, entry.scope)
    setWorkspace(ws)
    editorRef.value?.rescan()
  } catch (e) {
    console.error('failed to save suggestion', e)
  } finally {
    savingSuggestion.value = ''
  }
}

function dismissSuggestion(s: Suggestion) {
  state.dismissedSuggestions.add(s.key) // hide immediately
  // Persist so it stays dismissed across sessions (and syncs to other devices).
  void DismissSuggestion(s.key).catch((e) => console.error('persist dismissal failed', e))
}

function jumpToFlag(pos: number) {
  editorRef.value?.jumpTo(pos)
}

function onKey(e: KeyboardEvent) {
  if (e.ctrlKey && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
    e.preventDefault()
    state.focusMode = !state.focusMode
  } else if (e.key === 'Escape' && state.focusMode) {
    state.focusMode = false
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="shell">
    <SidebarTrees v-if="!state.focusMode" />
    <main class="main">
      <div class="tabbar" v-if="state.tabs.length">
        <div
          v-for="t in state.tabs"
          :key="tabKey(t)"
          class="tab"
          :class="{
            active: state.activeTab === tabKey(t),
            codex: t.kind === 'codex',
            preview: !t.pinned,
          }"
          :title="!t.pinned ? 'Preview — double-click to keep open' : ''"
          @click="activateTab(tabKey(t))"
          @dblclick="pinTab(tabKey(t))"
          @mousedown.middle.prevent="closeTab(tabKey(t))"
        >
          <span class="tab-kind">{{ tabIcon(t) }}</span>
          <span class="tab-label">{{ tabLabel(t) }}</span>
          <span
            v-if="t.kind === 'chapter' && state.dirtyChapters.has(`${t.bookId}/${t.chapter}`)"
            class="tab-dirty"
            >●</span
          >
          <button class="tab-close" @click.stop="closeTab(tabKey(t))">✕</button>
        </div>
      </div>

      <div class="content">
        <ChapterEditor
          v-if="activeChapterTab"
          ref="editorRef"
          :key="tabKey(activeChapterTab)"
          :book-id="activeChapterTab.bookId"
          :chapter="activeChapterTab.chapter"
        />
        <CodexEditor
          v-else-if="activeCodexTab"
          :key="tabKey(activeCodexTab)"
          :entry-id="activeCodexTab.entryId"
          :draft="activeCodexTab.draft"
        />
        <SchemaEditor v-else-if="activeTabObj?.kind === 'schema'" />
        <SettingsEditor v-else-if="activeTabObj?.kind === 'settings'" />
        <PlanView
          v-else-if="activeTabObj?.kind === 'plan'"
          :key="tabKey(activeTabObj)"
          :book-id="activeTabObj.bookId"
        />
        <SeriesPlanView v-else-if="activeTabObj?.kind === 'series-plan'" />
        <ExportView v-else-if="activeTabObj?.kind === 'export'" />
        <GraphView v-else-if="activeTabObj?.kind === 'graph'" />
        <CorkboardView
          v-else-if="activeTabObj?.kind === 'corkboard'"
          :key="tabKey(activeTabObj)"
          :book-id="activeTabObj.bookId"
        />
        <TimelineView v-else-if="activeTabObj?.kind === 'timeline'" />
        <SearchView v-else-if="activeTabObj?.kind === 'search'" />
        <HistoryView v-else-if="activeTabObj?.kind === 'history'" />
        <div v-else class="empty">
          <p>Open a chapter from the Manuscript tree, or a Codex entry.</p>
          <p class="empty-hint">
            Mentions of Codex entities get highlighted as you write — hover them for the card.
          </p>
        </div>
      </div>

      <div
        v-if="activeChapterTab && !state.focusMode"
        class="problems"
        :class="{ open: problemsOpen }"
      >
        <div class="problems-head" @click="problemsOpen = !problemsOpen">
          <span class="chev">{{ problemsOpen ? '▾' : '▸' }}</span>
          Consistency
          <span class="badge" :class="{ none: !problems.length }">{{ problems.length }}</span>
          <span v-if="suggestions.length" class="badge suggest">💡 {{ suggestions.length }}</span>
          <button
            v-if="state.settings?.deepEnabled"
            class="btn mini deep"
            :disabled="deepScanning"
            :title="'Run the transformer model over this chapter (first run downloads the model)'"
            @click.stop="runDeepScan"
          >
            {{ deepScanning ? 'Deep scanning…' : '🔬 Deep scan' }}
          </button>
          <span v-if="deepError" class="deep-error" :title="deepError" @click.stop="deepError = ''">
            {{ deepError }}
          </span>
          <span v-if="infoCount" class="info-note">{{ infoCount }} dead-entity mention(s), info only</span>
        </div>
        <div v-if="problemsOpen" class="problems-list">
          <div
            v-for="s in suggestions"
            :key="s.key"
            class="problem suggestion"
          >
            <span class="sev" @click="jumpToFlag(s.start)">💡</span>
            <span class="suggestion-msg" @click="jumpToFlag(s.start)">{{ s.message }}</span>
            <span class="suggestion-actions">
              <button
                class="btn mini primary"
                :disabled="savingSuggestion === s.key"
                @click.stop="acceptSuggestion(s)"
              >
                {{
                  savingSuggestion === s.key
                    ? 'Saving…'
                    : s.kind === 'entity'
                      ? 'Create entry…'
                      : 'Add to Codex'
                }}
              </button>
              <button class="btn mini" @click.stop="dismissSuggestion(s)">Dismiss</button>
            </span>
          </div>
          <div v-if="!problems.length && !suggestions.length" class="problem none">
            No consistency problems detected in this chapter.
          </div>
          <div
            v-for="(f, i) in problems"
            :key="i"
            class="problem"
            :class="f.severity"
            @click="jumpToFlag(f.start)"
          >
            <span class="sev">{{ f.severity === 'error' ? '⛔' : '⚠️' }}</span>
            {{ f.message }}
          </div>
        </div>
      </div>

      <StatusBar />
    </main>
    <AIPanel v-show="state.aiPanelOpen" />
  </div>
</template>

<style scoped>
.shell {
  height: 100%;
  display: flex;
  min-height: 0;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}
.tabbar {
  display: flex;
  background: var(--nv-panel);
  border-bottom: 1px solid var(--nv-border);
  overflow-x: auto;
  flex-shrink: 0;
  /* Scroll with the wheel/trackpad; the native bar covered the tabs' close buttons. */
  scrollbar-width: none;
}
.tabbar::-webkit-scrollbar {
  display: none;
}
.tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  font-size: 12px;
  color: var(--nv-muted);
  border-right: 1px solid var(--nv-border);
  cursor: pointer;
  white-space: nowrap;
  text-transform: capitalize;
}
.tab.active {
  background: var(--nv-bg);
  color: var(--nv-text);
  box-shadow: inset 0 2px 0 var(--nv-accent);
}
/* Preview (transient) tabs read in italic, like an unpinned document. */
.tab.preview .tab-label {
  font-style: italic;
}
.tab-kind {
  font-size: 11px;
}
.tab-dirty {
  color: var(--nv-accent);
  font-size: 10px;
}
.tab-close {
  background: none;
  border: none;
  color: var(--nv-faint);
  cursor: pointer;
  font-size: 10px;
  padding: 2px;
}
.tab-close:hover {
  color: var(--nv-text);
}
.content {
  flex: 1;
  min-height: 0;
}
.empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--nv-muted);
}
.empty-hint {
  font-size: 12px;
  color: var(--nv-faint);
}
.problems {
  border-top: 1px solid var(--nv-border);
  background: var(--nv-panel);
  flex-shrink: 0;
}
.problems-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  font-size: 12px;
  cursor: pointer;
  color: var(--nv-muted);
}
.badge {
  background: var(--nv-error);
  color: #fff;
  border-radius: 8px;
  padding: 0 7px;
  font-size: 11px;
}
.badge.none {
  background: var(--nv-border);
  color: var(--nv-muted);
}
.badge.suggest {
  background: color-mix(in srgb, var(--nv-accent) 25%, transparent);
  color: var(--nv-accent);
}
.info-note {
  color: var(--nv-faint);
  font-size: 11px;
  margin-left: auto;
}
.problems-list {
  max-height: 140px;
  overflow-y: auto;
  padding-bottom: 6px;
}
.problem {
  padding: 4px 14px;
  font-size: 12px;
  cursor: pointer;
  display: flex;
  gap: 8px;
}
.problem:hover {
  background: var(--nv-hover);
}
.problem.none {
  color: var(--nv-faint);
  cursor: default;
}
.problem.error {
  color: var(--nv-error);
}
.problem.warning {
  color: var(--nv-warning);
}
.problem.suggestion {
  align-items: center;
  color: var(--nv-text);
}
.suggestion-msg {
  flex: 1;
  cursor: pointer;
}
.suggestion-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.btn.mini {
  padding: 2px 9px;
  font-size: 11px;
}
.btn.mini.deep {
  margin-left: 4px;
}
.deep-error {
  color: var(--nv-error);
  font-size: 11px;
  max-width: 340px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}
</style>
