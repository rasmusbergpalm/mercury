"""Goods and their physical traits.

bulk:     transport units needed per unit shipped per unit of distance
spoilage: fraction of unsold stock lost per tick, and per unit of distance in transit
"""
from __future__ import annotations

from dataclasses import dataclass

TRANSPORT = "transport"


@dataclass(frozen=True)
class Good:
    name: str
    bulk: float
    spoilage: float


GOODS: dict[str, Good] = {g.name: g for g in [
    # basic consumer goods
    Good("bread",        1.0, 0.30),
    Good("potatoes",     2.0, 0.10),
    Good("meat",         1.0, 0.40),
    Good("fish",         1.0, 0.50),
    Good("clothes",      0.5, 0.02),
    Good("wood",         2.0, 0.02),
    Good("coal",         2.0, 0.00),
    # intermediate consumer goods
    Good("furniture",    1.5, 0.02),
    Good("ironware",     1.0, 0.00),
    Good("beer",         1.5, 0.10),
    Good("tea",          0.3, 0.02),
    Good("sugar",        0.5, 0.02),
    # luxury consumer goods
    Good("wine",         0.5, 0.01),
    Good("fine clothes", 0.2, 0.01),
    Good("porcelain",    0.5, 0.00),
    # industrial inputs (not consumed by pops)
    Good("grain",        1.5, 0.05),
    Good("iron ore",     2.0, 0.00),
    Good("iron",         1.0, 0.00),
    Good("wool",         1.0, 0.02),
    Good("cotton",       1.0, 0.02),
    Good("cloth",        0.5, 0.02),
    Good("sugar cane",   2.0, 0.20),
    Good("grapes",       1.0, 0.30),
    Good("silk",         0.2, 0.01),
    Good("clay",         2.0, 0.00),
    # a service: whatever isn't used in the tick it's produced is lost
    Good(TRANSPORT,      0.0, 1.00),
]}

# Substitutable inputs for production: name -> (goods, elasticity of substitution)
COMPOSITES: dict[str, tuple[tuple[str, ...], float]] = {
    "fuel":  (("wood", "coal"), 3.0),
    "fiber": (("wool", "cotton"), 3.0),
}

TRADABLE = [g for g in GOODS if g != TRANSPORT]
