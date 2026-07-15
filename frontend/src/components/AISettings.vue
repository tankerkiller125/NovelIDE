<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { GetAIConfig, SaveAIConfig, SecretStorageSecure, TestAIConnection } from '../api'
import { state } from '../store'
import type { AIConfig, AIModeConfig, AINamedProvider, AIProviderKind } from '../types'

const cfg = ref<AIConfig>(blankConfig())
const busy = ref(false)
const message = ref('')
const error = ref('')
const testing = ref<'assistant' | 'planning' | ''>('')
const testResult = ref<{ mode: string; ok: boolean; text: string } | null>(null)
const secureStorage = ref(true)

const DEFAULT_URL: Record<AIProviderKind, string> = {
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com',
}

function blankMode(): AIModeConfig {
  return { providerId: '', model: '', contextTokens: 0, maxOutputTokens: 0, temperature: 0 }
}
function blankConfig(): AIConfig {
  return { enabled: false, providers: [], assistant: blankMode(), planning: blankMode() }
}

onMounted(async () => {
  try {
    const c = await GetAIConfig()
    cfg.value = { ...c, providers: c.providers ?? [] }
    secureStorage.value = await SecretStorageSecure()
  } catch (e) {
    error.value = String(e)
  }
})

function addProvider() {
  cfg.value.providers = cfg.value.providers ?? []
  cfg.value.providers.push({
    id: crypto.randomUUID(),
    name: `Provider ${cfg.value.providers.length + 1}`,
    kind: 'openai',
    baseUrl: DEFAULT_URL.openai,
    apiKey: '',
  })
}
function removeProvider(i: number) {
  const removed = cfg.value.providers?.[i]
  cfg.value.providers?.splice(i, 1)
  // Clear mode references to a deleted provider.
  for (const m of [cfg.value.assistant, cfg.value.planning]) {
    if (removed && m.providerId === removed.id) m.providerId = ''
  }
}
function onKindChange(p: AINamedProvider) {
  // If the URL was still the other kind's default, switch it too.
  if (p.baseUrl === '' || p.baseUrl === DEFAULT_URL.openai || p.baseUrl === DEFAULT_URL.anthropic) {
    p.baseUrl = DEFAULT_URL[p.kind]
  }
}

async function save() {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const saved = await SaveAIConfig(cfg.value)
    cfg.value = { ...saved, providers: saved.providers ?? [] }
    // Mirror into settings so the general settings save round-trips it.
    if (state.settings) state.settings.ai = cfg.value
    message.value = 'Saved.'
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}

async function test(mode: 'assistant' | 'planning') {
  const m = cfg.value[mode]
  const provider = (cfg.value.providers ?? []).find((p) => p.id === m.providerId)
  if (!provider) {
    testResult.value = { mode, ok: false, text: 'Pick a provider first.' }
    return
  }
  testing.value = mode
  testResult.value = null
  try {
    const reply = await TestAIConnection(provider, m.model)
    testResult.value = { mode, ok: true, text: `OK — “${reply}”` }
  } catch (e) {
    testResult.value = { mode, ok: false, text: String(e) }
  } finally {
    testing.value = ''
  }
}
</script>

