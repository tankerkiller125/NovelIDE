<script setup lang="ts">
// Reusable sync sign-in: connect to a server, then log in via SSO and/or
// username+password. On success it updates the persisted settings and the
// store's sync status; the parent reacts to state.sync.loggedIn. Shared by the
// Settings pane and the launch screen.
import { onMounted, ref } from 'vue'
import { GetSettings, SyncAuthConfig, SyncLogin, SyncLoginSSO, SyncRegister } from '../api'
import { refreshSyncStatus, state } from '../store'
import type { AuthConfig } from '../types'

const server = ref('')
const username = ref('')
const password = ref('')
const busy = ref(false)
const connecting = ref(false)
const error = ref('')
const authConfig = ref<AuthConfig | null>(null)

onMounted(async () => {
  server.value = state.sync?.server || state.settings?.syncServer || ''
  username.value = state.sync?.username || ''
  if (server.value) void connect()
})

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

// After a successful sign-in, refresh persisted settings (so the token is kept)
// and the store status (which flips loggedIn → parents update).
async function afterSignIn() {
  state.settings = await GetSettings()
  await refreshSyncStatus()
}

async function loginSSO() {
  busy.value = true
  error.value = ''
  try {
    await SyncLoginSSO(server.value.trim())
    await afterSignIn()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function auth(kind: 'login' | 'register') {
  if (!server.value.trim() || !username.value.trim() || !password.value) {
    error.value = 'Server, username, and password are required.'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const fn = kind === 'login' ? SyncLogin : SyncRegister
    await fn(server.value.trim(), username.value.trim(), password.value)
    password.value = ''
    await afterSignIn()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="signin">
    <p v-if="error" class="signin-error">{{ error }}</p>

    <!-- Step 1: choose a server -->
    <div v-if="!authConfig" class="signin-form">
      <label>Server URL
        <input v-model="server" placeholder="https://sync.example.com" :disabled="connecting" @keydown.enter="connect" />
      </label>
      <button class="btn primary" :disabled="connecting" @click="connect">
        {{ connecting ? 'Checking…' : 'Connect' }}
      </button>
    </div>

    <!-- Step 2: sign in with whatever the server offers -->
    <div v-else class="signin-methods">
      <p class="signin-server">
        {{ server }} · <a class="signin-link" @click="authConfig = null">change server</a>
      </p>

      <button
        v-if="authConfig.ssoEnabled"
        class="btn primary signin-sso"
        :disabled="busy"
        @click="loginSSO"
      >
        {{ busy ? 'Waiting for your browser…' : `Sign in with ${authConfig.ssoName || 'SSO'}` }}
      </button>

      <div v-if="authConfig.ssoEnabled && authConfig.passwordEnabled" class="signin-or">— or —</div>

      <div v-if="authConfig.passwordEnabled" class="signin-form">
        <label>Username
          <input v-model="username" placeholder="you" :disabled="busy" autocomplete="username" />
        </label>
        <label>Password
          <input v-model="password" type="password" :disabled="busy" autocomplete="current-password" @keydown.enter="auth('login')" />
        </label>
        <div class="signin-actions">
          <button class="btn primary" :disabled="busy" @click="auth('login')">Log in</button>
          <button class="btn" :disabled="busy" @click="auth('register')">Create account</button>
        </div>
      </div>

      <p v-if="!authConfig.ssoEnabled && !authConfig.passwordEnabled" class="hint">
        This server has no sign-in methods enabled.
      </p>
    </div>
  </div>
</template>

<style scoped>
.signin {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.signin-error {
  color: var(--nv-error);
  font-size: 12px;
  margin: 0;
}
.signin-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.signin-form label {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 12px;
  color: var(--nv-muted);
}
.signin-methods {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.signin-server {
  font-size: 11px;
  color: var(--nv-faint);
  margin: 0;
}
.signin-sso {
  align-self: stretch;
}
.signin-or {
  font-size: 11px;
  color: var(--nv-faint);
  text-align: center;
}
.signin-actions {
  display: flex;
  gap: 8px;
}
.signin-link {
  cursor: pointer;
  color: var(--nv-accent);
}
.hint {
  color: var(--nv-faint);
  font-size: 11px;
}
</style>
