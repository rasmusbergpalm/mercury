"""Run the baseline multi-region world, then sweep the transport cost."""
from __future__ import annotations

import math
from collections import defaultdict

from scenarios import CONSUMER_GOODS, GOODS, build_world
from world import World


def avg(history: list[dict], key, last: int = 50):
    rows = history[-last:]
    return sum(key(r) for r in rows) / len(rows)


def report(world: World) -> None:
    h = world.history
    print(f"After {world.tick} ticks (averages over the last 50). Money in system: {h[-1]['money']:.1f}\n")

    print(f"{'region':<10} {'wage':>6} " + " ".join(f"{g:>9}" for g in GOODS) + f" {'utility':>8}")
    for r in world.regions:
        n = r.name
        print(f"{n:<10} {avg(h, lambda x: x['wages'][n]):>6.2f} " +
              " ".join(f"{avg(h, lambda x: x['prices'][n][g]):>9.2f}" for g in GOODS) +
              f" {avg(h, lambda x: x['utility'][n]):>8.3f}")

    print("\nWorkers by firm:")
    firms = [f.name for f in world.regions[0].firms]
    print(f"{'':<10} " + " ".join(f"{f:>9}" for f in firms))
    for r in world.regions:
        print(f"{r.name:<10} " + " ".join(f"{avg(h, lambda x: x['workers'][r.name][f]):>9.1f}" for f in firms))

    print("\nTrade flows (units shipped per tick, > 0.05 shown):")
    for t in sorted(world.traders, key=lambda t: -avg(h, lambda x: x["flows"][t.name])):
        q = avg(h, lambda x: x["flows"][t.name])
        if q > 0.05:
            print(f"  {t.name:<28} {q:6.2f}   (tau={t.tau:.1f}, margin={t.margin:+.2f})")


def sweep(ticks: int = 800) -> None:
    print("\nTransport-cost sweep (tau = cost x distance):")
    print(f"{'cost':>6} {'trade vol':>9} {'transp.workers':>14} {'bread price gap':>15} "
          + " ".join(f"{'U_'+r:>11}" for r in ["Plains", "Highlands", "Coast"]) + f" {'U_avg':>7}")
    for cost in [0.0, 0.25, 0.5, 1.0, 2.0, 4.0, math.inf]:
        w = build_world(cost)
        for _ in range(ticks):
            w.step()
        h = w.history
        vol = avg(h, lambda x: sum(x["flows"].values()))
        tw = avg(h, lambda x: sum(x["workers"][r.name]["Transport"] for r in w.regions))
        gap = avg(h, lambda x: max(p["bread"] for p in x["prices"].values())
                  / min(p["bread"] for p in x["prices"].values()))
        us = {r.name: avg(h, lambda x: x["utility"][r.name]) for r in w.regions}
        label = "inf" if math.isinf(cost) else f"{cost:.2f}"
        print(f"{label:>6} {vol:>9.2f} {tw:>14.1f} {gap:>15.2f} "
              + " ".join(f"{us[r]:>11.3f}" for r in us) + f" {sum(us.values())/len(us):>7.3f}")


if __name__ == "__main__":
    world = build_world(transport_cost=1.0)
    for _ in range(800):
        world.step()
    report(world)
    sweep()
