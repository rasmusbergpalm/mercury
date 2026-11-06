"""CES composites: Q = (sum_g x_g^rho)^(1/rho), with sigma = 1/(1-rho).

Used both for substitutable consumer goods (bread vs potatoes) and for
substitutable production inputs (wood vs coal as fuel).
"""
from __future__ import annotations

from collections.abc import Mapping, Sequence


def price_index(prices: Mapping[str, float], goods: Sequence[str], sigma: float) -> float:
    """Cost of one unit of the composite when buying the cheapest mix."""
    if len(goods) == 1:
        return prices[goods[0]]
    return sum(prices[g] ** (1 - sigma) for g in goods) ** (1 / (1 - sigma))


def split(quantity: float, prices: Mapping[str, float], goods: Sequence[str],
          sigma: float) -> dict[str, float]:
    """Cost-minimising goods mix that yields `quantity` of the composite."""
    if len(goods) == 1:
        return {goods[0]: quantity}
    p = price_index(prices, goods, sigma)
    return {g: quantity * (prices[g] / p) ** (-sigma) for g in goods}


def aggregate(quantities: Mapping[str, float], goods: Sequence[str], sigma: float) -> float:
    """Composite quantity from a goods mix."""
    if len(goods) == 1:
        return max(quantities.get(goods[0], 0.0), 0.0)
    rho = (sigma - 1) / sigma
    xs = [max(quantities.get(g, 0.0), 0.0) for g in goods]
    if rho < 0 and min(xs) == 0:      # complements: any missing good -> nothing
        return 0.0
    return sum(x ** rho for x in xs if x > 0) ** (1 / rho)