<template>
  <section class="ai">
    <h3>AI <span class="ai-optional">optional</span></h3>
    <p class="hint">
      Connect any OpenAI- or Anthropic-compatible provider (OpenAI, Anthropic, Ollama, OpenRouter,
      a self-hosted endpoint…). Configure a provider once, then pick a model for each mode. All AI
      is off unless you enable it below. Keys are stored in your local settings file.
    </p>

    <p v-if="error" class="ai-error">{{ error }}</p>
    <p v-if="message" class="ai-ok">{{ message }}</p>

    <label class="ai-check">
      <input type="checkbox" v-model="cfg.enabled" />
      Enable AI features
    </label>

    <!-- Providers -->
    <div class="ai-sub">
      <div class="ai-sub-head">
        <h4>Providers</h4>
        <button class="btn mini" @click="addProvider">+ Add provider</button>
      </div>
      <p class="hint ai-keynote">
        <template v-if="secureStorage">🔒 API keys are stored in your operating system's keychain, not in the settings file.</template>
        <template v-else>⚠ No system keychain was found, so API keys are kept in your settings file (readable only by
          your user account). Install/enable a secret service — GNOME Keyring or KWallet — for encrypted storage.</template>
      </p>
      <p v-if="!(cfg.providers && cfg.providers.length)" class="hint">
        No providers yet. Add one to get started.
      </p>
      <div v-for="(p, i) in cfg.providers ?? []" :key="p.id" class="ai-provider">
        <div class="ai-row">
          <input v-model="p.name" placeholder="Name" class="ai-name" />
          <select v-model="p.kind" class="ai-kind" @change="onKindChange(p)">
            <option value="openai">OpenAI-compatible</option>
            <option value="anthropic">Anthropic</option>
          </select>
          <button class="btn icon" title="Remove" @click="removeProvider(i)">✕</button>
        </div>
        <div class="ai-row">
          <input v-model="p.baseUrl" placeholder="Base URL" class="ai-url" />
          <input v-model="p.apiKey" type="password" placeholder="API key" class="ai-key" autocomplete="off" />
        </div>
        <label class="ai-check" title="Send the whole reply at once instead of token-by-token. Turn on if streaming returns nothing (e.g. Cloudflare's OpenAI gateway with Claude).">
          <input type="checkbox" v-model="p.noStream" />
          Disable streaming (request full reply in one response)
        </label>
      </div>
    </div>

    <!-- Modes -->
    <div v-for="mode in (['assistant', 'planning'] as const)" :key="mode" class="ai-sub">
      <div class="ai-sub-head">
        <h4>{{ mode === 'assistant' ? 'Writing assistant' : 'Planning (agent)' }}</h4>
        <button class="btn mini" :disabled="testing === mode" @click="test(mode)">
          {{ testing === mode ? 'Testing…' : 'Test' }}
        </button>
      </div>
      <p class="hint">
        {{
          mode === 'assistant'
            ? 'Grounded chat and prose help.'
            : 'Agentic planning with tools (proposes changes for your approval).'
        }}
      </p>
      <div class="ai-row">
        <select v-model="cfg[mode].providerId" class="ai-provsel">
          <option value="">— provider —</option>
          <option v-for="p in cfg.providers ?? []" :key="p.id" :value="p.id">{{ p.name }}</option>
        </select>
        <input v-model="cfg[mode].model" placeholder="Model (e.g. gpt-4o, claude-sonnet-5, llama3.1)" class="ai-model" />
      </div>
      <div class="ai-row ai-budget">
        <label>Context tokens
          <input type="number" v-model.number="cfg[mode].contextTokens" placeholder="8192" min="0" />
        </label>
        <label>Max reply tokens
          <input type="number" v-model.number="cfg[mode].maxOutputTokens" placeholder="2048" min="0" />
        </label>
        <label>Temperature
          <input type="number" v-model.number="cfg[mode].temperature" step="0.1" min="0" max="2" />
        </label>
      </div>
      <p v-if="testResult && testResult.mode === mode" class="ai-test" :class="{ bad: !testResult.ok }">
        {{ testResult.text }}
      </p>
    </div>

    <div class="ai-actions">
      <button class="btn primary" :disabled="busy" @click="save">Save AI settings</button>
    </div>
  </section>
</template>

<style scoped>
.ai h3 {
  margin: 0 0 4px;
  font-size: 14px;
}
.ai-optional {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--nv-faint);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
  padding: 0 6px;
  margin-left: 6px;
}
.ai-error {
  color: var(--nv-error);
  font-size: 12px;
}
.ai-ok {
  color: #6fcf97;
  font-size: 12px;
}
.ai-check {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 13px;
  color: var(--nv-muted);
  margin: 8px 0;
}
.ai-sub {
  border-top: 1px solid var(--nv-border);
  padding-top: 10px;
  margin-top: 10px;
}
.ai-sub-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.ai-sub-head h4 {
  margin: 0;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
}
.ai-keynote {
  margin: 2px 0 6px;
}
.ai-provider {
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  padding: 8px;
  margin: 6px 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ai-row {
  display: flex;
  gap: 6px;
}
.ai-check {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--nv-muted);
  cursor: pointer;
}
.ai-check input {
  cursor: pointer;
}
.ai-row input,
.ai-row select {
  font-size: 12px;
  padding: 4px 6px;
}
.ai-name {
  width: 140px;
}
.ai-kind {
  width: 160px;
}
.ai-url,
.ai-key,
.ai-model {
  flex: 1;
}
.ai-provsel {
  width: 200px;
}
.ai-budget {
  flex-wrap: wrap;
}
.ai-budget label {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 11px;
  color: var(--nv-muted);
}
.ai-budget input {
  width: 110px;
  font-size: 12px;
  padding: 4px 6px;
}
.ai-test {
  font-size: 12px;
  color: #6fcf97;
  margin: 4px 0 0;
}
.ai-test.bad {
  color: var(--nv-error);
}
.ai-actions {
  margin-top: 12px;
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
