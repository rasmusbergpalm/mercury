// Command run runs the economy's experiments by name (see package
// experiments), writing their results into -out:
//
//	go run ./experiments/run -out experiments/data corridor blight
//	go run ./experiments/run -out /tmp/money -record 30 money   # with the economy every 30 days
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rasmusbergpalm/mercury/experiments"
)

func main() {
	out := flag.String("out", "data", "directory for the results")
	every := flag.Int("record", 0, "record each run's whole economy every this many days, in <out>/records (0: not at all)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: run [-out dir] [-record days] experiment...\nexperiments: %v\n", experiments.Names())
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	for _, name := range flag.Args() {
		if err := experiments.Run(name, *out, *every); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
