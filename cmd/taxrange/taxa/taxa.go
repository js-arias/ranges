// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package taxa implements a command to print
// the list of taxa in a taxon range collection.
package taxa

import (
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: "taxa [--count] [<rng-file>...]",
	Short: "prints the list of taxa with distribution ranges",
	Long: `
Command taxa reads one or more geographic range files and prints the list of
taxa with a defined distribution range in each one of them.

One or more range files can be given as arguments. If no file is given, the
ranges will be read from the standard input.

If the flag --count is defined, the type of distribution map, and the number of
pixels for each taxon will be given.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var countFlag bool

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&countFlag, "count", false, "")
}

func run(c *command.Command, args []string) error {
	if len(args) == 0 {
		args = append(args, "-")
	}
	for i, a := range args {
		if i > 0 {
			fmt.Fprintf(c.Stdout(), "\n")
		}
		if err := printList(c.Stdin(), c.Stdout(), a); err != nil {
			return err
		}
	}
	return nil
}

func printList(r io.Reader, w io.Writer, name string) error {
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	} else {
		name = "stdin"
	}

	coll, err := ranges.ReadTSV(r, nil)
	if err != nil {
		return fmt.Errorf("when reading %q: %v", name, err)
	}

	fmt.Fprintf(w, "%s:\n", name)
	ls := coll.Taxa()
	for _, tax := range ls {
		fmt.Fprintf(w, "\t%s", tax)
		if countFlag {
			rng, tp := coll.Range(tax)
			fmt.Fprintf(w, "\t%s\t%d", tp, len(rng))
		}
		fmt.Fprintf(w, "\n")
	}
	return nil
}
