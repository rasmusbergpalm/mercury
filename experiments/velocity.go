package experiments

import . "github.com/rasmusbergpalm/mercury/economy"

func init() {
	register(Experiment{"velocity", "people spend a tenth of their cash a day instead of a twentieth", runVelocity})
}

// runVelocity doubles the velocity of money. People spend a share of their
// cash a day (Region.SpendShare, a twentieth), so the pounds spent across
// the map a day are that share of the money people hold: with the money
// fixed and output set by real things, the share sets the price level,
// P·Y = s·M. On the settled map everyone starts spending a tenth of their
// cash a day, against a twin where nothing changes. Day by day for two
// years: the price level (the basket the map's people bought the day
// before the change, at today's prices), the average wage, real output (net
// output at the prices before the change), the living standard, and the
// share of the map's money that people hold, each against the twin.
func runVelocity(o *Out) {
	control, shocked := settled(), settled()
	for _, r := range shocked.Regions {
		r.SpendShare = 2 * DefaultSpendShare
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
	held := func(w *World) float64 { // the share of the map's money people hold
		s := 0.0
		for _, r := range w.Regions {
			for _, p := range r.Pops {
				s += p.Cash
			}
		}
		return s / w.Money()
	}
	var out struct {
		Prices, Wage, Real, Living, Held []float64 // by day, against the twin (day 0: the day of the change)
	}
	wage := func(r *Region) float64 { return r.Wage }
	living := func(r *Region) float64 { return r.Utility() }
	out.Prices, out.Wage, out.Real, out.Living, out.Held = []float64{1}, []float64{1}, []float64{1}, []float64{1}, []float64{1}
	for d := 1; d <= 2*days; d++ {
		control.Step()
		shocked.Step()
		out.Prices = append(out.Prices, level(shocked)/level(control))
		out.Wage = append(out.Wage, perHead(shocked, wage)/perHead(control, wage))
		out.Real = append(out.Real, output(shocked)/output(control))
		out.Living = append(out.Living, perHead(shocked, living)/perHead(control, living))
		out.Held = append(out.Held, held(shocked)/held(control))
	}
	o.Write("velocity.json", out)
}
