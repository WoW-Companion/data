#!/usr/bin/env python3
"""Extract profession leveling route facts from the WoW Forever guides on wow-professions.com.

Usage: extract_routes.py <dir holding <slug>.html, saved from the guide URLs> > curated/profession_routes.json

Only facts are kept: skill ranges, recipe names, craft counts, per-craft material counts, vendor
and trainer names, level and cost requirements. Notes are our own short words; no page prose is
copied. The Go build (internal/tables/profroutes.go) resolves the names to dataset ids.

The main route (option 0) is what the page lists first for each skill range; a later block that
covers a range already covered is an alternative (option 1, 2, ...), linked by alternative_of to
the main step it replaces. Where the page lists first a route that needs a rare world-drop plan
and its own fallback second, PROMOTE puts the always-available fallback on the main route.
"""
import html, json, os, re, sys, datetime

BASE = "https://www.wow-professions.com/forever/%s-leveling-guide"
PROFS = [("Alchemy", "alchemy"), ("Blacksmithing", "blacksmithing"), ("Enchanting", "enchanting"),
         ("Engineering", "engineering"), ("Herbalism", "herbalism"), ("Leatherworking", "leatherworking"),
         ("Mining", "mining"), ("Skinning", "skinning"), ("Tailoring", "tailoring"),
         ("Cooking", "cooking"), ("First Aid", "first-aid"), ("Fishing", "fishing")]
CRAFTERS = {"Alchemy", "Blacksmithing", "Enchanting", "Engineering", "Leatherworking", "Tailoring", "Cooking", "First Aid"}
HORDE_CITIES = {"Orgrimmar", "Undercity", "Thunder Bluff", "Tirisfal Glades", "Durotar", "Mulgore"}

RANK_BY_CAP = [(225, "Artisan", 35), (150, "Expert", 20), (75, "Journeyman", 10)]
MIN_LEVEL = {"Journeyman": 10, "Expert": 20, "Artisan": 35}

# Routes the page lists first that need a world-drop plan; the guide's own always-available
# fallback (recipes learned from a quest) goes on the main route instead.
PROMOTE = {"Blacksmithing": ["Imperial Plate Boots"], "First Aid": ["Linen Bandage"]}

