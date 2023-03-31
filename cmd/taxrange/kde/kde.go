// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package kde implements a command
// to estimate a range distribution
// using a kernel density estimator.
package kde

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/earth/stat"
	"github.com/js-arias/earth/stat/dist"
	"github.com/js-arias/earth/stat/pixprob"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `kde --timepix <time-pixelation> [--prior <prior-file>]
	[--lambda <value>] [--bound <value>]
	[-o|--output <file>] [<rng-file>...]`,
	Short: "estimate a geographic range using a KDE",
	Long: `
Command kde reads one or more geographic range files, and produce a new range
map using a Kernel Density Estimation, based on an spherical normal.

One or more range files can be given as arguments. If no file is given, the
ranges will be read from the standard input.

The flag --timepix is required an defines the time pixelation that will contain
the raster values for each pixel. Prior probabilities for each pixel type can
be defined on a file and read with the flag --prior. The pixel prior file is a
tab-delimited file with the following columns:

	-key	the value used as identifier
	-prior	the prior probability for a pixel with that value

Any other columns, will be ignored. Here is an example of a pixel prior file:

	key	prior	comment
	0	0.000000	deep ocean
	1	0.010000	oceanic plateaus
	2	0.050000	continental shelf
	3	0.950000	lowlands
	4	1.000000	highlands
	5	0.001000	ice sheets

The flag --lambda defines the concentration parameter of the spherical normal
(equivalent to kappa parameter of the von Mises-Fisher distribution) in
1/radian^2 units. If no value is defined, it will use the 1/size^2 of a pixel
in the pixelation used for the range files.

By default only pixels at .95 of the spherical normal CDF will be used. Use
the flag --bound to set the bound for the normal CDF.

By default the output will be printed in the standard output. If the flag
--output, or -o, is defined, the indicated file will be used as output. If the
file exists, existing taxons will be replaced, and new taxon will be added to
the indicated file.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var lambdaFlag float64
var boundFlag float64
var modelFile string
var priorFile string
var output string

func setFlags(c *command.Command) {
	c.Flags().Float64Var(&lambdaFlag, "lambda", 0, "")
	c.Flags().Float64Var(&boundFlag, "bound", 0.95, "")
	c.Flags().StringVar(&modelFile, "timepix", "", "")
	c.Flags().StringVar(&priorFile, "prior", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) (err error) {
	if modelFile == "" {
		return c.UsageError("undefined time pixelation flag --timepix")
	}
	tPix, err := readTimePix(modelFile)

	var prior pixprob.Pixel
	if priorFile != "" {
		prior, err = readPixelPrior(priorFile)
	}

	coll := ranges.New(tPix.Pixelation())
	if len(args) == 0 {
		args = append(args, "-")
	}
	for _, a := range args {
		c, err := readCollection(c.Stdin(), a)
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
	kdeColl, err := readOutColl(output, coll.Pixelation())
	if err != nil {
		return err
	}

	if lambdaFlag == 0 {
		angle := earth.ToRad(coll.Pixelation().Step())
		lambdaFlag = 1 / (angle * angle)
		fmt.Fprintf(c.Stderr(), "# Using lambda value of: %.6f\n", lambdaFlag)
	}
	n := dist.NewNormal(lambdaFlag, tPix.Pixelation())

	for _, tax := range coll.Taxa() {
		rng := coll.Range(tax)
		kde := stat.KDE(n, rng, tPix, 0, prior, boundFlag)
		kdeColl.Set(tax, 0, kde)
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
	if err := kdeColl.TSV(w); err != nil {
		return err
	}

	return nil
}

func readCollection(r io.Reader, name string) (*ranges.Collection, error) {
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

	coll, err := ranges.ReadTSV(r, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}

	return coll, nil
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

func readTimePix(name string) (*model.TimePix, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp, err := model.ReadTimePix(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading file %q: %v", name, err)
	}
	return tp, nil
}

func readPixelPrior(name string) (pixprob.Pixel, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	prior, err := pixprob.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("when reading file %q: %v", name, err)
	}
	return prior, nil
}
