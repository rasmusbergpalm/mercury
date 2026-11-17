"""Sets the productivity (TFP) of every production method (engine.MethodRecipes).

Each firm has its 1820 method and one for each later era: the steam age,
steel, electricity and chemistry, and the motor age. A method turns labour
into its era's machinery and fuel, and its productivity is set so that a
level of it is ADV (a tenth) more profitable than a level of the method
before it, at the prices of its era. Those prices are the game's own: for
the steam age and for steel and electricity, the average over seeds of the
era's years in century runs (the wage, and the machinery, fuel and
materials the methods use); for the motor age, the last decade of a run
with every technology known (CENTURY_ALLTECH), where every industry wants
vehicles and oil at once (in a century those come only in its last years,
when few use them and they are still cheap). A later method is also at
least FLOOR times as productive per level as the one it replaces: motor
trucks and tractors didn't make haulage or farms less productive, they made
them more productive with fewer hands.

    python3 tools/method_design.py <all-tech CSV> <century CSV>...   # rewrites the TFPs in engine/scenario.go

(The century CSVs come from CENTURY_OUT=... go test ./engine -run TestCentury.)
"""
import csv
import math
import re
import sys

ADV = 1.10
FLOOR = 1.05
ERAS = {2: (1840, 1860), 3: (1870, 1890)}  # the years whose prices the steam age's and steel's methods are set at
COLS = dict(L="wage", tools="price tools", steam="price steam engines", coal="price coal", iron="price iron", wood="price wood",
            grain="price grain", fuel="price coal", clay="price clay", fert="price fertilizer", elec="price electricity",
            expl="price explosives", veh="price vehicles", oil="price oil")


