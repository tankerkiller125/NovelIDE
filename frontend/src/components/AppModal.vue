<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { modal, resolveModal } from '../store'

const input = ref<HTMLInputElement | null>(null)

watch(
  () => modal.kind,
  async (k) => {
    if (k === 'prompt') {
      await nextTick()
      input.value?.focus()
      input.value?.select()
    }
  },
)

function ok() {
  if (modal.kind === 'prompt') {
    const v = modal.value.trim()
    resolveModal(v || null)
  } else {
    resolveModal(true)
  }
}
function cancel() {
  resolveModal(modal.kind === 'prompt' ? null : false)
}
</script>

<template>
  <div v-if="modal.kind" class="modal-overlay" @mousedown.self="cancel" @keydown.esc="cancel">
    <div class="modal">
      <h3 class="modal-title">{{ modal.title }}</h3>
      <template v-if="modal.kind === 'prompt'">
        <label v-if="modal.label" class="modal-label">{{ modal.label }}</label>
        <input
          ref="input"
          v-model="modal.value"
          class="modal-input"
          :placeholder="modal.placeholder"
          @keyup.enter="ok"
          @keyup.esc="cancel"
        />
      </template>
      <p v-else class="modal-message">{{ modal.message }}</p>
      <div class="modal-actions">
        <button class="btn" @click="cancel">Cancel</button>
        <button class="btn" :class="modal.danger ? 'danger-solid' : 'primary'" @click="ok">
          {{ modal.confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.modal {
  width: 380px;
  max-width: 90vw;
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 10px;
  padding: 18px 20px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.55);
}
.modal-title {
  margin: 0 0 12px;
  font-size: 15px;
}
.modal-label {
  display: block;
  font-size: 12px;
  color: var(--nv-muted);
  margin-bottom: 4px;
}
.modal-input {
  width: 100%;
}
.modal-message {
  font-size: 13px;
  color: var(--nv-text);
  margin: 0 0 4px;
  line-height: 1.5;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 18px;
}
.btn.danger-solid {
  background: var(--nv-error);
  border-color: var(--nv-error);
  color: #fff;
  font-weight: 600;
}
</style>
