"""The world: several regions linked by traders that pay transport costs."""
from __future__ import annotations

from region import Region
from trader import Trader


class World:
    def __init__(self, regions: list[Region], traders: list[Trader], spoilage: float = 0.2):
        self.regions = regions
        self.traders = traders
        self.spoilage = spoilage
        self.tick = 0
        self.history: list[dict] = []

    def money(self) -> float:
        return sum(r.money() for r in self.regions) + sum(t.cash for t in self.traders)

    def step(self) -> dict:
        prices_now = {r.name: r.prices for r in self.regions}
        wages_now = {r.name: r.wage for r in self.regions}

        for r in self.regions:
            r.plan()
        for t in self.traders:
            t.plan()
        for r in self.regions:
            r.labor_market()
        for r in self.regions:
            r.goods_markets(outbound=[t for t in self.traders if t.origin is r],
                            inbound=[t for t in self.traders if t.dest is r])
        for t in self.traders:
            t.ship(self.spoilage)
        for r in self.regions:
            r.produce(self.spoilage)
            for f in r.firms:
                r.pay_dividends(f)
        for t in self.traders:
            t.origin.pay_dividends(t)

        self.tick += 1
        rec = {
            "tick": self.tick,
            "prices": prices_now,
            "wages": wages_now,
            "utility": {r.name: r.utility() for r in self.regions},
            "workers": {r.name: {f.name: f.labor for f in r.firms} for r in self.regions},
            "output": {r.name: {f.name: f.last_output for f in r.firms} for r in self.regions},
            "flows": {t.name: t.shipped for t in self.traders},
            "money": self.money(),
        }
        self.history.append(rec)
        return rec
