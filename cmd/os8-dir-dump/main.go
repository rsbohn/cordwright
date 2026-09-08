package main

import (
	"flag"
	"fmt"
	"github.com/rsbohn/cordwright/internal/os8"
	"os"
)

func main() {
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "usage: os8-dir-dump IMAGE.tu56 [IMAGE.tu56 ...]") }
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	failed := false
	for _, name := range flag.Args() {
		fmt.Printf("\n%s\n", name)
		raw, err := os.ReadFile(name)
		if err == nil {
			err = os8.Dump(os.Stdout, raw)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "os8-dir-dump:", name, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
