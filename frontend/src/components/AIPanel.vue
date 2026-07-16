<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { AICancel, AIChat } from '../api'
import {
  activeTabObj,
  addProposal,
  applyProposal,
  discardProposal,
  openTab,
  reviewProposal,
  state,
} from '../store'
import type { AIMessage, AIProposal } from '../types'

const mode = ref<'assistant' | 'planning'>('assistant')
const messages = ref<AIMessage[]>([])
const input = ref('')
const streaming = ref('') // current stream id, '' when idle
const cancelled = new Set<string>()
const listEl = ref<HTMLElement | null>(null)
// Proposals live in the store (prose ones also render inline in the editor).
const proposals = computed(() => state.proposals)
const busyProposal = ref('') // proposal id currently applying, '' when idle

// Per-turn model choice, remembered per mode. Keys are `${providerId}|${model}`.
const modelChoice = reactive<Record<string, string>>({})

// Every model the user has made available across providers, plus agent providers.
const availableModels = computed(() => {
  const opts: { key: string; label: string; providerId: string; model: string }[] = []
  for (const p of state.settings?.ai?.providers ?? []) {
    if (p.kind === 'acp' || p.kind === 'a2a') {
      opts.push({ key: `${p.id}|`, label: p.name, providerId: p.id, model: '' })
    } else {
      for (const m of p.models ?? []) {
        opts.push({ key: `${p.id}|${m}`, label: `${p.name} · ${m}`, providerId: p.id, model: m })
      }
    }
  }
  return opts
})

const modelOptions = computed(() => availableModels.value)

const selectedKey = computed<string>({
  get() {
    const c = modelChoice[mode.value]
    if (c && modelOptions.value.some((o) => o.key === c)) return c
    return modelOptions.value[0]?.key ?? '' // default to the first available model
  },
  set(v: string) {
    modelChoice[mode.value] = v
  },
})

const selectedProviderModel = computed(() => {
  const key = selectedKey.value
  const i = key.indexOf('|')
  return i < 0 ? { providerId: key, model: '' } : { providerId: key.slice(0, i), model: key.slice(i + 1) }
})

const configured = computed(() => {
  const ai = state.settings?.ai
  if (!ai?.enabled) return false
  const sel = selectedProviderModel.value
  const p = (ai.providers ?? []).find((x) => x.id === sel.providerId)
  if (!p) return false
  if (p.kind === 'acp' || p.kind === 'a2a') return true
  return !!sel.model
})

const activeChapter = computed(() => {
  const t = activeTabObj.value
  return t?.kind === 'chapter' ? { bookId: t.bookId, chapter: t.chapter } : { bookId: '', chapter: '' }
})

// A short, human label for a tool invocation shown as activity under the reply.
const toolVerb: Record<string, string> = {
  search_codex: 'Searched codex',
  get_entry: 'Read codex entry',
  search_manuscript: 'Searched manuscript',
  read_chapter: 'Read chapter',
  list_structure: 'Read structure',
}
function toolLabel(name: string, args: string): string {
  const verb = toolVerb[name] ?? name
  let arg = ''
  try {
    const o = JSON.parse(args || '{}')
    arg = o.query || o.id || o.chapter || ''
  } catch {
    /* args may be malformed mid-stream */
  }
  return arg ? `${verb}: ${arg}` : verb
}

function scrollToBottom() {
  void nextTick(() => {
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  })
}

async function send() {
  const text = input.value.trim()
  if (!text || streaming.value) return
  messages.value.push({ role: 'user', content: text })
  input.value = ''
  const history = messages.value.slice() // prior turns + this user message
  messages.value.push({ role: 'assistant', content: '' })
  const id = crypto.randomUUID()
  streaming.value = id
  scrollToBottom()
  const sel = selectedProviderModel.value
  try {
    await AIChat(id, mode.value, history, activeChapter.value.bookId, activeChapter.value.chapter, sel.providerId, sel.model)
  } catch {
    /* errors surface via the ai:error event */
  }
}

function stop() {
  const id = streaming.value
  if (!id) return
  cancelled.add(id)
  void AICancel(id)
  streaming.value = ''
}

function newChat() {
  if (streaming.value) stop()
  messages.value = []
}

const proposalKind: Record<string, string> = {
  codex: 'Codex edit',
  plan: 'Plan edit',
  prose: 'Prose edit',
}

async function onApply(p: AIProposal) {
  if (busyProposal.value) return
  busyProposal.value = p.id
  try {
    await applyProposal(p.id)
  } catch (err) {
    messages.value.push({ role: 'assistant', content: `⚠ Couldn't apply that edit: ${err}` })
    scrollToBottom()
  } finally {
    busyProposal.value = ''
  }
}

