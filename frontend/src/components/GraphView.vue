<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { openTab, relationDefById, schemaTypes, state, typeDefById } from '../store'

interface GNode {
  id: string
  name: string
  type: string
  x: number
  y: number
  vx: number
  vy: number
  fixed: boolean
}
interface GEdge {
  s: string
  t: string
  label: string
}

// A distinct color per entry type, assigned by schema order.
const PALETTE = [
  '#d98f4e', '#6fa8dc', '#6fcf97', '#c58ce0', '#e06d6d',
  '#e8c893', '#5bc8c8', '#d97eae', '#9aa4b5', '#b0c25a',
]
const typeColor = computed(() => {
  const m = new Map<string, string>()
  schemaTypes.value.forEach((t, i) => m.set(t.id, PALETTE[i % PALETTE.length]))
  return m
})
const colorFor = (type: string) => typeColor.value.get(type) ?? '#9aa4b5'

const byId = computed(() => {
  const m = new Map<string, { id: string; name: string; type: string }>()
  for (const e of state.workspace?.codex ?? []) m.set(e.id, e)
  return m
})

// Which entry types are shown (toggled by the legend chips).
const enabledTypes = reactive(new Set<string>())
watch(
  schemaTypes,
  (types) => {
    if (enabledTypes.size === 0) types.forEach((t) => enabledTypes.add(t.id))
  },
  { immediate: true },
)

// All directed relationship edges, de-duplicated (symmetric relations once).
const allEdges = computed<GEdge[]>(() => {
  const out: GEdge[] = []
  const seen = new Set<string>()
  const defs = relationDefById.value
  for (const e of state.workspace?.codex ?? []) {
    for (const r of e.relations ?? []) {
      if (!byId.value.has(r.to)) continue
      const def = defs.get(r.type)
      const label = def?.label ?? r.type
      let key = `${e.id}|${r.type}|${r.to}`
      if (def?.symmetric) {
        const [a, b] = [e.id, r.to].sort()
        key = `${a}|${r.type}|${b}`
      }
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ s: e.id, t: r.to, label })
    }
  }
  return out
})

// Edges whose both endpoints are of an enabled type; nodes = their endpoints.
const visibleEdges = computed(() =>
  allEdges.value.filter(
    (e) => enabledTypes.has(byId.value.get(e.s)!.type) && enabledTypes.has(byId.value.get(e.t)!.type),
  ),
)
const visibleNodeIds = computed(() => {
  const s = new Set<string>()
  for (const e of visibleEdges.value) {
    s.add(e.s)
    s.add(e.t)
  }
  return s
})

// Types that actually participate in any relationship (for the legend).
const participatingTypes = computed(() => {
  const s = new Set<string>()
  for (const e of allEdges.value) {
    s.add(byId.value.get(e.s)!.type)
    s.add(byId.value.get(e.t)!.type)
  }
  return schemaTypes.value.filter((t) => s.has(t.id))
})

const nodes = reactive<GNode[]>([])
const edges = ref<GEdge[]>([])
const container = ref<HTMLElement | null>(null)
const size = reactive({ w: 800, h: 600 })
const hovered = ref<string | null>(null)

function rebuild() {
  const ids = visibleNodeIds.value
  const existing = new Map(nodes.map((n) => [n.id, n]))
  const cx = size.w / 2
  const cy = size.h / 2
  const next: GNode[] = []
  let i = 0
  for (const id of ids) {
    const e = byId.value.get(id)!
    const prev = existing.get(id)
    if (prev) {
      next.push(prev)
    } else {
      // seed new nodes on a ring so the sim spreads them out
      const a = (i / Math.max(1, ids.size)) * Math.PI * 2
      next.push({
        id,
        name: e.name,
        type: e.type,
        x: cx + Math.cos(a) * 180 + (Math.random() - 0.5) * 40,
        y: cy + Math.sin(a) * 180 + (Math.random() - 0.5) * 40,
        vx: 0,
        vy: 0,
        fixed: false,
      })
    }
    i++
  }
  nodes.splice(0, nodes.length, ...next)
  edges.value = visibleEdges.value
  alpha = 1
  ensureRunning()
}
watch([visibleNodeIds, () => size.w, () => size.h], rebuild)

