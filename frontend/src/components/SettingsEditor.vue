<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { AppVersion, GetSettings, SaveSettings, SelectFolder, SpellStatus } from '../api'
import { pinActiveTab, state } from '../store'

const FONT_PRESETS = ['serif', 'sans', 'mono']

const form = reactive({
  deepEnabled: false,
  deepModel: '',
  modelsDir: '',
  widthMode: 'full' as 'full' | 'column',
  editorWidth: 80,
  fontChoice: 'serif',
  customFont: '',
  fontSize: 15,
  lineHeight: 1.7,
  lineNumbers: false,
  spellcheck: true,
  spellcheckLang: '',
  livePreview: true,
})
const saving = ref(false)
const message = ref('')
const spellStatus = ref('')

async function refreshSpellStatus() {
  try {
    spellStatus.value = await SpellStatus()
  } catch {
    spellStatus.value = ''
  }
}
const version = ref('')
onMounted(refreshSpellStatus)
onMounted(async () => {
  try {
    version.value = await AppVersion()
  } catch {
    version.value = 'dev'
  }
})

async function load() {
  const s = state.settings ?? (await GetSettings())
  state.settings = s
  form.deepEnabled = s.deepEnabled
  form.deepModel = s.deepModel
  form.modelsDir = s.modelsDir
  form.widthMode = s.editorWidth > 0 ? 'column' : 'full'
  form.editorWidth = s.editorWidth > 0 ? s.editorWidth : 80
  if (FONT_PRESETS.includes(s.editorFont)) {
    form.fontChoice = s.editorFont
    form.customFont = ''
  } else {
    form.fontChoice = 'custom'
    form.customFont = s.editorFont
  }
  form.fontSize = s.editorFontSize || 15
  form.lineHeight = s.editorLineHeight || 1.7
  form.lineNumbers = s.editorLineNumbers
  form.spellcheck = s.editorSpellcheck
  form.spellcheckLang = s.spellcheckLang
  form.livePreview = !s.editorRawMarkup
}
watch(() => state.settings, load, { immediate: true })

async function pickModelsDir() {
  const dir = await SelectFolder('Choose a directory for downloaded models')
  if (dir) form.modelsDir = dir
}

