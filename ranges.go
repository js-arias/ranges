// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package ranges implements a pixelation
// for data about species distribution ranges.
//
// A range is a representation of a taxon distribution,
// and it can be either explicit sampling points,
// or a probability density for the presence of a taxon
// at a pixel.
package ranges

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/js-arias/earth"
	"golang.org/x/exp/slices"
)

// Type is the type of range map.
type Type string

// Type valid values.
const (
	// Points is a range map made of points
	// (i.e. a presence-absence pixelation).
	Points Type = "points"

	// A Range is a continuous range map
	// (for example a pixelation of a range map from literature,
	// the output from a distribution model,
	// or a density estimation for a set of points).
	Range Type = "range"
)

// A Collection is a collection of distribution ranges
// with an associated pixelation.
type Collection struct {
	pix  *earth.Pixelation
	taxa map[string]*taxon
}

// New creates a new collection of taxon ranges
// using an isolatitude pixelation.
func New(pix *earth.Pixelation) *Collection {
	return &Collection{
		pix:  pix,
		taxa: make(map[string]*taxon),
	}
}

// Add adds a point to a taxon at an specific age
// (in years).
//
// To add a point the range of the taxon must be defined
// as 'points'
// (i.e. a presence-absence pixelation).
func (c *Collection) Add(name string, age int64, lat, lon float64) {
	name = canon(name)
	if name == "" {
		return
	}

	tax, ok := c.taxa[name]
	if !ok {
		tax = &taxon{
			name: name,
			tp:   Points,
			rng:  make(map[int]float64),
		}
		c.taxa[name] = tax
	}
	if tax.tp != Points {
		return
	}

	pix := c.pix.Pixel(lat, lon).ID()
	tax.rng[pix] = 1
}

// Age returns the age
// (in years)
// used to set a range map
// for a taxon.
func (c *Collection) Age(name string) int64 {
	name = canon(name)
	if name == "" {
		return 0
	}

	tax, ok := c.taxa[name]
	if !ok {
		return 0
	}

	return tax.age
}

// Pixelation returns the underlying pixelation
// of a Collection.
func (c *Collection) Pixelation() *earth.Pixelation {
	return c.pix
}

// Range returns a range map of a taxon
// and the type of the range map.
//
// The range map is a map of pixel IDs
// to the probability field scaled to set
// the maximum value equal to 1.0
// (so in the case of points,
// all points will be set to be 1.0,
// and all other pixels will be 0.0).
func (c *Collection) Range(name string) (map[int]float64, Type) {
	name = canon(name)
	if name == "" {
		return nil, ""
	}

	tax, ok := c.taxa[name]
	if !ok {
		return nil, ""
	}

	return tax.rng, tax.tp
}

// Set sets a range map for a taxon at the indicated age
// (in years).
// The range is a map of pixel IDs
// to a probability.
// It will overwrite any range map previously set for the taxon.
func (c *Collection) Set(name string, age int64, rng map[int]float64) {
	name = canon(name)
	if name == "" {
		return
	}

	tax, ok := c.taxa[name]
	if !ok {
		tax = &taxon{
			name: name,
		}
		c.taxa[name] = tax
	}
	tax.age = age
	tax.tp = Range
	tax.rng = make(map[int]float64, len(rng))

	var max float64
	for _, v := range rng {
		if v > max {
			max = v
		}
	}

	for px, p := range rng {
		if px >= c.pix.Len() {
			msg := fmt.Sprintf("invalid pixel value: %d", px)
			panic(msg)
		}
		tax.rng[px] = p / max
	}
}

// Taxa returns an slice with the taxon names
// of the taxa in the collection of ranges.
func (c *Collection) Taxa() []string {
	ls := make([]string, 0, len(c.taxa))
	for _, tax := range c.taxa {
		ls = append(ls, tax.name)
	}
	slices.Sort(ls)

	return ls
}

// A Taxon is a representation of a taxon range.
type taxon struct {
	// Name of the taxon
	name string

	// Type of the range map defined for the taxon
	tp Type

	// Age used for the pixels of the range map.
	age int64

	// Range of the taxon.
	//
	// It is a probability field scaled
	// to set the maximum value equal to 1.0
	rng map[int]float64
}

// Canon returns a taxon name
// in its canonical form.
func canon(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	r, n := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[n:]
}
