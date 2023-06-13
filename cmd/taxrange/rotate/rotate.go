// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package rotate implements a command to rotate the points
// of a range distribution.
package rotate

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `rotate --model <motion-model> --ages <file>
	[-o|--output <file>] [<rng-file>...]`,
	Short: "rotate range using a plate motion model",
	Long: `
Command rotate reads one or more geographic range files, with present
locations, and produce a new range file with the pixels rotated to an
indicated age.

One or more range files can be given as arguments. If no file is given, the
range will be read from the standard input.

The flag --model is required and defines a pixelated plate motion model. The
model must be compatible with the pixelation defined by the range files.

The flag --ages define the name of the file file with the ages for each taxon
to be rotated. The age files is a TSV file without header, and the following
columns:

	- name	name of the taxon
	- age	the age (in million years) of the taxon

By default the output will be printed in the standard output. If the flag
--output, or -o, is defined, the indicated file will be used as output. If the
file exists, existing taxons will be replaced, and new taxon will be added to
the indicated file.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var modelFile string
var agesFile string
var output string

func setFlags(c *command.Command) {
	c.Flags().StringVar(&modelFile, "model", "", "")
	c.Flags().StringVar(&agesFile, "ages", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if modelFile == "" {
		return c.UsageError("flag --model required")
	}
	if agesFile == "" {
		return c.UsageError("flag --ages required")
	}

	tot, err := readRotation(modelFile)
	if err != nil {
		return err
	}

	ages, err := readAges()
	if err != nil {
		return err
	}

	coll := ranges.New(tot.Pixelation())
	if len(args) == 0 {
		args = append(args, "-")
	}
	for _, a := range args {
		c, err := readCollection(c.Stdin(), a, tot.Pixelation())
		if err != nil {
			return err
		}
		pix := c.Pixelation()

		for _, nm := range c.Taxa() {
			if c.Type(nm) != ranges.Points {
				continue
			}
			age := c.Age(nm)
			rng := c.Range(nm)
			for id := range rng {
				pt := pix.ID(id).Point()
				coll.Add(nm, age, pt.Latitude(), pt.Longitude())
			}
		}
	}
	if len(coll.Taxa()) == 0 {
		return nil
	}

	rotColl, err := readOutColl(output, coll.Pixelation())
	if err != nil {
		return err
	}

	for _, tax := range coll.Taxa() {
		rng := coll.Range(tax)

		age, ok := ages[strings.ToLower(tax)]
		if !ok {
			// store pixels with undefined rotations
			rotColl.SetPixels(tax, coll.Age(tax), rng)
			continue
		}

		// ignore taxa already rotated and warn the user
		if a := coll.Age(tax); a != 0 {
			fmt.Fprintf(c.Stderr(), "WARNING: taxon %q already rotated to age %.6f\n", tax, float64(a)/millionYears)
			rotColl.SetPixels(tax, a, rng)
			continue
		}

		// store un-rotated pixels
		if age == 0 {
			rotColl.SetPixels(tax, 0, rng)
			continue
		}

		rot := tot.Rotation(age)
		n := make(map[int]float64, len(rng))
		for px := range rng {
			dst := rot[px]
			for _, np := range dst {
				n[np] = 1.0
			}
		}
		if len(n) == 0 {
			fmt.Fprintf(c.Stderr(), "WARNING: taxon %q rotation to age %.6f: empty range\n", tax, float64(age)/millionYears)
			continue
		}
		rotColl.SetPixels(tax, age, n)
	}

	w := c.Stdout()
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer func() {
			e := f.Close()
			if e != nil && err == nil {
				err = e
			}
		}()
		w = f
	}
	if err := rotColl.TSV(w); err != nil {
		return err
	}

	return nil
}

func readRotation(name string) (*model.Total, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rot, err := model.ReadTotal(f, nil, false)
	if err != nil {
		return nil, fmt.Errorf("on file %q: %v", name, err)
	}

	return rot, nil
}

func readCollection(r io.Reader, name string, pix *earth.Pixelation) (*ranges.Collection, error) {
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	} else {
		name = "stdin"
	}

	coll, err := ranges.ReadTSV(r, pix)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}

const millionYears = 1_000_000

func readAges() (map[string]int64, error) {
	f, err := os.Open(agesFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tab := csv.NewReader(f)
	tab.Comma = '\t'
	tab.Comment = '#'

	fields := map[string]int{
		"taxon": 0,
		"age":   1,
	}
	ages := make(map[string]int64)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		ln, _ := tab.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("%q: on row %d: %v", agesFile, ln, err)
		}
		if len(row) < len(fields) {
			return nil, fmt.Errorf("%q: got %d rows, want %d", agesFile, len(row), len(fields))
		}

		ff := "taxon"
		name := strings.ToLower(strings.Join(strings.Fields(row[fields[ff]]), " "))
		if name == "" {
			continue
		}

		ff = "age"
		ageF, err := strconv.ParseFloat(row[fields[ff]], 64)
		if err != nil {
			return nil, fmt.Errorf("%q: on row %d: field %q: %v", agesFile, ln, ff, err)
		}

		age := int64(ageF * millionYears)
		ages[name] = age
	}
	return ages, nil
}

func readOutColl(name string, pix *earth.Pixelation) (*ranges.Collection, error) {
	if name == "" {
		return ranges.New(pix), nil
	}

	f, err := os.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return ranges.New(pix), nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, pix)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
}
