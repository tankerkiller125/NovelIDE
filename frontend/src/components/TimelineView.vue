<script setup lang="ts">
import { computed, ref } from 'vue'
import { codexById, openTab, relationDefById, state, storyPointLabel } from '../store'
import type { StoryPoint } from '../types'

// The timeline shows story *changes* — status transitions (a death, a
// destruction) and relationships that begin or end at a point in the story —
// laid over the book→chapter spine. Baseline facts ("alive from the start")
// are conditions, not events, so they're left off to keep the spine readable.

interface TLEvent {
  icon: string
  entryId: string
  text: string
  tag?: string // "begins" / "ends"
  note?: string
}
interface TLChapter {
  chapter: string
  title: string
  when: string
  synopsis: string
  events: TLEvent[]
}
interface TLBook {
  id: string
  title: string
  startEvents: TLEvent[]
  chapters: TLChapter[]
}

const chronology = ref(false)

const books = computed(() => state.workspace?.books ?? [])
const nameOf = (id: string) => codexById.value.get(id)?.name ?? id

function statusIcon(stateName: string): string {
  const s = stateName.toLowerCase()
  if (/(dead|deceased|killed|slain|died)/.test(s)) return '💀'
  if (/(destroyed|ruined|shattered)/.test(s)) return '💥'
  if (/(missing|lost|vanished|disappeared)/.test(s)) return '❓'
  if (/(born|created|founded|forged)/.test(s)) return '✨'
  if (/(alive|restored|reborn|revived)/.test(s)) return '🌱'
  return '◆'
}

const prettyChapter = (file: string) => file.replace(/\.md$/, '').replace(/^\d+-/, '').replace(/-/g, ' ')

// Bucket every anchored event under a story-point key: "book/chapter", or
// "book/" for an event anchored to a book with no specific chapter.
const eventsByKey = computed(() => {
  const map = new Map<string, TLEvent[]>()
  const push = (p: StoryPoint, ev: TLEvent) => {
    const key = `${p.book}/${p.chapter ?? ''}`
    const arr = map.get(key) ?? []
    arr.push(ev)
    map.set(key, arr)
  }
  const defs = relationDefById.value
  for (const e of state.workspace?.codex ?? []) {
    for (const sc of e.status ?? []) {
      if (!sc.at?.book) continue // baseline condition, not an event
      push(sc.at, {
        icon: statusIcon(sc.state),
        entryId: e.id,
        text: `${e.name} — ${sc.state}`,
        note: sc.note,
      })
    }
    for (const r of e.relations ?? []) {
      const label = defs.get(r.type)?.label ?? r.type
      if (r.from?.book) {
        push(r.from, {
          icon: '🔗',
          entryId: e.id,
          text: `${e.name} ${label} ${nameOf(r.to)}`,
          tag: 'begins',
          note: r.note,
        })
      }
      if (r.until?.book) {
        push(r.until, {
          icon: '🔗',
          entryId: e.id,
          text: `${e.name} ${label} ${nameOf(r.to)}`,
          tag: 'ends',
          note: r.note,
        })
      }
    }
  }
  return map
})

const planOf = (bookId: string, chapter: string) =>
  books.value.find((b) => b.id === bookId)?.plan?.find((p) => p.file === chapter)

const timeline = computed<TLBook[]>(() => {
  return books.value.map((b) => {
    let chapters = [...(b.chapters ?? [])]
    if (chronology.value) {
      chapters.sort((x, y) => {
        const wx = planOf(b.id, x)?.when ?? ''
        const wy = planOf(b.id, y)?.when ?? ''
        if (!wx && !wy) return 0
        if (!wx) return 1
        if (!wy) return -1
        return wx.localeCompare(wy)
      })
    }
    return {
      id: b.id,
      title: b.title,
      startEvents: eventsByKey.value.get(`${b.id}/`) ?? [],
      chapters: chapters.map((ch) => {
        const plan = planOf(b.id, ch)
        return {
          chapter: ch,
          title: prettyChapter(ch),
          when: plan?.when ?? '',
          synopsis: plan?.synopsis ?? '',
          events: eventsByKey.value.get(`${b.id}/${ch}`) ?? [],
        }
      }),
    }
  })
})

const totalEvents = computed(() =>
  [...eventsByKey.value.values()].reduce((n, arr) => n + arr.length, 0),
)

// Events whose anchor no longer resolves to a real chapter/book — surfaced so
// a renamed/deleted chapter doesn't silently swallow a plotted event.
const resolvableKeys = computed(() => {
  const keys = new Set<string>()
  for (const b of books.value) {
    keys.add(`${b.id}/`)
    for (const ch of b.chapters ?? []) keys.add(`${b.id}/${ch}`)
  }
  return keys
})
const orphanCount = computed(() => {
  let n = 0
  for (const [key, arr] of eventsByKey.value) if (!resolvableKeys.value.has(key)) n += arr.length
  return n
})

function pointLabelFor(bookId: string, chapter: string) {
  return storyPointLabel({ book: bookId, chapter })
}
</script>

