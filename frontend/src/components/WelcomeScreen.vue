<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CreateWorkspace, GetSettings, OpenWorkspace, SelectFolder } from '../api'
import { setWorkspace, state } from '../store'
import type { WorkspaceKind } from '../types'

const mode = ref<'home' | 'create'>('home')
const error = ref('')
const busy = ref(false)

const newName = ref('')
const newKind = ref<WorkspaceKind>('novel')
const newPath = ref('')

onMounted(async () => {
  if (!state.settings) {
    try {
      state.settings = await GetSettings()
    } catch {
      /* bindings not ready in dev — recents just stay hidden */
    }
  }
})

const recents = computed(() => state.settings?.recent ?? [])

function shortPath(p: string) {
  const parts = p.split('/')
  return parts.length > 3 ? '…/' + parts.slice(-2).join('/') : p
}

async function openPath(dir: string) {
  error.value = ''
  busy.value = true
  try {
    setWorkspace(await OpenWorkspace(dir))
    state.settings = await GetSettings() // refresh recents order
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function openExisting() {
  const dir = await SelectFolder('Open a NovelIDE workspace')
  if (dir) await openPath(dir)
}

async function pickCreateFolder() {
  const dir = await SelectFolder('Choose an empty folder for the new workspace')
  if (dir) newPath.value = dir
}

async function create() {
  error.value = ''
  if (!newName.value.trim() || !newPath.value) {
    error.value = 'Pick a name and a folder.'
    return
  }
  busy.value = true
  try {
    setWorkspace(await CreateWorkspace(newPath.value, newName.value.trim(), newKind.value))
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="welcome">
    <div class="welcome-card">
      <h1>NovelIDE</h1>
      <p class="tagline">An IDE for novels — with a Codex that keeps your story straight.</p>

      <template v-if="mode === 'home'">
        <button class="btn primary big" :disabled="busy" @click="openExisting">
          Open Workspace…
        </button>
        <button class="btn big" :disabled="busy" @click="mode = 'create'">
          New Novel or Series…
        </button>
        <div v-if="recents.length" class="recents">
          <div class="recents-head">Recent</div>
          <button
            v-for="r in recents"
            :key="r"
            class="recent-item"
            :disabled="busy"
            :title="r"
            @click="openPath(r)"
          >
            {{ shortPath(r) }}
          </button>
        </div>
      </template>

      <template v-else>
        <label>
          Title
          <input v-model="newName" placeholder="The Ember Cycle" @keyup.enter="create" />
        </label>
        <label>
          Kind
          <select v-model="newKind">
            <option value="novel">Single novel</option>
            <option value="series">Series (multiple books, shared codex)</option>
          </select>
        </label>
        <label>
          Folder
          <div class="folder-row">
            <input v-model="newPath" placeholder="Choose a folder…" readonly />
            <button class="btn" @click="pickCreateFolder">Browse</button>
          </div>
        </label>
        <div class="actions">
          <button class="btn" @click="mode = 'home'">Back</button>
          <button class="btn primary" :disabled="busy" @click="create">Create</button>
        </div>
      </template>

      <p v-if="error" class="error">{{ error }}</p>
    </div>
  </div>
</template>

<style scoped>
.welcome {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}
.welcome-card {
  width: 420px;
  padding: 36px;
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
h1 {
  margin: 0;
  font-size: 28px;
  letter-spacing: 0.5px;
}
.tagline {
  margin: 0 0 10px;
  color: var(--nv-muted);
  font-size: 13px;
}
.big {
  padding: 12px;
  font-size: 15px;
}
label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12px;
  color: var(--nv-muted);
}
.folder-row {
  display: flex;
  gap: 8px;
}
.folder-row input {
  flex: 1;
}
.actions {
  display: flex;
  justify-content: space-between;
  margin-top: 6px;
}
.error {
  color: var(--nv-error);
  font-size: 12px;
  margin: 0;
}
.recents {
  margin-top: 6px;
  border-top: 1px solid var(--nv-border);
  padding-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.recents-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.07em;
  color: var(--nv-faint);
  margin-bottom: 2px;
}
.recent-item {
  background: none;
  border: none;
  color: var(--nv-accent);
  text-align: left;
  padding: 4px 2px;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.recent-item:hover {
  text-decoration: underline;
}
</style>
