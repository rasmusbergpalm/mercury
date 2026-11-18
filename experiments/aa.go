package experiments

import (
	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"aa", "the Alchian-Allen effect: how far goods travel against their value per unit of bulk", runAA})
}

func runAA(o *Out) {
	// ---- the Alchian-Allen effect. With no spoilage, only carriage sets
	// how far a good goes. The goods table is changed for these runs, so
	// their worlds are built one at a time (a route takes its good's bulk
	// and spoilage when it is built) and then run together.
	spoil, bulk := make([]float64, NumGoods), make([]float64, NumGoods)
	for g := range Goods {
		spoil[g], bulk[g] = Goods[g].Spoilage, Goods[g].Bulk
	}
	defer func() {
		for g := range Goods {
			Goods[g].Spoilage, Goods[g].Bulk = spoil[g], bulk[g]
		}
	}()
	for g := range Goods {
		Goods[g].Spoilage = 0
	}
	type travel struct {
		Good                           string
		Bulk, Price, Made, HopsPerUnit float64
	}
	type aaRun struct {
		w     *World
		good  string // the good whose bulk is changed ("" for the base)
		bulks []float64
		out   []travel
	}
	var runs []*aaRun
	add := func(good string, k float64) {
		bs := append([]float64(nil), bulk...)
		if good != "" {
			Goods[G(good)].Bulk = bulk[G(good)] * k
			bs[G(good)] = Goods[G(good)].Bulk
		}
		runs = append(runs, &aaRun{w: world(), good: good, bulks: bs})
		for g := range Goods {
			Goods[g].Bulk = bulk[g]
		}
	}
	add("", 1)
	for _, name := range []string{"potatoes", "coal", "tea"} {
		for _, k := range []float64{0.25, 0.5, 2} {
			add(name, k)
		}
	}
	var jobs []func()
	for _, r := range runs {
		r := r
		jobs = append(jobs, func() {
			w := r.w
			made, hops, price := make([]float64, NumGoods), make([]float64, NumGoods), make([]float64, NumGoods)
			for d := 0; d < end; d++ {
				w.Step()
				if d < end-days {
					continue
				}
				for _, reg := range w.Regions {
					for _, f := range reg.Firms {
						made[f.Output] += f.LastOutput / TicksPerYear
					}
					for g, m := range reg.Markets {
						price[g] += m.Price / float64(len(w.Regions)) / TicksPerYear
					}
				}
				for _, tr := range w.Traders {
					hops[tr.Good] += tr.Shipped * tr.Distance / TicksPerYear
				}
			}
			for _, g := range FullTradable {
				if made[g] > 0.5 && r.bulks[g] > 0 {
					r.out = append(r.out, travel{Goods[g].Name, r.bulks[g], price[g], made[g], hops[g] / made[g]})
				}
			}
		})
	}
	parallel(jobs...)
	moves := map[string][]travel{}
	for _, r := range runs[1:] {
		for _, tv := range r.out {
			if tv.Good == r.good {
				moves[r.good] = append(moves[r.good], tv)
			}
		}
	}
	o.Write("aa.json", map[string]any{"base": runs[0].out, "moves": moves})

}
