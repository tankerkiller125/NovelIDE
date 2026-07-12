<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  CreateWorkspace,
  GetSettings,
  OpenWorkspace,
  RemoteWorkspaces,
  SelectFolder,
  SyncCloneWorkspace,
  SyncLogout,
} from '../api'
import { refreshSyncStatus, setWorkspace, state } from '../store'
import SyncSignIn from './SyncSignIn.vue'
import type { RemoteWorkspace, WorkspaceKind } from '../types'

const mode = ref<'home' | 'create' | 'sync'>('home')
const error = ref('')
const busy = ref(false)

const newName = ref('')
const newKind = ref<WorkspaceKind>('novel')
const newPath = ref('')

const remotes = ref<RemoteWorkspace[]>([])
const loggedIn = computed(() => state.sync?.loggedIn ?? false)

onMounted(async () => {
  if (!state.settings) {
    try {
      state.settings = await GetSettings()
    } catch {
      /* bindings not ready in dev — recents just stay hidden */
    }
  }
  await refreshSyncStatus()
})

// Once signed in (here or elsewhere), load the account's remote workspaces.
watch(loggedIn, (li) => {
  if (li) void loadRemotes()
})

async function openSync() {
  mode.value = 'sync'
  error.value = ''
  await refreshSyncStatus()
  if (loggedIn.value) await loadRemotes()
}

async function loadRemotes() {
  try {
    remotes.value = await RemoteWorkspaces()
  } catch (e) {
    error.value = String(e)
  }
}

async function openRemote(r: RemoteWorkspace) {
  const parent = await SelectFolder(`Choose where to save “${r.name}”`)
  if (!parent) return
  busy.value = true
  error.value = ''
  try {
    setWorkspace(await SyncCloneWorkspace(r.id, parent))
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function signOut() {
  busy.value = true
  try {
    await SyncLogout()
    state.settings = await GetSettings()
    await refreshSyncStatus()
    remotes.value = []
  } finally {
    busy.value = false
  }
}

function whenText(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? iso : d.toLocaleDateString()
}

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
      <div class="welcome-head">
        <h1>NovelIDE</h1>
        <button
          class="btn icon cog"
          :class="{ on: loggedIn }"
          :title="loggedIn ? `Signed in to sync as ${state.sync?.username}` : 'Sign in to sync'"
          @click="openSync"
        >
          ☁
        </button>
      </div>
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

      <template v-else-if="mode === 'sync'">
        <div class="sync-panel">
          <template v-if="!loggedIn">
            <p class="tagline">Sign in to open a project you've synced from another device.</p>
            <SyncSignIn />
          </template>
          <template v-else>
            <div class="sync-who">
              <span>Signed in as <strong>{{ state.sync?.username }}</strong></span>
              <button class="btn mini" :disabled="busy" @click="signOut">Sign out</button>
            </div>
            <p v-if="!remotes.length" class="tagline">
              No synced projects yet. Create or open a project, then use Sync to push it here.
            </p>
            <div v-else class="remotes">
              <div class="remotes-head">Synced projects</div>
              <button
                v-for="r in remotes"
                :key="r.id"
                class="remote-item"
                :disabled="busy"
                @click="openRemote(r)"
              >
                <span class="remote-name">{{ r.name }}</span>
                <span class="remote-meta">rev {{ r.revision }} · {{ whenText(r.updatedAt) }}</span>
              </button>
            </div>
            <button class="btn mini refresh" :disabled="busy" @click="loadRemotes">Refresh</button>
          </template>
        </div>
        <div class="actions">
          <button class="btn" @click="mode = 'home'">Back</button>
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
.welcome-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
h1 {
  margin: 0;
  font-size: 28px;
  letter-spacing: 0.5px;
}
.cog {
  font-size: 18px;
  opacity: 0.6;
}
.cog:hover {
  opacity: 1;
}
.cog.on {
  opacity: 1;
  color: var(--nv-accent);
}
.sync-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.sync-who {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}
.btn.mini {
  padding: 3px 10px;
  font-size: 11px;
}
.btn.mini.refresh {
  align-self: flex-start;
}
.remotes {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.remotes-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.07em;
  color: var(--nv-faint);
}
.remote-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  background: var(--nv-bg);
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
  font: inherit;
  text-align: left;
}
.remote-item:hover {
  border-color: var(--nv-accent);
}
.remote-name {
  font-weight: 600;
  font-size: 13px;
  text-transform: capitalize;
}
.remote-meta {
  font-size: 11px;
  color: var(--nv-faint);
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
