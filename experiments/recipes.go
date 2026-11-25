package experiments

import (
	. "github.com/rasmusbergpalm/mercury/economy"
)

func init() {
	register(Experiment{"recipes", "the production functions, and the growth and migration constants", runRecipes})
}

func runRecipes(o *Out) {
	// ---- the production functions and the constants
	type factor struct {
		Kind string
		Exp  float64
	}
	type method struct {
		Name    string
		TFP     float64
		Factors []factor
	}
	type firm struct {
		Name, Good string
		Methods    []method
		Sites      map[string]float64
	}
	facs := func(fs []Factor) []factor {
		var out []factor
		for _, fc := range fs {
			switch fc.Kind {
			case LaborFactor:
				out = append(out, factor{"labour", fc.Exp})
			case GoodFactor:
				out = append(out, factor{Goods[fc.Good].Name, fc.Exp})
			case CompositeFactor:
				out = append(out, factor{Composites[fc.Composite].Name, fc.Exp})
			case DepositFactor:
				out = append(out, factor{"deposit", fc.Exp})
			}
		}
		return out
	}
	var firms []firm
	addFirm := func(rc Recipe) {
		f := firm{Name: rc.Firm, Good: rc.Output, Methods: []method{{rc.Firm, rc.TFP, facs(rc.Factors)}}, Sites: map[string]float64{}}
		for _, mr := range MethodRecipes {
			if mr.Base == rc.Firm {
				f.Methods = append(f.Methods, method{mr.Firm, mr.TFP, facs(mr.Factors)})
			}
		}
		for _, tn := range Terrains {
			for _, b := range tn.Raw {
				if b.Firm == rc.Firm {
					f.Sites[tn.Name] = b.Mult
				}
			}
		}
		firms = append(firms, f)
	}
	for _, rc := range RawRecipes {
		addFirm(rc)
	}
	for _, rc := range ProcessingRecipes {
		addFirm(rc)
	}
	o.Write("recipes.json", map[string]any{
		"firms":     firms,
		"growth":    map[string]float64{"slope": PopGrowthSlope * TicksPerYear, "neutral": PopGrowthNeutral, "min": PopGrowthMin * TicksPerYear, "max": PopGrowthMax * TicksPerYear}, // a year's
		"migration": map[string]float64{"rate": MigrationRate, "minPopulation": MinPopulation},
		"settle":    map[string]int{"years": Settle, "experimentYears": Years},
	})
}
