"""Run the hex-map world and report; `python3 main.py --sweep` also sweeps transport cost."""
from __future__ import annotations

import math
import sys
from collections import defaultdict

from needs import TIERS
from scenarios import MAP, build_world
from world import World

TICKS = 1000
AVG = 50


def avg(history: list[dict], key, last: int = AVG):
    rows = history[-last:]
    return sum(key(r) for r in rows) / len(rows)


def hex_map(world: World, cell) -> str:
    """Render a value per region on the hex grid (odd rows shifted right)."""
    by_coord = {r.coord: r for r in world.regions}
    lines = []
    for row in range(len(MAP)):
        pad = "    " if row % 2 else ""
        lines.append(pad + "  ".join(cell(by_coord[(row, c)]) for c in range(len(MAP[0]))))
    return "\n".join(lines)


def tier_shares(h, name):
    spend = {t: avg(h, lambda x: x["tier_spend"][name].get(t, 0.0)) for t in TIERS}
    total = sum(spend.values()) or 1.0
    return {t: spend[t] / total for t in TIERS}


def report(world: World) -> None:
    h = world.history
    print(f"After {world.tick} ticks (averages over the last {AVG}). "
          f"Money in system: {h[-1]['money']:.0f}\n")

    print("Terrain  (P plains, F forest, H hills, M mountains, T tropical, C coast)")
    print(hex_map(world, lambda r: f"{r.name}:{MAP[r.coord[0]][r.coord[1]]}  "))
    print("\nUtility per person")
    print(hex_map(world, lambda r: f"{avg(h, lambda x: x['utility'][r.name]):6.2f}"))
    print("\nLuxury share of pop spending (%)")
    print(hex_map(world, lambda r: f"{100 * tier_shares(h, r.name)['luxury']:6.1f}"))
    for g in ["bread", "coal", "fish", "tea", "porcelain"]:
        print(f"\nPrice of {g} relative to the local wage")
        print(hex_map(world, lambda r, g=g: f"{avg(h, lambda x: x['prices'][r.name][g] / x['wages'][r.name]):6.2f}"))

    print("\nBy terrain:")
    print(f"  {'terrain':<10} {'n':>2} {'utility':>8} " + " ".join(f"{t:>12}" for t in TIERS))
    groups = defaultdict(list)
    for r in world.regions:
        groups[r.terrain].append(r)
    for t, rs in groups.items():
        u = sum(avg(h, lambda x: x["utility"][r.name]) for r in rs) / len(rs)
        sh = [sum(tier_shares(h, r.name)[k] for r in rs) / len(rs) for k in TIERS]
        print(f"  {t:<10} {len(rs):>2} {u:>8.2f} " + " ".join(f"{100 * s:>11.1f}%" for s in sh))

    print("\nEmployment by firm type (whole map, people):")
    labor = defaultdict(float)
    for r in world.regions:
        for f in r.firms:
            labor[f.name] += avg(h, lambda x: x["labor"][r.name][f.name])
    for name, l in sorted(labor.items(), key=lambda kv: -kv[1]):
        print(f"  {name:<18} {l:7.1f}")

    print("\nBusiest trade links (units loaded per tick):")
    flows = {t.name: avg(h, lambda x: x["flows"][t.name]) for t in world.traders}
    for name, q in sorted(flows.items(), key=lambda kv: -kv[1])[:15]:
        print(f"  {name:<26} {q:6.2f}")


def sweep() -> None:
    print("\nTransport-cost sweep (multiplier on every hop):")
    print(f"{'cost':>6} {'trade vol':>10} {'transport workers':>18} {'utility':>8} "
          + " ".join(f"{t:>13}" for t in TIERS))
    for cost in [0.25, 0.5, 1.0, 2.0, math.inf]:
        w = build_world(cost)
        for _ in range(TICKS):
            w.step()
        h = w.history
        vol = avg(h, lambda x: sum(x["flows"].values()))
        tw = avg(h, lambda x: sum(x["labor"][r.name]["Transport"] for r in w.regions))
        u = avg(h, lambda x: sum(x["utility"].values()) / len(w.regions))
        sh = [sum(tier_shares(h, r.name)[k] for r in w.regions) / len(w.regions) for k in TIERS]
        label = "inf" if math.isinf(cost) else f"{cost:.2f}"
        print(f"{label:>6} {vol:>10.1f} {tw:>18.1f} {u:>8.3f} "
              + " ".join(f"{100 * s:>12.1f}%" for s in sh))


if __name__ == "__main__":
    world = build_world()
    for _ in range(TICKS):
        world.step()
    report(world)
    if "--sweep" in sys.argv:
        sweep()
