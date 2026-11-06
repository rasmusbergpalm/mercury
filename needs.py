"""Pop needs: tiers of need groups, goods within a group substitute.

Utility per person:  U = sum_k w_k * log(1 + Q_k / r_k)
  Q_k  CES composite of the goods in need group k
  r_k  the 'requirement' that scales the group
  w_k  weight

The marginal utility of a group at zero is w_k / r_k, so a pop only starts
spending on a group once its budget is big enough. Basic needs have small
requirements (high value at first, quickly satisfied), luxuries large ones
(only worth buying once the basics are covered, but never satiated): poor
pops buy basics, richer ones add intermediate needs, then luxuries. As
income grows, spending shares tend to the ratio of the weights.
"""
from __future__ import annotations

import math
from collections.abc import Mapping
from dataclasses import dataclass

from ces import aggregate, price_index, split

TIERS = ("basic", "intermediate", "luxury")


@dataclass(frozen=True)
class Need:
    name: str
    tier: str
    goods: tuple[str, ...]
    sigma: float        # elasticity of substitution between the goods (≠ 1)
    weight: float
    requirement: float


NEEDS: list[Need] = [
    #     name        tier            goods                          sigma weight  req
    Need("staples",  "basic",        ("bread", "potatoes"),          5.0, 2.0, 0.05),
    Need("protein",  "basic",        ("meat", "fish"),               4.0, 2.0, 0.05),
    Need("clothing", "basic",        ("clothes",),                   2.0, 2.0, 0.05),
    Need("heating",  "basic",        ("wood", "coal"),               5.0, 2.0, 0.05),
    Need("home",     "intermediate", ("furniture", "ironware"),      0.5, 2.0, 0.015),
    Need("drink",    "intermediate", ("beer", "tea"),                2.0, 2.0, 0.1),
    Need("sweets",   "intermediate", ("sugar",),                     2.0, 2.0, 0.1),
    Need("wine",     "luxury",       ("wine",),                      2.0, 2.0, 0.2),
    Need("finery",   "luxury",       ("fine clothes", "porcelain"),  2.0, 2.0, 0.2),
]

CONSUMER_GOODS = [g for n in NEEDS for g in n.goods]
TIER_OF = {g: n.tier for n in NEEDS for g in n.goods}


def demand(prices: Mapping[str, float], budget: float) -> dict[str, float]:
    """Utility-maximising purchases for one person with `budget`.

    Spending E_k on group k buys Q_k = E_k / P_k. The FOC
        w_k / (r_k P_k + E_k) = lambda
    gives E_k = max(0, w_k/lambda - r_k P_k): water-filling. Groups enter in
    order of w_k / (r_k P_k); solve for lambda over the active set.
    """
    budget = max(budget, 0.0)
    unit = {n.name: price_index(prices, n.goods, n.sigma) for n in NEEDS}
    floor = {n.name: n.requirement * unit[n.name] for n in NEEDS}
    order = sorted(NEEDS, key=lambda n: n.weight / floor[n.name], reverse=True)

    for m in range(1, len(order) + 1):
        active = order[:m]
        inv_lambda = (budget + sum(floor[n.name] for n in active)) / sum(n.weight for n in active)
        if m == len(order) or order[m].weight * inv_lambda <= floor[order[m].name]:
            break

    out: dict[str, float] = {}
    for n in active:
        spend = max(0.0, n.weight * inv_lambda - floor[n.name])
        out.update(split(spend / unit[n.name], prices, n.goods, n.sigma))
    return out


def utility(consumed: Mapping[str, float]) -> float:
    """Utility of one person consuming `consumed`."""
    return sum(n.weight * math.log1p(aggregate(consumed, n.goods, n.sigma) / n.requirement)
               for n in NEEDS)
