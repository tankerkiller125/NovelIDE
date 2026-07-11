#!/usr/bin/env python3
"""Generate the original 'Saltglass Chronicles' example workspace.

A wholly invented magic-school / dark-antagonist saga of comparable
complexity to the (removed) Harry Potter example, exercising the same app
features: schema-driven codex, deaths anchored per book, time-bounded
relationships, plot-thread arcs. All names, terms, incantations, plot and
lore are original.
"""
import os, shutil, yaml

ROOT = os.path.dirname(os.path.abspath(__file__))

BOOKS = [
    ("01-the-marked-tide", "The Marked Tide"),
    ("02-the-sunken-warren", "The Sunken Warren"),
    ("03-the-sablefen-pact", "The Sablefen Pact"),
    ("04-the-trine-tournament", "The Trine Tournament"),
    ("05-the-drowned-court", "The Drowned Court"),
    ("06-the-reliquary", "The Reliquary"),
    ("07-the-last-canto", "The Last Canto"),
]
BOOK_IDS = {b for b, _ in BOOKS}

ENTRIES = []


def sp(book):
    assert book in BOOK_IDS, f"unknown book {book}"
    return {"book": book}


def st(state, at=None, note=None):
    d = {"state": state}
    if at:
        d["at"] = at
    if note:
        d["note"] = note
    return d


def rel(type, to, frm=None, until=None, note=None):
    d = {"type": type, "to": to}
    if frm:
        d["from"] = frm
    if until:
        d["until"] = until
    if note:
        d["note"] = note
    return d


def E(id, name, type, aliases=None, summary="", fields=None, status=None,
      relations=None, details=""):
    e = {"id": id, "name": name, "type": type}
    if aliases:
        e["aliases"] = aliases
    if summary:
        e["summary"] = summary
    if details:
        e["details"] = details
    if fields:
        e["fields"] = fields
    if status:
        e["status"] = status
    if relations:
        e["relations"] = relations
    ENTRIES.append(e)


