<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  DetectACPAgents,
  GetAIConfig,
  SaveAIConfig,
  SecretStorageSecure,
  TestAIConnection,
} from '../api'
import { state } from '../store'
import type { ACPAgent, AIConfig, AINamedProvider, AIProviderKind } from '../types'

const cfg = ref<AIConfig>(blankConfig())
const busy = ref(false)
const message = ref('')
const error = ref('')
const testingId = ref('') // provider id currently being tested
const testResult = ref<{ id: string; ok: boolean; text: string } | null>(null)
const secureStorage = ref(true)
const acpAgents = ref<ACPAgent[]>([])

const DEFAULT_URL: Record<AIProviderKind, string> = {
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com',
  gemini: '',
  a2a: '',
  acp: '',
}

function blankConfig(): AIConfig {
  return { enabled: false, providers: [] }
}

onMounted(async () => {
  try {
    const c = await GetAIConfig()
    cfg.value = { ...c, providers: c.providers ?? [] }
    secureStorage.value = await SecretStorageSecure()
    acpAgents.value = (await DetectACPAgents()) ?? []
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
  cfg.value.providers?.splice(i, 1)
}
function setModels(p: AINamedProvider, value: string) {
  p.models = value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
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

// test verifies a provider works, using its first configured model.
async function test(p: AINamedProvider) {
  const model = p.models?.[0] ?? ''
  if (!model) {
    testResult.value = { id: p.id, ok: false, text: 'Add a model first.' }
    return
  }
  testingId.value = p.id
  testResult.value = null
  try {
    const reply = await TestAIConnection(p, model)
    testResult.value = { id: p.id, ok: true, text: `OK — “${reply}”` }
  } catch (e) {
    testResult.value = { id: p.id, ok: false, text: String(e) }
  } finally {
    testingId.value = ''
  }
}
</script>

<template>
  <section class="ai">
    <h3>AI <span class="ai-optional">optional</span></h3>
    <p class="hint">
      Connect OpenAI, Anthropic, Google Gemini, any OpenAI-compatible gateway, a local model, or an
      agent (A2A / a local coding CLI). List the models you want on each provider — then choose any of
      them for the assistant or planning agent from the model picker while chatting. All AI is off
      unless you enable it below.
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
            <option value="gemini">Google Gemini</option>
            <option value="a2a">A2A agent</option>
            <option value="acp">Local agent (ACP)</option>
          </select>
          <button class="btn icon" title="Remove" @click="removeProvider(i)">✕</button>
        </div>

        <!-- ACP: pick a detected local coding agent instead of a URL/key -->
        <template v-if="p.kind === 'acp'">
          <div class="ai-row">
            <select v-model="p.baseUrl" class="ai-provsel">
              <option value="">— installed agent —</option>
              <option v-for="ag in acpAgents" :key="ag.id" :value="ag.id">{{ ag.label }}</option>
            </select>
          </div>
          <p class="hint ai-keynote">
            <template v-if="acpAgents.length">
              Runs a coding agent installed on your machine (via the Agent Client Protocol). It reads your manuscript
              and its edits come back as proposals you approve. The model/mode settings below don't apply.
            </template>
            <template v-else>
              No supported ACP agents detected on your PATH. Install one (e.g. the Gemini CLI, or Node for Claude
              Code) and reopen Settings.
            </template>
          </p>
        </template>

        <template v-else>
          <div class="ai-row">
            <input
              v-model="p.baseUrl"
              :placeholder="p.kind === 'a2a' ? 'Agent card URL' : p.kind === 'gemini' ? 'Base URL (optional)' : 'Base URL'"
              class="ai-url"
            />
            <input
              v-if="p.kind !== 'a2a'"
              v-model="p.apiKey"
              type="password"
              placeholder="API key"
              class="ai-key"
              autocomplete="off"
            />
          </div>
          <div v-if="p.kind !== 'a2a'" class="ai-row">
            <input
              :value="(p.models ?? []).join(', ')"
              placeholder="Models for the chat picker (comma-separated, e.g. gpt-4o, gpt-4o-mini)"
              class="ai-url"
              @change="setModels(p, ($event.target as HTMLInputElement).value)"
            />
          </div>
          <div v-if="p.kind !== 'a2a'" class="ai-row ai-test-row">
            <button class="btn mini" :disabled="testingId === p.id" @click="test(p)">
              {{ testingId === p.id ? 'Testing…' : 'Test' }}
            </button>
            <span v-if="testResult && testResult.id === p.id" class="ai-test" :class="{ bad: !testResult.ok }">
              {{ testResult.text }}
            </span>
          </div>
          <p v-if="p.kind === 'a2a'" class="hint ai-keynote">
            Connects to a local or remote A2A agent by its card URL (use an <code>http://</code> URL for a plaintext
            agent on your machine). It runs its own model and tools, so the model setting doesn't apply.
          </p>
        </template>
      </div>
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
