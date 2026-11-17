import csv, json, sys, collections
S = sys.argv[1]
out = {}
# transport sweep
rows = list(csv.DictReader(open(f"{S}/sweep/sweep.csv")))
out["sweep"] = [{k: float(r[k]) for k in ("cost", "living", "people", "transportJobs", "gdpHead", "gdpHeadWithTransport", "priceGap")} for r in rows]
# map: tiles, prices and flows at x1
tiles = {}
prices = collections.defaultdict(dict)
made = collections.defaultdict(dict)
for r in csv.DictReader(open(f"{S}/sweep/map-prices.csv")):
    t = int(r["tile"])
    tiles[t] = {"id": t, "name": r["name"], "row": int(r["row"]), "col": int(r["col"]), "terrain": r["terrain"], "coastal": r["coastal"] == "true"}
    if r["active"] == "true":
        prices[r["good"]][t] = round(float(r["price"]), 3)
    made[r["good"]][t] = float(r["made"])
flows = collections.defaultdict(list)
for r in csv.DictReader(open(f"{S}/sweep/map-flows.csv")):
    q = float(r["shipped"])
    if q > 0.002:
        flows[r["good"]].append([int(r["from"]), int(r["to"]), round(q, 4)])
# goods worth showing: traded a lot, with prices in most tiles
vol = {g: sum(f[2] for f in fl) for g, fl in flows.items()}
goods = [g for g in sorted(vol, key=lambda g: -vol[g]) if len(prices[g]) >= 20][:10]
out["map"] = {"tiles": [tiles[i] for i in sorted(tiles)], "goods": goods,
              "prices": {g: prices[g] for g in goods}, "flows": {g: flows[g] for g in goods},
              "made": {g: {t: round(v, 3) for t, v in made[g].items() if v > 0} for g in goods}}
# tiles after five years
td = json.load(open(f"{S}/data/tiles.json"))
out["tiles"] = [{"id": t["ID"], "people": round(t["People"], 1), "wage": round(t["Wage"], 3), "utility": round(t["Utility"], 3),
                 "growth": round(t["Growth"], 5),
                 "firms": [{"name": f["Name"], "method": f["Method"], "good": f["Good"], "sector": f["Sector"],
                            "labor": round(f["Labor"], 2), "va": round(f["ValueAdded"], 2), "levels": f["Levels"]}
                           for f in t["Firms"] if f["Labor"] > 0.05][:8]} for t in td]
# migration shock
sh = json.load(open(f"{S}/data/shock.json"))
s, c, h = sh["shocked"], sh["control"], sh["hops"]
rings = sorted(set(h))
months = s["Months"]
out["shock"] = {"tile": sh["tile"], "shockMonth": sh["shockMonth"], "months": months, "hops": h,
                "rings": {str(k): [round(sum(s["People"][i][m] - c["People"][i][m] for i in range(len(h)) if h[i] == k), 2) for m in range(len(months))] for k in rings},
                "ringSize": {str(k): sum(1 for x in h if x == k) for k in rings},
                "utilShock": [round(x, 3) for x in s["Utility"][sh["tile"]]],
                "utilControl": [round(x, 3) for x in c["Utility"][sh["tile"]]],
                "utilNeighbours": [round(sum(s["Utility"][i][m] for i in range(len(h)) if h[i] == 1) / sum(1 for x in h if x == 1), 3) for m in range(len(months))],
                "final": [round(s["People"][i][-1] - c["People"][i][-1], 1) for i in range(len(h))],
                "peopleTile": [round(x, 1) for x in s["People"][sh["tile"]]], "peopleTileControl": [round(x, 1) for x in c["People"][sh["tile"]]]}
