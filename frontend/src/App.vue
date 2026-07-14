<script lang="ts" setup>
import { onMounted, onUnmounted } from 'vue'
import WelcomeScreen from './components/WelcomeScreen.vue'
import IdeShell from './components/IdeShell.vue'
import AppModal from './components/AppModal.vue'
import { GetSettings } from './api'
import { handleExternalChange, state } from './store'

// Wails injects window.runtime with the event API at runtime.
declare global {
  interface Window {
    runtime?: {
      EventsOn: (event: string, cb: (...data: any[]) => void) => () => void
    }
  }
}

let offFsChanged: (() => void) | undefined

onMounted(async () => {
  try {
    state.settings = await GetSettings()
  } catch {
    /* bindings not ready — components retry on demand */
  }
  // Pick up files changed outside NovelIDE (other editors, sync, AI agents).
  offFsChanged = window.runtime?.EventsOn?.('fs:changed', (ch: { modified?: string[]; structural?: string[] }) => {
    void handleExternalChange(ch?.modified ?? [], ch?.structural ?? [])
  })
})
onUnmounted(() => offFsChanged?.())
</script>

<template>
  <IdeShell v-if="state.workspace" />
  <WelcomeScreen v-else />
  <AppModal />
</template>
