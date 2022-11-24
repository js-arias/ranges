// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package ranges

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/js-arias/earth"
	"golang.org/x/exp/slices"
)

var headerFields = []string{
	"taxon",
	"type",
	"age",
	"equator",
	"pixel",
	"density",
}

// ReadTSV reads a collection of range maps
// from a TSV file.
//
// The TSV must contain the following columns:
//
//   - taxon, the name of the taxon
//   - type, the type of the range model.
//     Can be "points" (for presence-absence pixelation),
//     or "range" (for a pixelated range map).
//   - age, for the age stage of the pixels
//     (in million years)
//   - equator, for the number of pixels in the equator
//   - pixel, the ID of a pixel (from the pixelation)
//   - density, the density for the presence at that pixel
//
// Here is an example file:
//
//	# range distribution models
//	taxon	type	age	equator	pixel	density
//	Brontostoma discus	points	0	360	17319	1.000000
//	Brontostoma discus	points	0	360	19117	1.000000
//	Eoraptor lunensis	range	230000000	360	34661	0.200000
//	Eoraptor lunensis	range	230000000	360	34662	0.500000
//	Eoraptor lunensis	range	230000000	360	34663	1.000000
//	Eoraptor lunensis	range	230000000	360	34664	0.500000
//	Eoraptor lunensis	range	230000000	360	34665	0.200000
//	Rhododendron ericoides	points	0	360	18588	1.000000
//	Rhododendron ericoides	points	0	360	19305	1.000000
//	Rhododendron ericoides	points	0	360	19308	1.000000
func ReadTSV(r io.Reader, pix *earth.Pixelation) (*Collection, error) {
	tab := csv.NewReader(r)
	tab.Comma = '\t'
	tab.Comment = '#'

	head, err := tab.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range headerFields {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	var c *Collection
	max := make(map[string]float64)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "equator"
		eq, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if pix == nil {
			pix = earth.NewPixelation(eq)
		}
		if pix.Equator() != eq {
			return nil, fmt.Errorf("on row %d: field %q: got %d, want %d", ln, f, eq, pix.Equator())
		}

		if c == nil {
			c = New(pix)
		}

		f = "type"
		var tp Type
		switch strings.ToLower(row[fields[f]]) {
		case string(Points):
			tp = Points
		case string(Range):
			tp = Range
		case "":
			tp = Points
		default:
			return nil, fmt.Errorf("on row %d: field %q: invalid type %q", ln, f, row[fields[f]])
		}

		f = "age"
		age, err := strconv.ParseInt(row[fields[f]], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		f = "taxon"
		nm := canon(row[fields[f]])
		if nm == "" {
			continue
		}
		tax, ok := c.taxa[nm]
		if !ok {
			tax = &taxon{
				name: nm,
				tp:   tp,
				age:  age,
				rng:  make(map[int]float64),
			}
			c.taxa[nm] = tax
		}
		if tax.tp != tp {
			return nil, fmt.Errorf("on row %d: field %q: invalid type: got %q, want %q", ln, f, tp, tax.tp)
		}
		if tax.age != age {
			return nil, fmt.Errorf("on row %d: field %q: invalid age: got %d, want %d", ln, f, age, tax.age)
		}

		f = "pixel"
		px, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if px >= pix.Len() {
			return nil, fmt.Errorf("on row %d: field %q: invalid pixel value %d", ln, f, px)
		}

		density := float64(1)
		if tax.tp == Range {
			f = "density"
			d, err := strconv.ParseFloat(row[fields[f]], 64)
			if err != nil {
				return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
			}
			density = d
		}
		tax.rng[px] = density
		if max[tax.name] < density {
			max[tax.name] = density
		}
	}
	if c == nil {
		return nil, fmt.Errorf("while reading data: %v", io.EOF)
	}

	// scale values
	for _, tax := range c.taxa {
		if tax.tp == Points {
			continue
		}
		if max[tax.name] == 1 {
			continue
		}

		scale := max[tax.name]
		for px, v := range tax.rng {
			tax.rng[px] = v / scale
		}
	}

	return c, nil
}

// TSV encodes range maps in a collection
// to a TSV file.
func (c *Collection) TSV(w io.Writer) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# taxon distribution range models\n")
	fmt.Fprintf(bw, "# data save on : %s\n", time.Now().Format(time.RFC3339))
	tab := csv.NewWriter(bw)
	tab.Comma = '\t'
	tab.UseCRLF = true

	if err := tab.Write(headerFields); err != nil {
		return fmt.Errorf("while writing header: %v", err)
	}

	eq := strconv.Itoa(c.pix.Equator())

	for _, name := range c.Taxa() {
		tax := c.taxa[name]
		age := strconv.FormatInt(tax.age, 10)

		pixels := make([]int, 0, len(tax.rng))
		for px := range tax.rng {
			pixels = append(pixels, px)
		}
		slices.Sort(pixels)

		for _, px := range pixels {
			d := strconv.FormatFloat(tax.rng[px], 'f', 6, 64)
			row := []string{
				tax.name,
				string(tax.tp),
				age,
				eq,
				strconv.Itoa(px),
				d,
			}
			if err := tab.Write(row); err != nil {
				return fmt.Errorf("while writing data: %v", err)
			}
		}
	}

	tab.Flush()
	if err := tab.Error(); err != nil {
		return fmt.Errorf("while writing data: %v", err)
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("while writing data: %v", err)
	}
	return nil
}