# Where a recipe is learned when it is not simply from the profession trainer: (profession,
# recipe) -> (kind, [(npc, place)], short note). Read from the guides' "sold by" and "world
# drop" lines next to each step.
V = lambda *a: [{"name": n, "place": p} for n, p in a]
LEARNED = {
    ("Alchemy", "Nature Protection Potion"): ("vendor", V(("Bronk", "Feralas"), ("Logannas", "Feralas"), ("Glyx Brewright", "Stranglethorn Vale"), ("Alchemist Pestlezugg", "Tanaris")), "recipe binds when picked up"),
    ("Alchemy", "Elixir of Greater Defense"): ("vendor", V(("Kor'geld", "Orgrimmar"), ("Soolie Berryfizz", "Ironforge")), "sold as Recipe: Elixir of Superior Defense"),
    ("Alchemy", "Superior Mana Potion"): ("vendor", V(("Algernon", "Undercity"), ("Ulthir", "Darnassus")), ""),
    ("Blacksmithing", "Mithril Spurs"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Dazzling Mithril Rapier"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Runed Mithril Hammer"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Thorium Bracers"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Thorium Belt"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Radiant Boots"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Radiant Leggings"): ("drop", [], "plans are a world drop; buy on the AH"),
    ("Blacksmithing", "Thorium Leggings"): ("item", [], "plans bought on the AH; learned at 275"),
    ("Blacksmithing", "Imperial Plate Belt"): ("quest", V(("Derotain Mudsipper", "Tanaris")), "quest needs level 50 and 20 Thorium Bar"),
    ("Blacksmithing", "Imperial Plate Bracers"): ("quest", V(("Derotain Mudsipper", "Tanaris")), "quest needs level 50 and 20 Thorium Bar"),
    ("Blacksmithing", "Imperial Plate Boots"): ("quest", V(("Derotain Mudsipper", "Tanaris")), "quest needs 100 Thorium Bar"),
    ("Blacksmithing", "Imperial Plate Leggings"): ("quest", V(("Derotain Mudsipper", "Tanaris")), "quest needs 100 Thorium Bar"),
    ("Enchanting", "Enchant 2H Weapon - Lesser Intellect"): ("vendor", V(("Kithas", "Orgrimmar"), ("Nata Dawnstrider", "Thunder Bluff"), ("Leo Sarn", "Silverpine Forest"), ("Tilli Thistlefuzz", "Ironforge")), "formula, 5 silver"),
    ("Enchanting", "Minor Mana Oil"): ("vendor", V(("Kithas", "Orgrimmar"), ("Nata Dawnstrider", "Thunder Bluff"), ("Thaddeus Webb", "Undercity"), ("Vaean", "Darnassus"), ("Tilli Thistlefuzz", "Ironforge"), ("Jessara Cordell", "Stormwind City")), "formula, 30 silver"),
    ("Enchanting", "Lesser Wizard Oil"): ("vendor", [], "formula, 40 silver, from the Enchanting Supplies vendor in every capital"),
    ("Enchanting", "Lesser Mana Oil"): ("vendor", V(("Kania", "Silithus")), "formula, 1 gold"),
    ("Enchanting", "Wizard Oil"): ("vendor", V(("Kania", "Silithus")), "formula, 2 gold"),
    ("Enchanting", "Enchant Shield - Greater Stamina"): ("vendor", V(("Daniel Bartlett", "Undercity"), ("Mythrin'dir", "Darnassus")), "formula binds when picked up, 1 gold 20 silver"),
    ("Enchanting", "Runed Arcanite Rod"): ("vendor", V(("Lorelae Wintersong", "Moonglade")), "formula binds when picked up, 2 gold 20 silver"),
    ("Engineering", "Thorium Widget"): ("vendor", V(("Sovik", "Orgrimmar"), ("Gearcutter Cogspinner", "Ironforge")), "schematic"),
    ("Engineering", "Thorium Grenade"): ("vendor", V(("Sovik", "Orgrimmar"), ("Gearcutter Cogspinner", "Ironforge")), "schematic"),
    ("Engineering", "Thorium Tube"): ("vendor", V(("Xizzer Fizzbolt", "Winterspring"), ("Kor'gar", "Riverglades")), "schematic binds when picked up"),
    ("Engineering", "Thorium Shells"): ("drop", [], "schematic is a world drop"),
    ("Leatherworking", "Wicked Leather Gauntlets"): ("vendor", V(("Werg Thickblade", "Tirisfal Glades"), ("Leonard Porter", "Western Plaguelands")), "pattern"),
    ("Leatherworking", "Runic Leather Headband"): ("vendor", V(("Jase Farlane", "Eastern Plaguelands")), "pattern"),
    ("Leatherworking", "Barbaric Leggings"): ("vendor", V(("Jandia", "Freewind Post"), ("Lardan", "Astranaar")), "pattern"),
    ("Tailoring", "Runecloth Gloves"): ("vendor", V(("Qia", "Winterspring")), "pattern"),
    ("Cooking", "Spotted Yellowtail"): ("vendor", V(("Gikkix", "Steamwheedle Port")), "recipe; also bought on leaving Tanaris"),
    ("Cooking", "Nightfin Soup"): ("vendor", V(("Gikkix", "Steamwheedle Port")), "recipe"),
    ("Cooking", "Bristle Whisker Catfish"): ("vendor", V(("Derak Nightfall", "Hillsbrad Foothills"), ("Naal Mistrunner", "Thunder Bluff"), ("Ronald Burch", "Undercity"), ("Lindea Rabonne", "Hillsbrad Foothills"), ("Catherine Leland", "Stormwind City"), ("Kilxx", "The Barrens")), "recipe"),
    ("First Aid", "Heavy Silk Bandage"): ("vendor", V(("Deneb Walker", "Arathi Highlands"), ("Balai Lok'Wein", "Dustwallow Marsh")), "manual, sold with the Expert book"),
    ("First Aid", "Mageweave Bandage"): ("quest", V(("Doctor Gregory Victor", "Arathi Highlands")), "taught by the Triage quest doctor"),
    ("First Aid", "Heavy Mageweave Bandage"): ("quest", V(("Doctor Gregory Victor", "Arathi Highlands")), "taught by the Triage quest doctor at 240"),
    ("First Aid", "Runecloth Bandage"): ("quest", V(("Doctor Gregory Victor", "Arathi Highlands")), "taught by the Triage quest doctor at 260"),
    ("First Aid", "Heavy Runecloth Bandage"): ("quest", V(("Doctor Gregory Victor", "Arathi Highlands")), "taught by the Triage quest doctor at 290"),
    ("First Aid", "Major Healing Potion"): ("vendor", V(("Pexmit", "Mount Hyjal")), "manual, 8 gold"),
}

