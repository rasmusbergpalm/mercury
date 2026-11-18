package economy

import (
	"embed"
	"encoding/json"
	"fmt"
)

// The economy's calibration (its goods, needs, recipes and terrains) is
// data, in the JSON files of data/, built into the package. Edit them to
// model another economy; each file says what its fields mean.
//
//go:embed data/*.json
var dataFiles embed.FS

func loadJSON(name string, v any) {
	b, err := dataFiles.ReadFile("data/" + name)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		panic(fmt.Sprintf("data/%s: %v", name, err))
	}
}

type goodsFile struct {
	Goods []struct {
		Name           string
		Bulk, Spoilage float64
		Service, Full  bool
	}
	Composites []struct {
		Name  string
		Goods []string
		Sigma float64
	}
}

var goodsData = func() goodsFile {
	var f goodsFile
	loadJSON("goods.json", &f)
	return f
}()

type needsFile struct {
	Needs []struct {
		Name                       string
		Tier                       string
		Goods                      []string
		Sigma, Weight, Requirement float64
	}
}

// recipeData is a recipe as the data files give it: its factors are pairs
// of a name (labor, a good or a composite) and an exponent.
type recipeData struct {
	Firm, Output string
	TFP          float64
	Factors      [][2]any
	Sector       string
	Requires     string
	Base         string
	Cap          int
}

func (d recipeData) recipe() Recipe {
	r := Recipe{Firm: d.Firm, Output: d.Output, TFP: d.TFP, Requires: d.Requires, Service: d.Sector == "service"}
	for _, f := range d.Factors {
		name, _ := f[0].(string)
		exp, ok := f[1].(float64)
		if !ok {
			panic(fmt.Sprintf("data/recipes.json: %s: a factor's exponent must be a number", d.Firm))
		}
		switch {
		case name == "labor":
			r.Factors = append(r.Factors, Labor(exp))
		case compositeIndex(name) >= 0:
			r.Factors = append(r.Factors, Comp(name, exp))
		default:
			r.Factors = append(r.Factors, Input(name, exp))
		}
	}
	return r
}

type recipesFile struct {
	Raw, Processing, Full []recipeData
	FullRaw               []recipeData `json:"full_raw"`
	Methods               []recipeData
}

var recipesData = func() recipesFile {
	var f recipesFile
	loadJSON("recipes.json", &f)
	return f
}()

func recipes(ds []recipeData) []Recipe {
	out := make([]Recipe, len(ds))
	for i, d := range ds {
		out[i] = d.recipe()
	}
	return out
}

type terrainsFile struct {
	Terrains []struct {
		Name, Symbol string
		Move         float64
		Raw          [][2]any
		FullRaw      [][2]any `json:"full_raw"`
	}
	Coast struct {
		Move float64
		Raw  [][2]any
	}
	RawMaxLevels int `json:"raw_max_levels"`
}

var terrainsData = func() terrainsFile {
	var f terrainsFile
	loadJSON("terrains.json", &f)
	return f
}()

func bonuses(xs [][2]any) []Bonus {
	var out []Bonus
	for _, x := range xs {
		firm, _ := x[0].(string)
		mult, ok := x[1].(float64)
		if !ok {
			panic(fmt.Sprintf("data/terrains.json: %s: a site's multiplier must be a number", firm))
		}
		out = append(out, Bonus{firm, mult})
	}
	return out
}