async function save() {
  saving.value = true
  message.value = ''
  try {
    const width =
      form.widthMode === 'column' ? Math.min(160, Math.max(40, form.editorWidth || 80)) : 0
    const font =
      form.fontChoice === 'custom' ? form.customFont.trim() || 'serif' : form.fontChoice
    state.settings = await SaveSettings({
      deepEnabled: form.deepEnabled,
      deepModel: form.deepModel.trim(),
      modelsDir: form.modelsDir.trim(),
      editorWidth: width,
      editorFont: font,
      editorFontSize: form.fontSize,
      editorLineHeight: form.lineHeight,
      editorLineNumbers: form.lineNumbers,
      editorSpellcheck: form.spellcheck,
      editorRawMarkup: !form.livePreview,
      spellcheckLang: form.spellcheckLang.trim(),
      recent: state.settings?.recent ?? [],
    })
    message.value = 'Saved.'
    // The spell engine reloads asynchronously; give it a beat.
    setTimeout(refreshSpellStatus, 800)
  } catch (e) {
    message.value = `Save failed: ${e}`
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="settings-editor" @input="pinActiveTab" @change="pinActiveTab">
    <div class="se-toolbar">
      <h2>Settings</h2>
      <span class="se-msg">{{ message }}</span>
      <button class="btn primary" :disabled="saving" @click="save">Save</button>
    </div>

    <section>
      <h3>Editor</h3>
      <label class="inline">
        Text width
        <select v-model="form.widthMode">
          <option value="full">Full pane width</option>
          <option value="column">Centered column</option>
        </select>
        <template v-if="form.widthMode === 'column'">
          <input
            v-model.number="form.editorWidth"
            type="number"
            min="40"
            max="160"
            class="width-input"
          />
          <span class="hint">characters (40–160)</span>
        </template>
      </label>
      <label class="inline">
        Font
        <select v-model="form.fontChoice">
          <option value="serif">Serif (Georgia)</option>
          <option value="sans">Sans-serif (Nunito)</option>
          <option value="mono">Monospace</option>
          <option value="custom">Custom…</option>
        </select>
        <input
          v-if="form.fontChoice === 'custom'"
          v-model="form.customFont"
          placeholder="Font family name, e.g. Palatino"
          class="font-input"
        />
      </label>
      <label class="inline">
        Font size
        <input v-model.number="form.fontSize" type="number" min="10" max="32" class="width-input" />
        <span class="hint">px (10–32)</span>
      </label>
      <label class="inline">
        Line spacing
        <input
          v-model.number="form.lineHeight"
          type="number"
          min="1.0"
          max="3.0"
          step="0.1"
          class="width-input"
        />
        <span class="hint">1.0–3.0</span>
      </label>
      <label class="inline check">
        <input type="checkbox" v-model="form.lineNumbers" />
        Show line numbers
      </label>
      <label class="inline check">
        <input type="checkbox" v-model="form.livePreview" />
        Live Markdown preview
        <span class="hint">render **bold**, *italic*, headings &amp; `code` inline; markers reappear when the cursor is on them</span>
      </label>
      <label class="inline check">
        <input type="checkbox" v-model="form.spellcheck" />
        Spellcheck while typing
        <template v-if="form.spellcheck">
          <input v-model="form.spellcheckLang" placeholder="en_US" class="width-input lang" />
          <span class="hint">dictionary language — needs a matching hunspell dictionary installed</span>
        </template>
      </label>
      <p v-if="form.spellcheck && spellStatus" class="hint spell-warn">
        ⚠ Spellchecker unavailable: {{ spellStatus }}
      </p>
      <p class="hint">
        All of these affect only the writing view — never the book itself. A centered
        column of 70–90 characters is a comfortable reading measure. Changes apply
        immediately after saving.
      </p>
    </section>

    <section>
      <h3>Language analysis</h3>
      <p class="hint">
        The standard engine (part-of-speech tagging, named-entity recognition, feature
        extraction) always runs — it's fast, fully local, and needs no downloads.
      </p>

      <label class="toggle">
        <input type="checkbox" v-model="form.deepEnabled" />
        <span>
          <strong>Enable deep NLP (Cybertron)</strong><br />
          <span class="hint">
            Runs a transformer model locally for higher-quality entity detection — catches
            names the fast engine misses. Adds a <em>Deep scan</em> button to the editor's
            Consistency panel; it never runs automatically. The first scan downloads the
            model (several hundred MB) and a scan takes a few seconds. Everything stays on
            this machine — no accounts, no API calls.
          </span>
        </span>
      </label>

      <div class="se-fields" :class="{ disabled: !form.deepEnabled }">
        <label>
          Model (Hugging Face name)
          <input
            v-model="form.deepModel"
            :disabled="!form.deepEnabled"
            placeholder="dbmdz/bert-large-cased-finetuned-conll03-english"
          />
          <span class="hint">
            Token-classification models only. Alternatives:
            <code>djagatiya/ner-bert-base-cased-ontonotesv5-englishv4</code> (more entity
            types), <code>Babelscape/wikineural-multilingual-ner</code> (multilingual).
          </span>
        </label>
        <label>
          Model storage directory
          <div class="dir-row">
            <input v-model="form.modelsDir" :disabled="!form.deepEnabled" />
            <button class="btn" :disabled="!form.deepEnabled" @click="pickModelsDir">
              Browse
            </button>
          </div>
        </label>
      </div>
    </section>

    <section>
      <h3>Recent projects</h3>
      <p v-if="!state.settings?.recent?.length" class="hint">Nothing yet.</p>
      <ul class="recent">
        <li v-for="r in state.settings?.recent ?? []" :key="r">{{ r }}</li>
      </ul>
    </section>

    <section>
      <h3>About</h3>
      <p class="hint">
        NovelIDE <strong>{{ version }}</strong> · AGPL-3.0-or-later ·
        <a href="https://github.com/tankerkiller125/NovelIDE" target="_blank" rel="noopener">
          github.com/tankerkiller125/NovelIDE
        </a>
      </p>
    </section>
  </div>
</template>

<style scoped>
.settings-editor {
  height: 100%;
  overflow-y: auto;
  padding: 20px 28px 60px;
  max-width: 760px;
}
.se-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.se-toolbar h2 {
  flex: 1;
  margin: 0;
  font-size: 18px;
}
.se-msg {
  color: var(--nv-muted);
  font-size: 12px;
}
section {
  margin: 22px 0;
}
h3 {
  margin: 0 0 8px;
  font-size: 14px;
}
.inline {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: var(--nv-muted);
  margin: 10px 0 6px;
}
.width-input {
  width: 76px;
}
.font-input {
  width: 240px;
}
.inline.check {
  cursor: pointer;
}
.spell-warn {
  color: var(--nv-warning);
}
.toggle {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin: 12px 0;
  cursor: pointer;
}
.toggle input {
  margin-top: 3px;
}
.se-fields {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 14px 0 0 26px;
}
.se-fields.disabled {
  opacity: 0.55;
}
.se-fields label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--nv-muted);
}
.dir-row {
  display: flex;
  gap: 8px;
}
.dir-row input {
  flex: 1;
}
.hint {
  color: var(--nv-faint);
  font-size: 12px;
}
code {
  background: var(--nv-hover);
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 11px;
}
.recent {
  margin: 6px 0 0;
  padding-left: 18px;
  color: var(--nv-muted);
  font-size: 12px;
}
.recent li {
  padding: 2px 0;
}
</style>
