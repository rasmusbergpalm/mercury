package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"doubling", "double every price and wage overnight, the money unchanged", runDoubling})
}

// runDoubling tests what holds the price level. People spend a share of
// the cash they hold each day, and the stock of money is fixed, so the
// pounds spent a day are fixed too: if every price and wage doubled for no
// reason, the same pounds would buy half as much, and the markets would
// bring prices back down. On the settled map every price and wage is
// doubled on one day, the money left alone, against a twin where nothing
// happens. Day by day for a year: the price level (the basket the map's
// people bought the day before the change, at today's prices), the average
// wage, real output (net output at the prices before the change) and the
// living standard, each against the twin.
func runDoubling(o *Out) {
	control, shocked := settled(), settled()
	for _, r := range shocked.Regions {
		r.Wage *= 2
		for _, m := range r.Markets {
			m.Price *= 2
			m.Trend *= 2
		}
	}
	basket := make([][]float64, len(control.Regions))
	base := make([][]float64, len(control.Regions))
	for i, r := range control.Regions {
		basket[i] = make([]float64, NumGoods)
		for _, p := range r.Pops {
			for g, q := range p.Consumed {
				basket[i][g] += q
			}
		}
		base[i] = r.Prices()
	}
	level := func(w *World) float64 {
		s := 0.0
		for i, r := range w.Regions {
			for g, q := range basket[i] {
				s += q * r.Markets[g].Price
			}
		}
		return s
	}
	perHead := func(w *World, f func(r *Region) float64) float64 {
		s, n := 0.0, 0.0
		for _, r := range w.Regions {
			p := r.Population()
			s += f(r) * p
			n += p
		}
		return s / n
	}
	output := func(w *World) float64 {
		s := 0.0
		for i, r := range w.Regions {
			for _, f := range r.Firms {
				s += f.LastOutput * base[i][f.Output]
				for g, q := range f.Used {
					s -= q * base[i][g]
				}
			}
		}
		return s
	}
	var out struct {
		Prices, Wage, Real, Living []float64 // by day, against the twin (day 0: the day of the change)
	}
	wage := func(r *Region) float64 { return r.Wage }
	living := func(r *Region) float64 { return r.Utility() }
	out.Prices, out.Wage, out.Real, out.Living = []float64{2}, []float64{2}, []float64{1}, []float64{1}
	for d := 1; d <= days; d++ {
		control.Step()
		shocked.Step()
		out.Prices = append(out.Prices, level(shocked)/level(control))
		out.Wage = append(out.Wage, perHead(shocked, wage)/perHead(control, wage))
		out.Real = append(out.Real, output(shocked)/output(control))
		out.Living = append(out.Living, perHead(shocked, living)/perHead(control, living))
	}
	o.Write("doubling.json", out)
}