def era_prices(alltech, paths):
    mean = lambda sel: {k: sum(float(r[c]) for r in sel) / len(sel) for k, c in COLS.items()}
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
 'Farm': ((2.0, dict(L=.6)), [('Seed-drill farm',2,'Seed drill',dict(L=.44,tools=.16)),('Fertilized farm',3,'Fertilizers',dict(L=.32,tools=.1,fert=.18)),('Tractor farm',4,'Mechanized agriculture',dict(L=.18,veh=.2,oil=.1,fert=.12))]),
 'Potato farm': ((2.5, dict(L=.6)), [('Horse-hoe potato farm',2,'Seed drill',dict(L=.48,tools=.12)),('Fertilized potato farm',3,'Fertilizers',dict(L=.36,fert=.16,tools=.08)),('Motorized potato farm',4,'Mechanized agriculture',dict(L=.24,fert=.14,veh=.12,oil=.1))]),
 'Ranch': ((1.2, dict(L=.6)), [('Fodder ranch',2,'Seed drill',dict(L=.38,grain=.14,tools=.08)),('Fenced ranch',3,'Steel',dict(L=.3,grain=.14,iron=.08,tools=.08)),('Motorized ranch',4,'Mechanized agriculture',dict(L=.22,grain=.14,veh=.14,oil=.1))]),
 'Fishery': ((1.5, dict(L=.6)), [('Ice-packed fishery',2,'Railways',dict(L=.5,tools=.1)),('Steam trawler',3,'Steel',dict(L=.3,steam=.18,coal=.12)),('Motor trawler',4,'Internal combustion',dict(L=.22,veh=.2,oil=.18))]),
 'Forestry': ((2.0, dict(L=.6)), [('Steam sawmill',2,'Steam power',dict(L=.32,steam=.16,coal=.12)),('Band-saw mill',3,'Steel',dict(L=.24,steam=.14,coal=.1,tools=.12)),('Motor logging',4,'Internal combustion',dict(L=.18,veh=.18,oil=.12,tools=.12))]),
 'Coal mine': ((2.0, dict(L=.6)), [('Steam coal mine',2,'Steam power',dict(L=.45,steam=.1,coal=.05)),('Mechanized coal mine',3,'Electricity',dict(L=.32,steam=.08,elec=.12,tools=.08)),('Conveyor coal mine',4,'Mass production',dict(L=.22,elec=.18,tools=.1,steam=.1))]),
 'Iron mine': ((1.5, dict(L=.6)), [('Steam iron mine',2,'Steam power',dict(L=.45,steam=.1,coal=.05)),('Blasting iron mine',3,'Industrial chemistry',dict(L=.4,expl=.12,steam=.08)),('Motor ore mine',4,'Internal combustion',dict(L=.26,expl=.12,veh=.12,oil=.1))]),
 'Sheep farm': ((1.5, dict(L=.6)), [('Bred-flock sheep farm',2,'Seed drill',dict(L=.48,grain=.12)),('Machine-shorn sheep farm',3,'Steel',dict(L=.38,grain=.12,tools=.1)),('Motorized sheep station',4,'Mechanized agriculture',dict(L=.28,grain=.12,veh=.12,oil=.08))]),
 'Cotton plantation': ((1.5, dict(L=.6)), [('Steam-gin plantation',2,'Steam power',dict(L=.46,steam=.08,coal=.06)),('Fertilized cotton plantation',3,'Fertilizers',dict(L=.36,steam=.08,coal=.06,fert=.1)),('Mechanized cotton plantation',4,'Mechanized agriculture',dict(L=.24,fert=.1,veh=.14,oil=.12))]),
 'Tea plantation': ((1.0, dict(L=.6)), [('Estate tea garden',2,'Seed drill',dict(L=.48,tools=.12)),('Tea estate with rollers',3,'Steel',dict(L=.36,steam=.12,coal=.06,tools=.06)),('Mechanized tea estate',4,'Mechanized agriculture',dict(L=.26,veh=.14,oil=.1,tools=.1))]),
 'Sugar plantation': ((2.0, dict(L=.6)), [('Steam-mill plantation',2,'Steam power',dict(L=.34,steam=.12,coal=.14)),('Fertilized cane plantation',3,'Fertilizers',dict(L=.28,steam=.1,coal=.1,fert=.12)),('Mechanized cane plantation',4,'Mechanized agriculture',dict(L=.2,fert=.12,veh=.16,oil=.12))]),
 'Vineyard': ((1.5, dict(L=.6)), [('Trellised vineyard',2,'Seed drill',dict(L=.5,tools=.1)),('Grafted vineyard',3,'Industrial chemistry',dict(L=.42,tools=.1,fert=.08)),('Motorized vineyard',4,'Mechanized agriculture',dict(L=.3,tools=.1,fert=.08,veh=.08,oil=.04))]),
 'Silk farm': ((0.8, dict(L=.6)), [('Steam filature',2,'Steam filatures',dict(L=.46,steam=.08,coal=.06)),('Selected-egg silk farm',3,'Industrial chemistry',dict(L=.4,steam=.08,coal=.06,fert=.06)),('Electric filature',4,'Mass production',dict(L=.3,elec=.14,tools=.1,fert=.06))]),
 'Clay pit': ((2.0, dict(L=.6)), [('Steam clay works',2,'Steam power',dict(L=.46,steam=.08,coal=.06)),('Blasted clay pit',3,'Industrial chemistry',dict(L=.38,expl=.1,steam=.12)),('Excavator clay pit',4,'Internal combustion',dict(L=.26,veh=.16,oil=.1,expl=.08))]),
 'Oil well': ((1.5, dict(L=.6)), [('Rotary-drilled well',4,'Internal combustion',dict(L=.4,steam=.1,tools=.1))]),
 'Bakery': ((2.0, dict(L=.3,grain=.4,fuel=.1)), [('Steam bakery',2,'Steam power',dict(L=.22,grain=.4,coal=.12,steam=.06)),('Electric bakery',3,'Electricity',dict(L=.16,grain=.4,elec=.1,tools=.04,coal=.1)),('Industrial bakery',4,'Mass production',dict(L=.1,grain=.4,elec=.16,tools=.06,coal=.08))]),
 'Brewery': ((1.5, dict(L=.3,grain=.4)), [('Steam brewery',2,'Steam power',dict(L=.18,grain=.4,coal=.07,steam=.05)),('Refrigerated brewery',3,'Refrigeration',dict(L=.14,grain=.4,coal=.06,steam=.04,elec=.06)),('Bottling brewery',4,'Mass production',dict(L=.1,grain=.4,elec=.1,tools=.06,coal=.04))]),
 'Mill': ((1.5, dict(L=.3,fiber=.4)), [('Textile factory',2,'Mechanized textiles',dict(L=.15,fiber=.4,coal=.1,steam=.05)),('Ring-spinning mill',3,'Electricity',dict(L=.12,fiber=.4,elec=.1,tools=.08)),('Automatic-loom mill',4,'Assembly line',dict(L=.08,fiber=.4,elec=.14,tools=.08))]),
 'Tailor': ((1.5, dict(L=.3,cloth=.4)), [('Sewing-machine tailor',2,'Interchangeable parts',dict(L=.24,cloth=.4,tools=.06)),('Garment factory',3,'Electricity',dict(L=.18,cloth=.4,elec=.08,tools=.04)),('Garment assembly line',4,'Assembly line',dict(L=.12,cloth=.4,elec=.1,tools=.08))]),
 'Smelter': ((1.0, dict(L=.2,ore=.3,fuel=.2)), [('Hot-blast furnace',2,'Steam power',dict(L=.18,ore=.3,coal=.2,steam=.02)),('Steel works',3,'Steel',dict(L=.17,ore=.3,coal=.18,steam=.05)),('Electric arc furnace',4,'Mass production',dict(L=.1,ore=.3,elec=.15,coal=.1,tools=.05))]),
 'Smithy': ((1.2, dict(L=.3,iron=.3,fuel=.1)), [('Steam-hammer forge',2,'Steam power',dict(L=.22,iron=.3,coal=.12,steam=.06)),('Electric smithy',3,'Electricity',dict(L=.2,iron=.3,elec=.2)),('Drop-forge works',4,'Mass production',dict(L=.12,iron=.3,elec=.2,tools=.08))]),
 'Carpenter': ((1.2, dict(L=.3,wood=.4)), [('Steam joinery',2,'Steam power',dict(L=.22,wood=.4,coal=.04,steam=.04)),('Electric joinery',3,'Electricity',dict(L=.18,wood=.4,elec=.12)),('Furniture factory',4,'Mass production',dict(L=.1,wood=.4,elec=.14,tools=.06))]),
 'Refinery': ((1.2, dict(L=.2,cane=.4,fuel=.1)), [('Vacuum-pan refinery',2,'Steam power',dict(L=.12,cane=.4,coal=.12,steam=.06)),('Centrifugal refinery',3,'Electricity',dict(L=.08,cane=.4,coal=.08,elec=.1,steam=.04)),('Continuous refinery',4,'Mass production',dict(L=.06,cane=.4,elec=.14,coal=.06,tools=.04))]),
 'Winery': ((1.2, dict(L=.3,grapes=.4)), [('Steam-press winery',2,'Steam power',dict(L=.24,grapes=.4,coal=.03,steam=.03)),('Pasteurizing winery',3,'Research laboratories',dict(L=.18,grapes=.4,coal=.06,steam=.06)),('Industrial winery',4,'Mass production',dict(L=.12,grapes=.4,elec=.1,tools=.08))]),
 'Couturier': ((1.0, dict(L=.4,silk=.3)), [('Sewing-machine atelier',2,'Interchangeable parts',dict(L=.32,silk=.3,tools=.08)),('Electric atelier',3,'Electricity',dict(L=.26,silk=.3,elec=.08,tools=.06)),('Couture workshop line',4,'Assembly line',dict(L=.2,silk=.3,elec=.12,tools=.08))]),
 'Kiln': ((1.0, dict(L=.4,clay=.2,fuel=.2)), [('Factory kiln',2,'Interchangeable parts',dict(L=.25,clay=.2,coal=.25,steam=.05,tools=.05)),('Tunnel kiln',3,'Industrial chemistry',dict(L=.18,clay=.2,coal=.3,steam=.06,tools=.06)),('Electric kiln',4,'Mass production',dict(L=.12,clay=.2,elec=.3,tools=.08,steam=.1))]),
 'Transport': ((2.0, dict(L=.6)), [('Steam transport',2,'Locomotives',dict(L=.45,steam=.1,coal=.05)),('Heavy rail freight',3,'Rail network',dict(L=.34,steam=.14,coal=.08,iron=.04)),('Motor transport',4,'Motor trucks',dict(L=.26,veh=.2,oil=.14))]),
 'Construction': ((1.5, dict(L=.5,wood=.2)), [('Steam shovels',2,'Locomotives',dict(L=.42,wood=.2,steam=.05,coal=.03)),('Steam cranes',3,'Engineering',dict(L=.32,wood=.12,iron=.12,steam=.08,coal=.06)),('Motorized construction',4,'Internal combustion',dict(L=.22,wood=.1,iron=.14,veh=.14,oil=.1))]),
 'Armory': ((1.0, dict(L=.3,iron=.25,wood=.1)), [('Armory system',2,'Interchangeable parts',dict(L=.22,iron=.25,wood=.1,tools=.08)),('Steel arms works',3,'Steel',dict(L=.16,iron=.28,wood=.05,tools=.08,coal=.08)),('Mass-production armory',4,'Mass production',dict(L=.1,iron=.28,wood=.04,tools=.1,elec=.13))]),
 'Foundry': ((0.8, dict(L=.3,iron=.3,coal=.1)), [('Steam foundry',2,'Steam power',dict(L=.22,iron=.3,coal=.12,steam=.06)),('Steel gun works',3,'Steel',dict(L=.16,iron=.32,coal=.14,steam=.08)),('Electric foundry',4,'Mass production',dict(L=.1,iron=.32,elec=.18,tools=.1))]),
 'Toolmaker': ((1.2, dict(L=.3,iron=.25,wood=.1)), [('Machine-tool works',2,'Interchangeable parts',dict(L=.25,iron=.25,steam=.08,coal=.07)),('Electric tool works',3,'Electricity',dict(L=.16,iron=.25,elec=.14,steam=.04,tools=.06)),('Tool assembly line',4,'Assembly line',dict(L=.1,iron=.25,elec=.2,tools=.1))]),
 'Engine works': ((0.8, dict(L=.3,iron=.3,coal=.1,tools=.1)), [('Compound-engine works',3,'Steel',dict(L=.2,iron=.3,coal=.1,tools=.14,elec=.06)),('Engine assembly line',4,'Assembly line',dict(L=.12,iron=.3,tools=.16,elec=.16,coal=.06))]),
 'Chemical works': ((1.0, dict(L=.3,coal=.2,clay=.1)), [('Electrochemical works',4,'Mass production',dict(L=.18,coal=.14,clay=.1,elec=.18))]),
 'Fertilizer works': ((1.2, dict(L=.3,coal=.15,clay=.15)), [('Haber-process plant',4,'Mass production',dict(L=.16,coal=.2,clay=.06,elec=.18))]),
 'Power station': ((2.0, dict(L=.2,coal=.3,steam=.1)), [('Turbine power station',4,'Mass production',dict(L=.1,coal=.32,steam=.14,tools=.04))]),
 'Munitions plant': ((1.5, dict(L=.3,iron=.2,coal=.2)), [('Shell production line',4,'Mass production',dict(L=.16,iron=.22,coal=.16,elec=.16))]),
 'Motor works': ((0.8, dict(L=.3,iron=.3,coal=.1)), [('Assembly-line works',4,'Assembly line',dict(L=.18,iron=.3,coal=.05,elec=.17))]),
}


def main(alltech, paths):
    P = era_prices(alltech, paths)
    tfps = {}
    for firm, (base, meths) in F.items():
        prev_t, prev_f = base
        prev_f = {("coal" if k == "fuel" else k): v for k, v in prev_f.items()}
        for name, era, tech, f in meths:
            calc = prev_t * math.exp(term(prev_f, P[era]) - term(f, P[era])) * ADV
            tfp = round(max(calc, prev_t * FLOOR), 2)
            print(f"{firm:18s} {name:30s} era {era}  tfp {tfp:5.2f}  (a tenth more profitable: {calc:5.2f})")
            tfps[name] = tfp
            prev_t, prev_f = tfp, f
    src = open("engine/scenario.go", encoding="utf-8").read()
    for name, tfp in tfps.items():
        src, n = re.subn(r'(Recipe\{"' + re.escape(name) + r'", "[^"]+", )[0-9.]+', lambda m: m.group(1) + f"{tfp:g}", src)
        assert n == 1, name
    open("engine/scenario.go", "w", encoding="utf-8").write(src)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2:])
