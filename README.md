# Mercury

A deterministic, spatial, agent-based economy in Go, calibrated on the
nineteenth century.

Mercury is a map of tiles. Each tile has its own people, firms and markets.
Merchants carry goods between neighbouring tiles and pay for the transport
it takes. Firms hire workers and buy inputs, and owners build more capacity
where it pays. People spend their income on what they need most and move to
where they live better. Nothing is solved for: prices, wages, trade routes,
specialisation and where people live all come out of these choices, day by
day.

From those few rules come many of the textbook results of economics:

- **Prices** differ between tiles by the cost of moving goods between
  them, and the law of one price holds net of it.
- **Weight matters**: cheap, heavy goods stay home and valuable, light ones
  travel far (the Alchian–Allen effect).
- **Trade in goods alone** brings wages closer together, even when nobody
  moves (factor-price equalisation).
- **Cheaper transport** makes tiles specialise in what they do best
  (comparative advantage).
- **People switch** to substitutes when a good gets dear, and spend a
  smaller share of a rising income on food (Engel's law).
- **Migration** equalises living standards between places, and a cheap
  route across the map draws people to it (spatial equilibrium).

**[The economy page](https://rasmusbergpalm.github.io/mercury/)** shows each
of these with experiments run on the engine itself.

## Quick start

```bash
go get github.com/rasmusbergpalm/mercury/economy
```

```go
package main

import (
	"fmt"

	"github.com/rasmusbergpalm/mercury/economy"
)

func main() {
	// the full world on the classic 6×6 map, transport at its normal cost
	w, err := economy.BuildFullWorld(1, economy.DefaultMap, nil)
	if err != nil {
		panic(err)
	}
	w.Investment, w.Migration = true, true // owners invest, people move
	for day := 0; day < 2*economy.TicksPerYear; day++ {
		w.Step()
	}
	bread := economy.G("bread")
	for _, r := range w.Regions {
		fmt.Printf("%s: bread £%.2f, wage £%.2f, %.0f people\n",
			r.Name, r.Markets[bread].Price, r.Wage, r.Population())
	}
}
```

The same world stepped the same number of days always gives bit-identical
state. Random choices are hashes of the seed and stable keys. The tiles'
own work runs in parallel and settles what it owes other tiles in a fixed
order afterwards. So an experiment compares like with like: two runs differ
by the change you made, not by luck.

## The model

Each day, on every tile:

- **Firms** plan with a Cobb–Douglas recipe,
  `q = TFP · levels^(1−s) · Π inputᵢ^aᵢ`, where `levels` is the firm's
  buildings and `s = Σ aᵢ`. A firm hires workers and buys inputs up to
  where they pay, so its wage bill is its labour exponent times its sales.
  Land-bound producers (farms, mines) have a site that suits them more or
  less, and their levels are capped because land is scarce.
- **Owners** build a new level where it would repay its cost within
  ten years of profit, and refit levels to newer production methods when
  the difference pays. Levels wear out unless maintained, and are closed
  when they keep losing money.
- **People** spend on needs in three tiers (basic, intermediate, luxury).
  Each need group is a CES composite of its goods: bread and potatoes for
  staples, meat and fish for protein. The utility a person gets is
  `Σ wₖ · log(1 + Qₖ / rₖ)`. Basic needs are met first, and luxuries are
  never sated.
- **Markets** clear for every good and for labour, and prices move with
  the excess of demand over supply.
- **Merchants**: each tile's trading house runs a route for every good to
  every neighbour. A route grows while
  `p_dest · arriving − p_origin − τ · p_transport` is positive, where
  `τ = cost × distance × bulk` is the transport a unit needs. Transport is
  a service like any other, made by carters and bought in the market.
  Perishable goods lose some of their quantity on every hop.
- **People** move a little each day towards the neighbouring tiles where
  they live better, and their numbers grow when they eat well.
- **Real GDP** is chained monthly with a Fisher index, so growth is
  measured in quantities, not pounds.

### Calibration is data

The goods, needs, recipes and terrains are JSON files in
[`economy/data/`](economy/data). Each file starts with a note on what its
fields mean. Edit them to model another economy.

| File | What it holds |
|---|---|
| `goods.json` | Goods with their bulk and spoilage, and substitutable inputs (fuel = wood or coal) |
| `needs.json` | Need groups, their tiers, goods, elasticities, weights and requirements |
| `recipes.json` | Every firm's recipe, and the later production methods that technologies unlock |
| `terrains.json` | Tile types, their movement cost, and the land-bound producers each suits |

`BuildWorld` makes the classic world: the core goods and firms, and one
class of people. `BuildFullWorld` makes the full world: every good and
firm, production methods waiting for their technology, and workers and
owners.

### Levers, accounts and steps

The economy knows nothing of states, and never calls out. Anything outside
it, such as a model of policy or the game below, acts on it in three
ways. It sets plain levers, it reads the accounts where what the economy
collects piles up, and, if it needs to act during the day, it runs the
day's steps itself. [`economy/levers.go`](economy/levers.go) lists them
all:

| Where | Levers | Accounts |
|---|---|---|
| a tile (`Region`) | `IncomeTax`, `PaybackMult` (cost of capital), `GrowthBonus`, `RoadMult`, `CoastMult`, `SpoilageMult`, `Educated`, `Healthy`, `Urban` | `Withheld` (taxes), `Duties` |
| a tile's markets | `Buyers`: outside buyers with their own bids | each buyer's `Account` and `Got` |
| a route (`Trader`) | `Tariff`, `Closed` (an embargo) | |
| a firm | `Mult` (productivity), `Managed` (run for someone else) | |
| the world | `Known` (technologies known somewhere) | |

The day's steps are `BeginDay`, `PlanTrade`, `Markets`, `Ship`, `Produce`
and `EndDay`; `Step` runs them in turn.

A customs border down the middle of the map takes a few lines (from
`Example_tariff` in [`economy/example_test.go`](economy/example_test.go)):

```go
west := func(r *economy.Region) bool { return r.Col < 3 }
for _, t := range w.Traders {
	if west(t.Origin) != west(t.Dest) {
		t.Tariff = 0.3 // a 30% duty on everything that crosses
	}
}
```

It cuts the trade across the border by three quarters, and the duty piles
up in each tile's `Duties`.

## The game

[`game/`](game) is a strategy game built on the economy with nothing but
its levers and steps. Nations own the tiles, tax them, set
tariffs, research a tech tree from 1820 to the motor age, build roads,
railways and ports, and run schools, public health and universities. They
raise armies, go to war and form alliances. People or the computer play
them.

```bash
go run ./cmd/server    # then open http://localhost:8080
```

## Repository layout

| Path | What it is |
|---|---|
| `economy/` | The economy engine, and its data in `economy/data/` |
| `game/` | The game: nations, technology, services, armies, diplomacy, the computer player |
| `internal/server/`, `cmd/server/` | The game's HTTP server |
| `web/` | The web client and the economy page |
| `tools/` | Plotting and data tools, and the economy page's build (`tools/explainer/build.sh`) |

## Testing and experiments

```bash
go test -short ./...    # the fast tests
```

```bash
go test ./game -run TestHealth    # the health ladder: worlds of rising complexity, every key number within bounds
```

```bash
CENTURY_OUT=/tmp/run CENTURY_SEEDS=1,2 go test ./game -run TestCentury -timeout 60m    # a century, 1820–1920
```

```bash
python3 tools/century_plot.py /tmp/run    # ...plotted against history
```

```bash
sh tools/explainer/build.sh    # reruns the economy page's experiments and rebuilds it
```

## Licence

[MIT](LICENSE).
