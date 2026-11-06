"""Write the Python reference trajectory as JSON for the Go parity test.

Usage: python3 reference/export_reference.py engine/testdata/reference.json
"""
from __future__ import annotations

import json
import sys

from goods import GOODS
from scenarios import build_world

CHECKPOINTS = [1, 2, 3, 5, 10, 20, 50, 100, 200, 400]


def snapshot(world) -> dict:
    goods = list(GOODS)
    return {
        "tick": world.tick,
        "money": world.money(),
        "regions": [{
            "name": r.name,
            "wage": r.wage,
            "prices": [r.markets[g].price for g in goods],
            "utility": r.utility(),
            "pop_cash": [p.cash for p in r.pops],
            "firms": [{"name": f.name, "cash": f.cash, "inventory": f.inventory,
                       "labor": f.labor} for f in r.firms],
        } for r in world.regions],
        "trade_volume": sum(t.shipped for t in world.traders),
    }


if __name__ == "__main__":
    world = build_world(1.0)
    out = []
    for tick in range(1, CHECKPOINTS[-1] + 1):
        world.step()
        if tick in CHECKPOINTS:
            out.append(snapshot(world))
    with open(sys.argv[1], "w") as fh:
        json.dump({"transport_cost": 1.0, "checkpoints": out}, fh)
