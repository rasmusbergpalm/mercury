// Package experiments runs the economy's experiments: the runs behind the
// economy page (web/static/economy.html), each a textbook result tested on
// the economy alone, with no nations (no state, armies, roads or new
// technology) and no population growth (a growing population moves
// everything, and the experiments compare like with like).
//
// Each experiment writes its results to data/<name>.json (and the
// transport sweep to data/sweep/), which tools/explainer/build.sh turns
// into the page. Each has its own go generate target, so one can be rerun
// without the rest:
//
//	go generate ./experiments                  # every experiment
//	go generate -run corridor ./experiments    # one
//
// An experiment can also record its runs' whole economy as it goes, a
// snapshot every so many days, to read out afterwards whatever it didn't
// measure (see Out.Recorder):
//
//	go run ./experiments/run -out /tmp/money -record 30 money
//
// Most runs first settle the map for Settle years (owners build up their
// firms and people sort themselves between tiles), then run their
// experiment for Years more. The settled map is computed once per
// experiment and every run restores its own copy (economy.Snapshot).
package experiments

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	. "github.com/rasmusbergpalm/mercury/economy"
)

// The runs settle for Settle years before each experiment, which then
// runs Years.
const (
	Settle = 15
	Years  = 5
)

var (
	days   = int(TicksPerYear)
	settle = Settle * days // the day each experiment starts
	end    = settle + Years*days
)

// An Experiment is one of the economy page's experiments.
type Experiment struct {
	Name, About string
	Run         func(o *Out)
}

var all = map[string]Experiment{}

func register(e Experiment) { all[e.Name] = e }

// Names lists the experiments.
func Names() []string {
	var out []string
	for n := range all {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run runs the named experiment, writing its results into dir. With every
// above 0 its runs also record the economy every that many days (see
// Out.Recorder).
func Run(name, dir string, every int) error {
	e, ok := all[name]
	if !ok {
		return fmt.Errorf("no experiment %q (there are %v)", name, Names())
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	e.Run(&Out{Dir: dir, Every: every, experiment: name})
	return nil
}

// Out is where an experiment writes its results.
type Out struct {
	Dir        string
	Every      int // record the economy every this many days (0: not at all)
	experiment string
}

// Recorder records a run's economy, if the experiment was asked to:
// call it every day, and it writes a snapshot of the whole economy
// (economy.Snapshot) every o.Every days, one JSON line each, to
// records/<experiment>-<run>.jsonl.gz. Every number about every tile,
// firm, market and route is in each line, to read out afterwards whatever
// the experiment didn't. A line is about 1 MB on the classic map (a
// quarter of it compressed): a day a line, five years would take some
// 450 MB, so record every month or so unless the days matter. The
// returned close must be called once the run is done.
func (o *Out) Recorder(run string) (record func(w *World), close func()) {
	if o.Every <= 0 {
		return func(*World) {}, func() {}
	}
	dir := filepath.Join(o.Dir, "records")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	f, err := os.Create(filepath.Join(dir, o.experiment+"-"+run+".jsonl.gz"))
	if err != nil {
		panic(err)
	}
	z := gzip.NewWriter(f)
	enc := json.NewEncoder(z)
	record = func(w *World) {
		if w.Tick%o.Every == 0 {
			if err := enc.Encode(w.Snapshot()); err != nil {
				panic(err)
			}
		}
	}
	close = func() {
		if err := z.Close(); err != nil {
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}
	return record, close
}

// Write writes v as JSON to the file name.
func (o *Out) Write(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(o.Dir, name), b, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", filepath.Join(o.Dir, name))
}

// The map: the classic map, with its 1820 names.
var (
	theMap   = DefaultMap
	theNames = NameTiles(DefaultMap, 1820)
)

// worldAt is the full world on the map at a transport cost, with owners
// investing and people free to move, and no population growth.
func worldAt(cost float64) *World {
	w, err := BuildFullWorld(cost, theMap, theNames)
	if err != nil {
		panic(err)
	}
	w.Migration, w.Investment, w.Demography = true, true, false
	return w
}

// world is the map at the normal cost of transport.
func world() *World { return worldAt(1) }

var (
	settledOnce sync.Once
	settledSnap *Snapshot
)

// settled is a copy of the map settled for Settle years.
func settled() *World {
	settledOnce.Do(func() {
		w := world()
		for d := 0; d < settle; d++ {
			w.Step()
		}
		settledSnap = w.Snapshot()
	})
	w, err := Restore(settledSnap)
	if err != nil {
		panic(err)
	}
	return w
}

// parallel runs the jobs four at a time.
func parallel(jobs ...func()) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, j := range jobs {
		wg.Add(1)
		go func(j func()) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			j()
		}(j)
	}
	wg.Wait()
}

// basics are the goods that meet basic needs: what living costs.
var basics = func() []GoodID {
	var out []GoodID
	for _, nd := range Needs {
		if nd.Tier == Basic {
			out = append(out, nd.Goods...)
		}
	}
	return out
}()

// hopsFrom is every tile's distance in hops from tile.
func hopsFrom(w *World, tile int) []int {
	dist := make([]int, len(w.Regions))
	for i := range dist {
		dist[i] = -1
	}
	dist[tile] = 0
	q := []int{tile}
	for len(q) > 0 {
		x := q[0]
		q = q[1:]
		for _, nb := range w.Adjacent(x) {
			if dist[nb[0]] < 0 {
				dist[nb[0]] = dist[x] + 1
				q = append(q, nb[0])
			}
		}
	}
	return dist
}