# ===================== CHARACTERS =====================
# -- the trio --
E("wren-alcott", "Wren Alcott", "character",
  ["Wren", "the Marked", "the Tideborn"],
  "Orphaned as an infant when the Drowned King's death-canto broke against her; a Riptide weaver and the subject of the Saltmere prophecy.",
  {"order": "Riptide", "lineage": "half-woven", "focus": "storm-glass lenset", "beacon": "sea-hawk", "hair": "dark", "eyes": "grey", "born": "the year of the low tide"},
  [st("alive")],
  [rel("member-of", "riptide-order"),
   rel("leads", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("enemy-of", "corvane-mirehold"),
   rel("loves", "elsie-marsh", frm=sp("06-the-reliquary")),
   rel("owns", "the-fogmantle"),
   rel("owns", "the-wanderers-chart", frm=sp("03-the-sablefen-pact")),
   rel("owns", "the-sovereign-lens", frm=sp("07-the-last-canto"),
       note="Becomes its true master by disarming Julian Ashenford at Mirehold Keep.")])

E("perrin-marsh", "Perrin Marsh", "character", ["Perrin"],
  "Wren's steadfast friend and the middle Marsh child; a skiff-rider with more courage than caution.",
  {"order": "Riptide", "lineage": "woven", "beacon": "otter-hound"},
  [st("alive")],
  [rel("member-of", "riptide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("loves", "odile-sarkany", frm=sp("06-the-reliquary"))])

E("odile-sarkany", "Odile Sarkany", "character", ["Odile"],
  "Unwoven-born and the sharpest mind at Thornfall; the trio's planner, founder of the Deepkin Society, and the reason the others survive.",
  {"order": "Riptide", "lineage": "unwoven-born", "beacon": "heron"},
  [st("alive")],
  [rel("member-of", "riptide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("created", "deepkin-society", frm=sp("04-the-trine-tournament")),
   rel("loves", "perrin-marsh", frm=sp("06-the-reliquary")),
   rel("pet-of", "sootpaw", note="Sootpaw is her cat; the relation reads from the cat's side.")])

# -- the Marsh family (found family) --
E("edwin-marsh", "Edwin Marsh", "character", ["Mr Marsh"],
  "A Concord tide-clerk fascinated by Unwoven contraptions; warm-hearted patriarch of the Marsh family and a Grey Dawn agent.",
  {"order": "Riptide (formerly)", "lineage": "woven", "occupation": "clerk, Bureau of Unwoven Affairs"},
  [st("alive")],
  [rel("married-to", "rosalind-marsh"),
   rel("parent-of", "cade-marsh"), rel("parent-of", "perrin-marsh"),
   rel("parent-of", "tam-marsh"), rel("parent-of", "wick-marsh"),
   rel("parent-of", "elsie-marsh"),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("serves", "tidal-concord")])

E("rosalind-marsh", "Rosalind Marsh", "character", ["Mrs Marsh"],
  "Matriarch of the Marshes and Wren's surrogate mother; strikes down Vespera Locke in the siege of Thornfall.",
  {"order": "Riptide (formerly)", "lineage": "woven"},
  [st("alive")],
  [rel("married-to", "edwin-marsh"),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("killed", "vespera-locke", frm=sp("07-the-last-canto"),
       note="A duel in the tide-hall, defending her daughter.")])

E("cade-marsh", "Cade Marsh", "character", ["Cade"],
  "Eldest Marsh son, curio-dealer and champion skiff-racer; scarred by Fenris Graff and wed to Amelie Brvolar.",
  {"lineage": "woven", "occupation": "curio-dealer, the Tinker's Cove"},
  [st("alive")],
  [rel("married-to", "amelie-brvolar", frm=sp("07-the-last-canto"),
       note="Wedding at Marsh Hollow on the eve of the Concord's fall."),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("owns", "the-tinkers-cove")])

E("tam-marsh", "Tam Marsh", "character", ["Tam"],
  "Ambitious Concord careerist whose rise estranges him from the Marshes for three books before he returns for the last stand.",
  {"lineage": "woven", "occupation": "under-secretary to the High Warden", "born": "the year of the red tide"},
  [st("alive"),
   st("estranged", at=sp("05-the-drowned-court"),
      note="Rows with Edwin over his promotion and severs ties with the family."),
   st("reconciled", at=sp("07-the-last-canto"),
      note="Returns through the Grey Gull tunnel minutes before the siege and asks forgiveness.")],
  [rel("serves", "tidal-concord", frm=sp("05-the-drowned-court"))])

E("wick-marsh", "Wick Marsh", "character", ["Wick"],
  "The gentlest Marsh child, a greenwork prodigy; falls in the siege of Thornfall shielding a younger student.",
  {"order": "Millpond", "lineage": "woven"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed when a warding-wall collapsed during the siege.")],
  [rel("member-of", "millpond-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court"))])

E("elsie-marsh", "Elsie Marsh", "character", ["Elsie"],
  "Youngest Marsh and only daughter; a fierce duellist who becomes the heart Wren fights to come home to.",
  {"order": "Riptide", "lineage": "woven", "beacon": "vixen"},
  [st("alive")],
  [rel("member-of", "riptide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("loves", "wren-alcott")])

# -- mentors / staff --
E("eamon-hollis", "Eamon Hollis", "character",
  ["Headmaster Hollis", "the Warden of Thornfall"],
  "Headmaster of Thornfall and the only weaver the Drowned King ever feared; keeper of the war's deepest plans and its heaviest secret.",
  {"order": "Riptide", "lineage": "half-woven", "beacon": "pyre-heron", "occupation": "Headmaster of Thornfall Conservatory"},
  [st("alive"),
   st("dead", at=sp("06-the-reliquary"),
      note="Slain atop the Beacon Spire — arranged; a reliquary's curse was already killing him.")],
  [rel("leads", "order-of-the-grey-dawn", until=sp("06-the-reliquary")),
   rel("leads", "thornfall-conservatory", until=sp("06-the-reliquary")),
   rel("mentor-of", "wren-alcott"),
   rel("owns", "the-sovereign-lens", until=sp("06-the-reliquary"),
       note="Won from Halbrecht Mourn long ago; mastery passes to Julian Ashenford, who disarms him."),
   rel("sibling-of", "gideon-hollis"),
   rel("enemy-of", "corvane-mirehold")])

E("gideon-hollis", "Gideon Hollis", "character", ["Gideon"],
  "Eamon's estranged brother and keeper of the Grey Gull; his hidden tunnel turns the tide before the final siege.",
  {"lineage": "half-woven", "beacon": "ram", "occupation": "innkeeper, the Grey Gull"},
  [st("alive")],
  [rel("member-of", "order-of-the-grey-dawn"),
   rel("owns", "the-grey-gull")])

E("cassian-dorn", "Cassian Dorn", "character",
  ["Master Dorn", "the Tidemaster"],
  "Thornfall's severe tide-master and the saga's great ambiguity — a former Saltsworn whose love for Miriel Alcott bound him to Hollis's side.",
  {"order": "Undertow", "lineage": "half-woven", "beacon": "vixen", "occupation": "Tidemaster (books 1-5), Warding-master (book 6), Headmaster (book 7)"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed by Nharla in the Wailing Boathouse on Mirehold's order; gives Wren his memories as he dies.")],
  [rel("member-of", "the-saltsworn", until=sp("01-the-marked-tide"),
       note="Turned before the saga out of remorse when Mirehold marked Miriel; a Saltsworn in name only, as a spy."),
   rel("member-of", "order-of-the-grey-dawn", note="Hollis's agent inside the Drowned King's circle."),
   rel("serves", "eamon-hollis"),
   rel("loves", "miriel-alcott", note="The reason behind everything, revealed in the tide-master's memories."),
   rel("killed", "eamon-hollis", frm=sp("06-the-reliquary"),
       note="By prior arrangement — Hollis was already dying of the signet's curse and asked it to spare Julian's soul and hold Dorn's cover."),
   rel("created", "the-sever-canto"),
   rel("teaches", "thornfall-conservatory", until=sp("07-the-last-canto"))])

E("marisol-quill", "Marisol Quill", "character",
  ["Mistress Quill", "the Deputy"],
  "Shaping-mistress, deputy head, and the spine of Thornfall; cut down defending the school during the Concord's purge.",
  {"order": "Riptide", "lineage": "woven", "occupation": "Shaping-mistress, Deputy Headmistress"},
  [st("alive"),
   st("dead", at=sp("05-the-drowned-court"),
      note="Killed holding the gate against Inquisitor Crow's wardens.")],
  [rel("teaches", "thornfall-conservatory", until=sp("05-the-drowned-court")),
   rel("leads", "riptide-order", until=sp("05-the-drowned-court")),
   rel("member-of", "order-of-the-grey-dawn")])

E("bogdan-turl", "Bogdan Turl", "character", ["Bogdan"],
  "Half-fathomkin groundskeeper and Keeper of the Tide-Locks; expelled unjustly as a boy, devoted to Wren and to every monster he can smuggle into his hut.",
  {"lineage": "half-fathomkin", "occupation": "groundskeeper, later beastlore master"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory", frm=sp("03-the-sablefen-pact")),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("sibling-of", "grull")])

E("odris-fenn", "Odris Fenn", "character", ["Master Fenn"],
  "Diminutive canto-master and head of Springtide, with grindle ancestry and a duelling champion's past.",
  {"order": "Springtide", "lineage": "part-grindle", "occupation": "Canto-master"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory"), rel("leads", "springtide-order")])

E("hesper-dale", "Hesper Dale", "character", ["Mistress Dale"],
  "Greenwork mistress and head of Millpond; her tide-lilies revive the petrified in the second year.",
  {"order": "Millpond", "lineage": "woven", "occupation": "Greenwork mistress"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory"), rel("leads", "millpond-order")])

E("volimar-brix", "Volimar Brix", "character", ["Master Brix"],
  "Collector of promising students and returning brewing-master; his doctored memory hides the secret of the Drowned King's reliquaries.",
  {"order": "Undertow", "lineage": "woven", "occupation": "Brewing-master (from book 6)"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory", frm=sp("06-the-reliquary")),
   rel("leads", "the-salon-of-brix"),
   rel("mentor-of", "corvane-mirehold", note="Unwittingly told the young Aldous Mirehold how reliquaries are made.")])

E("sethra-cole", "Sethra Cole", "character", ["Mistress Cole"],
  "Star-reading mistress, a fraud at the lectern who nonetheless spoke the true prophecy that set the whole saga turning.",
  {"lineage": "woven", "occupation": "Star-reading mistress"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory"), rel("created", "the-prophecy")])

E("thaddeus-crole", "Thaddeus Crole", "character", ["the Grey Lecturer"],
  "The only ghost on Thornfall's staff; drowned in the tide-hall and rose to lecture on history without noticing.",
  {"lineage": "shade", "occupation": "History lecturer"},
  [st("shade", note="Drowned long before the saga; lectures on regardless.")],
  [rel("teaches", "thornfall-conservatory")])

# defense/warding teachers by book
E("eldon-frey", "Eldon Frey", "character", ["Master Frey"],
  "Stammering warding teacher hosting the Drowned King's shade beneath his hood; burns at Wren's touch and dies when his master flees him.",
  {"lineage": "woven", "occupation": "Warding-master (book 1)"},
  [st("alive"),
   st("dead", at=sp("01-the-marked-tide"),
      note="Mirehold's shade abandoned their shared body in the tide-engine vault, leaving him to die.")],
  [rel("serves", "corvane-mirehold"),
   rel("teaches", "thornfall-conservatory", until=sp("01-the-marked-tide"))])

E("gaultier-plume", "Gaultier Plume", "character", ["Master Plume"],
  "Celebrity fraud who stole other weavers' deeds with memory-cantos — until a broken lenset turned his own forgetting back on him.",
  {"order": "Springtide", "lineage": "woven", "occupation": "Warding-master (book 2)"},
  [st("alive"),
   st("memory lost", at=sp("02-the-sunken-warren"),
      note="His own backfiring memory-canto; a permanent resident of the Menders' Hall thereafter.")],
  [rel("teaches", "thornfall-conservatory", until=sp("02-the-sunken-warren"))])

E("rowan-thistle", "Rowan Thistle", "character",
  ["Master Thistle", "the Grey Wolf"],
  "Moon-touched warding-master and the best teacher Wren ever had; a Nightstray who falls in the siege of Thornfall.",
  {"lineage": "moon-touched", "beacon": "wolf", "occupation": "Warding-master (book 3)"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed in the siege; the narrative shows his body, not his killer.")],
  [rel("member-of", "the-nightstray"),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("created", "the-wanderers-chart"),
   rel("married-to", "nessa-brightwater", frm=sp("07-the-last-canto")),
   rel("mentor-of", "wren-alcott", note="Taught her the beacon-canto.")])

E("dain-hollow", "Dain Hollow", "character",
  ["Grey Dain", "the Old Warden"],
  "Legendary scarred Tidewarden with a seeing-glass eye; impersonated for a whole year, then struck down covering Wren's escape.",
  {"lineage": "woven", "occupation": "Tidewarden (retired)"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Cut down by Mirehold himself during the flight of the seven decoys.")],
  [rel("member-of", "order-of-the-grey-dawn")])

E("delphine-crow", "Delphine Crow", "character",
  ["Inquisitor Crow"],
  "Concord inquisitor in dove-grey who tortures students with a bleeding quill and later chairs the Lineage Commission.",
  {"order": "Undertow", "lineage": "woven", "occupation": "Warding-master and High Inquisitor (book 5)"},
  [st("alive")],
  [rel("serves", "tidal-concord"),
   rel("teaches", "thornfall-conservatory", frm=sp("05-the-drowned-court"), until=sp("06-the-reliquary")),
   rel("leads", "the-wardens-watch"),
   rel("enemy-of", "wrens-watch")])

E("mabon-skint", "Mabon Skint", "character", ["Skint"],
  "Dimglass caretaker of Thornfall, forever prowling the tide-halls with Grimalkin and dreaming of hanging students by their thumbs.",
  {"lineage": "dimglass", "occupation": "caretaker"},
  [st("alive")],
  [rel("serves", "thornfall-conservatory")])

E("mercy-fell", "Mercy Fell", "character", ["Matron Fell"],
  "Thornfall's matron, who regrows bone and mends the survivors of every book's finale.",
  {"lineage": "woven", "occupation": "matron, the mending-ward"},
  [st("alive")],
  [rel("serves", "thornfall-conservatory")])

# -- the Nightstray & Wren's parents --
E("tomas-alcott", "Tomas Alcott", "character", ["the Stag of the Nightstray"],
  "Wren's father, a Nightstray and stag-skinshift; murdered by the Drowned King at Saltmere before the saga opens.",
  {"order": "Riptide", "lineage": "woven"},
  [st("dead", note="Killed defending his family, sixteen years before the first book.")],
  [rel("parent-of", "wren-alcott"),
   rel("married-to", "miriel-alcott"),
   rel("member-of", "the-nightstray"),
   rel("created", "the-wanderers-chart")])

E("miriel-alcott", "Miriel Alcott", "character", ["Miriel Prewitt"],
  "Wren's mother, whose sacrificial death wove a blood-ward around her daughter; the lifelong love of Cassian Dorn.",
  {"order": "Riptide", "lineage": "unwoven-born", "beacon": "vixen"},
  [st("dead", note="Killed at Saltmere shielding Wren, sixteen years before the first book.")],
  [rel("parent-of", "wren-alcott"),
   rel("married-to", "tomas-alcott"),
   rel("sibling-of", "agatha-prewitt")])

E("dorian-vell", "Dorian Vell", "character",
  ["the Hound of the Nightstray", "V."],
  "Wren's godfather and unregistered hound-skinshift; ten years in Bleakhold for a betrayal he never committed, the last of the drowned House of Vell.",
  {"order": "Riptide", "lineage": "woven"},
  [st("alive"),
   st("dead", at=sp("03-the-sablefen-pact"),
      note="Struck through the tide-veil in the Vault of Tides by Vespera Locke.")],
  [rel("godparent-of", "wren-alcott"),
   rel("member-of", "the-nightstray"),
   rel("member-of", "order-of-the-grey-dawn"),
   rel("created", "the-wanderers-chart"),
   rel("owns", "vell-house", until=sp("03-the-sablefen-pact"),
       note="The house passes to Wren on his death."),
   rel("sibling-of", "alric-mirehold", note="Half-kin through the drowned houses.")])

E("silvo-crane", "Silvo Crane", "character",
  ["the Rat of the Nightstray", "Nibb"],
  "The Nightstray who sold the Alcotts to the Drowned King and hid ten years as the Marshes' pet marsh-rat; his silvered hand turns on him in the end.",
  {"order": "Riptide", "lineage": "half-woven"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Throttled by his own silvered hand when a flicker of mercy toward Wren betrayed his master.")],
  [rel("member-of", "the-nightstray", until=sp("01-the-marked-tide"),
       note="Betrayed the Alcotts to Mirehold sixteen years before the saga."),
   rel("member-of", "the-saltsworn"),
   rel("serves", "corvane-mirehold"),
   rel("created", "the-wanderers-chart")])

# -- antagonist & Saltsworn --
E("corvane-mirehold", "Corvane Mirehold", "character",
  ["the Drowned King", "Aldous Mirehold", "the Nameless Tide"],
  "The greatest dark weaver of the age, who drowned fragments of his self in reliquaries to cheat death — and died of his own rebounding canto when the Sovereign Lens refused him.",
  {"order": "Undertow", "lineage": "half-woven", "born": "the year of the black tide"},
  [st("unbodied", note="His death-canto rebounded off the infant Wren at Saltmere, unmaking his body."),
   st("restored", at=sp("04-the-trine-tournament"),
      note="Reborn in the Sablefen shallows from bone, brine, and stolen blood."),
   st("dead", at=sp("07-the-last-canto"),
      note="His own killing-canto rebounds in the tide-hall — the Sovereign Lens knew Wren for its master.")],
  [rel("leads", "the-saltsworn"),
   rel("enemy-of", "wren-alcott"),
   rel("enemy-of", "eamon-hollis"),
   rel("created", "the-tideglass-journal"),
   rel("created", "the-drowned-signet"),
   rel("created", "the-first-weaver-conch"),
   rel("created", "the-brine-crown"),
   rel("created", "the-star-lens"),
   rel("killed", "tomas-alcott"),
   rel("killed", "miriel-alcott"),
   rel("killed", "dain-hollow", frm=sp("07-the-last-canto"),
       note="Struck him down personally during the flight of the seven decoys."),
   rel("killed", "halbrecht-mourn", frm=sp("07-the-last-canto"),
       note="Murdered in his cell at Fastness Mourn for refusing to yield the Sovereign Lens."),
   rel("pet-of", "nharla",
       note="Nharla is his familiar and final reliquary; the relation reads from the serpent's side.")])

E("vespera-locke", "Vespera Locke", "character", ["Vespera"],
  "The Drowned King's most fanatical lieutenant — broke the Hale family's minds, kills Dorian Vell and the hob Pib, and falls at Rosalind Marsh's hand.",
  {"lineage": "woven"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed by Rosalind Marsh in the tide-hall, the last Saltsworn to fall before Mirehold.")],
  [rel("member-of", "the-saltsworn"),
   rel("serves", "corvane-mirehold"),
   rel("killed", "dorian-vell", frm=sp("03-the-sablefen-pact"),
       note="Her canto sent him through the tide-veil in the Vault of Tides."),
   rel("killed", "pib", frm=sp("07-the-last-canto"),
       note="Her thrown salt-knife, during the escape from Mirehold Keep."),
   rel("sibling-of", "sable-ashenford")])

E("rennick-ashenford", "Rennick Ashenford", "character",
  ["Lord Ashenford"],
  "Silken aristocrat of the Saltsworn who slips the tideglass journal into a child's satchel, and ends the war broken, begging for his son.",
  {"lineage": "woven"},
  [st("alive"),
   st("imprisoned", at=sp("05-the-drowned-court"),
      note="Sent to Bleakhold after the ruin at the Vault of Tides."),
   st("released", at=sp("07-the-last-canto"),
      note="Out of Bleakhold but disgraced, his keep seized as Mirehold's court.")],
  [rel("member-of", "the-saltsworn"),
   rel("married-to", "sable-ashenford"),
   rel("parent-of", "julian-ashenford"),
   rel("owns", "mirehold-keep")])

E("sable-ashenford", "Sable Ashenford", "character", ["Lady Ashenford"],
  "Never a branded Saltsworn, but the mother whose lie to the Drowned King's face wins the war for love of her son.",
  {"lineage": "woven"},
  [st("alive")],
  [rel("married-to", "rennick-ashenford"),
   rel("parent-of", "julian-ashenford"),
   rel("sibling-of", "vespera-locke")])

E("julian-ashenford", "Julian Ashenford", "character", ["Julian"],
  "Wren's schoolyard rival, pressed into the Drowned King's service at sixteen and set an impossible task he cannot bring himself to finish.",
  {"order": "Undertow", "lineage": "woven", "born": "the year of the low tide"},
  [st("alive")],
  [rel("member-of", "undertow-order"),
   rel("member-of", "the-wardens-watch", frm=sp("05-the-drowned-court"), until=sp("06-the-reliquary")),
   rel("member-of", "the-saltsworn", frm=sp("06-the-reliquary")),
   rel("enemy-of", "wren-alcott"),
   rel("owns", "the-sovereign-lens", frm=sp("06-the-reliquary"), until=sp("07-the-last-canto"),
       note="Mastery only — he disarmed Hollis on the spire without ever holding the lens.")])

E("hurst-gorrel", "Hurst Gorrel", "character", ["Gorrel"],
  "Julian's hulking crony, deadlier by the last book than anyone expected — killed by the wild-fire he himself unleashed.",
  {"order": "Undertow", "lineage": "woven"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Consumed by his own wild-fire canto in the Room of Want.")],
  [rel("member-of", "undertow-order"), rel("serves", "julian-ashenford")])

E("dell-vench", "Dell Vench", "character", ["Vench"],
  "The other half of Julian's muscle; survives the Room of Want fire that takes Gorrel.",
  {"order": "Undertow", "lineage": "woven"},
  [st("alive")],
  [rel("member-of", "undertow-order"), rel("serves", "julian-ashenford")])

E("fenris-graff", "Fenris Graff", "character", ["Graff"],
  "Savage moon-touched who chooses to maul children — the one who turned Rowan Thistle, and who scars Cade and Prue.",
  {"lineage": "moon-touched"},
  [st("alive")],
  [rel("allied-with", "the-saltsworn")])

E("selvon-kord", "Selvon Kord", "character", ["Kord"],
  "Kaldmarch's headmaster and a Saltsworn who named names to stay out of Bleakhold; flees the Drowned King's return and is hunted down.",
  {"lineage": "woven", "occupation": "Headmaster of Kaldmarch"},
  [st("alive"),
   st("dead", at=sp("06-the-reliquary"),
      note="Found dead in a smuggler's shack with the brine-mark burned above it — deserters last barely a year.")],
  [rel("member-of", "the-saltsworn", until=sp("01-the-marked-tide"),
       note="Defected by informing; fled rather than answer the mark's return."),
   rel("leads", "kaldmarch-institute", until=sp("04-the-trine-tournament"))])

E("alric-mirehold", "Alric Mirehold", "character", ["Alric", "A.M."],
  "The Drowned King's younger kin, a Saltsworn who found his conscience — stole the drowned signet and died for it, leaving only the initials A.M.",
  {"order": "Undertow", "lineage": "woven"},
  [st("dead", note="Drowned by the vault-wraiths retrieving the signet reliquary, having left a forgery in its place — revealed in the last book.")],
  [rel("member-of", "the-saltsworn", note="Defected in secret; his theft of the signet surfaces only in the final book.")])

E("halbrecht-mourn", "Halbrecht Mourn", "character", ["Mourn"],
  "The dark weaver before the Drowned King — Hollis's brilliant, terrible first friend, defeated by him and caged in Fastness Mourn.",
  {"lineage": "woven"},
  [st("imprisoned", note="In Fastness Mourn since his defeat by Hollis, decades before the saga."),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed in his cell by Mirehold for refusing to betray the Sovereign Lens.")],
  [rel("owns", "the-sovereign-lens", until=sp("01-the-marked-tide"),
       note="Held it until his defeat, long before the saga; lost it to Hollis.")])

# -- Concord / government --
E("cornick-blythe", "Cornick Blythe", "character", ["High Warden Blythe"],
  "High Warden of the Concord whose year of denying the Drowned King's return costs the Isles their head start.",
  {"lineage": "woven", "occupation": "High Warden of the Concord"},
  [st("alive"),
   st("deposed", at=sp("06-the-reliquary"),
      note="Forced out in disgrace once Mirehold's return became undeniable.")],
  [rel("leads", "tidal-concord", until=sp("06-the-reliquary"))])

E("rufus-sallow", "Rufus Sallow", "character", ["High Warden Sallow"],
  "Lion-grey war-time High Warden, a former Tidewarden; tortured and killed in the Concord's fall — he never gave Wren up.",
  {"lineage": "woven", "occupation": "High Warden of the Concord"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Murdered by the Saltsworn when the Concord fell.")],
  [rel("leads", "tidal-concord", frm=sp("06-the-reliquary"), until=sp("07-the-last-canto"))])

E("pell-thicket", "Pell Thicket", "character", ["Thicket"],
  "Mind-bound puppet High Warden installed after the coup; the Drowned King's respectable mask on the Concord.",
  {"lineage": "woven", "occupation": "High Warden (puppet)"},
  [st("alive"), st("mind-bound", at=sp("07-the-last-canto"))],
  [rel("leads", "tidal-concord", frm=sp("07-the-last-canto")),
   rel("serves", "corvane-mirehold")])

E("barnaby-crole-sr", "Barnaby Crole", "character", ["Warden Crole"],
  "Rigid Concord official who sent his own son to Bleakhold — and then secretly freed him, a mercy that kills him.",
  {"lineage": "woven", "occupation": "Head of Inter-Isles Concord"},
  [st("alive"),
   st("dead", at=sp("04-the-trine-tournament"),
      note="Murdered by his own son and shaped into driftwood, buried in Bogdan's garden.")],
  [rel("serves", "tidal-concord"), rel("parent-of", "lucan-crole")])

E("lucan-crole", "Lucan Crole", "character", ["Lucan"],
  "Fanatic Saltsworn who spent the fourth book wearing Dain Hollow's face and steering Wren into the shallows.",
  {"lineage": "woven"},
  [st("alive"),
   st("soul-hollowed", at=sp("04-the-trine-tournament"),
      note="Given the Hollowing on the High Warden's arrival — worse than dead, and with him died the proof of the Drowned King's return.")],
  [rel("member-of", "the-saltsworn"),
   rel("serves", "corvane-mirehold"),
   rel("killed", "barnaby-crole-sr", frm=sp("04-the-trine-tournament"))])

E("saul-brand", "Saul Brand", "character", ["Warden Brand"],
  "Calm, commanding Tidewarden of the Grey Dawn; his lynx-beacon warns the wedding, and he ends the saga as High Warden.",
  {"lineage": "woven", "beacon": "lynx", "occupation": "Tidewarden"},
  [st("alive")],
  [rel("member-of", "order-of-the-grey-dawn"),
   rel("serves", "tidal-concord"),
   rel("leads", "tidal-concord", frm=sp("07-the-last-canto"),
       note="Named interim High Warden after the siege of Thornfall.")])

E("nessa-brightwater", "Nessa Brightwater", "character", ["Nessa"],
  "Faceshifter Tidewarden who hates her given name; marries Rowan Thistle and falls beside him in the siege.",
  {"lineage": "faceshifter", "beacon": "wolf (changed)", "occupation": "Tidewarden"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed in the siege; the narrative shows her body beside Rowan's without naming her killer.")],
  [rel("member-of", "order-of-the-grey-dawn"),
   rel("married-to", "rowan-thistle", frm=sp("07-the-last-canto"))])

E("mundo-flint", "Mundo Flint", "character", ["Mundo"],
  "Petty crook on the Grey Dawn's books; loots Vell House and unknowingly hands a reliquary to Inquisitor Crow.",
  {"lineage": "woven"},
  [st("alive")],
  [rel("member-of", "order-of-the-grey-dawn")])

# -- students & others --
E("halden-brooke", "Halden Brooke", "character", ["Halden"],
  "Millpond's champion — skiff-captain, prefect, and Trine co-winner; the first casualty of the Drowned King's return.",
  {"order": "Millpond", "lineage": "woven", "born": "the year of the grey tide"},
  [st("alive"),
   st("dead", at=sp("04-the-trine-tournament"),
      note="Killed by Lucan Crole's canto in the Sablefen shallows.")],
  [rel("member-of", "millpond-order"), rel("loves", "su-lin-marr")])

E("su-lin-marr", "Su-Lin Marr", "character", ["Su-Lin"],
  "Springtide skiff-catcher; Wren's first fondness, grieving Halden through the fifth book.",
  {"order": "Springtide", "lineage": "woven", "beacon": "swan"},
  [st("alive")],
  [rel("member-of", "springtide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court"))])

E("luthien-quess", "Luthien Quess", "character", ["Luthien", "Loony Quess"],
  "Dreamy Springtide seer who sees tide-mares and truths nobody else will; unshakeable in every fight that matters.",
  {"order": "Springtide", "lineage": "woven", "beacon": "hare"},
  [st("alive")],
  [rel("member-of", "springtide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("parent-of", "alder-quess", note="Alder is her father; relation reads from his side.")])

E("alder-quess", "Alder Quess", "character", ["Alder"],
  "Luthien's father and editor of the Contrary Tide; explains the Saltglass Relics — then sells the trio out to save his daughter.",
  {"lineage": "woven", "occupation": "editor, the Contrary Tide"},
  [st("alive")],
  [rel("parent-of", "luthien-quess"), rel("leads", "the-contrary-tide")])

E("tobin-hale", "Tobin Hale", "character", ["Tobin"],
  "The child the prophecy might have chosen; grows from timid greenwork student to the leader of Thornfall's resistance — and beheads Nharla.",
  {"order": "Riptide", "lineage": "woven", "born": "the year of the low tide"},
  [st("alive")],
  [rel("member-of", "riptide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court")),
   rel("leads", "wrens-watch", frm=sp("07-the-last-canto"),
       note="Leads the resistance inside occupied Thornfall."),
   rel("killed", "nharla", frm=sp("07-the-last-canto"),
       note="Beheaded her with the Sword of Storrow, unmaking the last reliquary.")])

E("finn-dabb", "Finn Dabb", "character", ["Finn"],
  "Unwoven-born with a tide-lens and boundless awe of Wren; sneaks back underage to fight and die in the siege.",
  {"order": "Riptide", "lineage": "unwoven-born"},
  [st("alive"),
   st("dead", at=sp("07-the-last-canto"),
      note="Killed in the siege; Tobin and Darrow carry his body in.")],
  [rel("member-of", "riptide-order"),
   rel("member-of", "wrens-watch", frm=sp("05-the-drowned-court"))])

E("prue-callow", "Prue Callow", "character", ["Prue"],
  "Riptide girl and Perrin's sixth-year fling; savaged by Fenris Graff in the siege of Thornfall.",
  {"order": "Riptide", "lineage": "woven"},
  [st("alive"),
   st("gravely wounded", at=sp("07-the-last-canto"),
      note="Attacked by Graff; her fate is left uncertain.")],
  [rel("member-of", "riptide-order"),
   rel("loves", "perrin-marsh", frm=sp("06-the-reliquary"), until=sp("07-the-last-canto"))])

E("darrow-keel", "Darrow Keel", "character", ["Darrow"],
  "Obsessive Riptide skiff-captain who recruits first-year Wren as a catcher.",
  {"order": "Riptide", "lineage": "woven"},
  [st("alive")],
  [rel("member-of", "riptide-order")])

E("anselm-varga", "Anselm Varga", "character", ["Varga"],
  "Kaldmarch's stormy skiff-catcher and Trine champion; takes Odile to the Tide-Ball and never quite gets over it.",
  {"lineage": "woven", "occupation": "champion skiff-catcher"},
  [st("alive")],
  [rel("member-of", "kaldmarch-institute"),
   rel("loves", "odile-sarkany", frm=sp("04-the-trine-tournament"))])

E("amelie-brvolar", "Amelie Brvolar", "character", ["Amelie"],
  "Aurelon's part-siren Trine champion; marries Cade Marsh and shelters the trio at Shell-and-Bone Cottage.",
  {"lineage": "part-siren"},
  [st("alive")],
  [rel("member-of", "aurelon-lyceum"),
   rel("married-to", "cade-marsh", frm=sp("07-the-last-canto")),
   rel("owns", "shell-and-bone-cottage")])

E("verity-sloane", "Verity Sloane", "character", ["Verity"],
  "Poison-quilled Isles Herald reporter and unregistered gull-skinshift; Odile keeps her in a jar for a while.",
  {"lineage": "woven", "occupation": "reporter"},
  [st("alive")],
  [rel("serves", "the-isles-herald")])

E("halruun", "Halruun", "character", ["Halruun of the Starkin"],
  "The Starkin who carried Wren to safety through the Shrouded Wood — cast out by his herd for serving weavers as Thornfall's star-reader.",
  {"lineage": "starkin"},
  [st("alive")],
  [rel("teaches", "thornfall-conservatory", frm=sp("05-the-drowned-court"))])

E("grull", "Grull", "character", ["Grull"],
  "Bogdan's sixteen-foot fathomkin half-brother, dragged home from the sea-cliffs and slowly gentled in the Shrouded Wood.",
  {"lineage": "fathomkin"},
  [st("alive")],
  [rel("sibling-of", "bogdan-turl")])

E("sir-edric-holloway", "Sir Edric Holloway", "character", ["the Half-Drowned"],
  "Riptide's resident shade, drowned incompletely in the founders' age — his head still hangs by a thread of kelp.",
  {"lineage": "shade"},
  [st("shade", note="Drowned in the founders' age; his half-severed head anchors the saga's chronology.")],
  [rel("member-of", "riptide-order")])

E("wailing-nell", "Wailing Nell", "character", ["Nell"],
  "Shade of a bullied Springtide girl killed by the Leviathan in the founders' echo — the Warren's first victim, haunting the flooded washroom ever since.",
  {"lineage": "shade"},
  [st("shade", note="Killed by the Leviathan when the Warren first opened; the Drowned King's first murder and the price of the tideglass journal.")],
  [rel("member-of", "springtide-order")])

E("ephraim-thornby", "Ephraim Thornby", "character", ["Thornby"],
  "Lensmaker of the Coilrow since the founders' age; abducted and tortured for the lore of the Sovereign Lens.",
  {"lineage": "woven", "occupation": "lensmaker"},
  [st("alive"),
   st("imprisoned", at=sp("06-the-reliquary"), note="Abducted by Mirehold for lens-lore."),
   st("freed", at=sp("07-the-last-canto"), note="Rescued from Mirehold Keep's cellar with Luthien and a Kaldmarch boy.")],
  [rel("owns", "thornbys-lensery")])

E("snurr", "Snurr", "character", ["Snurr"],
  "Gornhollow grindle who helps the trio rob the Ashenford vault — for the price of the Sword of Storrow, and betrays them mid-heist.",
  {"lineage": "grindle"},
  [st("alive")],
  [rel("serves", "gornhollow-bank")])

E("osric-prewitt", "Osric Prewitt", "character", ["Uncle Osric"],
  "Ledger-selling Unwoven uncle who spent ten years pretending weaving away and the next six failing at it.",
  {"lineage": "unwoven", "occupation": "ledger-broker"},
  [st("alive")],
  [rel("married-to", "agatha-prewitt"),
   rel("parent-of", "gully-prewitt"),
   rel("owns", "nine-prewitt-lane")])

E("agatha-prewitt", "Agatha Prewitt", "character", ["Aunt Agatha"],
  "Miriel's Unwoven sister, who took Wren in with resentment — her grudge born of a childhood letter begging to be let into Thornfall too.",
  {"lineage": "unwoven"},
  [st("alive")],
  [rel("married-to", "osric-prewitt"),
   rel("parent-of", "gully-prewitt"),
   rel("sibling-of", "miriel-alcott")])

E("gully-prewitt", "Gully Prewitt", "character", ["Gully"],
  "Wren's bullying cousin; a Griever attack in the fifth book begins the slow thaw that ends with a mumbled thanks.",
  {"lineage": "unwoven"},
  [st("alive")])

E("pib", "Pib", "character", ["Pib the hob"],
  "The Ashenfords' abused house-hob, freed by a knotted rag and fiercely loyal to Wren; dies pulling prisoners out of Mirehold Keep.",
  {"lineage": "hob"},
  [st("alive"),
   st("freed", at=sp("02-the-sunken-warren"),
      note="Freed when Wren tricks Rennick Ashenford with a rag folded into the journal."),
   st("dead", at=sp("07-the-last-canto"),
      note="Vespera's salt-knife, thrown as he tide-stepped the prisoners to Shell-and-Bone Cottage.")],
  [rel("serves", "rennick-ashenford", until=sp("02-the-sunken-warren")),
   rel("allied-with", "wren-alcott")])

E("grael", "Grael", "character", ["Grael"],
  "The Vells' bitter old house-hob; his treatment by Dorian helps doom his master, but Alric's stolen signet wins him to Wren's side.",
  {"lineage": "hob"},
  [st("alive")],
  [rel("serves", "dorian-vell", until=sp("03-the-sablefen-pact")),
   rel("serves", "wren-alcott", frm=sp("04-the-trine-tournament"))])

# -- the four founders --
E("aldric-storrow", "Aldric Storrow", "character", ["Storrow the Bold"],
  "Founder who prized courage; his cowl sorts the school and his sword answers those who ask bravely.",
  {"lineage": "woven"},
  [st("dead", note="Died a thousand tides before the saga.")],
  [rel("created", "riptide-order"),
   rel("created", "the-sword-of-storrow"),
   rel("created", "the-tideglass-cowl")])

E("sabine-vex", "Sabine Vex", "character", ["Vex the Deep"],
  "Founder who prized cunning and pure lineage; left the school a tidesinger's legacy coiled beneath it.",
  {"lineage": "woven"},
  [st("dead", note="Died a thousand tides before the saga.")],
  [rel("created", "undertow-order"),
   rel("created", "the-sunken-warren"),
   rel("created", "the-first-weaver-conch",
       note="The conch was her heirloom long before Mirehold drowned his self in it.")])

E("halvard-denn", "Halvard Denn", "character", ["Denn the Patient"],
  "Founder who took the rest and prized fairness and toil; his cup becomes one of the Drowned King's reliquaries.",
  {"lineage": "woven"},
  [st("dead", note="Died a thousand tides before the saga.")],
  [rel("created", "millpond-order"),
   rel("created", "the-denn-cup")])

E("orla-wynn", "Orla Wynn", "character", ["Wynn the Wise"],
  "Founder who prized wit; her lost circlet, stolen by her daughter and defiled by the young Mirehold, hides in the school all along.",
  {"lineage": "woven"},
  [st("dead", note="Died a thousand tides before the saga.")],
  [rel("created", "springtide-order"),
   rel("created", "the-wynn-circlet")])

# ===================== SPELLS (cantos) =====================
def C(id, incant, common, effect, kind="canto", light="", summary="", extra=None):
    f = {"incantation": incant, "type": kind, "effect": effect}
    if light:
        f["light"] = light
    E(id, incant, "spell", aliases=[common], summary=summary, fields=f,
      relations=extra)

C("sunderhold", "Sunderhold", "the Unhanding Canto",
  "strips the lenset from an opponent's grip", light="pale gold",
  summary="Wren's signature canto — it tears the focus from another weaver's hand and is what her lenset throws in the last exchange.")
C("luvenar", "Luvenar", "the Beacon Canto",
  "conjures a guardian beacon-shape from a bright memory", light="silver",
  summary="Advanced weaving: a guardian summoned from a single joyful memory, the only ward against Grievers. Wren learns it at thirteen from Rowan Thistle.")
C("mordath", "Mordath", "the Ending Canto",
  "instant death", kind="forbidden canto", light="black-green",
  summary="Instant, unblockable death; one of the three Forbidden Cantos. Wren alone is known to have survived it.")
C("vexal", "Vexal", "the Racking Canto",
  "unbearable pain", kind="forbidden canto",
  summary="Inflicts crippling agony; one of the three Forbidden Cantos. The Locke sisters used it to break the Hale family.")
C("thrallis", "Thrallis", "the Binding Canto",
  "total control of the victim", kind="forbidden canto",
  summary="Places the victim wholly under the singer's will; one of the three Forbidden Cantos. Wren alone in her year can throw it off.")
C("the-sever-canto", "Rivenmark", "the Sever Canto",
  "opens deep slashing wounds", kind="forbidden canto",
  summary="Cuts as though by an unseen tide-blade — invented by the young Cassian Dorn, and used against its own maker's allies before anyone knew its name.")
C("ospertide", "Ospertide", "the Unlatching Canto",
  "unlocks doors and tide-gates",
  summary="The thief's tide — opens what is locked, including the door that hides a warren.")
C("aeravel", "Aeravel", "the Lifting Canto",
  "raises objects into the air",
  summary="Lifts and floats objects; Perrin's first real triumph, dropping a beam on a marsh-troll.")
C("stavecage", "Stavecage", "the Bodylock Canto",
  "locks the whole body rigid", kind="canto",
  summary="Snaps the target's body straight and still; Odile's reluctant answer to anyone standing in the way.")
C("kneelbind", "Kneelbind", "the Leglock Canto",
  "binds the legs together",
  summary="Locks the legs mid-stride — Julian's idea of a joke.")
C("callward", "Callward", "the Summoning Canto",
  "draws an object to the singer",
  summary="Summons objects across a distance; Wren drills it for weeks to pull her skiff to her mid-trial.")
C("fellstun", "Fellstun", "the Stilling Canto",
  "knocks the target senseless", light="red",
  summary="Drops the target unconscious in a flash of red; the workhorse of every skirmish from the Vault onward.")
C("wardveil", "Wardveil", "the Shield Canto",
  "raises a rebounding shield",
  summary="Throws up an unseen shield that turns hexes back; a staple of Wren's Watch.")
C("glimmer", "Glimmer", "the Lightwick Canto",
  "kindles light at the lenset's tip",
  summary="Lights the focus like a lantern. Undone by the Duskwick counter.")
C("mirthbreak", "Mirthbreak", "the Unmaking Canto",
  "forces a dreadmimic into a shape you find funny",
  summary="Turns a dreadmimic into something absurd — laughter finishes it. Tobin's version is the textbook case.")
C("blankmere", "Blankmere", "the Forgetting Canto",
  "erases memory",
  summary="Wipes memory clean. Gaultier Plume's whole career, until a broken lenset handed it back to him; Odile uses it on her own parents to keep them safe.")
C("knitfast", "Knitfast", "the Mending Canto",
  "repairs broken objects",
  summary="Mends what is broken — lensets, most often, on the tide-ferry.")
C("heelhoist", "Heelhoist", "the Ankle Canto",
  "hauls the victim up by one ankle", kind="jinx",
  summary="Dangles the target in the air by the heel — another of Dorn's schoolboy inventions, later turned against him.")
C("hushmurk", "Hushmurk", "the Muffling Canto",
  "fills nearby ears with a dull roar",
  summary="Drowns eavesdroppers' ears in tide-noise so a conversation stays private; another Dorn invention.")
C("wellspring", "Wellspring", "the Water Canto",
  "conjures fresh water",
  summary="Draws clean water from the air; Wren sings it desperately in the sea-cave and against a burning hut.")
C("wyrmfire", "Wyrmfire", "the Wild-Fire Canto",
  "unquenchable, beast-shaped fire", kind="forbidden canto",
  summary="Cursed fire of serpent-shaped flame, nearly impossible to master — one of the few forces that unmakes reliquaries. It takes the Room of Want, the star-lens, and Hurst Gorrel with it.")

# ===================== POTIONS =====================
def P(id, name, effect, summary, appearance="", difficulty="", aka=None):
    f = {"effect": effect}
    if appearance:
        f["appearance"] = appearance
    if difficulty:
        f["difficulty"] = difficulty
    E(id, name, "potion", aliases=aka, summary=summary, fields=f)

P("draught-of-stillwater", "Draught of Stillwater",
  "deathlike sleep",
  "Wormroot and low-tide kelp make a sleeping draught so deep it mimics drowning — Brix's first-lesson riddle, and Wren's chart-aided triumph in the sixth book.",
  difficulty="advanced")
P("the-everdraught", "the Everdraught",
  "extends life indefinitely",
  "Distilled from the Tidestone; holds death at bay, but must be drunk with every tide — immortality on a tether.",
  aka=["the Tidewater of Life"])
P("truebrine", "Truebrine",
  "compels the drinker to speak the truth",
  "Three drops of this clear brine and the deepest secret spills out; Dorn's standing threat, Hollis's tool on Lucan Crole.",
  appearance="clear as seawater, odourless")
P("skinbrew", "Skinbrew",
  "assume another person's shape for an hour",
  "Transforms the drinker into another person — human shapes only, as Odile's cat-hair mishap proves. Brewed illegally by second-years; a whole year's disguise for Lucan Crole.",
  appearance="silt-thick, changes with the added hair", difficulty="very advanced",
  aka=["Mirrorbrew"])
P("goldwake", "Goldwake",
  "extraordinary luck for a span of hours",
  "Molten-gold fortune in a vial, banned from contest. Wren's single dose pries the true reliquary memory out of Volimar Brix.",
  appearance="molten gold", difficulty="perilous to brew",
  aka=["Fortune's Tincture"])
P("heartsnare", "Heartsnare",
  "obsessive infatuation (never true love)",
  "The strongest love-draught in the Isles — it smells to each drinker of what they love most, and can only counterfeit love, never make it. The Drowned King was conceived under its pull.",
  appearance="mother-of-pearl sheen, spiralling steam")
P("moonsbane-draught", "Moonsbane Draught",
  "keeps a moon-touched weaver in their right mind through the change",
  "Fiendishly hard to brew; Dorn's monthly gobletful is what makes Rowan Thistle's teaching year possible.",
  difficulty="extremely advanced", aka=["Moonsbane"])
P("bonemend-tonic", "Bonemend Tonic",
  "regrows missing bone overnight, painfully",
  "Regrows a whole arm — Wren's, after Gaultier Plume vanished the bones instead of mending them.")

# ===================== ITEMS =====================
def I(id, name, effect_or_type, summary, aka=None, maker="", powers="", kind="",
      status=None, relations=None):
    f = {}
    if kind:
        f["type"] = kind
    if maker:
        f["maker"] = maker
    if powers:
        f["powers"] = powers
    E(id, name, "item", aliases=aka, summary=summary, fields=(f or None),
      status=status, relations=relations)

I("the-tidestone", "the Tidestone", "",
  "The legendary stone of the sea-alchemist Perenna Flask — turns dross to gold and yields the Everdraught. Guarded beneath Thornfall, and unmade by agreement once the Drowned King comes for it.",
  maker="Perenna Flask", powers="transmutation; the Everdraught",
  status=[st("intact"),
          st("destroyed", at=sp("01-the-marked-tide"),
             note="Hollis and Flask agree to unmake it after Frey's attempt.")])
I("the-tideglass-journal", "the Tideglass Journal", "",
  "The Drowned King's first reliquary — a memory of his sixteen-year-old self that possesses a Marsh child and reopens the Sunken Warren.",
  kind="reliquary",
  status=[st("intact"),
          st("destroyed", at=sp("02-the-sunken-warren"),
             note="Pierced by Wren with a Leviathan fang in the Warren.")])
I("the-drowned-signet", "the Drowned Signet", "",
  "Reliquary made from the Mirehold family seal — its stone secretly the Recalling Pearl. Its curse costs Hollis his hand, and within the year his life.",
  kind="reliquary",
  status=[st("intact"),
          st("destroyed", at=sp("06-the-reliquary"),
             note="Broken by Hollis with the Sword of Storrow the tide before the sixth book; the curse in it is already killing him.")])
I("the-first-weaver-conch", "the First-Weaver Conch", "",
  "Sabine Vex's heirloom turned reliquary; swapped for a forgery by Alric Mirehold, looted from Vell House, worn like a poison at the trio's throats.",
  kind="reliquary",
  status=[st("intact"),
          st("destroyed", at=sp("07-the-last-canto"),
             note="Split by Perrin with the Sword of Storrow at the forest tide-pool, after it shows him his worst fears.")])
I("the-brine-crown", "the Brine Crown", "",
  "A circlet of hardened sea-salt the Drowned King drowned a shard of his self within; hidden in the vaults of Mirehold Keep.",
  kind="reliquary",
  status=[st("intact"),
          st("destroyed", at=sp("07-the-last-canto"),
             note="Stabbed by Odile with a Leviathan fang during the sack of Mirehold Keep.")])
I("the-star-lens", "the Star-Lens", "",
  "A reliquary ground from black star-glass and hidden in the Room of Want, in the school the Drowned King swore he never touched.",
  kind="reliquary",
  status=[st("intact"),
          st("destroyed", at=sp("07-the-last-canto"),
             note="Consumed by Hurst Gorrel's wyrmfire in the Room of Want.")])
I("the-sovereign-lens", "the Sovereign Lens", "",
  "The first Relic — an unbeatable focus that answers only to whoever bested its last master. Its bloody chain of mastery decides the final duel.",
  aka=["the Deathless Lens", "the Lens of Tides"], kind="Saltglass Relic",
  maker="the tide (per the tale); black star-glass, kraken-sinew core",
  powers="unmatched weaving for its true master",
  status=[st("intact",
             note="Mastery chain across the saga: Mourn -> Hollis -> Julian (spire, book 6) -> Wren (Mirehold Keep, book 7). Mirehold holds it in book 7 without ever mastering it. Wren returns it to Hollis's cairn.")])
I("the-recalling-pearl", "the Recalling Pearl", "",
  "The second Relic — calls back the shades of the dead. Set by the Drowned King into the signet reliquary without his knowing what it was; Wren turns it thrice in the Shrouded Wood and walks to her death accompanied.",
  aka=["the Wakestone"], kind="Saltglass Relic",
  status=[st("intact"),
          st("lost", at=sp("07-the-last-canto"),
             note="Slips from Wren's numb fingers in the Shrouded Wood, deliberately unrecovered.")])
I("the-fogmantle", "the Fogmantle", "",
  "The third Relic — a true mantle of unseeing that never frays, handed down from Ignatius Vell to the Alcotts. Hollis had it the night the Alcotts died.",
  aka=["the Mantle of Unseeing"], kind="Saltglass Relic",
  powers="perfect, permanent unseeing",
  status=[st("intact", note="Passed to Wren anonymously at midwinter of her first year.")])
I("the-sword-of-storrow", "the Sword of Storrow", "",
  "Grindle-forged sea-glass sword that presents itself to any true Riptide in need — and, having drunk Leviathan venom, unmakes reliquaries.",
  aka=["Storrow's sword"], maker="grindle-forged (Gornhollow's first smith)",
  powers="imbibes what makes it stronger",
  status=[st("intact", note="Drawn from the Tideglass Cowl by Tobin in the final siege to behead Nharla.")])
I("the-tideglass-cowl", "the Tideglass Cowl", "",
  "Aldric Storrow's enchanted cowl, sorting first-years into their Orders for a thousand tides — and hiding a sword for those who need it.",
  aka=["the Sorting Cowl"], maker="the four founders",
  status=[st("intact")])
I("the-wanderers-chart", "the Wanderer's Chart", "",
  "The Nightstray's masterwork — a living chart of Thornfall showing everyone within it, unfooled by skinbrew or skinshift. Confiscated for years, then gifted to Wren.",
  aka=["the Chart"], maker="the Nightstray",
  powers="shows all of Thornfall and everyone in it",
  status=[st("intact", note="Handed to Wren by Cade in the third book.")])
I("the-yearnglass", "the Yearnglass", "",
  "Shows not your face but your heart's deepest want — weavers have wasted away before it. Hollis's last hiding place for the Tidestone: only one who wants to find it, not use it, can.",
  powers="shows the viewer's deepest want", status=[st("intact")])
I("the-mindwell", "the Mindwell", "",
  "A basin of still tidewater for storing and reliving memories; the saga's window into the past — the orphanage, the trials, and a tide-master's dying gift.",
  powers="stores and replays memories", status=[st("intact")])
I("the-turnglass", "the Turnglass", "",
  "An hourglass on a chain that winds the tide back; issued to Odile for classes, spent instead on saving a tidesteed and a godfather in a single night.",
  powers="short-range time-turning",
  status=[st("intact", note="The Concord's whole stock is smashed in the Vault of Tides battle in book 5.")])
I("the-chalice-of-flame", "the Chalice of Flame", "",
  "An impartial flame-filled cup that chooses each school's Trine champion — until a beguiled fourth name rises from it.",
  powers="binding contest-oath", status=[st("intact")])
I("the-twin-cabinet", "the Twin Cabinet", "",
  "One of a broken pair linking Thornfall to a shop on the Blackwash; Julian's year-long repair smuggles Saltsworn into the school.",
  powers="passage to its twin",
  status=[st("broken", note="Damaged by a poltergeist in the second book."),
          st("repaired", at=sp("06-the-reliquary"),
             note="Mended by Julian in the Room of Want.")])
I("the-gleamcatcher", "the Gleamcatcher", "",
  "Hollis's silver device that drinks the light from lamps — and, left to Perrin, carries a voice that leads him home.",
  maker="Eamon Hollis", powers="captures light; guides its bearer back",
  status=[st("intact", note="Left to Perrin in Hollis's will in the seventh book.")])
I("the-first-flitwing", "the First Flitwing", "",
  "The flitwing Wren nearly swallowed in her first match — bequeathed by Hollis with a memory sealed in its shell. Within it: the Recalling Pearl.",
  aka=["the Flitwing"],
  status=[st("intact", note="Opens for Wren in the Shrouded Wood in the seventh book.")])
I("the-stormlance-skiff", "the Stormlance", "",
  "The fastest wind-skiff on the Isles — an anonymous midwinter gift that proves to be from Dorian, making up for ten missed name-days.",
  kind="racing skiff",
  status=[st("intact"),
          st("lost", at=sp("07-the-last-canto"),
             note="Falls from Bogdan's sky-barge during the flight of the seven decoys.")])
I("the-galeworth-skiff", "the Galeworth", "",
  "Wren's first skiff, bought after one reckless dive proved her a natural catcher.",
  kind="racing skiff",
  status=[st("intact"),
          st("destroyed", at=sp("03-the-sablefen-pact"),
             note="Blown into the strangle-willow during the Griever match.")])
I("the-denn-cup", "Denn's Cup", "",
  "Halvard Denn's golden tide-cup, a Millpond heirloom coveted by collectors and lost for an age in the vaults of Gornhollow.",
  status=[st("intact")])
I("the-wynn-circlet", "Wynn's Circlet", "",
  "Orla Wynn's lost circlet of wit — hidden in the Room of Want, in the school everyone swore the young Mirehold never entered.",
  status=[st("intact"),
          st("recovered", at=sp("07-the-last-canto"),
             note="Found among the hidden things of the Room of Want during the siege.")])

# ===================== CREATURES =====================
def CR(id, name, classification, summary, aka=None, status=None, relations=None,
       rating=""):
    f = {"classification": classification}
    if rating:
        f["ministry-rating"] = rating
    E(id, name, "creature", aliases=aka, summary=summary, fields=f,
      status=status, relations=relations)

CR("nharla", "Nharla", "great sea-serpent",
   "The Drowned King's serpent, familiar, and final reliquary — milked for his rebirth, fed his enemies, and beheaded by Tobin Hale.",
   status=[st("alive"),
           st("dead", at=sp("07-the-last-canto"),
              note="Beheaded by Tobin with the Sword of Storrow — the last reliquary, unmade before Mirehold's eyes.")],
   relations=[rel("serves", "corvane-mirehold"),
              rel("killed", "cassian-dorn", frm=sp("07-the-last-canto"),
                  note="On Mirehold's order, in the Wailing Boathouse.")])
CR("the-leviathan", "the Leviathan", "leviathan",
   "Sabine Vex's thousand-tide sea-serpent, whose gaze kills and whose reflection petrifies — loosed on the school twice through its flooded pipes.",
   aka=["the Serpent of the Warren"],
   status=[st("alive"),
           st("dead", at=sp("02-the-sunken-warren"),
              note="Killed by Wren with the Sword of Storrow; its fangs unmake reliquaries for the rest of the saga.")])
CR("wisp", "Wisp", "pyre-heron",
   "Hollis's pyre-heron — burns to ash and rises again, carries impossible weights, weeps mending tears, and leaves Thornfall forever with one last cry.",
   relations=[rel("pet-of", "eamon-hollis")])
CR("old-fathom", "Old Fathom", "grindlewyrm",
   "Bogdan's grindlewyrm, raised from a hatchling and wrongly blamed for the Warren's killings — innocent of that, though glad to feed the trio to his brood.",
   status=[st("alive"),
           st("dead", at=sp("06-the-reliquary"),
              note="Dies of great age; Brix attends the burial for the venom.")],
   relations=[rel("pet-of", "bogdan-turl")])
CR("cinderhorn", "Cinderhorn", "reef-drake",
   "Bogdan's illegal reef-drake, hatched by a hearth in a wooden hut — bites Perrin, gets smuggled to the northern sanctuaries, and turns out to be a she.",
   relations=[rel("pet-of", "bogdan-turl")])
CR("gullwing", "Gullwing", "tidesteed",
   "The tidesteed who slashed Julian Ashenford for bad manners — condemned to die, saved by a turnglass, and Dorian's getaway mount.",
   status=[st("alive", note="Condemned in the third book, rescued the same night.")],
   relations=[rel("pet-of", "bogdan-turl")])
CR("marlow", "Marlow", "storm-petrel",
   "Wren's storm-petrel and first name-day gift — her one constant at Prewitt Lane, killed in the flight that opens the seventh book.",
   status=[st("alive"),
           st("dead", at=sp("07-the-last-canto"),
              note="Killed by a stray canto during the flight of the seven decoys.")],
   relations=[rel("pet-of", "wren-alcott")])
CR("sootpaw", "Sootpaw", "cat (part-marsh-lynx)",
   "Odile's flat-faced, half-wild cat — the only one who knew the marsh-rat was lying all along.",
   relations=[rel("pet-of", "odile-sarkany")])
CR("grimalkin", "Grimalkin", "wraith-cat",
   "Skint's scrawny, tide-grey cat and second pair of eyes — petrified by the Leviathan, to the school's quiet delight.",
   status=[st("alive"),
           st("petrified", at=sp("02-the-sunken-warren")),
           st("restored", at=sp("02-the-sunken-warren"), note="Revived by tide-lily draught.")],
   relations=[rel("pet-of", "mabon-skint")])
CR("grievers", "Grievers", "non-being; joy-feeding wraith",
   "Hooded wraiths of the cold tide that feed on gladness and can drink a soul entirely — Bleakhold's keepers, until they defect to whichever side offers more despair.",
   aka=["Griever"], rating="uncontrollable")
CR("grindle-folk", "the grindle-folk", "being",
   "Bankers and master smiths with a long memory for a weaver's betrayals — to a grindle, the maker owns the made, whatever a weaver paid for it.",
   aka=["grindle"])
CR("hobs", "hobs", "being",
   "Bound servants of the old weaving houses, wielding fierce magic of their own — freed only by a gift of cloth.",
   aka=["hob"])
CR("tide-mares", "Tide-Mares", "beast",
   "Skeletal sea-horses of the sky, visible only to those who have watched someone die — unfairly ill-omened, unerring, and the Watch's ride to the Concord.",
   aka=["tide-mare"])
CR("fathomkin", "the fathomkin", "being",
   "Sea-cliff giants driven near to nothing by weavers; both sides court the last of them in both wars.",
   aka=["fathomkin"])
CR("reef-drakes", "reef-drakes", "beast, five-tide",
   "Sea-dragons of the reef, hoarded and hunted; a nesting drake is the first trial of the Trine Tournament.",
   aka=["reef-drake"])
CR("the-moon-touched", "the moon-touched", "human affliction",
   "Weavers cursed to change at the full tide-moon; feared and unhirable, which is rather the point of Rowan Thistle.",
   aka=["moon-touched"])
CR("dreadmimics", "dreadmimics", "non-being",
   "Shape-thieves that become whatever you most fear — no one knows what one looks like alone in the dark.",
   aka=["dreadmimic"])
CR("pyre-herons", "pyre-herons", "beast",
   "Grey shore-birds that die in flame and hatch again from their own ash; their tears mend, and their cry lends courage to the true of heart.",
   aka=["pyre-heron"])
CR("the-starkin", "the Starkin", "beast (by their own insistence)",
   "Proud, star-reading horse-folk of the Shrouded Wood, who name themselves beasts rather than share 'being' with weavers.",
   aka=["Starkin"])
CR("grindlewyrms", "grindlewyrms", "beast, five-tide",
   "Cliff-sized talking kraken-spiders with a taste for fresh meat; Old Fathom's brood rules a hollow of the Shrouded Wood.",
   aka=["grindlewyrm"])

# ===================== LOCATIONS =====================
def L(id, name, ltype, summary, aka=None, region="", status=None):
    f = {"type": ltype}
    if region:
        f["region"] = region
    E(id, name, "location", aliases=aka, summary=summary, fields=f, status=status)

L("thornfall-conservatory", "Thornfall Conservatory", "school",
  "The thousand-tide conservatory on a tidal isle — shifting stairs, singing portraits, and more secrets than any Headmaster has ever known.",
  aka=["Thornfall", "the Conservatory"], region="the Meridian Isles",
  status=[st("standing"),
          st("besieged", at=sp("07-the-last-canto"),
             note="Occupied under Dorn's headship, then half-drowned in the final siege.")])
L("hollowmere", "Hollowmere", "village",
  "The only all-weaver village on the Isles, a walk from Thornfall — the Salted Gull, the sweet-shop, and third-years with signed leave.",
  region="near Thornfall")
L("the-coilrow", "the Coilrow", "market street",
  "The Isles' hidden weaving market, entered through the Kelp and Kettle's back wall — lensets, robes, cauldrons, and a grindle bank.",
  aka=["Saltmarket"], region="Port Meridian")
L("the-blackwash", "the Blackwash", "street",
  "The Coilrow's crooked shadow, given to the dark arts — where a mispronounced tide-way lands twelve-year-olds.",
  region="Port Meridian")
L("gornhollow-bank", "Gornhollow Bank", "bank",
  "The grindle-run bank burrowed leagues beneath the harbour — the safest vault on the Isles, robbed exactly twice in the saga.",
  region="the Coilrow, Port Meridian")
L("thornbys-lensery", "Thornby's Lensery", "shop",
  "Makers of fine lensets since the founders' age; narrow, dusty, and the place where the lenset chooses the weaver.",
  region="the Coilrow, Port Meridian")
L("marsh-hollow", "Marsh Hollow", "family home",
  "The Marshes' impossibly stacked house above a tidal creek, held up by weaving and love; Wren's favourite place in the world.",
  region="the Saltmarsh Coast")
L("nine-prewitt-lane", "Number Nine, Prewitt Lane", "unwoven house",
  "The Prewitts' aggressively ordinary house, and the cupboard beneath its stairs — Wren's blood-ward anchor until her seventeenth name-day.",
  aka=["Prewitt Lane"], region="the mainland town of Dunmoor")
L("vell-house", "Vell House", "town-house",
  "The unplottable Vell family town-house — the Grey Dawn's headquarters under a hidden-keeping charm, complete with a shrieking portrait and a reliquary in a cabinet.",
  aka=["Vellmoor"], region="Port Meridian",
  status=[st("Grey Dawn headquarters", at=sp("05-the-drowned-court")),
          st("trio's hideout", at=sp("07-the-last-canto"),
             note="Abandoned after a Saltsworn slips in on a coat-tail.")])
L("the-concord-halls", "the Concord Halls", "government complex",
  "The weaver-government's tide-drowned warren beneath Port Meridian — the Atrium, the courtrooms, and the Vault of Tides at the bottom.",
  region="Port Meridian, below the waterline")
L("the-vault-of-tides", "the Vault of Tides", "research department",
  "The Concord's deepest floor: the Hall of Prophecy, the still-room of thought, the locked chamber of love, and the tide-veil that Dorian falls through.",
  region="the Concord Halls, the ninth deep")
L("bleakhold", "Bleakhold", "prison",
  "The weaver-prison on a drowned North reef, kept by Grievers who eat every glad thought — escape-proof until Dorian Vell.",
  region="the Northern Reefs")
L("the-shrouded-wood", "the Shrouded Wood", "forest",
  "The dark wood on Thornfall's grounds — Starkin, grindlewyrms, sea-unicorns, and every detention that goes wrong. Wren walks into it to die.",
  aka=["the Wood"], region="Thornfall grounds")
L("the-sunken-warren", "the Sunken Warren", "hidden chamber",
  "Sabine Vex's flooded warren beneath the school, opened only in tidespeech through a washroom drain — home to a thousand-tide Leviathan.",
  region="beneath Thornfall")
L("the-room-of-want", "the Room of Want", "shifting room",
  "The room that becomes whatever you truly need — the Watch's training-hall, the smugglers' cabinet-room, the resistance's dormitory, and the heap where a circlet hid for an age.",
  aka=["the Come-and-Go Room"], region="the ninth stair, Thornfall",
  status=[st("intact"),
          st("burned", at=sp("07-the-last-canto"), note="The hidden-things room is taken by wyrmfire.")])
L("the-wailing-boathouse", "the Wailing Boathouse", "abandoned building",
  "The most haunted building on the Isles — in truth a young moon-touched weaver's monthly refuge, reached by the strangle-willow's tunnel. Dorn dies here.",
  region="Hollowmere")
L("saltmere-village", "Saltmere", "village",
  "The tide-side village where the first weavers landed, the Hollis family broke, the Alcotts died, and their daughter's story began.",
  region="the Meridian Isles")
L("sablefen-shallows", "the Sablefen Shallows", "tidal flat",
  "The desolate tide-flat of the Mireholds — its bone-strewn shallows host the Drowned King's rebirth.",
  region="the Sablefen Coast")
L("mirehold-keep", "Mirehold Keep", "keep",
  "The Ashenfords' sea-cliff keep, seized as the Drowned King's court — its drawing-room hosts the war's darkest councils and its cellar the prisoners.",
  region="the Sablefen Coast",
  status=[st("the Drowned King's court", at=sp("07-the-last-canto"))])
L("shell-and-bone-cottage", "Shell-and-Bone Cottage", "cottage",
  "Cade and Amelie's clifftop home by the sea — refuge after Mirehold Keep, and the place where Pib is buried without weaving.",
  region="the western cliffs, near Tinmouth")
L("the-menders-hall", "the Menders' Hall", "hospital",
  "The Isles' weaver-infirmary, behind a shuttered shopfront — serpent-bites on the ground floor, unmendable canto-damage on the fourth.",
  region="Port Meridian")
L("pier-thirteen", "Pier Thirteen", "harbour pier",
  "Through the salt-mist between piers twelve and fourteen at Port Meridian — where every school year begins with a scarlet tide-ferry.",
  aka=["the Tideward Pier"], region="Port Meridian harbour")
L("the-kelp-and-kettle", "the Kelp and Kettle", "inn",
  "The grubby inn only weavers can see, the gateway between the mainland and the Coilrow.",
  region="Port Meridian")
L("the-salted-gull", "the Salted Gull", "inn",
  "Hollowmere's warm, crowded inn — spiced tide-ale with Mistress Rue, and half the plot's overheard conversations.",
  region="Hollowmere")
L("the-grey-gull", "the Grey Gull", "inn",
  "Hollowmere's dingy other inn, sawdust and goat-smell, kept by Gideon — where the Watch is founded, the prophecy was overheard, and the last tunnel into Thornfall opens.",
  region="Hollowmere")
L("the-beacon-spire", "the Beacon Spire", "tower",
  "Thornfall's tallest tower — midnight star-reading, a smuggled drake's departure, and the ramparts where Hollis falls.",
  aka=["the tide-struck spire"], region="Thornfall")

# ===================== ORGANIZATIONS =====================
def O(id, name, summary, leader="", hq="", purpose="", founded="", aka=None, status=None):
    f = {}
    if leader:
        f["leader"] = leader
    if hq:
        f["headquarters"] = hq
    if purpose:
        f["purpose"] = purpose
    if founded:
        f["founded"] = founded
    E(id, name, "organization", aliases=aka, summary=summary, fields=(f or None),
      status=status)

O("riptide-order", "the Riptide Order",
  "Order of the bold at heart — storm-blue and gold, the sea-hawk, and a tower with a tide-warden portrait on the door.",
  aka=["Riptide"], founded="by Aldric Storrow, the founders' age")
O("undertow-order", "the Undertow Order",
  "Order of the deep and cunning — green and pewter, the serpent, a drowned common-room under the reef, and more than its share of dark weavers.",
  aka=["Undertow"], founded="by Sabine Vex, the founders' age")
O("millpond-order", "the Millpond Order",
  "Order of the steadfast and patient — amber and slate, the otter, and the fewest dark weavers of any Order.",
  aka=["Millpond"], founded="by Halvard Denn, the founders' age")
O("springtide-order", "the Springtide Order",
  "Order of wit and brightness — pale blue and bronze, the heron, and a door that asks riddles instead of passwords.",
  aka=["Springtide"], founded="by Orla Wynn, the founders' age")
O("order-of-the-grey-dawn", "the Order of the Grey Dawn",
  "Hollis's secret society against the Drowned King, raised in the first war and re-formed the night of his return; headquartered at Vell House.",
  aka=["the Grey Dawn"], leader="Eamon Hollis", hq="Vell House",
  purpose="resistance against the Drowned King",
  status=[st("disbanded", note="Stood down after the first war."),
          st("re-formed", at=sp("04-the-trine-tournament"))])
O("the-saltsworn", "the Saltsworn",
  "The Drowned King's inner circle, branded with the brine-mark — lineage-supremacists who rule by terror in both wars.",
  leader="Corvane Mirehold", purpose="pure-lineage supremacy; the Drowned King's will",
  status=[st("scattered", note="Leaderless after Saltmere — imprisoned, hidden, or acquitted as 'mind-bound'."),
          st("re-formed", at=sp("04-the-trine-tournament"),
             note="Summoned to the shallows by the restored brine-mark."),
          st("defeated", at=sp("07-the-last-canto"))])
O("wrens-watch", "Wren's Watch",
  "The students' illegal warding club, founded in the Grey Gull under Inquisitor Crow's nose — coins that burn, a room that hides, and the core of Thornfall's resistance.",
  aka=["the Watch"], leader="Wren Alcott", hq="the Room of Want",
  founded="the fifth book",
  status=[st("founded", at=sp("05-the-drowned-court")),
          st("revived", at=sp("07-the-last-canto"),
             note="Re-formed under Tobin, Elsie, and Luthien inside occupied Thornfall.")])
O("tidal-concord", "the Tidal Concord",
  "The weaver-government of the Isles — bureaucratic in peace, obstructive in denial, and a puppet court after the coup.",
  aka=["the Concord"], leader="the High Warden", hq="beneath Port Meridian",
  status=[st("functioning"),
          st("in denial", at=sp("05-the-drowned-court"),
             note="Spends a year denying the Drowned King's return and smearing Wren and Hollis."),
          st("fallen", at=sp("07-the-last-canto"),
             note="Sallow murdered; Thicket installed under a binding-canto.")])
O("the-isles-herald", "the Isles Herald",
  "The Isles' newspaper of record — which mostly records whatever the Concord prefers.",
  aka=["the Herald"], purpose="news (and, periodically, propaganda)")
O("the-contrary-tide", "the Contrary Tide",
  "Alder Quess's broadsheet of sea-monsters and conspiracy — and, when it prints Wren's account, briefly the only honest paper on the Isles.",
  leader="Alder Quess", purpose="alternative press")
O("the-nightstray", "the Nightstray",
  "Tomas, Dorian, Rowan, and Silvo — three of them illegal skinshifts so a moon-touched friend need not change alone. Chartmakers, rule-breakers, and the generation the war broke.",
  aka=["the four skinshifts"], founded="at Thornfall, a generation before the saga",
  status=[st("broken", note="By Saltmere one was dead, one caged, one a spy, and one a rat.")])
O("the-wardens-watch", "the Wardens' Watch",
  "Inquisitor Crow's hand-picked student enforcers, licensed to strip points from prefects — Undertow with armbands.",
  leader="Delphine Crow",
  status=[st("active", at=sp("05-the-drowned-court")),
          st("disbanded", at=sp("06-the-reliquary"))])
O("the-tinkers-cove", "the Tinker's Cove",
  "Cade Marsh's curio-and-trick shop on the Coilrow, seeded by Wren's Trine winnings — the one bright shopfront on a darkening street.",
  leader="Cade Marsh", hq="the Coilrow, Port Meridian", founded="the sixth book")
O("the-salon-of-brix", "the Salon of Brix",
  "Volimar Brix's supper-club of the talented and well-connected — parties where influence is the whole curriculum.",
  leader="Volimar Brix")
O("kaldmarch-institute", "the Kaldmarch Institute",
  "The severe northern school that teaches the dark arts outright; arrives at the Trine Tournament by a ship of black ice.",
  aka=["Kaldmarch"], leader="Selvon Kord (books 1-4)")
O("aurelon-lyceum", "the Aurelon Lyceum",
  "The gilded southern academy whose delegation arrives by sea-drawn barge, led by the part-fathomkin Directrice Maroux.",
  aka=["Aurelon"], leader="Directrice Maroux")
O("deepkin-society", "the Deepkin Society",
  "Odile's two-shell campaign for hob rights, membership roughly three — mocked by everyone and right about everything.",
  leader="Odile Sarkany", founded="the fourth book")

# ===================== CONCEPTS =====================
def K(id, name, category, summary, aka=None):
    E(id, name, "concept", aliases=aka, summary=summary, fields={"category": category})

K("reliquary", "reliquary", "dark weaving",
  "An object drowned with a torn shard of a weaver's self, anchoring its maker to life — made by murder, the foulest weaving there is. The Drowned King made five on purpose, a sixth of his serpent, and a seventh by accident.",
  aka=["reliquaries", "soul-anchor"])
K("the-saltglass-relics", "the Saltglass Relics", "legend (true)",
  "The three gifts of the old tide-tale — Lens, Pearl, and Mantle. Unite them and you master death; Hollis chased them, the Drowned King only ever wanted the lens.",
  aka=["the Relics", "the Three Gifts"])
K("skinshift", "skinshift", "shaping",
  "A weaver who can become one particular beast at will — years of work, Concord registration required, and the saga is full of unregistered ones.",
  aka=["skinshifts"])
K("the-beacon", "the Beacon", "canto-work",
  "The guardian conjured by the beacon-canto, shaped by the singer's soul — it can change with grief or love, and the Grey Dawn uses beacons to carry word.",
  aka=["beacon-shape"])
K("tidespeech", "tidespeech", "rare gift",
  "The tongue of sea-serpents — hereditary in the Vex line, and a black mark on anyone who speaks it. Wren's gift is a splinter of another's self.",
  aka=["tidesinger"])
K("kestrel", "Kestrel", "sport",
  "The weaver-sport of the air, flown on wind-skiffs — the drift-ball through the rings for ten, the batter-gulls to the ribs, and a golden Flitwing worth one hundred and fifty and the game's end.",
  aka=["the skiff-game"])
K("the-unwoven", "the Unwoven", "lineage",
  "A person born without the gift of weaving — most of the world, kept carefully unaware of the rest of it.",
  aka=["Unwoven"])
K("dimglass", "Dimglass", "lineage",
  "Born to weavers but without the gift — the Isles' awkward secret, mostly living quietly among the Unwoven.",
  aka=["Dimglasses"])
K("dregblood", "dregblood", "slur / lineage politics",
  "A vile slur for an Unwoven-born weaver — lineage-supremacy in a single word; Odile wears the scar of it by the seventh book.")
K("tidestepping", "tidestepping", "travel",
  "Vanishing from one place to another by will — mark, mind, and measure. Licensed at seventeen; get it wrong and you leave part of yourself behind.",
  aka=["tidestep"])
K("gleaning", "gleaning", "mind-work",
  "The weaving-out of thoughts and memory — the mind is no open book, but the Drowned King and Dorn read them anyway.")
K("shuttering", "shuttering", "mind-work",
  "The defence against gleaning — sealing the mind behind shutters. Dorn tries to teach it to Wren, disastrously.")
K("the-forbidden-cantos", "the Forbidden Cantos", "dark weaving / law",
  "Mordath, Vexal, Thrallis — sing any one on a person and it is a life sentence in Bleakhold, a rule the war grinds to salt.",
  aka=["Forbidden Canto"])
K("the-trine-tournament", "the Trine Tournament", "event",
  "The revived contest of three schools' champions and three deadly trials — which acquires a fourth champion and ends in the shallows.")
K("the-ebb", "the Ebb", "travel",
  "Travel by tide-hearth and grey flame — speak clearly, keep your elbows in, and do not step out onto the Blackwash.",
  aka=["the tide-ways"])
K("tideknot", "tideknot", "travel",
  "Any humble object knotted to yank travellers elsewhere at a touch or an hour — an old buoy to the games, a chalice to the shallows.",
  aka=["tideknots"])
K("the-prophecy", "the Prophecy", "star-reading",
  "The one with the tide to unmake the Drowned King draws near... Sethra Cole's true reading, half-overheard and wholly self-fulfilling: Mirehold chose Wren, and in choosing, marked his equal.")
K("the-brine-mark", "the Brine-Mark", "dark weaving",
  "The Drowned King's brand — a serpent coiled through a drowned skull, burned into his sworn arms and raised over the houses of the dead.")
K("the-hollowing", "the Hollowing", "dark weaving / punishment",
  "The Grievers' ultimate act — it drinks the soul, leaving the body alive and empty. Worse than death, and the Concord uses it as a sentence.")
K("the-echo-canto", "the Echo", "lens-lore",
  "When kindred lensets duel, one forces the other to sing back its cantos in reverse; in the shallows, it brings the shades of the Drowned King's victims to Wren's side.",
  aka=["the Reverse-Song"])

# ===================== ARCS =====================
def A(id, name, summary, premise="", payoff=""):
    f = {}
    if premise:
        f["premise"] = premise
    if payoff:
        f["payoff"] = payoff
    E(id, name, "arc", summary=summary, fields=(f or None))

A("the-reliquary-hunt", "the Reliquary Hunt",
  "The slow discovery that the Drowned King cannot die while his reliquaries endure, and the trio's quest to unmake each in turn.",
  premise="The Drowned King has drowned his self in hidden objects.",
  payoff="The last reliquary falls and Mirehold is made mortal again.")
A("the-return-of-the-drowned-king", "the Return of the Drowned King",
  "Mirehold's long climb back from an unbodied shade to a crowned tyrant, and the Concord's fatal refusal to believe it.",
  premise="A shade that should be dead is gathering strength.",
  payoff="The Drowned King rises in the shallows and the second war begins.")
A("the-tidemasters-secret", "the Tidemaster's Secret",
  "Cassian Dorn's hidden loyalty — a former Saltsworn bound to Hollis by an old love, mistrusted to the last and vindicated only in death.",
  premise="Whose side is the tide-master truly on?",
  payoff="Dorn's memories reveal a lifetime of grief and a debt paid in full.")
A("the-marsh-estrangement", "the Marsh Estrangement",
  "Tam Marsh's break with his family over ambition and Concord loyalty, and his return in time for the last stand.",
  premise="A son chooses status over kin.",
  payoff="Tam comes home minutes before the siege and is forgiven.")
A("the-three-relics", "the Three Relics",
  "The old tide-tale of the Lens, the Pearl, and the Mantle — legend to most, obsession to Hollis, and the hinge of the final duel.",
  premise="Three objects that together master death.",
  payoff="Wren gathers all three, then gives up the Lens and the Pearl by choice.")

# ===================== EMIT =====================
ids = {e["id"] for e in ENTRIES}
assert len(ids) == len(ENTRIES), "duplicate id!"
problems = []
for e in ENTRIES:
    for r in e.get("relations", []) or []:
        if r["to"] not in ids:
            problems.append(f"{e['id']}: relation {r['type']} -> unknown {r['to']}")
        for key in ("from", "until"):
            if key in r and r[key]["book"] not in BOOK_IDS:
                problems.append(f"{e['id']}: {key} book {r[key]['book']}")
    for s in e.get("status", []) or []:
        if "at" in s and s["at"]["book"] not in BOOK_IDS:
            problems.append(f"{e['id']}: status at book {s['at']['book']}")
if problems:
    print("VALIDATION FAILED:")
    print("\n".join(problems))
    raise SystemExit(1)

# type -> count
from collections import Counter
counts = Counter(e["type"] for e in ENTRIES)

# Clean only the generated artifacts — never ROOT itself (this script lives
# in it). Regenerating the example is idempotent.
for sub in ("codex", "books"):
    p = os.path.join(ROOT, sub)
    if os.path.isdir(p):
        shutil.rmtree(p)
for f in ("novelide.yaml", "codex-schema.yaml", "series-plan.yaml"):
    fp = os.path.join(ROOT, f)
    if os.path.exists(fp):
        os.remove(fp)


def dump(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        yaml.dump(obj, f, sort_keys=False, allow_unicode=True, width=100, default_flow_style=False)


# manifest
dump(f"{ROOT}/novelide.yaml",
     {"name": "The Saltglass Chronicles", "kind": "series",
      "books": [b for b, _ in BOOKS]})

# schema (generic types + relations, same as the app default)
schema = {
    "types": [
        {"id": "character", "label": "Character", "icon": "👤",
         "fields": ["order", "lineage", "focus", "beacon", "occupation", "born"]},
        {"id": "location", "label": "Location", "icon": "🗺",
         "fields": ["region", "type"]},
        {"id": "spell", "label": "Canto", "icon": "🎵",
         "fields": ["incantation", "type", "effect", "light"]},
        {"id": "potion", "label": "Draught", "icon": "⚗️",
         "fields": ["effect", "appearance", "difficulty"]},
        {"id": "item", "label": "Object & Relic", "icon": "🪞",
         "fields": ["type", "powers", "maker"]},
        {"id": "creature", "label": "Creature & Being", "icon": "🐉",
         "fields": ["classification", "ministry-rating"]},
        {"id": "organization", "label": "Order & Faction", "icon": "🏛",
         "fields": ["leader", "headquarters", "purpose", "founded"]},
        {"id": "arc", "label": "Arc / Thread", "icon": "🧵",
         "fields": ["premise", "payoff"]},
        {"id": "concept", "label": "Concept", "icon": "✦", "fields": ["category"]},
    ],
    "relations": [
        {"id": "parent-of", "label": "parent of", "inverseLabel": "child of"},
        {"id": "sibling-of", "label": "sibling of", "symmetric": True},
        {"id": "married-to", "label": "married to", "symmetric": True},
        {"id": "loves", "label": "loves", "inverseLabel": "loved by"},
        {"id": "godparent-of", "label": "godparent of", "inverseLabel": "godchild of"},
        {"id": "mentor-of", "label": "mentor of", "inverseLabel": "student of"},
        {"id": "teaches", "label": "teaches at", "inverseLabel": "employs"},
        {"id": "allied-with", "label": "allied with", "symmetric": True},
        {"id": "enemy-of", "label": "enemy of", "symmetric": True},
        {"id": "member-of", "label": "member of", "inverseLabel": "has member"},
        {"id": "leads", "label": "leads", "inverseLabel": "led by"},
        {"id": "serves", "label": "serves", "inverseLabel": "served by"},
        {"id": "owns", "label": "owns", "inverseLabel": "owned by"},
        {"id": "created", "label": "created", "inverseLabel": "created by"},
        {"id": "killed", "label": "killed", "inverseLabel": "killed by"},
        {"id": "located-in", "label": "located in", "inverseLabel": "contains"},
        {"id": "pet-of", "label": "pet / companion of", "inverseLabel": "keeps"},
    ],
}
dump(f"{ROOT}/codex-schema.yaml", schema)

# Seeded manuscript chapters — original prose planted with deliberate
# continuity mistakes so the detection features light up when opened:
#   * dead characters shown acting (dead-entity-agency errors)
#   * facts the codex doesn't record (death / kill / marriage / new-entity
#     suggestions, kinship + appearance field suggestions)
#   * an appearance value that contradicts the codex (field-contradiction)
#   * a few misspellings (spellchecker)
MANUSCRIPTS = {
    "05-the-drowned-court": [
        ("01-the-empty-chair.md",
         "# The Empty Chair\n\n"
         "The Room of Want was colder than Wren remembered.\n\n"
         "Dorian Vell pushed the door open and shook the rain from his coat. "
         "\"You all waited for me,\" he said, and dropped into the nearest chair.\n\n"
         "Wren managed a tired smile. She had not slept since Halden Brooke's "
         "funeral, and the grief still sat behind her ribs like a swallowed stone.\n\n"
         "\"We have news,\" said Corin Sedgewick, mud to his knees. It was Corin "
         "Sedgewick who had ridden hardest from the coast, and his face was grey "
         "with it. \"They are saying Vespera Locke killed Perrin Marsh at the crossing.\"\n\n"
         "Wren did not want to beleive it. Perrin had promised her he would be carefull.\n\n"
         "Odile Sarkany's hair was fire-red in the lamplight as she leaned over the "
         "map, and for a long moment no one dared to speak.\n"),
    ],
    "07-the-last-canto": [
        ("01-after-the-spire.md",
         "# After the Spire\n\n"
         "Thornfall had never felt so empty.\n\n"
         "Eamon Hollis walked the length of the gallery, his hands folded behind "
         "him, and watched the storm break against the tall windows. \"You should "
         "sleep, Wren,\" he said gently.\n\n"
         "Wren only shook her head. Her hair was pale gold, and her grey eyes "
         "were rimmed with red.\n\n"
         "Downstairs, Perrin Marsh had finally married Odile Sarkany, and for one "
         "evening the old tower was loud with music and light. Cade Marsh was "
         "Perrin's brother, and he had carried up enough spiced ale for twice the "
         "company.\n\n"
         "But it was a seperate grief that kept Wren at the window — a grief that "
         "had its own tide, and would not ebb before morning.\n"),
    ],
}

# books
for bid, title in BOOKS:
    dump(f"{ROOT}/books/{bid}/book.yaml", {"title": title})
    os.makedirs(f"{ROOT}/books/{bid}/manuscript", exist_ok=True)
    for fn, content in MANUSCRIPTS.get(bid, []):
        with open(f"{ROOT}/books/{bid}/manuscript/{fn}", "w") as fh:
            fh.write(content)

# codex entries
for e in ENTRIES:
    body = {k: v for k, v in e.items() if k != "type" or True}
    # keep 'type' inside the file too (loader tolerates it)
    dump(f"{ROOT}/codex/{e['type']}/{e['id']}.yaml", e)

# series plan
dump(f"{ROOT}/series-plan.yaml", {
    "synopsis": ("The Saltglass Chronicles: an orphan marked by a dark weaver's broken "
                 "canto rises through Thornfall Conservatory to unmake the Drowned King, "
                 "one drowned shard of his soul at a time."),
    "books": [
        {"id": "01-the-marked-tide",
         "synopsis": "Wren learns she is a weaver and uncovers a reliquary hidden in Thornfall's tide-engine.",
         "status": "final", "arcs": ["the-return-of-the-drowned-king"], "targetWords": 80000},
        {"id": "02-the-sunken-warren",
         "synopsis": "The Leviathan wakes beneath the school; the first reliquary is unmade.",
         "status": "final", "arcs": ["the-reliquary-hunt"], "targetWords": 85000},
        {"id": "03-the-sablefen-pact",
         "synopsis": "Wren's wrongly-blamed godfather resurfaces and dies saving her.",
         "status": "revised", "arcs": ["the-return-of-the-drowned-king"], "targetWords": 95000},
        {"id": "04-the-trine-tournament",
         "synopsis": "The three schools compete; the Drowned King engineers his full return; the first champion falls.",
         "status": "revised", "arcs": ["the-return-of-the-drowned-king"], "targetWords": 110000},
        {"id": "05-the-drowned-court",
         "synopsis": "The Concord denies the war; a purge takes the deputy head; Wren's Watch is born.",
         "status": "drafted", "arcs": ["the-marsh-estrangement"], "targetWords": 130000},
        {"id": "06-the-reliquary",
         "synopsis": "The soul-anchor secret is laid bare and Headmaster Hollis dies by arrangement.",
         "status": "drafted", "arcs": ["the-reliquary-hunt", "the-tidemasters-secret"], "targetWords": 120000},
        {"id": "07-the-last-canto",
         "synopsis": "The siege of Thornfall; the last reliquary falls and the Drowned King with it.",
         "status": "outlined", "arcs": ["the-reliquary-hunt", "the-three-relics", "the-tidemasters-secret"], "targetWords": 150000},
    ],
})

print(f"Wrote {len(ENTRIES)} codex entries to {ROOT}")
for t, n in sorted(counts.items()):
    print(f"  {t}: {n}")

