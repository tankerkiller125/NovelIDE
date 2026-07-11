<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { SaveSchema } from '../api'
import { pinActiveTab, setWorkspace, state } from '../store'
import type { RelationDef, TypeDef } from '../types'

const form = reactive({
  types: [] as TypeDef[],
  relations: [] as RelationDef[],
})
const saving = ref(false)
const message = ref('')

function load() {
  const s = state.workspace?.schema
  form.types = (s?.types ?? []).map((t) => ({ ...t, fields: [...(t.fields ?? [])] }))
  form.relations = (s?.relations ?? []).map((r) => ({ ...r }))
}
watch(() => state.workspace?.schema, load, { immediate: true })

function addType() {
  form.types.push({ id: '', label: '', icon: '✦', fields: [] })
}
function addRelation() {
  form.relations.push({ id: '', label: '', inverseLabel: '', symmetric: false })
}

function fieldsText(t: TypeDef): string {
  return (t.fields ?? []).join(', ')
}
function setFieldsText(t: TypeDef, v: string) {
  t.fields = v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

async function save() {
  const bad = form.types.find((t) => !t.label.trim() && !t.id.trim())
  if (bad) {
    message.value = 'Every type needs at least a label.'
    return
  }
  saving.value = true
  message.value = ''
  try {
    const ws = await SaveSchema({
      types: form.types.filter((t) => t.label.trim() || t.id.trim()),
      relations: form.relations.filter((r) => r.label.trim() || r.id.trim()),
    })
    setWorkspace(ws)
    message.value = 'Saved. New types and relations are available everywhere.'
  } catch (e) {
    message.value = `Save failed: ${e}`
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="schema-editor" @input="pinActiveTab" @change="pinActiveTab">
    <div class="se-toolbar">
      <h2>Codex Schema</h2>
      <span class="se-msg">{{ message }}</span>
      <button class="btn primary" :disabled="saving" @click="save">Save schema</button>
    </div>
    <p class="hint">
      This defines what your world is made of. Building Dune? Add <em>planet</em>,
      <em>great house</em>, and <em>bloodline</em> types, then relations like
      <em>heir of</em>. Stored as <code>codex-schema.yaml</code> in the workspace — also
      hand-editable.
    </p>

    <section>
      <div class="se-sect-head">
        <h3>Entry types</h3>
        <button class="btn" @click="addType">+ Add type</button>
      </div>
      <div class="se-head-row">
        <span class="se-icon-col">Icon</span>
        <span class="se-label-col">Label</span>
        <span class="se-id-col">ID (folder name)</span>
        <span class="se-flex">Suggested fact keys (comma-separated)</span>
      </div>
      <div v-for="(t, i) in form.types" :key="i" class="se-row">
        <input v-model="t.icon" class="se-icon-col" />
        <input v-model="t.label" class="se-label-col" placeholder="Great House" />
        <input v-model="t.id" class="se-id-col" :placeholder="'auto from label'" />
        <input
          class="se-flex"
          :value="fieldsText(t)"
          placeholder="words, motto, homeworld"
          @change="setFieldsText(t, ($event.target as HTMLInputElement).value)"
        />
        <button class="btn icon" @click="form.types.splice(i, 1)">✕</button>
      </div>
    </section>

    <section>
      <div class="se-sect-head">
        <h3>Relationship types</h3>
        <button class="btn" @click="addRelation">+ Add relation</button>
      </div>
      <div class="se-head-row">
        <span class="se-label-col">Label (A → B)</span>
        <span class="se-label-col">Inverse label (B → A)</span>
        <span class="se-id-col">ID</span>
        <span class="se-sym">Symmetric</span>
      </div>
      <div v-for="(r, i) in form.relations" :key="i" class="se-row">
        <input v-model="r.label" class="se-label-col" placeholder="heir of" />
        <input
          v-model="r.inverseLabel"
          class="se-label-col"
          :placeholder="r.symmetric ? '(same both ways)' : 'names as heir'"
          :disabled="r.symmetric"
        />
        <input v-model="r.id" class="se-id-col" placeholder="auto from label" />
        <label class="se-sym"><input type="checkbox" v-model="r.symmetric" /></label>
        <button class="btn icon" @click="form.relations.splice(i, 1)">✕</button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.schema-editor {
  height: 100%;
  overflow-y: auto;
  padding: 20px 28px 60px;
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
.se-sect-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.se-sect-head h3 {
  margin: 0;
  font-size: 14px;
}
.se-head-row {
  display: flex;
  gap: 8px;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--nv-faint);
  margin-bottom: 4px;
  padding-right: 40px;
}
.se-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
  align-items: center;
}
.se-icon-col {
  width: 52px;
  text-align: center;
}
.se-label-col {
  width: 200px;
}
.se-id-col {
  width: 170px;
}
.se-flex {
  flex: 1;
}
.se-sym {
  width: 70px;
  display: flex;
  justify-content: center;
}
.hint {
  color: var(--nv-faint);
  font-size: 12px;
  margin: 0 0 10px;
}
code {
  background: var(--nv-hover);
  padding: 1px 5px;
  border-radius: 4px;
}
</style>
