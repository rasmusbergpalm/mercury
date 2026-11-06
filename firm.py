"""Firms: turn labor + inputs into one output good (decreasing-returns Cobb-Douglas).

A factor can be labor, a good, or a composite of substitutable goods (see
goods.COMPOSITES, e.g. fuel = wood or coal).
"""
from __future__ import annotations

from dataclasses import dataclass, field

from ces import aggregate, price_index, split
from goods import COMPOSITES

LABOR = "labor"


@dataclass
class Firm:
    name: str
    output: str
    tfp: float                      # A in q = A * prod f_i^a_i
    exponents: dict[str, float]     # factor -> exponent; sum < 1
    cash: float
    cash_buffer: float = 20.0       # cash kept beyond what the plan needs
    inventory: float = 0.0          # stock of own output, for sale
    inputs: dict[str, float] = field(default_factory=dict)  # goods bought this tick
    labor: float = 0.0              # labor employed this tick
    last_output: float = 0.0
    desired_spend: float = 0.0      # unconstrained cost of the optimal plan

    def _unit_cost(self, factor: str, prices: dict[str, float], wage: float) -> float:
        if factor == LABOR:
            return wage
        if factor in COMPOSITES:
            goods, sigma = COMPOSITES[factor]
            return price_index(prices, goods, sigma)
        return prices[factor]

    def plan(self, prices: dict[str, float], wage: float) -> dict[str, float]:
        """Profit-maximising labor and goods demand at current prices, capped by cash.

        With q = A prod f_i^a_i and s = sum a_i < 1:
            q* = (A prod (a_i p / w_i)^a_i)^(1/(1-s)),   f_i* = a_i p q* / w_i
        Composite factors are then split into their cheapest goods mix.
        """
        p = prices[self.output]
        cost = {i: self._unit_cost(i, prices, wage) for i in self.exponents}
        s = sum(self.exponents.values())
        k = self.tfp
        for i, a in self.exponents.items():
            k *= (a * p / cost[i]) ** a
        q = k ** (1.0 / (1.0 - s))
        want = {i: a * p * q / cost[i] for i, a in self.exponents.items()}

        spend = sum(x * cost[i] for i, x in want.items())
        self.desired_spend = spend
        budget = 0.9 * max(self.cash, 0.0)
        if spend > budget:                       # liquidity constraint
            want = {i: x * budget / spend for i, x in want.items()}

        out: dict[str, float] = {}
        for i, x in want.items():
            if i in COMPOSITES:
                goods, sigma = COMPOSITES[i]
                out.update(split(x, prices, goods, sigma))
            else:
                out[i] = x
        return out

    def factor_amount(self, factor: str) -> float:
        if factor == LABOR:
            return self.labor
        if factor in COMPOSITES:
            goods, sigma = COMPOSITES[factor]
            return aggregate(self.inputs, goods, sigma)
        return max(self.inputs.get(factor, 0.0), 0.0)

    def produce(self) -> float:
        q = self.tfp
        for i, a in self.exponents.items():
            q *= self.factor_amount(i) ** a
        self.inventory += q
        self.inputs = {}
        self.last_output = q
        return q