# Extra short notes in our own words, per (profession, recipe).
NOTES = {
    ("Alchemy", "Elixir of Greater Defense"): "keep the Stonescale Oil from the earlier step",
    ("Blacksmithing", "Mithril Spurs"): "cheaper than the main route if the plans are on the AH",
    ("Enchanting", "Runed Copper Rod"): "copper rod and Mote of Magic come from the Enchanting supply vendor",
    ("Enchanting", "Enchant Bracer - Inferior Stamina"): "disenchanting also gives skill points until about 60",
    ("Engineering", "Thorium Grenade"): "PvP-ruleset variant of the widget step",
    ("Cooking", "Roast Raptor"): "usually the cheaper of the two",
    ("First Aid", "Major Healing Potion"): "only if you want sellable potions; stop the bandages at 275",
}

# Trainer-tier facts the guides give: (profession, tier) -> list of dicts.
TIER_EXTRA = {
    ("Alchemy", "Expert"): dict(cost=5000, trainers=[("Doctor Herbert Halsey", "Undercity", "Horde"), ("Ainethil", "Darnassus", "Alliance"), ("Master Mycologist Shurome", "Emerald Sanctuary, Felwood", "Both")]),
    ("Alchemy", "Artisan"): dict(skill=200, trainers=[("Rogvar", "Stonard, Swamp of Sorrows", "Horde"), ("Kylanna Windwhisper", "Feathermoon Stronghold, Feralas", "Alliance"), ("Master Mycologist Shurome", "Emerald Sanctuary, Felwood", "Both")]),
    ("Blacksmithing", "Expert"): dict(cost=5000, trainers=[("Saru Steelfury", "Orgrimmar", "Horde"), ("Bengus Deepforge", "Ironforge", "Alliance")]),
    ("Blacksmithing", "Artisan"): dict(trainers=[("Brikk Keencraft", "Booty Bay, Stranglethorn Vale", "Both")]),
    ("Enchanting", "Expert"): dict(cost=5000, trainers=[("Hgarth", "Sun Rock Retreat, Stonetalon Mountains", "Horde"), ("Kitta Firewind", "Tower of Azora, Elwynn Forest", "Alliance")]),
    ("Enchanting", "Artisan"): dict(skill=200, cost=50000, trainers=[("Annora", "Uldaman", "Both")]),
    ("Engineering", "Expert"): dict(cost=5000, trainers=[("Roxxik", "Orgrimmar", "Horde"), ("Springspindle Fizzlegear", "Ironforge", "Alliance")]),
    ("Engineering", "Artisan"): dict(trainers=[("Buzzek Bracketswing", "Gadgetzan, Tanaris", "Both"), ("Kaylaena Springwhisper", "Mount Hyjal", "Both")]),
    ("Leatherworking", "Expert"): dict(cost=5000, trainers=[("Una", "Thunder Bluff", "Horde"), ("Telonis", "Darnassus", "Alliance")]),
    ("Leatherworking", "Artisan"): dict(trainers=[("Hahrana Ironhide", "Camp Mojache, Feralas", "Horde"), ("Drakk Stonehand", "Aerie Peak, The Hinterlands", "Alliance")]),
    ("Tailoring", "Expert"): dict(cost=5000, trainers=[("Josef Gregorian", "Undercity", "Horde"), ("Georgio Bolero", "Stormwind City", "Alliance")]),
    ("Tailoring", "Artisan"): dict(trainers=[("Daryl Stack", "Tarren Mill, Hillsbrad Foothills", "Horde"), ("Timothy Worthington", "Theramore Isle, Dustwallow Marsh", "Alliance")]),
    ("Cooking", "Expert"): dict(skill=125, how="book", trainers=[("Wulan", "Shadowprey Village, Desolace", "Horde"), ("Shandrina", "Mystral Lake, Ashenvale", "Alliance")], note="Expert Cookbook"),
    ("Cooking", "Artisan"): dict(skill=225, how="quest", trainers=[("Dirge Quikcleave", "Gadgetzan, Tanaris", "Both")], note="Clamlette Surprise quest"),
    ("First Aid", "Expert"): dict(skill=125, how="book", trainers=[("Deneb Walker", "Arathi Highlands", "Alliance"), ("Balai Lok'Wein", "Dustwallow Marsh", "Horde")], note="Expert First Aid - Under Wraps book"),
    ("First Aid", "Artisan"): dict(skill=225, how="quest", trainers=[("Doctor Gregory Victor", "Hammerfall, Arathi Highlands", "Both")], note="Triage quest"),
    ("Fishing", "Journeyman"): dict(skill=50),
    ("Fishing", "Expert"): dict(skill=125, how="book", trainers=[("Old Man Heming", "Booty Bay, Stranglethorn Vale", "Both")], note="Expert Fishing - The Bass and You book"),
    ("Fishing", "Artisan"): dict(skill=225, how="quest", trainers=[("Nat Pagle", "Dustwallow Marsh", "Both")], note="Nat Pagle, Angler Extreme quest"),
    ("Herbalism", "Journeyman"): dict(skill=50), ("Herbalism", "Expert"): dict(skill=125), ("Herbalism", "Artisan"): dict(skill=200),
    ("Mining", "Journeyman"): dict(skill=50), ("Mining", "Expert"): dict(skill=125), ("Mining", "Artisan"): dict(skill=200),
    ("Skinning", "Journeyman"): dict(skill=50), ("Skinning", "Expert"): dict(skill=125), ("Skinning", "Artisan"): dict(skill=200),
    ("Cooking", "Journeyman"): dict(skill=50), ("Alchemy", "Journeyman"): dict(), ("Enchanting", "Journeyman"): dict(),
}
# Skill at which each tier is trained, for the tiers the guides do not give: the next tier opens
# when the previous one is capped.
TIER_SKILL = {"Journeyman": 50, "Expert": 125, "Artisan": 200}
TIER_CAP = {"Apprentice": (1, 75), "Journeyman": (50, 150), "Expert": (125, 225), "Artisan": (200, 300)}