<template>
  <div class="tl">
    <div class="tl-toolbar">
      <h2>Story Timeline</h2>
      <span class="tl-stats">{{ totalEvents }} event(s)</span>
      <label class="tl-chrono">
        <input type="checkbox" v-model="chronology" />
        Sort by story time
      </label>
    </div>
    <p class="tl-hint">
      Deaths, destructions, and relationships that begin or end — plotted where they happen in the
      story. Anchor these on Codex entries (status timeline &amp; relationship <em>from/until</em>).
    </p>
    <p v-if="orphanCount" class="tl-warn">
      ⚠ {{ orphanCount }} event(s) are anchored to a chapter that no longer exists — open the Codex
      entry to re-anchor them.
    </p>

    <div v-for="book in timeline" :key="book.id" class="tl-book">
      <div class="tl-book-head">
        <span class="tl-book-title">{{ book.title }}</span>
      </div>
      <div v-if="book.startEvents.length" class="tl-node tl-book-start">
        <div class="tl-spine"><span class="tl-dot book" /></div>
        <div class="tl-body">
          <div class="tl-when">Book opening</div>
          <div v-for="(ev, i) in book.startEvents" :key="i" class="tl-event" @click="openTab({ kind: 'codex', entryId: ev.entryId })">
            <span class="tl-ev-icon">{{ ev.icon }}</span>
            <span class="tl-ev-text">{{ ev.text }}</span>
            <span v-if="ev.tag" class="tl-ev-tag" :class="ev.tag">{{ ev.tag }}</span>
            <span v-if="ev.note" class="tl-ev-note">— {{ ev.note }}</span>
          </div>
        </div>
      </div>

      <div v-for="ch in book.chapters" :key="ch.chapter" class="tl-node">
        <div class="tl-spine"><span class="tl-dot" :class="{ empty: !ch.events.length }" /></div>
        <div class="tl-body">
          <div class="tl-chapter-head">
            <span class="tl-chapter-title" @click="openTab({ kind: 'chapter', bookId: book.id, chapter: ch.chapter })">
              {{ ch.title }}
            </span>
            <span v-if="ch.when" class="tl-when-chip" :title="pointLabelFor(book.id, ch.chapter)">🕐 {{ ch.when }}</span>
          </div>
          <p v-if="ch.synopsis" class="tl-synopsis">{{ ch.synopsis }}</p>
          <div v-for="(ev, i) in ch.events" :key="i" class="tl-event" @click="openTab({ kind: 'codex', entryId: ev.entryId })">
            <span class="tl-ev-icon">{{ ev.icon }}</span>
            <span class="tl-ev-text">{{ ev.text }}</span>
            <span v-if="ev.tag" class="tl-ev-tag" :class="ev.tag">{{ ev.tag }}</span>
            <span v-if="ev.note" class="tl-ev-note">— {{ ev.note }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="!totalEvents" class="tl-empty">
      No plotted events yet. Add a status change (e.g. a character's death) or a time-bounded
      relationship to a Codex entry and it will appear here at its anchor point.
    </div>
  </div>
</template>

<style scoped>
.tl {
  height: 100%;
  overflow-y: auto;
  padding: 18px 24px 60px;
}
.tl-toolbar {
  display: flex;
  align-items: baseline;
  gap: 14px;
  margin-bottom: 4px;
}
.tl-toolbar h2 {
  margin: 0;
  font-size: 18px;
}
.tl-stats {
  color: var(--nv-muted);
  font-size: 12px;
}
.tl-chrono {
  margin-left: auto;
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  color: var(--nv-muted);
  cursor: pointer;
}
.tl-hint {
  font-size: 12px;
  color: var(--nv-faint);
  margin: 0 0 12px;
}
.tl-warn {
  font-size: 12px;
  color: var(--nv-warning);
  margin: 0 0 12px;
}
.tl-book {
  margin-bottom: 8px;
}
.tl-book-head {
  margin: 14px 0 8px;
}
.tl-book-title {
  font-weight: 700;
  font-size: 15px;
  text-transform: capitalize;
}
.tl-node {
  display: flex;
  gap: 12px;
}
.tl-spine {
  position: relative;
  width: 14px;
  flex-shrink: 0;
  display: flex;
  justify-content: center;
}
/* The vertical line runs through the spine column. */
.tl-spine::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: -0px;
  width: 2px;
  background: var(--nv-border);
}
.tl-node:last-child .tl-spine::before {
  bottom: auto;
  height: 14px;
}
.tl-dot {
  position: relative;
  z-index: 1;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--nv-accent);
  margin-top: 4px;
  border: 2px solid var(--nv-bg);
  box-sizing: content-box;
}
.tl-dot.empty {
  background: var(--nv-border);
}
.tl-dot.book {
  background: var(--nv-heading, var(--nv-accent));
  width: 12px;
  height: 12px;
}
.tl-body {
  flex: 1;
  padding-bottom: 14px;
  min-width: 0;
}
.tl-chapter-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.tl-chapter-title {
  font-weight: 600;
  text-transform: capitalize;
  cursor: pointer;
}
.tl-chapter-title:hover {
  color: var(--nv-accent);
}
.tl-when,
.tl-when-chip {
  font-size: 11px;
  color: var(--nv-faint);
}
.tl-when-chip {
  white-space: nowrap;
}
.tl-synopsis {
  margin: 3px 0 4px;
  font-size: 12px;
  color: var(--nv-muted);
  line-height: 1.4;
}
.tl-event {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 13px;
  padding: 3px 6px;
  border-radius: 6px;
  cursor: pointer;
}
.tl-event:hover {
  background: var(--nv-hover);
}
.tl-ev-icon {
  flex-shrink: 0;
}
.tl-ev-text {
  color: var(--nv-text);
}
.tl-ev-tag {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-radius: 6px;
  padding: 0 6px;
}
.tl-ev-tag.begins {
  background: color-mix(in srgb, #6fcf97 25%, transparent);
  color: #6fcf97;
}
.tl-ev-tag.ends {
  background: color-mix(in srgb, var(--nv-warning) 22%, transparent);
  color: var(--nv-warning);
}
.tl-ev-note {
  color: var(--nv-faint);
  font-size: 11px;
}
.tl-empty {
  color: var(--nv-muted);
  padding: 30px 0;
  text-align: center;
  font-size: 13px;
}
</style>
