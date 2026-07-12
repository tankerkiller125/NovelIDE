<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  GetSettings,
  RemoteWorkspaces,
  SyncAuthConfig,
  SyncLogin,
  SyncLoginSSO,
  SyncLogout,
  SyncRegister,
} from '../api'
import { confirmDialog, refreshSyncStatus, state, syncLinkPull, syncNow } from '../store'
import type { AuthConfig, RemoteWorkspace, SyncResult } from '../types'

const server = ref('')
const username = ref('')
const password = ref('')
const busy = ref(false)
const connecting = ref(false)
const error = ref('')
const message = ref('')
const remotes = ref<RemoteWorkspace[]>([])
const authConfig = ref<AuthConfig | null>(null)

const status = computed(() => state.sync)

onMounted(async () => {
  await refreshSyncStatus()
  server.value = state.sync?.server || state.settings?.syncServer || ''
  username.value = state.sync?.username || ''
  if (state.sync?.loggedIn) void loadRemotes()
  else if (server.value) void connect()
})

// Ask the server which sign-in methods it offers, so we show the right UI.
async function connect() {
  if (!server.value.trim()) {
    error.value = 'Enter a server URL.'
    return
  }
  connecting.value = true
  error.value = ''
  try {
    authConfig.value = await SyncAuthConfig(server.value.trim())
  } catch (e) {
    authConfig.value = null
    error.value = `Couldn't reach that server: ${e}`
  } finally {
    connecting.value = false
  }
}

