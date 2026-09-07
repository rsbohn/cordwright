package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rsbohn/cordwright/internal/shell"
)

type hosts []string

func (h *hosts) String() string         { return strings.Join(*h, ",") }
func (h *hosts) Set(value string) error { *h = append(*h, value); return nil }

func run(args []string, in io.Reader, out, diagnostics io.Writer, terminal bool) int {
	flags := flag.NewFlagSet("cordwright", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	var mounts hosts
	flags.Var(&mounts, "host", "mount a read-only host folder as NAME=PATH (repeatable)")
	batch := flags.Bool("batch", false, "read commands without a prompt")
	personality := flags.String("personality", "unix", "shell personality (unix)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(diagnostics, "cordwright:", err); return 1 }
	if *personality != "unix" {
		return fail(fmt.Errorf("unsupported personality %q", *personality))
	}
	sh := shell.NewUnix()
	defer sh.Close()
	for _, spec := range mounts {
		name, source, ok := strings.Cut(spec, "=")
		if !ok || name == "" || source == "" {
			return fail(fmt.Errorf("-host expects NAME=PATH"))
		}
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		if err := sh.MountHost(source, name); err != nil {
			return fail(err)
		}
	}
	if flags.NArg() > 0 {
		_, err := sh.Execute(flags.Args(), out)
		if err != nil {
			return fail(err)
		}
		return 0
	}
	if err := sh.Run(in, out, diagnostics, terminal && !*batch); err != nil {
		return fail(err)
	}
	return 0
}

func main() {
	info, err := os.Stdin.Stat()
	terminal := err == nil && info.Mode()&os.ModeCharDevice != 0
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, terminal))
}