// ---- force simulation ----
let alpha = 1
let raf = 0
function tick() {
  const n = nodes
  const cx = size.w / 2
  const cy = size.h / 2
  const REPULSE = 3000
  const SPRING = 0.02
  const REST = 70
  // repulsion (O(n^2), fine for a few hundred nodes)
  for (let i = 0; i < n.length; i++) {
    for (let j = i + 1; j < n.length; j++) {
      let dx = n[i].x - n[j].x
      let dy = n[i].y - n[j].y
      let d2 = dx * dx + dy * dy || 0.01
      const f = (REPULSE / d2) * alpha
      const d = Math.sqrt(d2)
      const fx = (dx / d) * f
      const fy = (dy / d) * f
      n[i].vx += fx
      n[i].vy += fy
      n[j].vx -= fx
      n[j].vy -= fy
    }
  }
  const idx = new Map(n.map((node, i) => [node.id, i]))
  for (const e of edges.value) {
    const a = n[idx.get(e.s)!]
    const b = n[idx.get(e.t)!]
    if (!a || !b) continue
    const dx = a.x - b.x
    const dy = a.y - b.y
    const d = Math.sqrt(dx * dx + dy * dy) || 0.01
    const f = (d - REST) * SPRING * alpha
    const fx = (dx / d) * f
    const fy = (dy / d) * f
    a.vx -= fx
    a.vy -= fy
    b.vx += fx
    b.vy += fy
  }
  for (const node of n) {
    node.vx += (cx - node.x) * 0.006 * alpha
    node.vy += (cy - node.y) * 0.006 * alpha
    if (!node.fixed) {
      node.x += node.vx
      node.y += node.vy
      node.x = Math.max(24, Math.min(size.w - 24, node.x))
      node.y = Math.max(24, Math.min(size.h - 24, node.y))
    }
    node.vx *= 0.82
    node.vy *= 0.82
  }
  alpha *= 0.985
  if (alpha > 0.02) {
    raf = requestAnimationFrame(tick)
  } else {
    raf = 0
  }
}
function ensureRunning() {
  if (!raf) raf = requestAnimationFrame(tick)
}

// neighbor set of the hovered node (for highlight)
const neighborhood = computed(() => {
  if (!hovered.value) return null
  const s = new Set<string>([hovered.value])
  for (const e of edges.value) {
    if (e.s === hovered.value) s.add(e.t)
    if (e.t === hovered.value) s.add(e.s)
  }
  return s
})
function dim(id: string) {
  return neighborhood.value ? !neighborhood.value.has(id) : false
}
function edgeDim(e: GEdge) {
  return hovered.value ? e.s !== hovered.value && e.t !== hovered.value : false
}
function nodeAt(id: string) {
  return nodes.find((n) => n.id === id)
}

// ---- interaction: drag vs click ----
let drag: { node: GNode; moved: boolean } | null = null
function toSvg(ev: PointerEvent) {
  const r = container.value!.getBoundingClientRect()
  return { x: ev.clientX - r.left, y: ev.clientY - r.top }
}
function onNodeDown(ev: PointerEvent, node: GNode) {
  ;(ev.target as Element).setPointerCapture(ev.pointerId)
  node.fixed = true
  drag = { node, moved: false }
}
function onMove(ev: PointerEvent) {
  if (!drag) return
  const p = toSvg(ev)
  drag.node.x = p.x
  drag.node.y = p.y
  drag.moved = true
  alpha = Math.max(alpha, 0.3)
  ensureRunning()
}
function onUp(node: GNode) {
  if (drag && !drag.moved) openTab({ kind: 'codex', entryId: node.id })
  if (drag) drag.node.fixed = false
  drag = null
}

let ro: ResizeObserver | null = null
onMounted(() => {
  if (container.value) {
    const measure = () => {
      const r = container.value!.getBoundingClientRect()
      size.w = Math.max(320, r.width)
      size.h = Math.max(240, r.height)
    }
    measure()
    ro = new ResizeObserver(measure)
    ro.observe(container.value)
    rebuild()
  }
})
onUnmounted(() => {
  if (raf) cancelAnimationFrame(raf)
  ro?.disconnect()
})

function toggleType(id: string) {
  if (enabledTypes.has(id)) enabledTypes.delete(id)
  else enabledTypes.add(id)
}
</script>

