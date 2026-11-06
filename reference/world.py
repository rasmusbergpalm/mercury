"""The world: several regions linked by traders that pay transport costs."""
from __future__ import annotations

from collections import defaultdict

from needs import TIER_OF
from region import Region
from trader import Trader


class World:
    def __init__(self, regions: list[Region], traders: list[Trader]):
        self.regions = regions
        self.traders = traders
        for r in regions:
            r.connect(traders)
        self.tick = 0
        self.history: list[dict] = []

    def money(self) -> float:
        return sum(r.money() for r in self.regions) + sum(t.cash for t in self.traders)

    def step(self) -> dict:
        prices_now = {r.name: r.prices for r in self.regions}   # the prices trade happens at
        wages_now = {r.name: r.wage for r in self.regions}

        for r in self.regions:
            r.plan()
        for t in self.traders:
            t.plan()
        for r in self.regions:
            r.labor_market()
        for r in self.regions:
            r.goods_markets()
        for t in self.traders:
            t.ship()
        for r in self.regions:
            r.produce()
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
            "tier_spend": {r.name: self._tier_spend(r, prices_now[r.name]) for r in self.regions},
            "labor": {r.name: {f.name: f.labor for f in r.firms} for r in self.regions},
            "output": {r.name: {f.name: f.last_output for f in r.firms} for r in self.regions},
            "flows": {t.name: t.shipped for t in self.traders},
            "money": self.money(),
        }
        self.history.append(rec)
        return rec

    @staticmethod
    def _tier_spend(region: Region, prices: dict[str, float]) -> dict[str, float]:
        spend: dict[str, float] = defaultdict(float)
        for p in region.pops:
            for g, q in p.consumed.items():
                spend[TIER_OF[g]] += q * prices[g]
        return dict(spend)
