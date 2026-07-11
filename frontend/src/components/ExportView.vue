<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ExportPreview, ExportSave, ExportThemes } from '../api'
import { state } from '../store'
import type { ExportFormat, ExportTheme } from '../types'

const themes = ref<ExportTheme[]>([])
const previewHtml = ref('')
const previewError = ref('')
const exporting = ref(false)
const message = ref('')

const books = computed(() => state.workspace?.books ?? [])
const isSeries = computed(() => (state.workspace?.manifest.kind ?? 'novel') === 'series')

const form = reactive({
  format: 'epub' as ExportFormat,
  themeId: '',
  title: state.workspace?.manifest.name ?? '',
  author: '',
  selected: new Set<string>(books.value.map((b) => b.id)),
  titlePage: true,
})

const activeTheme = computed(() => themes.value.find((t) => t.id === form.themeId))

/** Book ids to send: [] means "all", otherwise the chosen subset. */
function bookList(): string[] {
  if (form.selected.size === books.value.length) return []
  return books.value.filter((b) => form.selected.has(b.id)).map((b) => b.id)
}

function opts(format: ExportFormat) {
  return {
    format,
    themeId: form.themeId,
    title: form.title.trim() || (state.workspace?.manifest.name ?? 'Untitled'),
    author: form.author.trim(),
    books: bookList(),
    titlePage: form.titlePage,
  }
}

onMounted(async () => {
  try {
    themes.value = await ExportThemes()
    if (themes.value.length && !form.themeId) form.themeId = themes.value[0].id
  } catch (e) {
    previewError.value = String(e)
  }
})

let previewTimer: number | undefined
function schedulePreview() {
  window.clearTimeout(previewTimer)
  previewTimer = window.setTimeout(refreshPreview, 300)
}
async function refreshPreview() {
  if (!form.themeId || form.selected.size === 0) {
    previewHtml.value = ''
    return
  }
  try {
    previewHtml.value = await ExportPreview(opts('html'))
    previewError.value = ''
  } catch (e) {
    previewError.value = String(e)
    previewHtml.value = ''
  }
}
watch(
  () => [form.themeId, form.title, form.author, form.titlePage, [...form.selected].join(',')],
  schedulePreview,
)
watch(() => form.themeId, refreshPreview) // first paint once themes load

function toggleBook(id: string) {
  if (form.selected.has(id)) form.selected.delete(id)
  else form.selected.add(id)
}

async function doExport() {
  if (form.selected.size === 0) return
  exporting.value = true
  message.value = ''
  try {
    const path = await ExportSave(opts(form.format))
    message.value = path ? `Exported to ${path}` : 'Export cancelled.'
  } catch (e) {
    message.value = `Export failed: ${e}`
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <div class="export-view">
    <div class="ev-panel">
      <h2>Export book</h2>

      <label class="ev-field">
        Format
        <select v-model="form.format">
          <option value="epub">EPUB e-book</option>
          <option value="html">Print-ready HTML (→ Save as PDF)</option>
        </select>
      </label>

      <label class="ev-field">
        Theme
        <select v-model="form.themeId">
          <option v-for="t in themes" :key="t.id" :value="t.id">{{ t.label }}</option>
        </select>
      </label>
      <p v-if="activeTheme" class="ev-hint">{{ activeTheme.description }}</p>

      <label class="ev-field">Title <input v-model="form.title" placeholder="Book title" /></label>
      <label class="ev-field">Author <input v-model="form.author" placeholder="Your name" /></label>

      <label class="ev-check">
        <input type="checkbox" v-model="form.titlePage" />
        Include a title page
      </label>

      <div v-if="isSeries && books.length > 1" class="ev-books">
        <div class="ev-books-head">Include books</div>
        <label v-for="b in books" :key="b.id" class="ev-check">
          <input
            type="checkbox"
            :checked="form.selected.has(b.id)"
            @change="toggleBook(b.id)"
          />
          {{ b.title }}
        </label>
      </div>

      <button
        class="btn primary ev-export"
        :disabled="exporting || form.selected.size === 0"
        @click="doExport"
      >
        {{ exporting ? 'Exporting…' : form.format === 'epub' ? 'Export EPUB…' : 'Export HTML…' }}
      </button>
      <p class="ev-msg">{{ message }}</p>
      <p v-if="form.format === 'html'" class="ev-hint">
        Open the exported HTML in a browser and choose <em>Print → Save as PDF</em> to produce a
        PDF in your chosen theme.
      </p>
    </div>

    <div class="ev-preview">
      <div class="ev-preview-label">Preview</div>
      <div v-if="previewError" class="ev-preview-error">{{ previewError }}</div>
      <iframe
        v-else
        class="ev-frame"
        :srcdoc="previewHtml"
        sandbox=""
        title="Export preview"
      />
    </div>
  </div>
</template>

<style scoped>
.export-view {
  height: 100%;
  display: flex;
  min-height: 0;
}
.ev-panel {
  width: 320px;
  min-width: 320px;
  border-right: 1px solid var(--nv-border);
  padding: 20px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ev-panel h2 {
  margin: 0 0 4px;
  font-size: 18px;
}
.ev-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--nv-muted);
}
.ev-check {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--nv-text);
  cursor: pointer;
}
.ev-books {
  border-top: 1px solid var(--nv-border);
  padding-top: 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ev-books-head {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--nv-faint);
}
.ev-export {
  margin-top: 8px;
}
.ev-msg {
  font-size: 12px;
  color: var(--nv-muted);
  margin: 0;
  word-break: break-all;
}
.ev-hint {
  font-size: 11px;
  color: var(--nv-faint);
  margin: 0;
}
.ev-preview {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: #6b6f76;
}
.ev-preview-label {
  padding: 6px 12px;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: rgba(255, 255, 255, 0.7);
  background: rgba(0, 0, 0, 0.25);
}
.ev-preview-error {
  padding: 16px;
  color: var(--nv-error);
  background: var(--nv-bg);
}
.ev-frame {
  flex: 1;
  border: 0;
  width: 100%;
}
</style>