def text_of(path):
    t = open(path, encoding="utf-8").read()
    t = re.sub(r"<script.*?</script>|<style.*?</style>", "", t, flags=re.S)
    t = re.sub(r"<(br|/p|/li|/tr|/h\d|/div|/td|/th)[^>]*>", "\n", t)
    t = re.sub(r"<[^>]+>", " ", t)
    t = html.unescape(t)
    t = re.sub(r"[ \t]+", " ", t)
    return [l.strip() for l in t.split("\n") if l.strip()]


def parse_mats(s):
    s = re.sub(r"\([^)]*\)", "", s).strip().rstrip(".").strip()
    out = []
    for part in re.split(r"\s*,\s*", s):
        m = re.match(r"^(\d+)x? (.+?)$", part.strip())
        if not m:
            return None
        out.append((m.group(2).strip(), int(m.group(1))))
    return out


def per_craft(mats, crafts):
    res, approx = [], False
    for name, tot in mats:
        note = None
        if " or " in name:
            first, rest = name.split(" or ", 1)
            name, note = first, "or " + rest
        c = tot // crafts if tot % crafts == 0 else max(1, round(tot / crafts))
        approx = approx or tot % crafts != 0
        d = {"item": name, "count": c}
        if note:
            d["note"] = note
        res.append(d)
    return res, approx


TIER = re.compile(r"^(Apprentice|Journeyman|Expert|Artisan) (.+?) \((\d+)-(\d+)\)$")
RANGE = re.compile(r"^(\d+)-(\d+)$")
CRAFT = re.compile(r"^(\d+)x (.+)$")
MATSLINE = re.compile(r"^\d+x? [A-Za-z]")
LEAD = re.compile(r"^(Alternative recipes?|Alternative:?$|If .*(make|use) .*(instead|these)|.*, make these instead)", re.I)
VEND_LINE = re.compile(r"^([A-Z][\w' .-]+): ([A-Z][\w' .-]+)$")


