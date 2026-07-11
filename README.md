# NovelIDE

An IDE for creative writing. Manuscripts are Markdown, world knowledge lives in
a **Codex**, and a consistency engine flags things like a dead character
walking into a scene three chapters after their funeral.

Built with [Wails](https://wails.io) (Go backend) and Vue 3 + CodeMirror 6.

## Features

- **Workspaces for novels and series.** A workspace is a plain folder: one book
  for a standalone novel, many books for a series. Adding a second book
  promotes a novel to a series automatically.
- **Schema-driven Codex.** Entry types are defined per workspace in
  `codex-schema.yaml` (editable in-app under ⚙ *Edit types & relations*). The
  starter set is character/location/item/faction/concept, but a Dune-scale
  project can add `planet`, `great-house`, `bloodline`, `order` — anything.
  Series-level entries are shared across every book; book-local entries live
  inside a single book.
- **Relationships.** Typed, directional edges between entries with proper
  inverse semantics (`parent of` ↔ `child of`, symmetric `married to`) and
  optional story-time bounds — "serves Aria *until* book 1, chapter 12" and
  "allied with House Harkonnen *from* book 2" are first-class facts.
  Relationship types are schema-defined too. Both directions show up
  everywhere: hover cards list them, and each codex entry shows a
  "Referenced by" section with incoming edges.
- **Live entity highlighting.** As you write, mentions of any Codex entity
  (by name or alias, longest match wins, word-boundary and case aware) are
  highlighted. Hover one for a card with the entry's summary, facts, aliases,
  and any consistency notes. Click through to open the full entry.
- **Consistency detection.** Codex entries carry a *status timeline*
  ("alive from the start; dead as of book 1, chapter 12"). The rule engine
  orders all chapters across all books into one story timeline and flags:
  - `dead-entity-agency` (error): a dead/destroyed entity *acting* —
    "Aria walked", "said Aria" — after their death point.
  - `dead-entity-mention` (info): a mention after death, which is usually
    fine (memories, references) but shown for awareness.
  Detection runs locally on every keystroke (debounced) using real NLP —
  part-of-speech tagging via the pure-Go
  [tsawler/prose v3](https://github.com/tsawler/prose) library (the
  maintained fork of the archived jdkato/prose), fully offline, no AI APIs. Grammar decides what counts as acting:
  any tagged verb after a dead character's name flags ("Aria sauntered",
  "Aria conjured", "Aria was walking"), while possessives ("Aria's sword"),
  passives ("Aria was carried out"), perfect-tense recollection ("Aria had
  walked these halls"), modals, and copula descriptions ("Aria was dead")
  stay informational. The architecture is still hybrid-ready if an LLM
  "deep check" pass is ever wanted.
- **Codex-gap suggestions.** The inverse of consistency checking: when the
  manuscript states a fact the Codex doesn't record — "Kael killed Aria",
  "Aria was dead", "Bill married Fleur" — a 💡 suggestion appears in the
  Consistency panel with one-click **Add to Codex** (anchored to the current
  book/chapter) or **Dismiss**. Lemma- and POS-aware: verb forms conjugate
  freely, passive voice reverses the killer ("was killed by"), negations
  ("nearly died") are skipped, noun homographs don't fire ("Kael's love for
  Aria" is not a `loves` edge), and recorded facts are never re-suggested.
- **New-entity detection.** Named-entity recognition spots proper names that
  appear repeatedly in a chapter but have no Codex entry, and offers to
  create one prefilled — introduce "John Carter" twice and the Codex asks
  about him.
- **Character-feature extraction.** A pattern engine over the POS/NER layer
  (plus the [golem](https://github.com/aaaton/golem) dictionary lemmatizer)
  mines prose for facts about characters:
  - *Appearance*: "Aria's hair was copper-red", "her green eyes", "with
    silver hair", "Kael was tall / 31 years old" → suggested `fields`.
    If the Codex already records a **different** value, the passage gets a
    contradiction warning instead — "her brown eyes" lights up when the
    Codex says green.
  - *Gender*: pronoun statistics ("referred to as she/her 7 times") and
    honorifics (Lord/Lady/Mr/Miss) suggest a gender field, with the evidence
    shown; mixed evidence abstains.
  - *Kinship & roles*: "Kael was Aria's brother", "Aria's mother, Mira",
    "X was Y's mentor/servant/rival" → typed relationship suggestions with
    correct direction (son/daughter reverse `parent-of`).
  - *Ownership & membership*: "wielded/wore/bore the Ashblade" → `owns`;
    "joined the Cinder Guard" → `member-of`.
  - *Aliases*: appositive epithets ("Aria, the Ash Queen, …", "known as …")
    become alias suggestions, which immediately improve mention matching.
  Pronouns resolve by recency with a strict ambiguity guard: "her" only
  counts when exactly one character is in scope, so crowded scenes abstain
  instead of guessing.
- **Optional deep tier (Cybertron).** Off by default; enabled in Settings
  (⚙ in the sidebar footer). Runs a Hugging Face transformer NER model
  locally via [Cybertron](https://github.com/nlpodyssey/cybertron) — pure Go
  CPU inference, no accounts or API calls. Adds a **🔬 Deep scan** button to
  the Consistency panel for on-demand, higher-recall entity detection; it
  never runs on keystrokes. The first scan downloads the model (several
  hundred MB) to the configured models directory. The model is swappable in
  Settings (CoNLL, OntoNotes, or multilingual variants).
- **Projects.** The welcome screen lists recent workspaces; the sidebar
  footer shows the open project with one-click **⇄ switch project**.
  Settings persist app-wide in the OS config directory
  (`~/.config/novelide/settings.json` on Linux).
- **Story planning.** Every book has a Plan view (📋 on the book in the
  sidebar): a card per chapter with synopsis, status
  (outlined → drafted → revised → final), POV, location, in-world time, and
  plot-thread tags. Cards fill themselves in from the manuscript — who
  actually appears (via the entity matcher) and live word counts — and run
  plan-vs-manuscript checks: a dead POV character, a POV who never appears
  in the chapter text, or a tagged thread none of whose entities show up.
  Plot threads are just codex entries of the **Arc / Thread** type, so they
  get relations, hover cards, and story-time bounds like everything else.
  Chapters can be reordered from the plan (↑/↓) — files are renumbered and
  every codex anchor and plan entry pointing at a renamed chapter is
  rewritten automatically. A "sort by story time" toggle orders cards by
  the in-world `when` field (ISO-ish values like `3127-04` sort correctly).
  Plans live in `books/<id>/plan.yaml` — plain YAML like everything else.
- **Series planning.** One level up (📋 *Series plan* at the top of the
  Manuscript tree): a series synopsis, a card per book with premise, status,
  live word count against an optional target (with progress bar), and a
  rolled-up chapter-status summary. A **thread matrix** shows every arc
  against every book — ● planned, ◐ actually tagged on chapters, ⬤ both —
  so a thread that silently drops out of a book is visible at a glance.
  Books can be reordered (↑/↓); order lives in `novelide.yaml`, so no files
  move, but the story timeline (and all death/relation logic) follows.
  Stored in `series-plan.yaml` at the workspace root.
- **Plain-file storage.** Everything is Markdown + YAML — git-friendly,
  greppable, portable. No database.

## Workspace layout

```
my-series/
  novelide.yaml               # name, kind (novel|series), book order
  codex-schema.yaml           # entry types + relationship types (user-defined)
  codex/                      # shared across the series
    character/aria-voss.yaml  # one directory per schema type id
    location/emberfall.yaml
  books/
    01-the-ember-crown/
      book.yaml               # title
      codex/                  # optional book-local entries
      manuscript/
        01-chapter-one.md     # chapters ordered by filename
```

A codex entry:

```yaml
id: aria-voss
name: Aria Voss
type: character
aliases: [Aria, the Ember Witch]
summary: Fire mage of Emberfall.
fields:
  age: "27"
status:
  - state: alive
  - state: dead
    at: { book: 01-the-ember-crown, chapter: 02-the-battle-of-cinders.md }
    note: Sacrifices herself to seal the Ash Gate.
relations:
  - type: leads
    to: the-cinder-guard
    until: { book: 01-the-ember-crown, chapter: 02-the-battle-of-cinders.md }
  - type: owns
    to: the-ember-crown
```

The schema (excerpt):

```yaml
types:
  - id: character
    label: Character
    icon: 👤
    fields: [age, appearance, goal, flaw]   # suggested fact keys
relations:
  - id: parent-of
    label: parent of
    inverseLabel: child of        # how the target side reads
  - id: married-to
    label: married to
    symmetric: true
```

## Development

Prerequisites (Linux):

```sh
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Run in dev mode (hot reload). Ubuntu ships WebKit2GTK 4.1, so pass the tag:

```sh
wails dev -tags webkit2_41
```

Build a release binary:

```sh
wails build -tags webkit2_41    # output in build/bin/novelide
```

Run the backend tests:

```sh
go test ./internal/...
```

Package as a flatpak (builds the app, then the flatpak, and installs it for
the current user; add `--bundle` to also produce a shareable `.flatpak`):

```sh
./build/flatpak/build-flatpak.sh
flatpak run dev.kilgore.NovelIDE
```

The manifest (`build/flatpak/dev.kilgore.NovelIDE.yml`) builds the app
**from source inside the sandbox** — the same process Flathub requires: the
Go compiler comes from the `org.freedesktop.Sdk.Extension.golang` SDK
extension, Go modules from `vendor/`, and the frontend from a pre-built
`frontend/dist` (both prepared by the script), so no network is needed
during the sandboxed build. The GNOME 50 runtime provides GTK3 and
WebKitGTK 4.1; en_US/en_GB hunspell dictionaries are bundled since the
sandbox can't see the host's. Sandbox permissions: home access (workspaces
are plain folders), display/GPU, and network (used only by the optional
deep-NLP model download).

**Releases.** Tag a release (`vX.Y.Z`) and the release workflow
(`.github/workflows/release.yml`) builds every platform on its own native
runner and attaches all archives to one GitHub release:

| Platform | Archive | Notes |
| --- | --- | --- |
| Linux | `novelide-<v>-linux-amd64.tar.gz` | needs GTK3 + WebKitGTK 4.1 at runtime |
| Windows | `novelide-<v>-windows-amd64.zip` | `.exe`; WebView2 bootstrapped on first run |
| macOS | `novelide-<v>-macos-universal.zip` | `.app` bundle, Intel + Apple Silicon |
| Flatpak | `novelide-<v>.flatpak` | self-contained; `flatpak install` it |
| Flathub | `novelide-<v>-vendored.tar.xz` + `.sha256` | vendored source for the Flathub manifest |

The Windows and macOS binaries are **unsigned**, so SmartScreen / Gatekeeper
will warn on first launch (right-click → Open on macOS; "More info → Run
anyway" on Windows). Code signing needs paid certificates and is left as a
follow-up. Note also that the built-in hunspell spellchecker is Linux-only
(the `spell` package falls back to a no-op stub elsewhere); Windows and
macOS rely on their native webview spellcheck.

**Publishing to Flathub:** the release also carries a ready `.flatpak`
bundle and the vendored source tarball. Fill the tarball URL + sha256 into
`build/flatpak/flathub/dev.kilgore.NovelIDE.yml` (and the repo URL in the
metainfo) and submit that manifest per
https://docs.flathub.org/docs/for-app-authors/submission. CI
(`.github/workflows/ci.yml`) runs tests plus the exact vendored build the
Flathub sandbox performs, so a green build means a buildable submission.

Try it with the demo project: open `examples/demo-series` from the welcome
screen. Book 2's chapter "What Remains" contains a deliberate contradiction —
Aria acts after her death in book 1 — which shows up in the Consistency panel
and as a wavy underline in the editor.

For a large-scale example, open `examples/saltglass-chronicles`: an original
seven-book fantasy saga (the Saltglass Chronicles) with ~215 codex entries
across the full schema —
characters, cantos (spells), draughts (potions), objects & relics,
creatures, locations, orders & factions, plot-thread arcs, and concepts.
It's built to stress every feature: deaths are anchored to specific books
and deliberately spread across the series (a godfather in book 3, a champion
in book 4, a deputy head in book 5, the headmaster in book 6, and the rest
in the final siege), so hover cards and the consistency engine stay
spoiler-accurate per book; soul-vessel "reliquaries" carry destruction
timelines; and the relationship web includes time-bounded arcs like a
double-agent's hidden loyalties and a family member's estrangement and
return (modeled as free-form status states). Start a chapter in any book and
mention a character to see book-aware hover cards. Two books ship a short
manuscript chapter (books 5 and 7) deliberately seeded with continuity
mistakes so the detection features light up on open — a character who died
two books earlier shown acting (a red consistency error), an appearance that
contradicts the codex (a warning), plus facts the codex doesn't record yet
(💡 suggestions: a death, a killing, a marriage, a kinship tie, and a repeated
name with no entry) and a couple of misspellings. `internal/detect`'s
`TestSeededManuscriptMistakes` asserts every one of those is actually flagged.
The whole world — names, terminology, incantations, plot — is original; the
generator that builds it lives at `examples/saltglass-chronicles/generate.py`.

## Architecture

- `internal/model` — domain types and the on-disk format contract.
- `internal/workspace` — load/create workspaces, codex CRUD, chapter IO.
- `internal/match` — entity mention scanner (rune-offset spans).
- `internal/detect` — story timeline + rule engine producing flags.
- `app.go` — Wails bindings exposing the above to the frontend.
- `frontend/src/editor/entityPlugin.ts` — CodeMirror extension: decorations,
  hover cards, rune→UTF-16 offset mapping.
- `frontend/src/components` — welcome screen, IDE shell, sidebar trees,
  chapter editor, codex editor.
