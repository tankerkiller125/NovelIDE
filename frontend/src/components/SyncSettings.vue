<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { GetSettings, RemoteWorkspaces, SyncLogout } from '../api'
import { confirmDialog, refreshSyncStatus, state, syncLinkPull, syncNow } from '../store'
import type { RemoteWorkspace, SyncResult } from '../types'
import SyncSignIn from './SyncSignIn.vue'

const busy = ref(false)
const error = ref('')
const message = ref('')
const remotes = ref<RemoteWorkspace[]>([])

const status = computed(() => state.sync)

onMounted(async () => {
  await refreshSyncStatus()
  if (state.sync?.loggedIn) void loadRemotes()
})
// Sign-in happens in the child component; load remotes once it succeeds.
watch(
  () => state.sync?.loggedIn,
  (li) => {
    if (li) void loadRemotes()
  },
)

async function logout() {
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

    <!-- Signed out: reusable sign-in -->
    <SyncSignIn v-if="!status?.loggedIn" class="sync-form" />

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