def rank_for(frm, to):
    """The tier a step needs: points above 75, 150 or 225 need Journeyman, Expert or Artisan."""
    top = to - 1
    for cap, name, _ in RANK_BY_CAP:
        if top >= cap:
            return name
    return "Apprentice"


def craft_profession(prof, lines):
    steps, journey = [], []
    tier = None
    rng = None
    covered = 0       # highest skill the main route reaches
    option = 0
    nopt = 0
    alt_last_to = None
    lead = choose = False
    table = False
    i = 0
    while i < len(lines):
        l = lines[i]
        m = TIER.match(l)
        if m and m.group(2).lower() == prof.lower():
            tier = m.group(1)
            rng = (int(m.group(3)), int(m.group(4)))
            lead = choose = table = False
            if tier == "Journeyman":
                j = i + 1
                while j < len(lines) and not RANGE.match(lines[j]) and not CRAFT.match(lines[j]) and not TIER.match(lines[j]):
                    mm = VEND_LINE.match(lines[j])
                    if mm and "trainers" not in lines[j]:
                        journey.append((mm.group(2).split(" at ")[0].split(" in ")[0], mm.group(1)))
                    j += 1
            i += 1
            continue
        if tier is None:
            i += 1
            continue
        if "from one of these two" in l or l.startswith("Make one of these"):
            choose = True
            i += 1
            continue
        if LEAD.match(l):
            lead = True
            i += 1
            continue
        if l == "Recipe Source":
            table = True
            i += 1
            continue
        m = RANGE.match(l)
        if m:
            frm, to = int(m.group(1)), int(m.group(2))
            rng = (frm, to)
            choose = False
            if frm >= covered:
                option = 0
                alt_last_to = None
            elif option > 0 and frm == alt_last_to and not lead:
                pass
            else:
                nopt += 1
                option = nopt
                lead = False
            i += 1
            continue
        m = CRAFT.match(l)
        if m and rng:
            crafts = int(m.group(1))
            rest = m.group(2).rstrip(".")
            name, mats = rest, None
            if " - " in rest:
                a, b = rest.rsplit(" - ", 1)
                if MATSLINE.match(b):
                    pm = parse_mats(b)
                    if pm:
                        name, mats = a, pm
            if mats is None:
                j = i + 1
                if j < len(lines) and MATSLINE.match(lines[j]):
                    pm = parse_mats(lines[j].replace(" , ", ", "))
                    if pm:
                        mats = pm
                        i = j
            if mats is None or crafts < 1:
                i += 1
                continue
            main = True
            if lead and rng[1] <= covered:
                nopt += 1
                option = nopt
                alt_last_to = None
                main = False
            lead = False
            if option > 0:
                main = False
            elif choose and any(s["option"] == 0 and s["skill_from"] == rng[0] and s["skill_to"] == rng[1] for s in steps):
                nopt += 1
                option = nopt
                main = False
            if main:
                covered = max(covered, rng[1])
            else:
                alt_last_to = rng[1]
            vend = []
            if table:
                j = i + 1
                while j < len(lines) and VEND_LINE.match(lines[j]) and not RANGE.match(lines[j]) and not CRAFT.match(lines[j]):
                    vend.append(lines[j]); j += 1
                i = j - 1
            recipe_names = name.split(" or ")
            pm, approx = per_craft(mats, crafts)
            st = {"option": option, "tier": rank_for(rng[0], rng[1]), "skill_from": rng[0], "skill_to": rng[1],
                  "recipe": recipe_names[0].strip(), "crafts": crafts, "materials": pm, "_approx": approx}
            if len(recipe_names) > 1:
                st["_or"] = recipe_names[1].strip()
            if vend:
                st["_vend"] = [dict(zip(("place", "name"), v.split(": ", 1))) for v in vend]
            steps.append(st)
            if option > 0 and choose and main is False:
                pass
            i += 1
            continue
        i += 1
    return steps, journey


