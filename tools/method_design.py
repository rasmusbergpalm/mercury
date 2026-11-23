"""Sets the productivity (TFP) of every production method (the methods in economy/data/recipes.json).

Each firm has its 1820 method and one for each later era: the steam age,
steel, electricity and chemistry, and the motor age. A method turns labour
into its era's machinery and fuel, and its productivity is set so that a
level of it is as profitable as a level of the method before it (ADV), at
the prices of its era: a new technique starts out barely better, and
spreads as its machines get cheaper and the techs that improve it come in
(Rosenberg; David's dynamo). Those prices are the game's own: for
the steam age and for steel and electricity, the average over seeds of the
era's years in century runs (the wage, and the machinery, fuel and
materials the methods use); for the motor age, the last decade of a run
with every technology known (CENTURY_ALLTECH), where every industry wants
vehicles and oil at once (in a century those come only in its last years,
when few use them and they are still cheap). A later method is also at
least FLOOR times as productive per level as the one it replaces: motor
trucks and tractors didn't make haulage or farms less productive, they made
them more productive with fewer hands.

Every method keeps the share of its value added (what it makes less the
materials and fuel it uses) that its firm's 1820 method pays its workers:
across the nineteenth century labour's share of income held near
three-fifths (Kaldor's stylised fact; Matthews, Feinstein and
Odling-Smee for Britain), machines saving labour by making each worker
more productive, which raised wages, not by taking labour's share. Its
machines take at most three-fifths of the rest, so every level keeps a
building.

A method's machines (tools, steam engines, vehicles) are capital: the
design gives them as cost shares, like its other factors, and the recipe
holds them in its level. A level of the method carries the stock whose
cost to own (the hurdle return and its wear) is that share of what the
level makes at its era's prices, and the method's productivity is set so
that a level of it makes as much, and earns as much, as it would buying
those machines' services by the day.

    python3 tools/method_design.py <all-tech CSV> <century CSV>...   # rewrites the methods in economy/data/recipes.json

(The century CSVs come from CENTURY_OUT=... go test ./game -run TestCentury.)
"""
import csv
import json
import math
import re
import sys

ADV = 1.0
FLOOR = 1.05
ERAS = {2: (1840, 1860), 3: (1870, 1890)}  # the years whose prices the steam age's and steel's methods are set at
# machines, held as capital: the good, and the share of it worn out a year
EQUIP = {"tools": ("tools", 0.08), "steam": ("steam engines", 0.04), "veh": ("vehicles", 0.10)}  # (as goods.json)
HURDLE = 0.10  # the return owners ask of a level (economy: investPayback)
YEAR = 365     # ticks a year
LEVEL, BUILDING_WEAR = 1200, 0.015  # the construction in a level, and its wear a year
BASE_ERA = {"Engine works": 2, "Power station": 3}  # 1820 firms that open later, with machines
# the design's names for goods (the rest are the goods' own names)
NAMES = dict(L="wage", steam="steam engines", fert="fertilizer", elec="electricity", expl="explosives", veh="vehicles",
             ore="iron ore", cane="sugar cane")
COMPOSITES = {"fuel": ("wood", "coal"), "fiber": ("wool", "cotton")}
SIGMA = 3
COLS = dict(L="wage", tools="price tools", steam="price steam engines", coal="price coal", iron="price iron", wood="price wood",
            grain="price grain", fuel="price coal", clay="price clay", fert="price fertilizer", elec="price electricity",
            expl="price explosives", veh="price vehicles", oil="price oil")


def era_prices(alltech, paths):
    def mean(sel):
        out = {k: sum(float(r[c]) for r in sel) / len(sel) for k, c in COLS.items()}
        for c in sel[0]:
            if c.startswith("p "):
                out[c[2:]] = sum(float(r[c]) for r in sel) / len(sel)
            elif c.startswith("rev "):
                out[c] = sum(float(r[c]) for r in sel) / len(sel)
        out["wage"] = sum(float(r["wage"]) for r in sel) / len(sel)
        for k, (a, b) in COMPOSITES.items():  # a CES composite of two, half each
            out[k] = (0.5 * out[a] ** (1 - SIGMA) + 0.5 * out[b] ** (1 - SIGMA)) ** (1 / (1 - SIGMA))
        return out
    rows = [r for p in paths for r in csv.DictReader(open(p))]
    P = {}
    for era, (a, b) in ERAS.items():
        P[era] = mean([r for r in rows if a <= 1820 + int(r["year"]) < b])  # (the CSV counts years from 1820)
    rows = list(csv.DictReader(open(alltech)))
    P[4] = mean(rows[-10:])
    return P


