"""Pops: groups of identical people that supply labor and consume goods."""
from __future__ import annotations

import math
from dataclasses import dataclass, field


@dataclass
class Pop:
    id: int
    size: float                     # number of people = units of labor supplied
    cash: float
    jobs: dict[str, float] = field(default_factory=dict)      # firm name -> labor employed
    consumed: dict[str, float] = field(default_factory=dict)  # this tick, whole pop

    @property
    def employed(self) -> float:
        return sum(self.jobs.values())

    def demand(self, prices: dict[str, float]) -> dict[str, float]:
        """Each member maximises sum_g sqrt(x_g) on an equal share of the cash.

        FOC: 1/(2 sqrt(x_g)) = lambda p_g  =>  x_g ∝ 1/p_g^2, and the budget gives
            x_g = cash / (p_g^2 * sum_h 1/p_h)
        (the same formula holds for the pop as a whole since it is homogeneous).
        """
        cash = max(self.cash, 0.0)
        inv = sum(1.0 / p for p in prices.values())
        return {g: cash / (p * p * inv) for g, p in prices.items()}

    def utility(self) -> float:
        """Utility per person."""
        return sum(math.sqrt(x / self.size) for x in self.consumed.values())
