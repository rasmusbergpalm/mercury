"""World set-ups."""
from __future__ import annotations

import math
from itertools import permutations

from firm import LABOR, Firm
from region import Region
from trader import TRANSPORT, Trader
from world import World

CONSUMER_GOODS = ["wheat", "wool", "bread", "cloth"]
GOODS = CONSUMER_GOODS + [TRANSPORT]
TRADABLE = CONSUMER_GOODS  # everything but transport itself can be shipped

# Per-region productivity bonuses (multiplies the firm's TFP).
BONUSES = {
    "Plains":    {"wheat": 1.5, "bread": 1.5},
    "Highlands": {"wool": 1.5, "cloth": 1.5},
    "Coast":     {TRANSPORT: 1.5},
}

# Distance between regions; transport needed per unit shipped = cost * distance.
DISTANCE = {
    frozenset({"Plains", "Highlands"}): 2.0,
    frozenset({"Plains", "Coast"}): 1.0,
    frozenset({"Highlands", "Coast"}): 1.0,
}


def make_firms(bonus: dict[str, float]) -> list[Firm]:
    b = lambda g: bonus.get(g, 1.0)
    return [
        Firm("Farm",      "wheat",   2.0 * b("wheat"),   {LABOR: 0.6}, cash=50),
        Firm("Ranch",     "wool",    1.5 * b("wool"),    {LABOR: 0.6}, cash=50),
        Firm("Bakery",    "bread",   2.0 * b("bread"),   {LABOR: 0.3, "wheat": 0.5}, cash=50),
        Firm("Weaver",    "cloth",   1.5 * b("cloth"),   {LABOR: 0.3, "wool": 0.5}, cash=50),
        Firm("Transport", TRANSPORT, 2.0 * b(TRANSPORT), {LABOR: 0.6}, cash=50),
    ]


def build_world(transport_cost: float = 1.0) -> World:
    """transport_cost scales all distances; math.inf means autarky (no trade)."""
    regions = {name: Region(name, make_firms(bonus), GOODS, CONSUMER_GOODS)
               for name, bonus in BONUSES.items()}
    traders = []
    if math.isfinite(transport_cost):
        for a, b in permutations(regions.values(), 2):
            tau = transport_cost * DISTANCE[frozenset({a.name, b.name})]
            traders += [Trader(g, a, b, tau) for g in TRADABLE]
    return World(list(regions.values()), traders)
