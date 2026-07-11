# NovelIDE

**An IDE for people who write novels.**

You already keep the whole story in your head — every character, every death,
every promise made in book one that has to pay off in book five. NovelIDE keeps
it *with* you. Write in a clean, distraction-light editor while a living
**Codex** of your world sits beside your prose, and a consistency engine quietly
watches for the mistakes that hide inside a 400-page manuscript: the character
who dies in chapter twelve and strolls into a scene in chapter thirty, the eyes
that were green in book one and brown in book three, the plot thread that
silently disappears for nine chapters.

Local-first. Plain files you own. **No cloud, no account, and no AI writing your
book for you.**

> _Desktop app for Linux, Windows, and macOS. Free and offline._

---

## Why NovelIDE

Your prose lives in one app and your worldbuilding lives in another — a wiki, a
spreadsheet, a folder of notes — and the two drift apart the moment you start
actually writing. Continuity errors stay invisible until a reader or an editor
finds them. Series make it exponentially worse: shared characters, shifting
allegiances, deaths that have to *stay* dead across thousands of pages.

NovelIDE closes the gap. Your worldbuilding and your manuscript are one project,
checked against each other as you type.

## What it does

### 📖 A Codex that actually knows your world
Characters, locations, items, factions, creatures, magic systems — whatever your
world is made of. **You define the entry types**, so a cozy mystery and an
eight-book space opera each get a Codex shaped to fit. Give entries portraits and
maps, and connect them with rich, directional relationships that change over
time — _allied with a rival house until book three, sworn enemies after_. Hover
any name in your draft to see its card, image and all, without ever leaving the
page.

### 🕸 See the whole web
Open the relationship graph and your entire cast lights up as a living map —
who's connected to whom, colored by type, filterable, and clickable straight
through to any entry. Hover a character to spotlight their corner of the story.

### 🔎 Catches continuity mistakes as you write
Give a character a life-timeline — alive, then dead as of book two, chapter nine
— and NovelIDE understands it. Have them *do* something after they've died and it
flags the line. State a fact your Codex contradicts and it warns you. It reads
grammar, not keywords: "her sword lay on the table" is fine; "she drew her sword"
is not.

### 💡 Learns your world from your own prose
Introduce a character you never catalogued and NovelIDE notices. Write "Kael
killed Aria" and it offers to record the death *and* who dealt it. Describe
someone's copper-red hair and it offers to save the detail — or, if your Codex
already says black, flags the contradiction. One click to accept, and your story
bible writes itself while you draft.

### 🧭 Plan at every altitude
Outline the whole series, each book, and every chapter. Track plot threads across
an entire saga in a matrix that shows exactly where a storyline goes quiet. Watch
word counts climb toward targets. Reorder chapters and every reference updates
itself.

### 📚 Built for series, not just single books
Share one Codex across an entire series. Reorder the books and the whole
timeline — every death, every relationship — re-checks itself against the new
reading order.

### 📦 Compile to a finished book
When the draft is done, stitch your chapters into a real book: a valid **EPUB**
for e-readers, or **print-ready HTML** you can Print → Save as PDF. Pick a theme —
double-spaced standard manuscript format for submissions, a classic serif
paperback, a clean modern look — and preview it live before you export. Export
the whole series or just the books you choose.

