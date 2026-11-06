"""A region: its own pops, firms, labor market and goods markets."""
from __future__ import annotations

import math
from typing import TYPE_CHECKING

from firm import LABOR, Firm
from market import Market
from pop import Pop

if TYPE_CHECKING:
    from trader import Trader


class Region:
    def __init__(self, name: str, firms: list[Firm], goods: list[str],
                 consumer_goods: list[str], n_pops: int = 10, pop_size: float = 10.0,
                 pop_cash: float = 100.0, price_speed: float = 0.1, wage_speed: float = 0.1):
        self.name = name
        self.firms = firms
        self.pops = [Pop(i, pop_size, pop_cash) for i in range(n_pops)]
        self.goods = goods
        self.consumer_goods = consumer_goods
        self.markets = {g: Market(g, 1.0, price_speed) for g in goods}
        self.wage = 1.0
        self.wage_speed = wage_speed
        self.plans: dict[str, dict[str, float]] = {}

    @property
    def prices(self) -> dict[str, float]:
        return {g: m.price for g, m in self.markets.items()}

    def plan(self) -> None:
        prices = self.prices
        self.plans = {f.name: f.plan(prices, self.wage) for f in self.firms}

    # ---- labor ----------------------------------------------------------------
    def labor_market(self) -> None:
        """Firms hire at the going wage; if they want more labor than exists, each
        gets the same fraction. Pops fill the jobs in turn (a pop's labor can be
        split across firms), and get paid by the firm that employs them."""
        demand = {f.name: self.plans[f.name].get(LABOR, 0.0) for f in self.firms}
        total_d = sum(demand.values())
        supply = sum(p.size for p in self.pops)
        scale = min(1.0, supply / total_d) if total_d > 0 else 0.0

        for p in self.pops:
            p.jobs = {}
        pops = iter(self.pops)
        pop, free = next(pops), self.pops[0].size
        for f in self.firms:
            f.labor = demand[f.name] * scale
            wage = min(self.wage, max(f.cash, 0.0) / f.labor) if f.labor > 0 else 0.0
            need = f.labor
            while need > 1e-12:
                take = min(need, free)
                pop.jobs[f.name] = pop.jobs.get(f.name, 0.0) + take
                pop.cash += take * wage
                f.cash -= take * wage
                need -= take
                free -= take
                if free <= 1e-12:
                    pop = next(pops, None)
                    if pop is None:
                        break
                    free = pop.size

        excess = (total_d - supply) / supply
        self.wage *= math.exp(self.wage_speed * max(-1.0, min(1.0, excess)))

    # ---- goods ----------------------------------------------------------------
    def goods_markets(self, outbound: list["Trader"], inbound: list["Trader"]) -> None:
        """Buyers: pops, firms (inputs), traders leaving from here.
        Sellers: local firms, traders arriving here."""
        cons_prices = {g: self.markets[g].price for g in self.consumer_goods}
        pop_orders = [(p, p.demand(cons_prices)) for p in self.pops]
        for p in self.pops:
            p.consumed = {}

        for g, market in self.markets.items():
            bids = [(p, d[g]) for p, d in pop_orders if g in d]
            bids += [(f, self.plans[f.name][g]) for f in self.firms if g in f.exponents]
            bids += [(t, t.bid(g)) for t in outbound if t.bid(g) > 0]
            asks = [(f, f.inventory) for f in self.firms if f.output == g and f.inventory > 0]
            asks += [(t, t.stock_dest) for t in inbound if t.good == g and t.stock_dest > 0]

            bid_fills, ask_fills = market.clear(bids, asks)

            for (agent, _), got in zip(bids, bid_fills):
                if isinstance(agent, Pop):
                    agent.consumed[g] = got
                elif isinstance(agent, Firm):
                    agent.inputs[g] = agent.inputs.get(g, 0.0) + got
                else:
                    agent.receive(g, got)
            for (agent, _), sold in zip(asks, ask_fills):
                if isinstance(agent, Firm):
                    agent.inventory -= sold
                else:
                    agent.stock_dest -= sold

    def produce(self, spoilage: float) -> None:
        for f in self.firms:
            f.inventory *= 1.0 - spoilage   # unsold stock decays; fresh output doesn't
            f.produce()

    def pay_dividends(self, owner, base: float = 20.0, rate: float = 0.5,
                      max_call: float = 0.1) -> None:
        """Pops own the firm/trader. Cash is pulled toward a target that covers
        what the owner wants to spend: excess is paid out as dividends, a
        shortfall is met by the pops investing (at most `max_call` of their cash
        per tick). Money is conserved either way."""
        target = base + 1.2 * owner.desired_spend
        div = (owner.cash - target) * rate
        if div < 0:
            div = max(div, -max_call * sum(p.cash for p in self.pops))
        pop_cash = sum(p.cash for p in self.pops)
        pop_size = sum(p.size for p in self.pops)
        owner.cash -= div
        for p in self.pops:
            share = p.size / pop_size if div >= 0 else p.cash / pop_cash
            p.cash += div * share

    # ---- stats ----------------------------------------------------------------
    def money(self) -> float:
        return sum(p.cash for p in self.pops) + sum(f.cash for f in self.firms)

    def utility(self) -> float:
        """Average utility per person."""
        return sum(p.utility() * p.size for p in self.pops) / sum(p.size for p in self.pops)