def term(f, prices):
    # a level's profit, in logs, up to what all methods share: sum of a*log(a/p)
    # over the priced factors (unpriced ones, as ore or cloth, keep their
    # exponent from method to method and cancel)
    return sum(a * math.log(a / prices[g]) for g, a in f.items() if g in prices)


# firm: its 1820 method (tfp, factors), then its later ones: (name, era, tech, factors)
F = {
 'Farm': ((2.0, dict(L=.6)), [('Seed-drill farm',2,'Seed drill',dict(L=.6,tools=.16)),('Fertilized farm',3,'Fertilizers',dict(L=.492,tools=.1,fert=.18)),('Tractor farm',4,'Mechanized agriculture',dict(L=.468,veh=.187,oil=.1,fert=.12))]),
 'Potato farm': ((2.5, dict(L=.6)), [('Horse-hoe potato farm',2,'Seed drill',dict(L=.6,tools=.12)),('Fertilized potato farm',3,'Fertilizers',dict(L=.504,fert=.16,tools=.08)),('Motorized potato farm',4,'Mechanized agriculture',dict(L=.456,fert=.14,veh=.12,oil=.1))]),
 'Ranch': ((1.2, dict(L=.6)), [('Fodder ranch',2,'Seed drill',dict(L=.516,grain=.14,tools=.08)),('Fenced ranch',3,'Steel',dict(L=.468,grain=.14,iron=.08,tools=.08)),('Motorized ranch',4,'Mechanized agriculture',dict(L=.456,grain=.14,veh=.14,oil=.1))]),
 'Fishery': ((1.5, dict(L=.6)), [('Ice-packed fishery',2,'Railways',dict(L=.6,tools=.1)),('Steam trawler',3,'Steel',dict(L=.528,steam=.18,coal=.12)),('Motor trawler',4,'Internal combustion',dict(L=.492,veh=.197,oil=.18))]),
 'Forestry': ((2.0, dict(L=.6)), [('Steam sawmill',2,'Steam power',dict(L=.528,steam=.16,coal=.12)),('Band-saw mill',3,'Steel',dict(L=.54,steam=.116,coal=.1,tools=.1)),('Motor logging',4,'Internal combustion',dict(L=.528,veh=.127,oil=.12,tools=.084))]),
 'Coal mine': ((2.0, dict(L=.6)), [('Steam coal mine',2,'Steam power',dict(L=.57,steam=.1,coal=.05)),('Mechanized coal mine',3,'Electricity',dict(L=.528,steam=.08,elec=.12,tools=.08)),('Conveyor coal mine',4,'Mass production',dict(L=.492,elec=.18,tools=.098,steam=.098))]),
 'Iron mine': ((1.5, dict(L=.6)), [('Steam iron mine',2,'Steam power',dict(L=.57,steam=.1,coal=.05)),('Blasting iron mine',3,'Industrial chemistry',dict(L=.528,expl=.12,steam=.08)),('Motor ore mine',4,'Internal combustion',dict(L=.468,expl=.12,veh=.12,oil=.1))]),
 'Sheep farm': ((1.5, dict(L=.6)), [('Bred-flock sheep farm',2,'Seed drill',dict(L=.528,grain=.12)),('Machine-shorn sheep farm',3,'Steel',dict(L=.528,grain=.12,tools=.1)),('Motorized sheep station',4,'Mechanized agriculture',dict(L=.48,grain=.12,veh=.12,oil=.08))]),
 'Cotton plantation': ((1.5, dict(L=.6)), [('Steam-gin plantation',2,'Steam power',dict(L=.564,steam=.08,coal=.06)),('Fertilized cotton plantation',3,'Fertilizers',dict(L=.504,steam=.08,coal=.06,fert=.1)),('Mechanized cotton plantation',4,'Mechanized agriculture',dict(L=.468,fert=.1,veh=.14,oil=.12))]),
 'Tea plantation': ((1.0, dict(L=.6)), [('Estate tea garden',2,'Seed drill',dict(L=.6,tools=.12)),('Tea estate with rollers',3,'Steel',dict(L=.564,steam=.12,coal=.06,tools=.06)),('Mechanized tea estate',4,'Mechanized agriculture',dict(L=.54,veh=.126,oil=.1,tools=.09))]),
 'Sugar plantation': ((2.0, dict(L=.6)), [('Steam-mill plantation',2,'Steam power',dict(L=.516,steam=.12,coal=.14)),('Fertilized cane plantation',3,'Fertilizers',dict(L=.468,steam=.1,coal=.1,fert=.12)),('Mechanized cane plantation',4,'Mechanized agriculture',dict(L=.456,fert=.12,veh=.16,oil=.12))]),
 'Vineyard': ((1.5, dict(L=.6)), [('Trellised vineyard',2,'Seed drill',dict(L=.6,tools=.1)),('Grafted vineyard',3,'Industrial chemistry',dict(L=.552,tools=.1,fert=.08)),('Motorized vineyard',4,'Mechanized agriculture',dict(L=.528,tools=.1,fert=.08,veh=.08,oil=.04))]),
 'Silk farm': ((0.8, dict(L=.6)), [('Steam filature',2,'Steam filatures',dict(L=.564,steam=.08,coal=.06)),('Selected-egg silk farm',3,'Industrial chemistry',dict(L=.528,steam=.08,coal=.06,fert=.06)),('Electric filature',4,'Mass production',dict(L=.48,elec=.14,tools=.1,fert=.06))]),
 'Clay pit': ((2.0, dict(L=.6)), [('Steam clay works',2,'Steam power',dict(L=.564,steam=.08,coal=.06)),('Blasted clay pit',3,'Industrial chemistry',dict(L=.54,expl=.1,steam=.12)),('Excavator clay pit',4,'Internal combustion',dict(L=.492,veh=.16,oil=.1,expl=.08))]),
 'Oil well': ((1.5, dict(L=.6)), [('Rotary-drilled well',4,'Internal combustion',dict(L=.6,steam=.1,tools=.1))]),
 'Bakery': ((2.0, dict(L=.3,grain=.4,fuel=.1)), [('Steam bakery',2,'Steam power',dict(L=.288,grain=.4,coal=.12,steam=.06)),('Electric bakery',3,'Electricity',dict(L=.24,grain=.4,elec=.1,tools=.04,coal=.1)),('Industrial bakery',4,'Mass production',dict(L=.216,grain=.4,elec=.16,tools=.06,coal=.08))]),
 'Brewery': ((1.5, dict(L=.3,grain=.4)), [('Steam brewery',2,'Steam power',dict(L=.265,grain=.4,coal=.07,steam=.05)),('Refrigerated brewery',3,'Refrigeration',dict(L=.24,grain=.4,coal=.06,steam=.04,elec=.06)),('Bottling brewery',4,'Mass production',dict(L=.23,grain=.4,elec=.1,tools=.06,coal=.04))]),
 'Mill': ((1.5, dict(L=.3,fiber=.4)), [('Textile factory',2,'Mechanized textiles',dict(L=.25,fiber=.4,coal=.1,steam=.05)),('Ring-spinning mill',3,'Electricity',dict(L=.25,fiber=.4,elec=.1,tools=.08)),('Automatic-loom mill',4,'Assembly line',dict(L=.23,fiber=.4,elec=.14,tools=.08))]),
 'Tailor': ((1.5, dict(L=.3,cloth=.4)), [('Sewing-machine tailor',2,'Interchangeable parts',dict(L=.3,cloth=.4,tools=.06)),('Garment factory',3,'Electricity',dict(L=.26,cloth=.4,elec=.08,tools=.04)),('Garment assembly line',4,'Assembly line',dict(L=.25,cloth=.4,elec=.1,tools=.08))]),
 'Smelter': ((1.0, dict(L=.2,ore=.3,fuel=.2)), [('Hot-blast furnace',2,'Steam power',dict(L=.2,ore=.3,coal=.2,steam=.02)),('Steel works',3,'Steel',dict(L=.208,ore=.3,coal=.18,steam=.05)),('Electric arc furnace',4,'Mass production',dict(L=.18,ore=.3,elec=.15,coal=.1,tools=.05))]),
 'Smithy': ((1.2, dict(L=.3,iron=.3,fuel=.1)), [('Steam-hammer forge',2,'Steam power',dict(L=.29,iron=.3,coal=.12,steam=.06)),('Electric smithy',3,'Electricity',dict(L=.25,iron=.3,elec=.2)),('Drop-forge works',4,'Mass production',dict(L=.25,iron=.3,elec=.2,tools=.08))]),
 'Carpenter': ((1.2, dict(L=.3,wood=.4)), [('Steam joinery',2,'Steam power',dict(L=.28,wood=.4,coal=.04,steam=.04)),('Electric joinery',3,'Electricity',dict(L=.24,wood=.4,elec=.12)),('Furniture factory',4,'Mass production',dict(L=.23,wood=.4,elec=.14,tools=.06))]),
 'Refinery': ((1.2, dict(L=.2,cane=.4,fuel=.1)), [('Vacuum-pan refinery',2,'Steam power',dict(L=.192,cane=.4,coal=.12,steam=.06)),('Centrifugal refinery',3,'Electricity',dict(L=.168,cane=.4,coal=.08,elec=.1,steam=.04)),('Continuous refinery',4,'Mass production',dict(L=.16,cane=.4,elec=.14,coal=.06,tools=.04))]),
 'Winery': ((1.2, dict(L=.3,grapes=.4)), [('Steam-press winery',2,'Steam power',dict(L=.285,grapes=.4,coal=.03,steam=.03)),('Pasteurizing winery',3,'Research laboratories',dict(L=.27,grapes=.4,coal=.06,steam=.06)),('Industrial winery',4,'Mass production',dict(L=.25,grapes=.4,elec=.1,tools=.08))]),
 'Couturier': ((1.0, dict(L=.4,silk=.3)), [('Sewing-machine atelier',2,'Interchangeable parts',dict(L=.4,silk=.3,tools=.08)),('Electric atelier',3,'Electricity',dict(L=.354,silk=.3,elec=.08,tools=.06)),('Couture workshop line',4,'Assembly line',dict(L=.331,silk=.3,elec=.12,tools=.08))]),
 'Kiln': ((1.0, dict(L=.4,clay=.2,fuel=.2)), [('Factory kiln',2,'Interchangeable parts',dict(L=.367,clay=.2,coal=.25,steam=.05,tools=.05)),('Tunnel kiln',3,'Industrial chemistry',dict(L=.333,clay=.2,coal=.3,steam=.05,tools=.05)),('Electric kiln',4,'Mass production',dict(L=.333,clay=.2,elec=.3,tools=.044,steam=.056))]),
 'Transport': ((2.0, dict(L=.6)), [('Steam transport',2,'Locomotives',dict(L=.57,steam=.1,coal=.05)),('Heavy rail freight',3,'Rail network',dict(L=.528,steam=.14,coal=.08,iron=.04)),('Motor transport',4,'Motor trucks',dict(L=.516,veh=.2,oil=.14))]),
 'Construction': ((1.5, dict(L=.5,wood=.2)), [('Steam shovels',2,'Locomotives',dict(L=.481,wood=.2,steam=.05,coal=.03)),('Steam cranes',3,'Engineering',dict(L=.438,wood=.12,iron=.12,steam=.08,coal=.06)),('Motorized construction',4,'Internal combustion',dict(L=.412,wood=.1,iron=.14,veh=.14,oil=.1))]),
 'Armory': ((1.0, dict(L=.3,iron=.25,wood=.1)), [('Armory system',2,'Interchangeable parts',dict(L=.3,iron=.25,wood=.1,tools=.08)),('Steel arms works',3,'Steel',dict(L=.272,iron=.28,wood=.05,tools=.08,coal=.08)),('Mass-production armory',4,'Mass production',dict(L=.254,iron=.28,wood=.04,tools=.1,elec=.13))]),
 'Foundry': ((0.8, dict(L=.3,iron=.3,coal=.1)), [('Steam foundry',2,'Steam power',dict(L=.29,iron=.3,coal=.12,steam=.06)),('Steel gun works',3,'Steel',dict(L=.27,iron=.32,coal=.14,steam=.08)),('Electric foundry',4,'Mass production',dict(L=.25,iron=.32,elec=.18,tools=.1))]),
 'Toolmaker': ((1.2, dict(L=.3,iron=.25,wood=.1)), [('Machine-tool works',2,'Interchangeable parts',dict(L=.314,iron=.25,steam=.08,coal=.07)),('Electric tool works',3,'Electricity',dict(L=.282,iron=.25,elec=.14,steam=.04,tools=.06)),('Tool assembly line',4,'Assembly line',dict(L=.254,iron=.25,elec=.2,tools=.1))]),
 'Engine works': ((0.8, dict(L=.3,iron=.3,coal=.1,tools=.1)), [('Compound-engine works',3,'Steel',dict(L=.27,iron=.3,coal=.1,tools=.14,elec=.06)),('Engine assembly line',4,'Assembly line',dict(L=.24,iron=.3,tools=.144,elec=.16,coal=.06))]),
 'Chemical works': ((1.0, dict(L=.3,coal=.2,clay=.1)), [('Electrochemical works',4,'Mass production',dict(L=.249,coal=.14,clay=.1,elec=.18))]),
 'Fertilizer works': ((1.2, dict(L=.3,coal=.15,clay=.15)), [('Haber-process plant',4,'Mass production',dict(L=.24,coal=.2,clay=.06,elec=.18))]),
 'Power station': ((2.0, dict(L=.2,coal=.3,steam=.1)), [('Turbine power station',4,'Mass production',dict(L=.194,coal=.32,steam=.14,tools=.04))]),
 'Munitions plant': ((1.5, dict(L=.3,iron=.2,coal=.2)), [('Shell production line',4,'Mass production',dict(L=.23,iron=.22,coal=.16,elec=.16))]),
 'Motor works': ((0.8, dict(L=.3,iron=.3,coal=.1)), [('Assembly-line works',4,'Assembly line',dict(L=.24,iron=.3,coal=.05,elec=.17))]),
}