### ✍️ Stay in flow
A distraction-free **focus mode** (Ctrl+Shift+F) hides the sidebar and the
consistency panel so it's just you and the page. Leave yourself **editorial
notes** right in the manuscript — select a passage and jot a note (_"check the
timeline", "POV slip?"_); it's highlighted as you write and quietly stripped from
every export, so your reminders never reach the finished book. A quiet status bar
keeps score — total words, words this session, and your progress toward a **daily
goal** with a writing streak. Manage your manuscript naturally: right-click any
book or chapter to rename or delete it, and NovelIDE keeps every codex timeline
anchor and plan reference pointing at the right place.

### 💾 Your files, forever
Everything is plain **Markdown and YAML** in a folder you control — git-friendly,
greppable, and readable in any text editor on earth. NovelIDE is a lens on your
work, not a vault that traps it.

```
my-series/
  novelide.yaml            # your series
  codex/character/aria-voss.yaml
  codex/location/emberfall.yaml
  books/01-the-ember-crown/
    manuscript/01-chapter-one.md
```

## Private by design

Your manuscript never leaves your computer. No accounts, no servers, no
telemetry — nothing about your writing is ever uploaded. NovelIDE runs fully
offline: on a plane, in a cabin, anywhere.

## 🤖 AI use — please read

**NovelIDE is not an AI writing tool.** It does not generate prose, autocomplete
your sentences, or "co-write" anything. Every word in your manuscript is yours.

- **Your writing stays on your machine.** The consistency engine, entity
  detection, and suggestions run locally using classical natural-language
  processing — part-of-speech tagging, lemmatization, and named-entity
  recognition — *not* a large language model and *not* a cloud service. Nothing
  you write is transmitted anywhere or used to train anything, ever.
- **One optional feature, clearly labeled.** NovelIDE includes an optional
  "Deep scan" that uses a transformer-based model to catch names you may have
  missed. It is **off by default**, must be switched on in Settings, and even
  when enabled runs entirely on your own computer (the model downloads once, then
  works offline). It never sends your manuscript to any external service, and it
  only *finds* entities — it never writes.
- **How NovelIDE itself was made.** This application was developed with
  substantial help from an AI coding assistant, and its bundled example world
  (the "Saltglass Chronicles") is an original work generated for demonstration.
  The code is covered by an automated test suite; as with any software, review it
  before trusting it with important work.

## Download

Grab the latest build for your platform from the **Releases** page:

| Platform | Download | Notes |
| --- | --- | --- |
| **Linux** | `.tar.gz` | needs GTK 3 + WebKitGTK 4.1 installed |
| **Windows** | `.zip` (`.exe`) | WebView2 is fetched on first run |
| **macOS** | `.zip` (`.app`, universal) | Intel + Apple Silicon in one build |
| **Flatpak** | `.flatpak` | `flatpak install novelide-<version>.flatpak` |

The Windows and macOS builds are currently **unsigned**, so on first launch
you'll see a SmartScreen / Gatekeeper warning — right-click → **Open** on macOS,
or **More info → Run anyway** on Windows.

## Try it in five minutes

Open one of the bundled example projects from the welcome screen:

- **`examples/demo-series`** — a tiny two-book series with a deliberate mistake:
  a character acts after she's died. Watch the Consistency panel light up.
- **`examples/saltglass-chronicles`** — an original seven-book fantasy saga with
  ~215 Codex entries (characters, spells, artifacts, creatures, factions, plot
  threads) and two seeded chapters full of planted continuity errors, so you can
  see every feature working against a real-scale world.

## Build from source

NovelIDE is a [Wails](https://wails.io) app — a Go backend with a Vue 3 +
CodeMirror 6 frontend.

```sh
# Linux prerequisites
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# run with hot reload (Ubuntu ships WebKitGTK 4.1 — pass the tag)
wails dev -tags webkit2_41

# build a release binary  ->  build/bin/novelide
wails build -tags webkit2_41

# backend tests
go test ./internal/...
```

Packaging (Flatpak bundle, native installers) and the CI/release pipeline live
under `build/` and `.github/workflows/`.

## License

NovelIDE is free software, licensed under the
[GNU Affero General Public License v3.0 or later](LICENSE) (AGPL-3.0-or-later).

You're free to use, study, share, and modify it. If you distribute a modified
version — or run one as a network service that users interact with — the AGPL
requires you to make your source changes available to those users under the same
license. Your novels, of course, are yours: the license covers NovelIDE, never
what you write with it.

© 2026 Matthew Kilgore.
