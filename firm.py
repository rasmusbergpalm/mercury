"""Firms: turn labor + input goods into one output good (decreasing-returns Cobb-Douglas)."""
from __future__ import annotations

from dataclasses import dataclass, field

LABOR = "labor"


@dataclass
class Firm:
    name: str
    output: str
    tfp: float                      # A in q = A * prod x_i^a_i
    exponents: dict[str, float]     # input -> exponent (may include LABOR); sum < 1
    cash: float
    inventory: float = 0.0          # stock of own output, for sale
    inputs: dict[str, float] = field(default_factory=dict)  # inputs bought this tick
    labor: float = 0.0              # labor employed this tick
    last_output: float = 0.0
    desired_spend: float = 0.0      # unconstrained cost of the optimal plan

    def plan(self, prices: dict[str, float], wage: float) -> dict[str, float]:
        """Profit-maximising input demand at current prices, capped by cash.

        With q = A prod x_i^a_i and s = sum a_i < 1:
            q* = (A prod (a_i p / w_i)^a_i)^(1/(1-s)),   x_i* = a_i p q* / w_i
        """
        p = prices[self.output]
        cost = lambda i: wage if i == LABOR else prices[i]
        s = sum(self.exponents.values())
        k = self.tfp
        for i, a in self.exponents.items():
            k *= (a * p / cost(i)) ** a
        q = k ** (1.0 / (1.0 - s))
        want = {i: a * p * q / cost(i) for i, a in self.exponents.items()}

        spend = sum(x * cost(i) for i, x in want.items())
        self.desired_spend = spend
        budget = 0.9 * max(self.cash, 0.0)
        if spend > budget:                       # liquidity constraint
            want = {i: x * budget / spend for i, x in want.items()}
        return want

    def produce(self) -> float:
        q = self.tfp
        for i, a in self.exponents.items():
            q *= max(self.labor if i == LABOR else self.inputs.get(i, 0.0), 0.0) ** a
        self.inventory += q
        self.inputs = {}
        self.last_output = q
        return q
