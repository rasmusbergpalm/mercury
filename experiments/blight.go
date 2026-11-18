package experiments

import (
	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"blight", "substitution: a potato blight, and what people eat instead", runBlight})
}

func runBlight(o *Out) {
	shockDay := settle + days
	// ---- substitution: a potato blight
	blight := func(on bool, out *map[string][]float64) func() {
		return func() {
			w := settled()
			o := map[string][]float64{}
			bread, potatoes, grain := G("bread"), G("potatoes"), G("grain")
			for d := settle; d < end; d++ {
				if on && d == shockDay {
					for _, r := range w.Regions {
						for _, f := range r.Firms {
							if f.Name == "Potato farm" {
								for _, m := range f.Methods {
									m.TFP *= 0.5
								}
							}
						}
					}
				}
				w.Step()
				if d < settle || (d-settle)%10 != 0 {
					continue
				}
				people, eat, price := 0.0, map[GoodID]float64{}, map[GoodID]float64{}
				for _, r := range w.Regions {
					people += r.Population()
					for _, p := range r.Pops {
						for _, g := range []GoodID{bread, potatoes} {
							eat[g] += p.Consumed[g]
						}
					}
					for _, g := range []GoodID{bread, potatoes, grain} {
						price[g] += r.Markets[g].Price / float64(len(w.Regions))
					}
				}
				o["day"] = append(o["day"], float64(d-settle))
				o["breadEaten"] = append(o["breadEaten"], eat[bread]/people)
				o["potatoesEaten"] = append(o["potatoesEaten"], eat[potatoes]/people)
				o["breadPrice"] = append(o["breadPrice"], price[bread])
				o["potatoPrice"] = append(o["potatoPrice"], price[potatoes])
				o["grainPrice"] = append(o["grainPrice"], price[grain])
			}
			*out = o
		}
	}
	var blighted, unblighted map[string][]float64

	parallel(blight(true, &blighted), blight(false, &unblighted))
	o.Write("blight.json", map[string]any{"blightDay": days, "sigma": Needs[0].Sigma, "blight": blighted, "control": unblighted})
}
