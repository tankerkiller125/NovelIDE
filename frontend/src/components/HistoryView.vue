<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CreateSnapshot,
  DeleteSnapshot,
  ListSnapshots,
  RestoreSnapshot,
  RestoreSnapshotFile,
  SnapshotChanges,
  SnapshotFileDiff,
} from '../api'
import { confirmDialog, promptInput, setWorkspace, state } from '../store'
import type { DiffResult, FileChange, Snapshot } from '../types'

const snapshots = ref<Snapshot[]>([])
const selected = ref<Snapshot | null>(null)
const changes = ref<FileChange[]>([])
const activeFile = ref<string>('')
const diff = ref<DiffResult | null>(null)
const message = ref('')
const busy = ref(false)

async function load() {
  try {
    snapshots.value = await ListSnapshots()
  } catch (e) {
    message.value = `Couldn't load history: ${e}`
  }
}
onMounted(load)

async function takeSnapshot() {
  const label = await promptInput({
    title: 'Take a snapshot',
    label: 'Label (optional)',
    placeholder: 'e.g. before rewriting the ending',
  })
  if (label === null) return
  busy.value = true
  message.value = ''
  try {
    const before = snapshots.value.length
    snapshots.value = await CreateSnapshot(label)
    message.value =
      snapshots.value.length > before ? 'Snapshot saved.' : 'No changes since the last snapshot.'
  } catch (e) {
    message.value = `Snapshot failed: ${e}`
  } finally {
    busy.value = false
  }
}

async function select(snap: Snapshot) {
  selected.value = snap
  activeFile.value = ''
  diff.value = null
  changes.value = []
  try {
    changes.value = await SnapshotChanges(snap.id)
  } catch (e) {
    message.value = `Couldn't compare: ${e}`
  }
}

async function viewDiff(rel: string) {
  if (!selected.value) return
  activeFile.value = rel
  diff.value = null
  try {
    diff.value = await SnapshotFileDiff(selected.value.id, rel)
  } catch (e) {
    message.value = `Diff failed: ${e}`
  }
}

async function restoreFile(rel: string) {
  if (!selected.value) return
  if (state.dirtyChapters.size > 0) {
    message.value = 'Save your open chapters first — there are unsaved edits.'
    return
  }
  const ok = await confirmDialog({
    title: 'Restore this file?',
    message: `Overwrite the current "${pretty(rel)}" with the version from "${selected.value.label}"?`,
    confirmText: 'Restore file',
    danger: true,
  })
  if (!ok) return
  busy.value = true
  try {
    setWorkspace(await RestoreSnapshotFile(selected.value.id, rel))
    state.reloadTick++
    message.value = `Restored ${pretty(rel)}.`
    await select(selected.value)
    if (activeFile.value === rel) await viewDiff(rel)
  } catch (e) {
    message.value = `Restore failed: ${e}`
  } finally {
    busy.value = false
  }
}

async function restoreAll() {
  if (!selected.value) return
  if (state.dirtyChapters.size > 0) {
    message.value = 'Save your open chapters first — there are unsaved edits.'
    return
  }
  const ok = await confirmDialog({
    title: 'Restore the whole project?',
    message: `Roll every file back to "${selected.value.label}"? A safety snapshot of the current state is taken first, so this can be undone.`,
    confirmText: 'Restore everything',
    danger: true,
  })
  if (!ok) return
  busy.value = true
  try {
    setWorkspace(await RestoreSnapshot(selected.value.id))
    state.reloadTick++
    snapshots.value = await ListSnapshots()
    message.value = 'Project restored. A “Before restore” snapshot was saved.'
    await select(selected.value)
  } catch (e) {
    message.value = `Restore failed: ${e}`
  } finally {
    busy.value = false
  }
}

