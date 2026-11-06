"""Pops: groups of identical people that supply labor and consume goods."""
from __future__ import annotations

from dataclasses import dataclass, field

import needs


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
        """Every member spends an equal share of the pop's cash (see needs.demand)."""
        per_person = needs.demand(prices, self.cash / self.size)
        return {g: q * self.size for g, q in per_person.items()}

    def utility(self) -> float:
        """Utility per person."""
        return needs.utility({g: q / self.size for g, q in self.consumed.items()})
