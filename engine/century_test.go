package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCentury runs the reference experiment for changes to the economy: one
// computer nation holding the classic map, from 1820, for a century, once
// per seed, and writes a CSV a year per seed (century-s<seed>.csv) with the
// numbers to judge it by: people, real GDP, living standard, investment,
// capital, returns, sector shares, techs, each production method's share of
// its good, and the prices of machinery and fuel. tools/century_plot.py
// turns the CSVs into graphs, against history and against a baseline run.
// It only runs when asked:
//
//	CENTURY_OUT=/tmp/run go test ./engine -run TestCentury -timeout 60m
//
// CENTURY_ALLTECH=1 grants every tech on the first day (see runCentury).
// CENTURY_MAP (say "small") plays on a generated map of that size instead of
// the classic one. CENTURY_SEEDS (default "1,2") and CENTURY_YEARS (default 100) change the
// seeds and the length. See CLAUDE.md for how it is used.
func TestCentury(t *testing.T) {
	dir := os.Getenv("CENTURY_OUT")
	if dir == "" {
		t.Skip("set CENTURY_OUT to run")
	}
	years := 100
	if y, err := strconv.Atoi(os.Getenv("CENTURY_YEARS")); err == nil {
		years = y
	}
	seeds := os.Getenv("CENTURY_SEEDS")
	if seeds == "" {
		seeds = "1,2"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, s := range strings.Split(seeds, ",") {
		seed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			t.Parallel()
			runCentury(t, seed, years, filepath.Join(dir, fmt.Sprintf("century-s%d.csv", seed)))
		})
	}
}

// mined are the goods of mines and wells: the historical series count their
// workers in industry, so the century's "primary" is farming, forestry and
// fishing.
var mined = map[GoodID]bool{G("coal"): true, G("iron ore"): true, G("clay"): true, G("oil"): true}

func runCentury(t *testing.T, seed int64, years int, path string) {
	spec := DefaultGame
	spec.NationMap = nil
	for range DefaultMap {
		spec.NationMap = append(spec.NationMap, strings.Repeat("A", len(DefaultMap[0])))
	}
	spec.Nations = DefaultGame.Nations[:1]
	// CENTURY_MAP=small (or medium...): a generated map of that size, made
	// from the seed, all of it the one nation's, instead of the classic one
	if size := os.Getenv("CENTURY_MAP"); size != "" {
		var err error
		for _, m := range MapSizes {
			if m.Name == size {
				if spec, err = GenerateGame(MapOptions{Rows: m.Rows, Cols: m.Cols, Nations: 1, Seed: seed}); err == nil {
					spec, err = DivideLand(spec)
				}
			}
		}
		if err != nil || spec.Map == nil {
			t.Fatalf("map %q: %v", size, err)
		}
	}
	spec.AI, spec.Seed = true, seed
	w, err := BuildGame(spec)
	if err != nil {
		t.Fatal(err)
	}
	n := w.Nations[0]
	// CENTURY_ALLTECH=1: the nation knows every tech from the first day, a
	// quick look (CENTURY_YEARS=15 or so) at where the late economy settles;
	// incomes start at 1820's, so luxuries weigh less than by 1913
	if os.Getenv("CENTURY_ALLTECH") != "" {
		for _, tc := range Techs {
			w.GrantTech(0, tc.Name)
		}
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	// each firm with production methods, and its methods
	groups := map[string][]string{}
	var order []string
	for _, m := range MethodRecipes {
		if groups[m.Base] == nil {
			groups[m.Base] = []string{m.Base}
			order = append(order, m.Base)
		}
		groups[m.Base] = append(groups[m.Base], m.Firm)
	}
	machinery := []string{"tools", "steam engines", "explosives", "fertilizer", "electricity", "coal", "iron", "vehicles", "oil"}
	cols := []string{"year", "pop", "gdp", "gdpHead", "living", "nominal", "invest", "investPriv", "KY", "ret", "laborShare",
		"primary", "industry", "service", "levels", "techs", "tax", "bigShare", "smallest"}
	for _, b := range order {
		for _, f := range groups[b] {
			cols = append(cols, "share "+b+"|"+f)
		}
	}
	for _, g := range machinery {
		cols = append(cols, "price "+g)
	}
	fmt.Fprintln(out, strings.Join(cols, ","))

	days := int(TicksPerYear)
	var ni, wages, constr, constrPriv, profit float64
	made := map[string]float64{}
	for d := 1; d <= years*days; d++ {
		building := map[*Firm][2]float64{} // private expansions under way: what they had, and need
		for _, r := range w.Regions {
			for _, f := range r.Firms {
				if e := f.Expansion; e != nil && !e.Public {
					building[f] = [2]float64{e.Delivered, e.Cost()}
				}
			}
		}
		w.Step()
		for _, r := range w.Regions {
			m := r.Markets[Construction]
			constr += math.Min(m.Supply, m.Demand) * m.Price
			for _, f := range r.Firms {
				if was, ok := building[f]; ok {
					got := was[1] - was[0] // finished today
					if f.Expansion != nil {
						got = f.Expansion.Delivered - was[0]
					}
					constrPriv += math.Max(0, got) * m.Price
				}
				if !f.State {
					profit += f.tickRevenue - f.tickCost
				}
				wages += f.Labor * r.Wage
				for _, m := range f.Methods {
					made[f.Name+"|"+m.Name] += m.Output
				}
			}
			for _, p := range r.Pops {
				ni += p.Income
			}
		}
		ni += n.Revenue + n.TariffRevenue
		if d%days != 0 {
			continue
		}
		var k, levels, big, people, util float64
		var labor [3]float64
		small := math.Inf(1)
		for _, r := range w.Regions {
			for _, f := range r.Firms {
				if !f.State {
					k += float64(f.Levels) * LevelCost * r.Markets[Construction].Price
					levels += float64(f.Levels)
				}
				sector := f.Sector
				if sector == Primary && mined[f.Output] {
					sector = Industry // mining is industry in the historical series (Kuznets, Maddison)
				}
				labor[sector] += f.Labor
			}
			p := r.Population()
			people += p
			util += r.Utility() * p
			big, small = math.Max(big, p), math.Min(small, p)
		}
		techs := 0
		for _, known := range n.Known {
			if known {
				techs++
			}
		}
		lt := labor[0] + labor[1] + labor[2]
		row := []string{fmt.Sprint(d / days), f0(people), f0(n.RealGDP), f4(n.RealGDP / people), f4(util / people), f4(ni / float64(days)),
			f4(constr / ni), f4(constrPriv / ni), f4(k / ni), f4(profit / k), f4(wages / ni),
			f4(labor[Primary] / lt), f4(labor[Industry] / lt), f4(labor[Service] / lt), f0(levels), fmt.Sprint(techs),
			f4(n.TaxRate), f4(big / people), f0(small)}
		for _, b := range order {
			total := 0.0
			for _, m := range groups[b] {
				total += made[b+"|"+m]
			}
			for _, m := range groups[b] {
				row = append(row, f4(made[b+"|"+m]/math.Max(total, 1e-12)))
			}
		}
		for _, g := range machinery {
			p := 0.0
			for _, r := range w.Regions {
				p += r.Markets[G(g)].Price / float64(len(w.Regions))
			}
			row = append(row, f4(p))
		}
		fmt.Fprintln(out, strings.Join(row, ","))
		ni, wages, constr, constrPriv, profit = 0, 0, 0, 0, 0
		made = map[string]float64{}
	}
}

func f0(x float64) string { return strconv.FormatFloat(x, 'f', 0, 64) }
func f4(x float64) string { return strconv.FormatFloat(x, 'f', 4, 64) }
