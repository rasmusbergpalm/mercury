# Mercury

Mercury is a deterministic, spatial, agent-based economy, calibrated on the
nineteenth century from 1820, where transport is a good like any other.
The repository also holds a strategy game built on it, not yet named, which
is an example of using the economy, not part of Mercury:

- `economy/`: the economy on its own (pops, firms, markets, merchants and
  transport, migration, investment, demography, real GDP). Its calibration
  (goods, needs, recipes, terrains) is JSON in `economy/data/`. It knows
  nothing of nations and never calls out: anything above it sets its
  levers and reads its accounts (`economy/levers.go`), and runs its day's
  steps itself when it must act within a day.
- `game/`: the game on the economy: nations, techs, public services,
  armies, wars, alliances and the computer player.
- `experiments/`: the economy page's experiments, on the economy alone,
  each a `go generate` target writing to `experiments/data`
  (`go generate -run <name> ./experiments`); `tools/explainer/build.sh`
  builds `web/static/economy.html` from those results (published to
  GitHub Pages). After a change to the economy, rerun them all and
  rebuild the page.
- `internal/server/` and `cmd/server/`: the server; `web/`: the web client.

Keep the split: nothing in `economy/` may refer to the game. A game rule
that changes the economy sets a lever (add one, in economic terms, if
none fits) or acts between the day's steps. `TestEconomyKnowsNothingOfTheGame`
guards it.

## Changing the economy

When a change alters how the economy behaves (production, investment,
prices, demography, technology, the state's budget), not when fixing a
small bug or tidying code, work in rounds:

1. **Implement** the change, with a reason rooted in how real economies
   worked, not an arbitrary limit. Write the reason in the code's comments.
2. **Run century games with fixed seeds**: one computer nation holding the
   classic map, no wars, from 1820 for a hundred years, seeds 1 and 2,
   before and after the change:

   ```bash
   CENTURY_OUT=/tmp/base CENTURY_SEEDS=1,2 go test ./game -run TestCentury -timeout 60m   # on main
   CENTURY_OUT=/tmp/run CENTURY_SEEDS=1,2 go test ./game -run TestCentury -timeout 60m    # with the change
   python3 tools/century_plot.py /tmp/run /tmp/base
   ```

   A run takes a couple of minutes. `CENTURY_MAP=small` (or `medium`)
   plays on a generated map. `CENTURY_ALLTECH=1 CENTURY_YEARS=20` grants
   every tech on the first day: under a minute, a quick check that the
   last methods work and where the whole tree settles, but not a stand-in
   for the century (which ends with a few techs still unknown, and with
   richer people). The same seed gives the same game, and
   the computer's random choices (its research, owners' investment) are
   keyed so that a change elsewhere doesn't re-roll them: before and after
   differ by the change, not by luck.
3. **Look at all the numbers**, not just the one the change was for: the
   decade table, people, GDP a head, investment, capital, returns, sector
   shares, techs, each production method's share, prices. A fix in one
   place often moves another.
4. **Aim for historical realism and stability.** Rough targets for
   Western Europe 1820-1913 (orders of magnitude, from Maddison, Feinstein,
   Crafts, Piketty): people ~0.7-1.2% a year, GDP a head ~1-1.3% a year,
   gross investment ~7-20% of output, capital ~2-5 years of output, a
   return on capital of ~5-10%, farms' share of jobs falling. Nothing may
   cycle, pile up unsold, empty or collapse.
5. **Show the graphs** to the user (`tools/century_plot.py` makes them:
   macro against history and the baseline, each method's share, prices),
   with what changed, what looks right, what doesn't and why, and iterate
   until it looks right.
6. **Run the health ladder** (`go test ./game -run TestHealth`, a few
   minutes; `go test -short ./...` skips it).
7. **Re-ship the server's starting game.** The server starts from snapshots
   of The Three Crowns and of the settled classic map, so it is ready at
   once; after a change to the economy `go test ./cmd/server` fails as they
   are stale, and `MERCURY_WRITE_SHIPPED=1 go test ./cmd/server` rewrites
   them.

## The health ladder

`game/health_ladder_test.go` runs worlds of rising complexity and checks
every important number against a bound. The bounds catch accidental damage
to the economy; they are signals, not laws. When a rung fails, find out
what moved and why: if a change broke something by accident, fix it; if
the economy now behaves differently on purpose and soundly, move the bound
and write why next to it.

## Conventions

- The engine is deterministic: the same scenario stepped the same number
  of times gives bit-identical state. Random choices are hashes of the
  seed and stable keys (not the day, where a shifted day would re-roll
  every choice after it).
- No backwards compatibility for saves or old code paths: the game is in
  development.
