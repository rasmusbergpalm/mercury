"""A single-good call market: proportional rationing + tatonnement price update."""
from __future__ import annotations

import math
from typing import Any


class Market:
    def __init__(self, good: str, price: float = 1.0, speed: float = 0.1):
        self.good = good
        self.price = price
        self.speed = speed
        self.supply = 0.0
        self.demand = 0.0

    def clear(self, bids: list[tuple[Any, float]], asks: list[tuple[Any, float]]
              ) -> tuple[list[float], list[float]]:
        """Trade at the current price, rationing the long side proportionally.

        Agents need a `.cash` attribute; cash is transferred here, goods are the
        caller's job (using the returned fill quantities). Then the price moves
        by exp(speed * excess demand).
        """
        price = self.price
        supply = sum(q for _, q in asks)
        demand = sum(q for _, q in bids)
        buy_frac = min(1.0, supply / demand) if demand > 0 else 0.0
        sell_frac = min(1.0, demand / supply) if supply > 0 else 0.0

        bid_fills = []
        for agent, q in bids:
            got = q * buy_frac
            agent.cash -= got * price
            bid_fills.append(got)
        ask_fills = []
        for agent, q in asks:
            sold = q * sell_frac
            agent.cash += sold * price
            ask_fills.append(sold)

        excess = (demand - supply) / max(supply, 1e-9)
        self.price *= math.exp(self.speed * max(-1.0, min(1.0, excess)))
        self.supply, self.demand = supply, demand
        return bid_fills, ask_fills
