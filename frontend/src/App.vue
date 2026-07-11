<script lang="ts" setup>
import { onMounted } from 'vue'
import WelcomeScreen from './components/WelcomeScreen.vue'
import IdeShell from './components/IdeShell.vue'
import { GetSettings } from './api'
import { state } from './store'

onMounted(async () => {
  try {
    state.settings = await GetSettings()
  } catch {
    /* bindings not ready — components retry on demand */
  }
})
</script>

<template>
  <IdeShell v-if="state.workspace" />
  <WelcomeScreen v-else />
</template>
