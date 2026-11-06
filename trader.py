"""Traders: merchants on one route (origin -> dest) for one good.

They buy the good in the origin market, plus `tau` units of the origin's
transport good per unit shipped. The goods arrive in the destination next
tick and are sold there. The quantity bought each tick adapts to the margin
    p_dest - p_origin - tau * p_transport_origin
so trade grows while the price gap is bigger than the transport cost, and
shrinks when it isn't.
"""
from __future__ import annotations

import math
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from region import Region

TRANSPORT = "transport"


class Trader:
    def __init__(self, good: str, origin: "Region", dest: "Region", tau: float,
                 cash: float = 20.0, speed: float = 0.1, damping: float = 10.0,
                 q_min: float = 0.02):
        self.good, self.origin, self.dest, self.tau = good, origin, dest, tau
        self.cash = cash
        self.speed, self.damping, self.q_min = speed, damping, q_min
        self.q = q_min              # target purchase per tick
        self.stock_origin = 0.0     # bought earlier, still waiting for transport
        self.bought = 0.0           # bought this tick
        self.stock_dest = 0.0       # arrived, for sale in dest
        self.transport = 0.0        # transport units bought this tick
        self.shipped = 0.0          # last tick's shipment
        self.margin = 0.0
        self.rel_margin = 0.0
        self.desired_spend = 0.0

    @property
    def name(self) -> str:
        return f"{self.good}:{self.origin.name}->{self.dest.name}"

    def plan(self) -> None:
        po, pd = self.origin.prices, self.dest.prices
        self.margin = pd[self.good] - po[self.good] - self.tau * po[TRANSPORT]
        rel = max(-1.0, min(1.0, self.margin / pd[self.good]))
        # grow with the margin, but brake when the margin is shrinking (damps cycles)
        signal = rel + self.damping * (rel - self.rel_margin)
        self.rel_margin = rel
        self.q = max(self.q_min, self.q * math.exp(self.speed * max(-1.0, min(1.0, signal))))
        cost = self.q * (po[self.good] + self.tau * po[TRANSPORT]) \
            + self.stock_origin * self.tau * po[TRANSPORT]
        self.desired_spend = cost
        if cost > 0.9 * self.cash:
            self.q *= 0.9 * self.cash / cost

    def bid(self, good: str) -> float:
        if good == self.good:
            return self.q
        if good == TRANSPORT:
            return self.tau * (self.q + self.stock_origin)
        return 0.0

    def receive(self, good: str, qty: float) -> None:
        if good == self.good:
            self.bought += qty
        elif good == TRANSPORT:
            self.transport += qty

    def ship(self, spoilage: float) -> None:
        """Ship what transport allows. Only stock that has already waited a tick
        (unsold at dest, or stuck at origin) spoils; fresh purchases don't."""
        self.stock_dest *= 1 - spoilage
        self.stock_origin = self.stock_origin * (1 - spoilage) + self.bought
        self.bought = 0.0
        n = self.stock_origin if self.tau == 0 else min(self.stock_origin, self.transport / self.tau)
        self.stock_origin -= n
        self.stock_dest += n        # arrives next tick
        self.shipped = n
        self.transport = 0.0        # transport is a service: unused capacity is lost