def price(P, k):
    return P[NAMES.get(k, k)]


def revenue(firm, f, P):
    # what a level of the firm makes a day in its era (century runs); where
    # it didn't run then, where owners have built as many as pay: its share
    # of what it makes (one less the factors' shares) earns the hurdle and
    # the building's wear on the building
    if P.get("rev " + firm, 0) > 0:
        return P["rev " + firm]
    return (HURDLE + BUILDING_WEAR) * LEVEL * P["construction"] / YEAR / (1 - sum(f.values()))


def as_capital(firm, tfp, f, P):
    """A method designed with its machines bought by the day, made into one
    that holds them in its level: (tfp, the factors left, the machines a
    level holds)."""
    R = revenue(firm, f, P)
    rest = {g: a for g, a in f.items() if g not in EQUIP}
    machines = {}
    for g, a in f.items():
        if g in EQUIP:
            good, wear = EQUIP[g]
            p = price(P, good)
            # a stock whose cost to own a day is the share a of what the level makes
            machines[good] = a * R * YEAR / (HURDLE + wear) / p
            # the level makes as much without buying them: tfp takes on
            # what the day's machines added, (a R / p)^a
            tfp *= (a * R / p) ** a
    return tfp, rest, machines


def main(alltech, paths):
    P = era_prices(alltech, paths)
    path = "economy/data/recipes.json"
    src = open(path, encoding="utf-8").read()
    designs = {}  # name: (firm, tfp, design factors, era)
    for firm, (base, meths) in F.items():
        prev_t, prev_f = base
        if any(g in EQUIP for g in prev_f):
            designs[firm] = (firm, prev_t, prev_f, BASE_ERA[firm])
        prev_f = {("coal" if k == "fuel" else k): v for k, v in prev_f.items()}
        for name, era, tech, f in meths:
            calc = prev_t * math.exp(term(prev_f, P[era]) - term(f, P[era])) * ADV
            tfp = round(max(calc, prev_t * FLOOR), 2)
            print(f"{firm:18s} {name:30s} era {era}  tfp {tfp:5.2f}  (as profitable: {calc:5.2f})")
            designs[name] = (firm, tfp, f, era)
            prev_t, prev_f = tfp, f
    for name, (firm, tfp, f, era) in designs.items():
        line = re.search(r'^( *)(\{"firm": "' + re.escape(name) + r'", .*\})(,?)$', src, re.M)
        assert line, name
        rc = json.loads(line.group(2))
        t, rest, machines = as_capital(firm, tfp, f, P[era])
        rc["tfp"] = round(t, 3)
        rc["factors"] = [["labor" if g == "L" else NAMES.get(g, g), a] for g, a in rest.items()]
        rc.pop("capital", None)
        if machines:
            rc["capital"] = [["construction", LEVEL]] + [[g, round(q, 1)] for g, q in machines.items()]
            print(f"{name:30s} tfp {tfp:5.2f} -> {t:6.3f}  a level holds " + ", ".join(f"{q:.0f} {g}" for g, q in machines.items()))
        out = json.dumps(rc, ensure_ascii=False)
        src = src[:line.start(2)] + out + src[line.end(2):]
    open(path, "w", encoding="utf-8").write(src)

if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2:])