function onDiscard(p: AIProposal) {
  discardProposal(p.id)
}

// Prose edits are approved inline in the editor — open the target chapter.
function onReview(p: AIProposal) {
  reviewProposal(p)
}

function onEnter(e: KeyboardEvent) {
  if (!e.shiftKey) {
    e.preventDefault()
    void send()
  }
}

let offs: Array<() => void> = []
onMounted(() => {
  const on = window.runtime?.EventsOn
  if (!on) return
  offs = [
    on('ai:delta', (p: { id: string; text: string }) => {
      if (p.id !== streaming.value) return
      const last = messages.value[messages.value.length - 1]
      if (last?.role === 'assistant') {
        last.content += p.text
        scrollToBottom()
      }
    }),
    on('ai:tool', (p: { id: string; name: string; args: string }) => {
      if (p.id !== streaming.value) return
      const last = messages.value[messages.value.length - 1]
      if (last?.role === 'assistant') {
        ;(last.tools ??= []).push(toolLabel(p.name, p.args))
        scrollToBottom()
      }
    }),
    on('ai:proposal', (p: AIProposal) => {
      addProposal(p)
      scrollToBottom()
    }),
    on('ai:done', (p: { id: string }) => {
      if (p.id === streaming.value) streaming.value = ''
    }),
    on('ai:error', (p: { id: string; error: string }) => {
      if (cancelled.has(p.id)) {
        cancelled.delete(p.id)
        return
      }
      if (p.id !== streaming.value) return
      const last = messages.value[messages.value.length - 1]
      if (last?.role === 'assistant') {
        last.content += (last.content ? '\n\n' : '') + `⚠ ${p.error}`
      }
      streaming.value = ''
    }),
  ]
})
onUnmounted(() => offs.forEach((off) => off?.()))
</script>

