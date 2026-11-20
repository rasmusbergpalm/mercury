# Mercury

A deterministic, spatial, agent-based economy, where transport is a good like any other: produced by workers and bought by merchants. It comes with an example economy to run: Europe around 1820.

Mercury is a map of tiles. Each tile has its own people, firms and markets for labour and goods.
Merchants carry goods between neighbouring tiles and pay for the transport
it takes. Firms hire workers and buy inputs, and owners build more capacity
where it pays. People spend their income on what they need most and move to
where they live better. Nothing is solved for: prices, wages, trade routes,
specialisation and where people live all come out of these choices, day by
day.

Most trade models treat transport as an “iceberg”, where a fixed share of each shipment melts away on the way, or as a cost set from outside the model. In Mercury, carriage is made by firms like any other good and bought by merchants at a market price. So the cost of trade is an outcome of the model, not a parameter. It rises with wages and with demand for carriage, and falls with better methods of transport and better roads. When trade booms, carters compete with every other trade for workers.

From those few rules many of the textbook economics results can be reproduced:

- **Prices** differ between tiles by the cost of moving goods between them, and the law of one price holds net of it.
- **Weight matters**: cheap, heavy goods stay home and valuable, light ones travel far (the Alchian–Allen effect).
- **Trade in goods alone** brings wages closer together, even when nobody moves (factor-price equalisation).
- **Cheaper transport** makes tiles specialise in what they do best (comparative advantage).
- **People switch** to substitutes when a good gets dear, and spend a smaller share of a rising income on food (Engel's law).
- **Migration** evens out living standards between places, and a cheap route across the map draws trade and people to it (spatial equilibrium).
- **Money is neutral** in the long run: double or halve it and prices follow while output doesn’t move.
- **Policy** has textbook effects: a duty on grain raises bread prices and farm profits (the Corn Laws), and free trade erodes a monopoly’s markup.

**[The economy page](https://rasmusbergpalm.github.io/mercury/)** explains the economy in detail and shows each of these with experiments run on the engine itself.

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
	// the example economy on its 6×6 example map, transport at its normal cost
	w, err := economy.BuildWorld(1, economy.DefaultMap, nil)
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

### The economy is data; 1820 is an example

The engine doesn't fix what is made or where. The goods, needs, recipes and
terrains are JSON files in [`economy/data/`](economy/data), and a map is a
list of rows of terrain letters. What comes with Mercury is an example
economy, Europe around 1820, and an example map (`DefaultMap`, 36 tiles).
Each data file starts with a note on what its fields mean. Edit them to
model another economy.

| File | What it holds |
|---|---|
| `goods.json` | Goods with their bulk and spoilage, and substitutable inputs (fuel = wood or coal) |
| `needs.json` | Need groups, their tiers, goods, elasticities, weights and requirements |
| `recipes.json` | Every firm's recipe, and later production methods, which start switched off until invented |
| `terrains.json` | Tile types, their movement cost, and the land-bound producers each suits |

`BuildWorld` builds a world from the data and a map: every good and firm,
with the later production methods switched off, and workers and owners in
every tile.

### Levers, accounts and steps

The economy knows nothing of states, and never calls out. Anything outside
it, such as a model of policy or the game below, acts on it in three
ways. It sets plain levers, it reads the accounts where what the economy
collects piles up, and, if it needs to act during the day, it runs the
day's steps itself. [`economy/levers.go`](economy/levers.go) lists them
all:

| Where | Levers | Accounts |
|---|---|---|
| a tile (`Region`) | `IncomeTax`, `PaybackMult` (cost of capital), `GrowthBonus`, `LandTransportMult` and `SeaTransportMult` (the cost of transport), `SpoilageMult` | `Withheld` (taxes), `Duties` |
| a tile's markets | `Buyers`: outside buyers and employers, with their own bids for goods and workers | each buyer's `Account`, `Got` and `Hired` |
| a route (`Trader`) | `Tariff`, `Closed` (an embargo) | |
| a firm | `Mult` (productivity), `Enabled` and each method's `Enabled` (which recipes it may use) and `TFP`, an `Expansion` paid from outside, `Monopoly` | |

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

## An example: a strategy game

[`game/`](game) is an example of building on Mercury: a strategy game, not
yet named, that uses nothing but the economy's levers and steps. Nations own the tiles, tax them, set
tariffs, research a tech tree from 1820 to the motor age (a tech switches
recipes on and raises productivity), build roads, railways and ports (which
lower the cost of transport), and run schools, public health and
universities (outside employers that raise productivity and growth). They
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
| `experiments/` | The economy page's experiments, each a `go generate` target, and their results |
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
go generate -run corridor ./experiments    # reruns one of the economy page's experiments (go generate ./experiments: all)
```

```bash
sh tools/explainer/build.sh    # rebuilds the economy page from the experiments' results
```

Each experiment on the economy page is a file in
[`experiments/`](experiments): the corridor, the coal boom, money's
neutrality, the Corn Laws, and the rest. They use nothing but the
`economy` package. An experiment can also record the whole economy as it
runs, as a snapshot every so many days, to read out afterwards whatever it
didn't measure:

```bash
go run ./experiments/run -out /tmp/money -record 30 money
```

## Licence

[MIT](LICENSE).