def promote(prof, steps):
    for recipe in PROMOTE.get(prof, []):
        tgt = [s for s in steps if s["recipe"] == recipe]
        if not tgt:
            continue
        opt = tgt[0]["option"]
        grp = [s for s in steps if s["option"] == opt]
        lo, hi = min(s["skill_from"] for s in grp), max(s["skill_to"] for s in grp)
        newopt = max(s["option"] for s in steps) + 1
        for s in steps:
            if s["option"] == 0 and s["skill_from"] >= lo and s["skill_to"] <= hi:
                s["option"] = newopt
        for s in grp:
            s["option"] = 0
    return steps


def finish_craft(prof, steps, url, fetched):
    steps = promote(prof, steps)
    # keep the page order, but the main route first within the table's own step numbering
    for n, s in enumerate(steps, 1):
        s["step"] = n
    main = [s for s in steps if s["option"] == 0]
    for s in steps:
        s["kind"] = "craft"
        if s["option"] > 0:
            best, bo = None, 0
            for p in main:
                o = min(p["skill_to"], s["skill_to"]) - max(p["skill_from"], s["skill_from"])
                if o > bo:
                    best, bo = p, o
            if best:
                s["alternative_of"] = best["step"]
        kind, vend, lnote = LEARNED.get((prof, s["recipe"]), (None, None, None))
        if kind is None and s.get("_vend"):
            kind, vend, lnote = "vendor", [{"name": v["name"], "place": v["place"]} for v in s["_vend"]], "recipe"
        s["learned_from"] = kind or "trainer"
        if vend:
            s["learned_by"] = vend
        notes = []
        if lnote:
            notes.append(lnote)
        if s["tier"] != "Apprentice":
            notes.append("needs %s, level %d" % (s["tier"], MIN_LEVEL[s["tier"]]) if prof not in ("Cooking", "First Aid") else "needs %s" % s["tier"])
        if s["recipe"].startswith("Runed ") and s["recipe"].endswith(" Rod"):
            notes.append("rod is Blacksmith-made; buy on the AH")
        if s.get("_or"):
            notes.append("the guide also names %s" % s["_or"])
        if s.get("_approx"):
            notes.append("per-craft counts rounded from the guide's totals")
        if any("note" in m for m in s["materials"]):
            notes.append("; ".join("%s %s" % (m["item"], m["note"]) for m in s["materials"] if "note" in m))
        extra = NOTES.get((prof, s["recipe"]))
        if extra:
            notes.append(extra)
        s["notes"] = "; ".join(dict.fromkeys(n for n in notes if n))
        for k in [k for k in s if k.startswith("_")]:
            del s[k]
        s["source_url"] = url
    return steps


def renumber(steps):
    """Order steps by skill range (main route first at a tie) and renumber, keeping alternative_of."""
    old = {s["step"]: s for s in steps}
    order = sorted(steps, key=lambda s: (s["skill_from"], s["option"], s["step"]))
    new = {id(s): n for n, s in enumerate(order, 1)}
    for s in order:
        if "alternative_of" in s:
            s["alternative_of"] = new[id(old[s["alternative_of"]])]
    for s in order:
        s["step"] = new[id(s)]
    return order


def step_row(prof, n, opt, tier, frm, to, kind, **kw):
    d = {"step": n, "option": opt, "kind": kind, "tier": tier, "skill_from": frm, "skill_to": to}
    d.update(kw)
    return d


