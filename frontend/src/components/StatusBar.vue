<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RecordWritingProgress, SetDailyGoal } from '../api'
import { promptInput, state } from '../store'
import type { WritingStats } from '../types'

const stats = ref<WritingStats | null>(null)
const sessionBaseline = ref<number | null>(null)

const sessionWords = computed(() => {
  if (!stats.value || sessionBaseline.value === null) return 0
  return Math.max(0, stats.value.total - sessionBaseline.value)
})
const goalPct = computed(() => {
  const s = stats.value
  if (!s || s.goal <= 0) return 0
  return Math.min(100, Math.round((s.todayWords / s.goal) * 100))
})
const goalMet = computed(() => !!stats.value && stats.value.goal > 0 && stats.value.todayWords >= stats.value.goal)

async function refresh() {
  try {
    const s = await RecordWritingProgress()
    if (sessionBaseline.value === null) sessionBaseline.value = s.total
    stats.value = s
  } catch {
    /* no workspace / bindings not ready */
  }
}

let timer: number | undefined
onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 15000)
})
onUnmounted(() => window.clearInterval(timer))
// Refresh promptly after a chapter saves.
watch(() => state.statsTick, refresh)

async function editGoal() {
  const cur = stats.value?.goal ? String(stats.value.goal) : ''
  const v = await promptInput({
    title: 'Daily word goal',
    label: 'Target words per day (0 for none)',
    value: cur,
    placeholder: '1000',
  })
  if (v === null) return
  const n = Math.max(0, parseInt(v, 10) || 0)
  try {
    stats.value = await SetDailyGoal(n)
  } catch {
    /* ignore */
  }
}

function toggleFocus() {
  state.focusMode = !state.focusMode
}
</script>

<template>
  <div class="statusbar">
    <span class="sb-metric" :title="`${stats?.total?.toLocaleString() ?? 0} words in the manuscript`">
      📚 {{ (stats?.total ?? 0).toLocaleString() }} words
    </span>
    <span v-if="sessionWords > 0" class="sb-metric accent">+{{ sessionWords.toLocaleString() }} this session</span>

    <span class="sb-goal" @click="editGoal" title="Click to set a daily goal">
      <template v-if="stats && stats.goal > 0">
        <span :class="{ met: goalMet }">
          today {{ stats.todayWords.toLocaleString() }} / {{ stats.goal.toLocaleString() }}
        </span>
        <span class="sb-bar"><span class="sb-bar-fill" :style="{ width: goalPct + '%' }" /></span>
      </template>
      <template v-else>
        today {{ (stats?.todayWords ?? 0).toLocaleString() }} · set goal
      </template>
    </span>

    <span v-if="stats && stats.streak > 0" class="sb-metric" :title="`${stats.streak}-day streak`">
      🔥 {{ stats.streak }}
    </span>

    <span class="sb-spacer" />

    <button
      class="sb-focus"
      :class="{ on: state.aiPanelOpen }"
      title="AI assistant"
      @click="state.aiPanelOpen = !state.aiPanelOpen"
    >
      🤖 AI
    </button>
    <button
      class="sb-focus"
      :class="{ on: state.focusMode }"
      title="Focus mode (Ctrl+Shift+F)"
      @click="toggleFocus"
    >
      {{ state.focusMode ? '◄ Exit focus' : '⛶ Focus' }}
    </button>
  </div>
</template>

<style scoped>
.statusbar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 3px 12px;
  border-top: 1px solid var(--nv-border);
  background: var(--nv-panel);
  font-size: 11.5px;
  color: var(--nv-muted);
  flex-shrink: 0;
  user-select: none;
}
.sb-metric.accent {
  color: var(--nv-accent);
}
.sb-goal {
  display: flex;
  align-items: center;
  gap: 7px;
  cursor: pointer;
}
.sb-goal:hover {
  color: var(--nv-text);
}
.sb-goal .met {
  color: #6fcf97;
}
.sb-bar {
  display: inline-block;
  width: 70px;
  height: 5px;
  border-radius: 3px;
  background: var(--nv-hover);
  overflow: hidden;
}
.sb-bar-fill {
  display: block;
  height: 100%;
  background: var(--nv-accent);
}
.sb-spacer {
  flex: 1;
}
.sb-focus {
  background: none;
  border: 1px solid transparent;
  color: var(--nv-muted);
  font: inherit;
  font-size: 11.5px;
  padding: 1px 8px;
  border-radius: 5px;
  cursor: pointer;
}
.sb-focus:hover {
  border-color: var(--nv-border);
  color: var(--nv-text);
}
.sb-focus.on {
  color: var(--nv-accent);
}
</style>
