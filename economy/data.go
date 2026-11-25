package economy

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

// The economy's calibration (its goods, needs, recipes and terrains) is
// data, in the JSON files of data/, built into the package: the example
// economy. Edit them, or give LoadData another set, to model another
// economy; each file says what its fields mean.
//
//go:embed data/*.json
var dataFiles embed.FS

// dataFS is the set of data files in use.
var dataFS fs.FS

func loadJSON(name string, v any) {
	b, err := fs.ReadFile(dataFS, name)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		panic(fmt.Sprintf("%s: %v", name, err))
	}
}

// ExampleData is the example economy's data files (data/), the economy in
// use unless LoadData gives another: LoadData(ExampleData) brings it back.
var ExampleData = func() fs.FS {
	sub, err := fs.Sub(dataFiles, "data")
	if err != nil {
		panic(err)
	}
	return sub
}()

func init() {
	if err := useData(ExampleData); err != nil {
		panic(err)
	}
}

// LoadData makes the economy the one described by another set of data
// files: goods.json, needs.json, recipes.json and terrains.json, in the
// format of data/ (each file's "about" says what its fields mean). Every
// economy has goods named "transport" and "construction". It replaces the
// example economy for the whole program, so call it before building any
// world: one built before it no longer fits the goods.
func LoadData(fsys fs.FS) error { return useData(fsys) }

// useData reads a set of data files and sets everything that follows from
// them.
func useData(fsys fs.FS) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("economy data: %v", r)
		}
	}()
	dataFS = fsys
	goodsData, recipesData, terrainsData = makeGoodsData(), makeRecipesData(), makeTerrainsData()
	Goods = makeGoods()
	NumGoods = len(Goods)
	goodIndex = makeGoodIndex()
	Transport, Construction = G("transport"), G("construction")
	Tradable = makeTradable()
	Composites = makeComposites()
	Needs = makeNeeds()
	if len(Needs) > maxNeeds {
		return fmt.Errorf("economy data: %d needs, at most %d", len(Needs), maxNeeds)
	}
	for _, n := range Needs {
		if len(n.Goods) > maxNeedGoods {
			return fmt.Errorf("economy data: need %s has %d goods, at most %d", n.Name, len(n.Goods), maxNeedGoods)
		}
	}
	ConsumerGoods, TierOf = makeConsumerGoods(), makeTierOf()
	RawRecipes, ProcessingRecipes, MethodRecipes = recipes(recipesData.Raw), recipes(recipesData.Processing), makeMethodRecipes()
	DepositSize = terrainsData.Deposit
	Terrains = makeTerrains()
	CoastMove, CoastRaw = terrainsData.Coast.Move, bonuses(terrainsData.Coast.Raw)
	publicSlot = NumGoods
	return nil
}

type goodsFile struct {
	Goods []struct {
		Name                        string
		Bulk, Spoilage, Wear, Lease float64
		Service                     bool
	}
	Composites []struct {
		Name  string
		Goods []string
		Sigma float64
	}
}

var goodsData goodsFile

func makeGoodsData() goodsFile {
	var f goodsFile
	loadJSON("goods.json", &f)
	return f
}

type needsFile struct {
	Needs []struct {
		Name                       string
		Tier                       string
		Goods                      []string
		Sigma, Weight, Requirement float64
		Subsistence                float64
	}
}

// recipeData is a recipe as the data files give it: its factors are pairs
// of a name (labor, a good or a composite) and an exponent.
type recipeData struct {
	Firm, Output string
	TFP          float64
	Factors      [][2]any
	Sector       string
	Enabled      *bool
	Base         string
	Capital      [][2]any
	Payback      float64
	Fixed        bool
	Levels       int
}

func (d recipeData) recipe() Recipe {
	r := Recipe{Firm: d.Firm, Output: d.Output, TFP: d.TFP, Payback: d.Payback, Fixed: d.Fixed, Levels: d.Levels, StartsOff: d.Enabled != nil && !*d.Enabled, Service: d.Sector == "service"}
	for _, f := range d.Factors {
		name, _ := f[0].(string)
		exp, ok := f[1].(float64)
		if !ok {
			panic(fmt.Sprintf("data/recipes.json: %s: a factor's exponent must be a number", d.Firm))
		}
		switch {
		case name == "labor":
			r.Factors = append(r.Factors, Labor(exp))
		case name == "deposit": // the firm's own, fixed, not bought
			r.Factors = append(r.Factors, Factor{Kind: DepositFactor, Exp: exp})
		case compositeIndex(name) >= 0:
			r.Factors = append(r.Factors, Comp(name, exp))
		default:
			r.Factors = append(r.Factors, Input(name, exp))
		}
	}
	for _, c := range d.Capital {
		name, _ := c[0].(string)
		qty, ok := c[1].(float64)
		if !ok {
			panic(fmt.Sprintf("data/recipes.json: %s: a capital good's quantity must be a number", d.Firm))
		}
		r.Capital = append(r.Capital, Stock{G(name), qty})
	}
	return r
}

type recipesFile struct {
	Raw, Processing []recipeData
	Methods         []recipeData
}

var recipesData recipesFile

func makeRecipesData() recipesFile {
	var f recipesFile
	loadJSON("recipes.json", &f)
	return f
}

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
	}
	Coast struct {
		Move float64
		Raw  [][2]any
	}
	Deposit float64
}

var terrainsData terrainsFile

func makeTerrainsData() terrainsFile {
	var f terrainsFile
	loadJSON("terrains.json", &f)
	return f
}

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