# production functions
rc = json.load(open(f"{S}/data/recipes.json"))
keep = ["Farm", "Potato farm", "Iron mine", "Coal mine", "Forestry", "Mill", "Smelter", "Tailor", "Transport", "Toolmaker", "Brewery"]
out["firms"] = [f for f in rc["firms"] if f["Name"] in keep]
out["growth"] = rc["growth"]
out["migration"] = rc["migration"]
# trade routes: each route's landed cost against its destination price, and
# the busiest routes' first year
rt = json.load(open(f"{S}/data/routes.json"))
busy = [r for r in rt["routes"] if r["Shipped"] > 0.01 and r["Landed"] > 0]
# (a market where the good never changes hands keeps its starting price of 1: not a price)
idle = [r for r in rt["routes"] if r["Shipped"] < 1e-4 and r["Landed"] > 0 and r["Dest"] > 0 and abs(r["Dest"] - 1) > 1e-6]
idle = idle[:: max(1, len(idle) // 500)]  # a sample: there are thousands
out["routes"] = {"busy": [[round(r["Landed"], 3), round(r["Dest"], 3)] for r in busy],
                 "idle": [[round(r["Landed"], 3), round(r["Dest"], 3)] for r in idle],
                 "everyDays": rt["everyDays"],
                 "converging": [{"good": c["Route"]["Good"], "from": c["Route"]["From"], "to": c["Route"]["To"],
                                 "ratio": [round(a / b, 4) for a, b in zip(c["Path"]["Dest"], c["Path"]["Landed"])]} for c in rt["converging"]]}
# the Alchian-Allen runs: every good's place, and the goods moved by their bulk
aa = json.load(open(f"{S}/data/aa.json"))
pt = lambda g: {"good": g["Good"], "bulk": g["Bulk"], "vpb": round(g["Price"] / g["Bulk"], 3), "hops": round(g["HopsPerUnit"], 4)}
out["aa"] = {"base": [pt(g) for g in aa["base"]], "moves": {k: [pt(g) for g in v] for k, v in aa["moves"].items()}}
# substitution: the potato blight
bl = json.load(open(f"{S}/data/blight.json"))
out["blight"] = {"day": bl["blightDay"], "sigma": bl["sigma"],
                 "runs": {k: {kk: [round(x, 4) for x in vv] for kk, vv in v.items()} for k, v in (("blight", bl["blight"]), ("control", bl["control"]))}}
# evening out: the struck tile against its neighbours, with and without migration
def even(run):
    r = sh[run]
    nb = [i for i, x in enumerate(sh["hops"]) if x == 1]
    t = sh["tile"]
    avg = lambda key, m: sum(r[key][i][m] for i in nb) / len(nb)
    n = len(r["Months"])
    return {"gap": [round(r["Utility"][t][m] / avg("Utility", m) - 1, 4) for m in range(n)],
            "wage": [round(r["Wage"][t][m] / avg("Wage", m), 4) for m in range(n)],
            "prices": [round(r["Prices"][t][m] / avg("Prices", m), 4) for m in range(n)],
            "people": [round(r["People"][t][m], 1) for m in range(n)]}
out["even"] = {"move": even("shocked"), "stay": even("shockedStill"), "control": even("control")}
# trade across transport costs: wages and specialisation, and each tile's farm share
tr = json.load(open(f"{S}/data/trade.json"))["runs"]
out["trade"] = [{"cost": r["Cost"], "migration": r["Migration"], "wageSpread": round(r["WageSpread"], 4),
                 "spec": round(r["Specialisation"], 4), "farm": [round(x, 4) for x in r["FarmShare"]]} for r in tr]
# Engel's law: each tile's share of spending on basic needs against its income a head
en = json.load(open(f"{S}/data/engel.json"))
out["engel"] = [{"tile": int(r["Day"]), "basic": round(r["BasicShare"], 4), "income": round(r["IncomeHead"], 3)} for r in en["tiles"]]
# a coal boom: the boom tile at each dose of coal, with and without migration
bo = json.load(open(f"{S}/data/boom.json"))["runs"]
# (the carters hauling the tile's coal away count as coal work, not services)
for r in bo:
    r["CoalAll"], r["OtherService"] = r["Coal"] + r["Haul"], r["Service"] - r["Haul"]
out["boom"] = [{k: (round(v, 3) if isinstance(v, float) else v) for k, v in r.items()} for r in bo]
# a corridor across the map: the trade over each link and each tile's people, on the map as it is
# (cheap coasts) and with the route instead
co = json.load(open(f"{S}/data/corridor.json"))
run = {r["Name"]: r for r in co["runs"]}
out["corridor"] = {"route": co["route"],
                   "trade": {k: [[int(a), int(b), round(v, 2)] for a, b, v in run[k]["Trade"]] for k in run},
                   "people": {k: [round(x, 1) for x in run[k]["People"]] for k in run}}
json.dump(out, open(f"{S}/explainer-data.json", "w"), separators=(",", ":"))
print(f"{len(json.dumps(out))} bytes of data; goods {goods}")