<template>
  <div class="graph-view">
    <div class="gv-toolbar">
      <h2>Relationship graph</h2>
      <span class="gv-count">{{ nodes.length }} entries · {{ edges.length }} links</span>
      <div class="gv-legend">
        <button
          v-for="t in participatingTypes"
          :key="t.id"
          class="gv-chip"
          :class="{ off: !enabledTypes.has(t.id) }"
          @click="toggleType(t.id)"
        >
          <span class="gv-dot" :style="{ background: colorFor(t.id) }" />
          {{ t.label || t.id }}
        </button>
      </div>
    </div>

    <div ref="container" class="gv-canvas">
      <div v-if="!nodes.length" class="gv-empty">
        No relationships to show. Add relationships to codex entries, or enable more
        types above.
      </div>
      <svg
        v-else
        :viewBox="`0 0 ${size.w} ${size.h}`"
        class="gv-svg"
        @pointermove="onMove"
      >
        <g class="gv-edges">
          <line
            v-for="(e, i) in edges"
            :key="i"
            :x1="nodeAt(e.s)?.x"
            :y1="nodeAt(e.s)?.y"
            :x2="nodeAt(e.t)?.x"
            :y2="nodeAt(e.t)?.y"
            class="gv-edge"
            :class="{ dim: edgeDim(e), lit: !edgeDim(e) && hovered }"
          />
        </g>
        <!-- edge labels only for the hovered node's links, to avoid clutter -->
        <g v-if="hovered" class="gv-edge-labels">
          <text
            v-for="(e, i) in edges.filter((e) => e.s === hovered || e.t === hovered)"
            :key="i"
            :x="((nodeAt(e.s)?.x ?? 0) + (nodeAt(e.t)?.x ?? 0)) / 2"
            :y="((nodeAt(e.s)?.y ?? 0) + (nodeAt(e.t)?.y ?? 0)) / 2"
            class="gv-edge-label"
          >
            {{ e.label }}
          </text>
        </g>
        <g class="gv-nodes">
          <g
            v-for="n in nodes"
            :key="n.id"
            :transform="`translate(${n.x},${n.y})`"
            class="gv-node"
            :class="{ dim: dim(n.id) }"
            @pointerdown="onNodeDown($event, n)"
            @pointerup="onUp(n)"
            @pointerenter="hovered = n.id"
            @pointerleave="hovered = null"
          >
            <circle :r="hovered === n.id ? 9 : 6" :fill="colorFor(n.type)" class="gv-circle" />
            <text class="gv-label" x="10" y="4">{{ n.name }}</text>
          </g>
        </g>
      </svg>
    </div>
  </div>
</template>

<style scoped>
.graph-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.gv-toolbar {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 18px;
  border-bottom: 1px solid var(--nv-border);
  flex-wrap: wrap;
}
.gv-toolbar h2 {
  margin: 0;
  font-size: 18px;
}
.gv-count {
  font-size: 12px;
  color: var(--nv-faint);
}
.gv-legend {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  margin-left: auto;
}
.gv-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--nv-hover);
  border: 1px solid var(--nv-border);
  border-radius: 12px;
  color: var(--nv-text);
  padding: 2px 10px;
  font: inherit;
  font-size: 11.5px;
  cursor: pointer;
}
.gv-chip.off {
  opacity: 0.4;
}
.gv-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
}
.gv-canvas {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: hidden;
  background: var(--nv-bg);
}
.gv-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--nv-faint);
  padding: 24px;
  text-align: center;
}
.gv-svg {
  width: 100%;
  height: 100%;
  display: block;
  touch-action: none;
}
.gv-edge {
  stroke: var(--nv-border);
  stroke-width: 1;
}
.gv-edge.dim {
  stroke-opacity: 0.15;
}
.gv-edge.lit {
  stroke: var(--nv-accent);
  stroke-opacity: 0.7;
}
.gv-edge-label {
  fill: var(--nv-muted);
  font-size: 10px;
  text-anchor: middle;
  paint-order: stroke;
  stroke: var(--nv-bg);
  stroke-width: 3px;
}
.gv-node {
  cursor: pointer;
}
.gv-node.dim {
  opacity: 0.25;
}
.gv-circle {
  stroke: var(--nv-bg);
  stroke-width: 1.5;
}
.gv-label {
  fill: var(--nv-text);
  font-size: 11px;
  paint-order: stroke;
  stroke: var(--nv-bg);
  stroke-width: 3px;
  pointer-events: none;
}
</style>