def gather_steps(prof, url):
    """Hand-read from the guides' range headings: zones (and mobs) are facts, not copied text."""
    G = []
    def g(frm, to, items=(), where=(), notes="", opt=0, alt=None, kind="gather", **kw):
        tier = rank_for(frm, to)
        s = step_row(prof, len(G) + 1, opt, tier, frm, to, kind, **kw)
        s["materials"] = [{"item": i, "count": 0} for i in items]
        s["where"] = list(where)
        s["learned_from"] = "trainer"
        s["notes"] = "; ".join(x for x in [notes, ("needs %s, level %d" % (tier, MIN_LEVEL[tier])) if tier != "Apprentice" else ""] if x)
        if alt:
            s["alternative_of"] = alt
        s["source_url"] = url
        G.append(s)
        return s["step"]
    if prof == "Herbalism":
        g(1, 70, ["Peacebloom", "Silverleaf", "Earthroot"], ["Durotar", "Mulgore", "Tirisfal Glades", "Elwynn Forest", "Dun Morogh", "Teldrassil", "Zephras Isle"], "any starting zone; Earthroot needs skill 15")
        g(70, 115, ["Mageroyal", "Briarthorn", "Stranglekelp"], ["Silverpine Forest", "Loch Modan", "Darkshore"], "Stranglekelp needs skill 85")
        g(115, 170, ["Bruiseweed", "Wild Steelbloom", "Stranglekelp", "Kingsblood", "Liferoot"], ["Hillsbrad Foothills", "Stonetalon Mountains", "Arathi Highlands"], "Kingsblood needs skill 125, Liferoot 150")
        g(170, 205, ["Kingsblood", "Liferoot", "Fadeleaf", "Goldthorn", "Khadgar's Whisker"], ["Stranglethorn Vale", "Arathi Highlands"], "Khadgar's Whisker needs skill 185")
        g(205, 230, ["Purple Lotus", "Firebloom"], ["Tanaris", "Searing Gorge"], "learn Artisan Herbalism (skill 200) before leaving; no trainer there")
        g(230, 270, ["Sungrass", "Purple Lotus", "Ghost Mushroom", "Golden Sansam"], ["The Hinterlands"], "Ghost Mushroom needs skill 245")
        g(270, 300, ["Sungrass", "Gromsblood", "Golden Sansam", "Dreamfoil", "Mountain Silversage", "Plaguebloom"], ["Felwood"])
    elif prof == "Mining":
        g(1, 65, ["Copper Ore"], ["Durotar", "Mulgore", "Tirisfal Glades", "Elwynn Forest", "Dun Morogh", "Teldrassil", "Zephras Isle"], "any starting zone; carry a Mining Pick")
        g(65, 125, ["Tin Ore", "Copper Ore", "Silver Ore"], ["Wetlands", "Silverpine Forest", "Westfall", "Hillsbrad Foothills", "The Barrens", "Ashenvale", "Stonetalon Mountains"])
        g(125, 175, ["Iron Ore", "Tin Ore", "Gold Ore"], ["Arathi Highlands", "Thousand Needles", "Desolace"], "Expert Mining needs skill 125")
        g(175, 245, ["Mithril Ore", "Truesilver Ore"], ["The Hinterlands", "Tanaris"], "Artisan Mining needs skill 200")
        g(245, 275, ["Mithril Ore", "Truesilver Ore", "Thorium Ore"], ["Un'Goro Crater", "Blasted Lands", "Felwood"])
        g(275, 300, ["Thorium Ore", "Mithril Ore"], ["Un'Goro Crater", "Eastern Plaguelands", "Winterspring", "Burning Steppes"])
        # smelting shortcut to ~105-120, an alternative to mining the first ranges
        g(1, 65, ["Copper Ore"], [], "buy ore on the AH; smelt 120-140 times, keep 70 Copper Bar", opt=1, alt=1, kind="craft", recipe="Smelt Copper", crafts=130)
        g(65, 75, ["Tin Ore"], [], "learn Journeyman Mining first; keep all 70 Tin Bar for Bronze", opt=1, alt=2, kind="craft", recipe="Smelt Tin", crafts=70)
        g(75, 105, ["Copper Bar", "Tin Bar"], [], "uses the bars from the two steps above; may end between 100 and 110", opt=1, alt=2, kind="craft", recipe="Smelt Bronze", crafts=70)
        g(105, 120, ["Silver Ore"], [], "price Silver Ore and Silver Bar first; the guide suggests mining from 120", opt=1, alt=2, kind="craft", recipe="Smelt Silver", crafts=40)
        for s in G:
            if s.get("recipe") in ("Smelt Copper", "Smelt Tin", "Smelt Bronze", "Smelt Silver"):
                for mm in s["materials"]:
                    mm["count"] = 1
    elif prof == "Skinning":
        g(1, 75, [], ["Valley of Trials, Durotar", "Northshire Valley, Elwynn Forest"], "skin any level 1-2 mob in a starting zone; carry a Skinning Knife")
        g(75, 130, [], ["Northwatch Hold area, Wetlands", "Loch Modan"], "skin level 16-18 mobs: Scytheclaw raptors, boars, bears; Expert Skinning is learned at 125")
        g(130, 150, [], ["Wetlands (Whelgar's Excavation Site)"], "skin level 25-27 raptors and razormaws")
        g(150, 160, [], ["Hillsbrad Foothills (in front of the yeti cave)"], "skin level 22-24 bears and lions")
        g(160, 225, [], ["Hillsbrad Foothills (Foothill Caverns)", "Duskwood (Rotting Orchard)"], "skin level 30-32 yetis; or level 26-29 Nightbane worgen in Duskwood")
        g(225, 280, [], ["Feral Scar area"], "skin level 43-46 yetis and scars; Artisan Skinning needs skill 200")
        g(280, 300, [], ["Un'Goro Crater"], "skin level 50-55 diemetradon and pterrordax")
    elif prof == "Fishing":
        g(1, 75, [], ["any starting zone lake, pond or river"], "buy a Fishing Pole and Shiny Baubles at a Fishing Supply vendor")
        g(75, 150, [], ["Darnassus", "Stormwind City", "Orgrimmar", "Undercity", "Thunder Bluff"], "learn Journeyman Fishing first; any capital city, or The Barrens, Darkshore, Ironforge, Loch Modan, Silverpine Forest, Westfall")
        g(150, 225, [], ["Dustwallow Marsh"], "learn Expert Fishing (book, Booty Bay); also Alterac Mountains, Arathi Highlands, Desolace, Stranglethorn Vale, Swamp of Sorrows, Thousand Needles; use Bright Baubles")
        g(225, 300, [], ["Felwood", "Feralas", "The Hinterlands", "Tanaris", "Un'Goro Crater", "Western Plaguelands"], "needs the Nat Pagle Artisan quest first; use Bright Baubles")
    for s in G:
        if s["kind"] == "gather":
            s["crafts"] = None
    return G