async function remove(snap: Snapshot) {
  const ok = await confirmDialog({
    title: 'Delete snapshot?',
    message: `Delete "${snap.label}"? Its captured content is discarded (other snapshots are unaffected).`,
    confirmText: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    snapshots.value = await DeleteSnapshot(snap.id)
    if (selected.value?.id === snap.id) {
      selected.value = null
      changes.value = []
      diff.value = null
    }
  } catch (e) {
    message.value = `Delete failed: ${e}`
  }
}

function pretty(rel: string) {
  // Show a readable label for a workspace-relative path.
  const file = rel.split('/').pop() ?? rel
  if (rel.includes('/manuscript/')) return file.replace(/\.md$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')
  return rel
}
function when(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? iso : d.toLocaleString()
}
function kb(bytes: number) {
  return bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KB`
}

const changeCounts = computed(() => {
  const c = { modified: 0, added: 0, removed: 0 }
  for (const ch of changes.value) c[ch.status]++
  return c
})
const statusIcon = (s: string) => (s === 'added' ? '＋' : s === 'removed' ? '－' : '±')
</script>

<template>
  <div class="hist">
    <aside class="hist-list">
      <div class="hist-list-head">
        <h2>History</h2>
        <button class="btn primary" :disabled="busy" @click="takeSnapshot">＋ Snapshot</button>
      </div>
      <p v-if="message" class="hist-msg">{{ message }}</p>
      <p v-if="!snapshots.length" class="hist-empty">
        No snapshots yet. Take one to capture the project as it is now — an automatic daily snapshot
        is also saved as you write.
      </p>
      <ul class="hist-snaps">
        <li
          v-for="snap in snapshots"
          :key="snap.id"
          class="hist-snap"
          :class="{ active: selected?.id === snap.id }"
          @click="select(snap)"
        >
          <div class="hist-snap-main">
            <span class="hist-snap-label">{{ snap.label }}</span>
            <button class="btn icon del" title="Delete snapshot" @click.stop="remove(snap)">✕</button>
          </div>
          <div class="hist-snap-meta">
            <span v-if="snap.auto" class="hist-badge">auto</span>
            {{ when(snap.time) }} · {{ snap.files }} files · {{ kb(snap.size) }}
          </div>
        </li>
      </ul>
    </aside>

    <section class="hist-detail">
      <template v-if="selected">
        <div class="hist-detail-head">
          <div>
            <h3>{{ selected.label }}</h3>
            <span class="hist-sub">{{ when(selected.time) }}</span>
          </div>
          <button class="btn danger" :disabled="busy" @click="restoreAll">Restore all…</button>
        </div>

        <p v-if="!changes.length" class="hist-empty">
          No differences — the workspace matches this snapshot.
        </p>
        <div v-else class="hist-changes-summary">
          {{ changeCounts.modified }} modified · {{ changeCounts.added }} added ·
          {{ changeCounts.removed }} removed <span class="hist-sub">(vs. current)</span>
        </div>

        <div class="hist-body">
          <ul class="hist-files">
            <li
              v-for="c in changes"
              :key="c.rel"
              class="hist-file"
              :class="[c.status, { active: activeFile === c.rel }]"
              @click="viewDiff(c.rel)"
            >
              <span class="hist-file-icon">{{ statusIcon(c.status) }}</span>
              <span class="hist-file-name" :title="c.rel">{{ pretty(c.rel) }}</span>
              <button
                class="btn icon restore"
                title="Restore this file to the snapshot version"
                @click.stop="restoreFile(c.rel)"
              >
                ⤺
              </button>
            </li>
          </ul>

          <div class="hist-diff">
            <p v-if="!activeFile" class="hist-empty">Select a file to see what changed.</p>
            <template v-else-if="diff">
              <div class="hist-diff-head">
                {{ pretty(activeFile) }}
                <span class="hist-sub">— snapshot vs. current</span>
              </div>
              <pre class="hist-diff-body"><code><span
                v-for="(l, i) in diff.lines"
                :key="i"
                class="dl"
                :class="l.op"
              >{{ l.op === 'add' ? '+' : l.op === 'del' ? '−' : ' ' }} {{ l.text }}
</span></code></pre>
            </template>
            <p v-else class="hist-empty">Loading diff…</p>
          </div>
        </div>
      </template>
      <div v-else class="hist-placeholder">
        <p>Select a snapshot to see what changed since, diff individual files, and restore them.</p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.hist {
  height: 100%;
  display: flex;
  min-height: 0;
}
.hist-list {
  width: 300px;
  flex-shrink: 0;
  border-right: 1px solid var(--nv-border);
  overflow-y: auto;
  padding: 16px 14px;
}
.hist-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.hist-list-head h2 {
  margin: 0;
  font-size: 17px;
}
.hist-msg {
  font-size: 12px;
  color: var(--nv-muted);
  margin: 4px 0;
}
.hist-empty {
  font-size: 12px;
  color: var(--nv-faint);
  line-height: 1.5;
}
.hist-snaps {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
}
.hist-snap {
  padding: 8px 10px;
  border-radius: 8px;
  cursor: pointer;
  border: 1px solid transparent;
  margin-bottom: 4px;
}
.hist-snap:hover {
  background: var(--nv-hover);
}
.hist-snap.active {
  border-color: var(--nv-accent);
  background: var(--nv-hover);
}
.hist-snap-main {
  display: flex;
  align-items: center;
  gap: 6px;
}
.hist-snap-label {
  flex: 1;
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hist-snap-meta {
  font-size: 11px;
  color: var(--nv-faint);
  margin-top: 2px;
}
.hist-badge {
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border: 1px solid var(--nv-border);
  border-radius: 6px;
  padding: 0 5px;
  color: var(--nv-muted);
  margin-right: 4px;
}
.btn.icon.del {
  opacity: 0;
  color: var(--nv-faint);
}
.hist-snap:hover .btn.icon.del {
  opacity: 1;
}
.hist-detail {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  min-width: 0;
}
.hist-detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.hist-detail-head h3 {
  margin: 0;
  font-size: 16px;
}
.hist-sub {
  font-size: 12px;
  color: var(--nv-faint);
}
.hist-changes-summary {
  font-size: 12px;
  color: var(--nv-muted);
  margin: 8px 0;
}
.hist-body {
  display: flex;
  gap: 14px;
  margin-top: 8px;
  min-height: 0;
}
.hist-files {
  list-style: none;
  margin: 0;
  padding: 0;
  width: 230px;
  flex-shrink: 0;
}
.hist-file {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12.5px;
}
.hist-file:hover {
  background: var(--nv-hover);
}
.hist-file.active {
  background: var(--nv-hover);
  box-shadow: inset 2px 0 0 var(--nv-accent);
}
.hist-file-icon {
  width: 14px;
  text-align: center;
  font-weight: 700;
}
.hist-file.modified .hist-file-icon {
  color: var(--nv-warning);
}
.hist-file.added .hist-file-icon {
  color: #6fcf97;
}
.hist-file.removed .hist-file-icon {
  color: var(--nv-error);
}
.hist-file-name {
  flex: 1;
  text-transform: capitalize;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.btn.icon.restore {
  opacity: 0;
  color: var(--nv-muted);
}
.hist-file:hover .btn.icon.restore {
  opacity: 1;
}
.hist-diff {
  flex: 1;
  min-width: 0;
}
.hist-diff-head {
  font-size: 12px;
  font-weight: 600;
  text-transform: capitalize;
  margin-bottom: 6px;
}
.hist-diff-body {
  margin: 0;
  overflow-x: auto;
  background: var(--nv-bg);
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px 4px;
  font-size: 12px;
  line-height: 1.5;
  max-height: calc(100vh - 240px);
}
.dl {
  display: block;
  padding: 0 8px;
  white-space: pre-wrap;
  word-break: break-word;
}
.dl.add {
  background: color-mix(in srgb, #6fcf97 16%, transparent);
  color: #a7e3bf;
}
.dl.del {
  background: color-mix(in srgb, var(--nv-error) 16%, transparent);
  color: #e59a9a;
}
.dl.eq {
  color: var(--nv-muted);
}
.hist-placeholder {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nv-faint);
  font-size: 13px;
  text-align: center;
  padding: 0 30px;
}
</style>
