"""World set-up: recipes, terrain, and a 6x6 hex map."""
from __future__ import annotations

import math

from firm import LABOR, Firm
from goods import GOODS, TRADABLE, TRANSPORT
from region import Region
from trader import Trader
from world import World

# ---- production chains ---------------------------------------------------------
# firm name: (output, base TFP, factor exponents). "fuel" = wood or coal,
# "fiber" = wool or cotton (see goods.COMPOSITES).
RAW = {
    "Farm":              ("grain",      2.0, {LABOR: 0.6}),
    "Potato farm":       ("potatoes",   2.5, {LABOR: 0.6}),
    "Ranch":             ("meat",       1.2, {LABOR: 0.6}),
    "Fishery":           ("fish",       1.5, {LABOR: 0.6}),
    "Forestry":          ("wood",       2.0, {LABOR: 0.6}),
    "Coal mine":         ("coal",       2.0, {LABOR: 0.6}),
    "Iron mine":         ("iron ore",   1.5, {LABOR: 0.6}),
    "Sheep farm":        ("wool",       1.5, {LABOR: 0.6}),
    "Cotton plantation": ("cotton",     1.5, {LABOR: 0.6}),
    "Tea plantation":    ("tea",        1.0, {LABOR: 0.6}),
    "Sugar plantation":  ("sugar cane", 2.0, {LABOR: 0.6}),
    "Vineyard":          ("grapes",     1.5, {LABOR: 0.6}),
    "Silk farm":         ("silk",       0.8, {LABOR: 0.6}),
    "Clay pit":          ("clay",       2.0, {LABOR: 0.6}),
}
# Processing can be set up anywhere.
PROCESSING = {
    "Bakery":    ("bread",        2.0, {LABOR: 0.3, "grain": 0.4, "fuel": 0.1}),
    "Brewery":   ("beer",         1.5, {LABOR: 0.3, "grain": 0.4}),
    "Mill":      ("cloth",        1.5, {LABOR: 0.3, "fiber": 0.4}),
    "Tailor":    ("clothes",      1.5, {LABOR: 0.3, "cloth": 0.4}),
    "Smelter":   ("iron",         1.0, {LABOR: 0.2, "iron ore": 0.3, "fuel": 0.2}),
    "Smithy":    ("ironware",     1.2, {LABOR: 0.3, "iron": 0.3, "fuel": 0.1}),
    "Carpenter": ("furniture",    1.2, {LABOR: 0.3, "wood": 0.4}),
    "Refinery":  ("sugar",        1.2, {LABOR: 0.2, "sugar cane": 0.4, "fuel": 0.1}),
    "Winery":    ("wine",         1.2, {LABOR: 0.3, "grapes": 0.4}),
    "Couturier": ("fine clothes", 1.0, {LABOR: 0.4, "silk": 0.3}),
    "Kiln":      ("porcelain",    1.0, {LABOR: 0.4, "clay": 0.2, "fuel": 0.2}),
    "Transport": (TRANSPORT,      2.0, {LABOR: 0.6}),
}

# ---- terrain ------------------------------------------------------------------
# symbol, cost to move through, raw producers available (with TFP multiplier)
TERRAIN = {
    "plains":    ("P", 1.0,  {"Farm": 1.5, "Potato farm": 1.2, "Ranch": 1.3,
                              "Sheep farm": 1.0, "Clay pit": 1.0}),
    "forest":    ("F", 1.0,  {"Forestry": 1.8, "Potato farm": 0.8, "Ranch": 0.7}),
    "hills":     ("H", 1.0,  {"Sheep farm": 1.5, "Vineyard": 1.5, "Coal mine": 1.0,
                              "Clay pit": 1.3, "Potato farm": 1.0}),
    "mountains": ("M", 1.0,  {"Coal mine": 1.5, "Iron mine": 1.5, "Sheep farm": 0.8}),
    "tropical":  ("T", 1.0,  {"Cotton plantation": 1.5, "Sugar plantation": 1.5,
                              "Tea plantation": 1.5, "Silk farm": 1.2, "Farm": 0.6}),
    "coast":     ("C", 0.25, {"Fishery": 1.5, "Farm": 0.8, "Potato farm": 1.0,
                              "Clay pit": 0.8}),
}
SYMBOL = {sym: name for name, (sym, _, _) in TERRAIN.items()}

# 6x6 hex map, "odd-r" offset layout (odd rows are shifted half a hex right).
MAP = [
    "MMHFFP",
    "HHFPPP",
    "FFPPHC",
    "PPPTTC",
    "CPTTCC",
    "CCCCCC",
]


def hex_neighbors(r: int, c: int, rows: int, cols: int) -> list[tuple[int, int]]:
    if r % 2 == 0:
        deltas = [(-1, -1), (-1, 0), (0, -1), (0, 1), (1, -1), (1, 0)]
    else:
        deltas = [(-1, 0), (-1, 1), (0, -1), (0, 1), (1, 0), (1, 1)]
    return [(r + dr, c + dc) for dr, dc in deltas
            if 0 <= r + dr < rows and 0 <= c + dc < cols]


def make_firms(terrain: str) -> list[Firm]:
    _, _, raw = TERRAIN[terrain]
    firms = [Firm(name, out, tfp * raw[name], dict(exps), cash=50)
             for name, (out, tfp, exps) in RAW.items() if name in raw]
    firms += [Firm(name, out, tfp, dict(exps), cash=50)
              for name, (out, tfp, exps) in PROCESSING.items()]
    return firms


def build_world(transport_cost: float = 1.0, game_map: list[str] = MAP) -> World:
    """transport_cost scales every hop; math.inf means autarky (no trade).

    The cost of a hop between neighbouring tiles is the average of the two
    tiles' movement costs (land 1, coast 1/4). Traders only run between
    neighbours; longer trade happens by re-export along a chain of tiles.
    """
    rows, cols = len(game_map), len(game_map[0])
    terrain = {(r, c): SYMBOL[game_map[r][c]] for r in range(rows) for c in range(cols)}
    regions = {}
    for (r, c), t in terrain.items():
        reg = Region(f"{r}{c}", make_firms(t), n_pops=2, pop_size=50, pop_cash=500)
        reg.terrain, reg.coord = t, (r, c)
        regions[(r, c)] = reg

    traders = []
    if math.isfinite(transport_cost):
        for (r, c), a in regions.items():
            for nb in hex_neighbors(r, c, rows, cols):
                b = regions[nb]
                hop = (TERRAIN[a.terrain][1] + TERRAIN[b.terrain][1]) / 2
                traders += [Trader(GOODS[g], a, b, hop, transport_cost) for g in TRADABLE]
    return World(list(regions.values()), traders)