def main():
    d = sys.argv[1]
    fetched = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    out = {"source": "wow-professions.com WoW Forever leveling guides", "fetched_at": fetched, "professions": {}}
    for prof, slug in PROFS:
        p = os.path.join(d, slug + ".html")
        if not os.path.exists(p):
            print("missing page", prof, file=sys.stderr)
            continue
        url = BASE % slug
        lines = text_of(p)
        entry = {"source_url": url, "tiers": [], "steps": []}
        journey = []
        if prof in CRAFTERS:
            steps, journey = craft_profession(prof, lines)
            entry["steps"] = renumber(finish_craft(prof, steps, url, fetched))
        else:
            entry["steps"] = renumber(gather_steps(prof, url))
        tiers = []
        for tier in ("Journeyman", "Expert", "Artisan"):
            ex = TIER_EXTRA.get((prof, tier), {})
            base = dict(profession=prof, tier=tier, skill_required=ex.get("skill", TIER_SKILL[tier] if tier != "Artisan" or "skill" in ex else 200),
                        min_level=MIN_LEVEL[tier], how=ex.get("how", "trainer"), cost_copper=ex.get("cost"), note=ex.get("note", ""))
            if prof in ("Herbalism", "Mining", "Skinning", "Fishing", "Cooking", "First Aid") and tier == "Journeyman":
                base["min_level"] = 10 if prof in ("Fishing",) else None
            tr = list(ex.get("trainers", []))
            if tier == "Journeyman" and journey:
                # A trainer both factions share (Zephras Isle) is listed under each faction's heading on the page: keep one row.
                tr = list(dict.fromkeys((n, c, "Both" if c.startswith("Zephras") else ("Horde" if c in HORDE_CITIES else "Alliance")) for n, c in journey))
            tiers.append((base, tr))
        for base, tr in tiers:
            if tr:
                for n, place, fac in tr:
                    b = dict(base); b.update(trainer=n, place=place, faction=fac)
                    entry["tiers"].append(b)
            else:
                entry["tiers"].append(base)
        out["professions"][prof] = entry
    json.dump(out, sys.stdout, indent=1, sort_keys=False)
    sys.stdout.write("\n")


main()