<template>
  <aside class="aip">
    <div class="aip-head">
      <div class="aip-modes">
        <button :class="{ on: mode === 'assistant' }" @click="mode = 'assistant'">Assistant</button>
        <button :class="{ on: mode === 'planning' }" @click="mode = 'planning'">Planning</button>
      </div>
      <button class="aip-icon" title="New chat" @click="newChat">🗑</button>
      <button class="aip-icon" title="Close" @click="state.aiPanelOpen = false">✕</button>
    </div>
    <div v-if="modelOptions.length" class="aip-model">
      <select v-model="selectedKey" :title="`Model for ${mode}`">
        <option v-for="o in modelOptions" :key="o.key" :value="o.key">{{ o.label }}</option>
      </select>
    </div>

    <div ref="listEl" class="aip-msgs">
      <p v-if="!messages.length" class="aip-empty">
        {{
          mode === 'assistant'
            ? 'Ask about your world, or get help drafting and revising. Grounded in your Codex and the open chapter.'
            : 'Plan structure, arcs, and consistency — grounded in your Codex.'
        }}
      </p>
      <div v-for="(m, i) in messages" :key="i" class="aip-msg" :class="m.role">
        <div class="aip-role">{{ m.role === 'user' ? 'You' : 'AI' }}</div>
        <div v-if="m.tools?.length" class="aip-tools">
          <span v-for="(t, ti) in m.tools" :key="ti" class="aip-tool">🔧 {{ t }}</span>
        </div>
        <div class="aip-text">{{ m.content
          }}<span v-if="streaming && i === messages.length - 1 && m.role === 'assistant'" class="aip-cursor">▍</span></div>
      </div>
    </div>

    <div v-if="proposals.length" class="aip-props">
      <div class="aip-props-head">Proposed edits · {{ proposals.length }}</div>
      <div v-for="p in proposals" :key="p.id" class="aip-prop">
        <div class="aip-prop-top">
          <span class="aip-prop-kind" :class="p.kind">{{ proposalKind[p.kind] ?? p.kind }}</span>
          <span class="aip-prop-target">{{ p.target }}</span>
        </div>
        <div class="aip-prop-sum">{{ p.summary }}</div>
        <div v-if="p.before" class="aip-diff before">{{ p.before }}</div>
        <div v-if="p.after" class="aip-diff after">{{ p.after }}</div>
        <div class="aip-prop-actions">
          <button class="btn mini" :disabled="!!busyProposal" @click="onDiscard(p)">Discard</button>
          <button
            v-if="p.kind === 'prose'"
            class="btn mini primary"
            title="Approve inline where it's edited"
            @click="onReview(p)"
          >
            Review in editor →
          </button>
          <button v-else class="btn mini primary" :disabled="!!busyProposal" @click="onApply(p)">
            {{ busyProposal === p.id ? 'Applying…' : 'Apply' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="!configured" class="aip-warn">
      This mode isn't set up.
      <a @click="openTab({ kind: 'settings' })">Configure AI in Settings →</a>
    </div>

    <div class="aip-input">
      <textarea
        v-model="input"
        rows="3"
        :placeholder="`Message the ${mode}… (Enter to send, Shift+Enter for a newline)`"
        @keydown.enter="onEnter"
      />
      <div class="aip-actions">
        <span v-if="activeChapter.chapter" class="aip-ctx" title="The open chapter is included as context">
          📄 context on
        </span>
        <button v-if="streaming" class="btn mini danger" @click="stop">Stop</button>
        <button v-else class="btn mini primary" :disabled="!input.trim()" @click="send">Send</button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.aip {
  width: 360px;
  flex-shrink: 0;
  border-left: 1px solid var(--nv-border);
  background: var(--nv-panel);
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.aip-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-bottom: 1px solid var(--nv-border);
}
.aip-model {
  padding: 4px 8px;
  border-bottom: 1px solid var(--nv-border);
}
.aip-model select {
  width: 100%;
  font-size: 11px;
  padding: 3px 6px;
}
.aip-modes {
  display: flex;
  gap: 4px;
  flex: 1;
}
.aip-modes button {
  background: none;
  border: 1px solid transparent;
  border-radius: 6px;
  color: var(--nv-muted);
  padding: 3px 10px;
  font-size: 12px;
  cursor: pointer;
}
.aip-modes button.on {
  background: var(--nv-hover);
  color: var(--nv-text);
  border-color: var(--nv-border);
}
.aip-icon {
  background: none;
  border: none;
  color: var(--nv-faint);
  cursor: pointer;
  font-size: 12px;
  padding: 3px 5px;
}
.aip-icon:hover {
  color: var(--nv-text);
}
.aip-msgs {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 0;
}
.aip-empty {
  color: var(--nv-faint);
  font-size: 12px;
  line-height: 1.5;
}
.aip-msg {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.aip-role {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
}
.aip-text {
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.aip-msg.user .aip-text {
  color: var(--nv-text);
  background: var(--nv-hover);
  border-radius: 8px;
  padding: 6px 9px;
}
.aip-msg.assistant .aip-text {
  color: var(--nv-text);
}
.aip-tools {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 2px;
}
.aip-tool {
  font-size: 11px;
  color: var(--nv-faint);
  font-family: var(--nv-mono, monospace);
}
.aip-cursor {
  color: var(--nv-accent);
  animation: aip-blink 1s steps(2) infinite;
}
@keyframes aip-blink {
  50% {
    opacity: 0;
  }
}
.aip-props {
  border-top: 1px solid var(--nv-border);
  max-height: 45%;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.aip-props-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
}
.aip-prop {
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 5px;
  background: var(--nv-bg);
}
.aip-prop-top {
  display: flex;
  align-items: center;
  gap: 6px;
}
.aip-prop-kind {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--nv-hover);
  color: var(--nv-muted);
  white-space: nowrap;
}
.aip-prop-kind.prose {
  color: var(--nv-accent);
}
.aip-prop-target {
  font-size: 11px;
  color: var(--nv-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.aip-prop-sum {
  font-size: 12px;
  color: var(--nv-text);
}
.aip-diff {
  font-size: 11px;
  font-family: var(--nv-mono, monospace);
  white-space: pre-wrap;
  word-break: break-word;
  border-radius: 5px;
  padding: 4px 6px;
  max-height: 8em;
  overflow-y: auto;
}
.aip-diff.before {
  background: color-mix(in srgb, var(--nv-danger, #e5484d) 14%, transparent);
  text-decoration: line-through;
  opacity: 0.85;
}
.aip-diff.after {
  background: color-mix(in srgb, var(--nv-accent, #3b82f6) 16%, transparent);
}
.aip-prop-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}
.aip-warn {
  font-size: 11px;
  color: var(--nv-warning);
  padding: 6px 10px;
  border-top: 1px solid var(--nv-border);
}
.aip-warn a {
  color: var(--nv-accent);
  cursor: pointer;
}
.aip-input {
  border-top: 1px solid var(--nv-border);
  padding: 8px;
}
.aip-input textarea {
  width: 100%;
  resize: vertical;
  font: inherit;
  font-size: 13px;
}
.aip-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}
.aip-ctx {
  flex: 1;
  font-size: 11px;
  color: var(--nv-faint);
}
.btn.mini {
  padding: 3px 12px;
  font-size: 12px;
}
</style>
