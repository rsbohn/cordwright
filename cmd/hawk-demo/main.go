// hawk-demo creates a deterministic, synthetic Hawk reader fixture.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rsbohn/cordwright/internal/hawk"
)

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("hawk-demo", flag.ContinueOnError)
	flags.SetOutput(out)
	stride := flags.Int("stride", 512, "image record size: 400 or 512")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: hawk-demo [-stride 400|512] OUTPUT.img (must not exist)")
	}
	image, err := hawk.DemoImage(*stride)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(flags.Arg(0), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, err := f.Write(image)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if n != len(image) {
		return io.ErrShortWrite
	}
	if closeErr != nil {
		return closeErr
	}
	_, err = fmt.Fprintf(out, "Created %s: %d bytes, %d sectors, stride %d. Synthetic, non-bootable.\n", flags.Arg(0), len(image), hawk.SectorLimit, *stride)
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, "hawk-demo:", err)
		os.Exit(1)
	}
}