async function loginSSO() {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await SyncLoginSSO(server.value.trim())
    await syncSettingsFromBackend()
    await refreshSyncStatus()
    await loadRemotes()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

// Keep the persisted settings (incl. the token) in sync with the backend so a
// later save of the general settings form doesn't wipe the sync credentials.
async function syncSettingsFromBackend() {
  state.settings = await GetSettings()
}

async function auth(kind: 'login' | 'register') {
  if (!server.value.trim() || !username.value.trim() || !password.value) {
    error.value = 'Server, username, and password are required.'
    return
  }
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const fn = kind === 'login' ? SyncLogin : SyncRegister
    await fn(server.value.trim(), username.value.trim(), password.value)
    await syncSettingsFromBackend()
    await refreshSyncStatus()
    password.value = ''
    await loadRemotes()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function logout() {
  busy.value = true
  try {
    await SyncLogout()
    await syncSettingsFromBackend()
    await refreshSyncStatus()
    remotes.value = []
  } finally {
    busy.value = false
  }
}

async function loadRemotes() {
  try {
    remotes.value = await RemoteWorkspaces()
  } catch (e) {
    error.value = String(e)
  }
}

function summarize(r: SyncResult): string {
  const parts = [`revision ${r.revision}`, `↑ ${r.pushed} pushed`, `↓ ${r.pulled} pulled`]
  if (r.deleted) parts.push(`${r.deleted} removed`)
  let msg = `Synced — ${parts.join(', ')}.`
  const conflicts = r.conflicts ?? []
  if (conflicts.length) {
    msg += ` ${conflicts.length} conflict(s) — kept your version and saved the remote copy alongside it.`
  }
  return msg
}

async function doSync() {
  if (!state.workspace) {
    error.value = 'Open a workspace first.'
    return
  }
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    message.value = summarize(await syncNow())
    await loadRemotes()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function linkPull(remoteId: string) {
  if (!state.workspace) return
  const ok = await confirmDialog({
    title: 'Link and pull?',
    message: `Link the open workspace “${state.workspace.manifest.name}” to remote “${remoteId}” and download its files? Local files that differ will be overwritten with the remote version (your other files are kept).`,
    confirmText: 'Link & pull',
    danger: true,
  })
  if (!ok) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    message.value = summarize(await syncLinkPull(remoteId))
    await loadRemotes()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

function when(iso: string) {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? iso : d.toLocaleString()
}
</script>

<template>
  <section class="sync">
    <h3>Sync <span class="sync-optional">optional</span></h3>
    <p class="hint">
      NovelIDE works fully offline. To sync a workspace across devices, run a
      <a href="https://github.com/tankerkiller125/NovelIDE/blob/main/docs/SYNC.md" target="_blank" rel="noopener">sync server</a>
      and connect an account below.
    </p>

    <p v-if="error" class="sync-error">{{ error }}</p>
    <p v-if="message" class="sync-ok">{{ message }}</p>

    <!-- Signed out -->
    <div v-if="!status?.loggedIn">
      <!-- Step 1: pick a server -->
      <div v-if="!authConfig" class="sync-form">
        <label>Server URL
          <input v-model="server" placeholder="https://sync.example.com" :disabled="connecting" @keydown.enter="connect" />
        </label>
        <div class="sync-actions">
          <button class="btn primary" :disabled="connecting" @click="connect">
            {{ connecting ? 'Checking…' : 'Connect' }}
          </button>
        </div>
      </div>

      <!-- Step 2: sign in with whatever the server offers -->
      <div v-else class="sync-signin">
        <p class="hint">
          {{ server }} · <a class="sync-link" @click="authConfig = null">change server</a>
        </p>

        <button
          v-if="authConfig.ssoEnabled"
          class="btn primary sync-sso"
          :disabled="busy"
          @click="loginSSO"
        >
          {{ busy ? 'Waiting for your browser…' : `Sign in with ${authConfig.ssoName || 'SSO'}` }}
        </button>

        <div v-if="authConfig.ssoEnabled && authConfig.passwordEnabled" class="sync-or">— or —</div>

        <div v-if="authConfig.passwordEnabled" class="sync-form">
          <label>Username
            <input v-model="username" placeholder="you" :disabled="busy" autocomplete="username" />
          </label>
          <label>Password
            <input v-model="password" type="password" :disabled="busy" autocomplete="current-password" @keydown.enter="auth('login')" />
          </label>
          <div class="sync-actions">
            <button class="btn primary" :disabled="busy" @click="auth('login')">Log in</button>
            <button class="btn" :disabled="busy" @click="auth('register')">Create account</button>
          </div>
        </div>

        <p v-if="!authConfig.ssoEnabled && !authConfig.passwordEnabled" class="hint">
          This server has no sign-in methods enabled.
        </p>
      </div>
    </div>

    <!-- Signed in -->
    <div v-else class="sync-account">
      <div class="sync-who">
        Signed in as <strong>{{ status.username }}</strong>
        <span class="hint">· {{ status.server }}</span>
        <button class="btn mini" :disabled="busy" @click="logout">Sign out</button>
      </div>

      <div class="sync-ws">
        <template v-if="state.workspace">
          <span v-if="status.linked" class="sync-linked">
            “{{ state.workspace.manifest.name }}” is linked to <code>{{ status.remoteId }}</code>
          </span>
          <span v-else class="hint">
            “{{ state.workspace.manifest.name }}” isn't linked yet — Sync now links it by folder name,
            or link it to an existing remote below.
          </span>
          <button class="btn primary" :disabled="busy" @click="doSync">
            {{ busy ? 'Syncing…' : 'Sync now' }}
          </button>
        </template>
        <span v-else class="hint">Open a workspace to sync it.</span>
      </div>

      <div class="sync-remotes">
        <div class="sync-remotes-head">
          <h4>Remote workspaces</h4>
          <button class="btn mini" :disabled="busy" @click="loadRemotes">Refresh</button>
        </div>
        <p v-if="!remotes.length" class="hint">None yet. “Sync now” will create one from this workspace.</p>
        <div v-for="r in remotes" :key="r.id" class="sync-remote">
          <div class="sync-remote-main">
            <span class="sync-remote-name">{{ r.name }}</span>
            <span class="hint">rev {{ r.revision }} · {{ when(r.updatedAt) }}</span>
          </div>
          <button
            v-if="state.workspace && status.remoteId !== r.id"
            class="btn mini"
            :disabled="busy"
            @click="linkPull(r.id)"
          >
            Link &amp; pull here
          </button>
          <span v-else-if="status.remoteId === r.id" class="sync-current">linked</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.sync h3 {
  margin: 0 0 4px;
  font-size: 14px;
}
.sync-optional {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--nv-faint);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
  padding: 0 6px;
  margin-left: 6px;
}
.sync-error {
  color: var(--nv-error);
  font-size: 12px;
}
.sync-ok {
  color: #6fcf97;
  font-size: 12px;
}
.sync-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 380px;
}
.sync-form label {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 12px;
  color: var(--nv-muted);
}
.sync-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
.sync-signin {
  max-width: 380px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.sync-sso {
  align-self: flex-start;
}
.sync-or {
  font-size: 11px;
  color: var(--nv-faint);
  text-align: center;
}
.sync-link {
  cursor: pointer;
  color: var(--nv-accent);
}
.sync-account {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.sync-who,
.sync-ws {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.sync-linked code {
  background: var(--nv-panel);
  padding: 1px 5px;
  border-radius: 4px;
}
.sync-remotes-head {
  display: flex;
  align-items: center;
  gap: 10px;
}
.sync-remotes-head h4 {
  margin: 0;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
}
.sync-remote {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 8px;
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  margin-bottom: 4px;
}
.sync-remote-main {
  flex: 1;
  display: flex;
  flex-direction: column;
}
.sync-remote-name {
  font-weight: 600;
  font-size: 13px;
}
.sync-current {
  font-size: 11px;
  color: #6fcf97;
}
.btn.mini {
  padding: 2px 9px;
  font-size: 11px;
}
.hint {
  color: var(--nv-faint);
  font-size: 11px;
}
</style>
