// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package imppoints implements a command
// to import taxon distribution ranges
// from a list of specimen records.
package imppoints

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/gbifer/tsv"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `imp.points [-e|--equator <value>] [--age <age>]
	[-f|--format <format>] [-o|--output <file>] [<input-file>...]`,
	Short: "import a list of specimen records",
	Long: `
Command imp.points reads one or more files with specimen records, and import
them into a range map into a isolatitude pixelation.

One or more input files can be given as arguments for the command. If no file
is given, the input will be read from the standard input. By default the files
will be simple text files, delimited by tab character and with the following
fields: "species", "latitude", and "longitude". Using the flag --format, or -f,
an alternative format can be defined. Valid formats are:

	darwin	DarwinCore format using tab characters as delimiters (i.e. as
		files download from GBIF). Key fields are: "species",
		"decimalLatitude", and "decimalLongitude".
	pbdb    Tab-delimited files downloaded from PaleoBiology DataBase, the
	        following fields are required: "accepted_name", "lat", and
	        "lng".
	csv	Darwin core files, but using commas as delimiters.
	text	The default value, a simple tab-delimited file, with the
		following fields: "species", "latitude", and "longitude".

By default the output will be printed in the standard output. If the flag
--output, or -o, is defined the indicated file will be used as output. If the
file exists, points will be added to the indicated file.

By default the pixelation will of 360 pixels at the equator. This can be
changed with the flag --equator, or -e. If an output file is defined, and the
file exists, then the pixelation will be read from that file.

By default points will be set at present time. Use flag --age to set a
different time. Take into account that this command does not make any rotation,
so the locations will be set at the given age, assuming that the indicated
coordinates are real paleo-coordinates. The age is set in million years.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var ageFlag float64
var equator int
var format string
var output string

func setFlags(c *command.Command) {
	c.Flags().IntVar(&equator, "e", 360, "")
	c.Flags().IntVar(&equator, "equator", 360, "")
	c.Flags().StringVar(&format, "format", "text", "")
	c.Flags().StringVar(&format, "f", "text", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) (err error) {
	coll, err := readCollection(output)
	if err != nil {
		return err
	}
	if equator != 360 && coll.Pixelation().Equator() != equator {
		return fmt.Errorf("invalid --equator value %d: want %d", equator, coll.Pixelation().Equator())
	}

	format = strings.ToLower(format)
	readFunc := readTextData
	switch format {
	case "text":
	case "":
	case "darwin":
		readFunc = readGBIFData
	case "csv":
		readFunc = readGBIFData
	case "pbdb":
		readFunc = readPaleoDBData
	default:
		return fmt.Errorf("format %q unknown", format)
	}

	if len(args) == 0 {
		args = append(args, "-")
	}
	for _, a := range args {
		if err := readFunc(c.Stdin(), a, coll); err != nil {
			return err
		}
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
	if err := coll.TSV(w); err != nil {
		return err
	}
	return nil
}

func readCollection(name string) (*ranges.Collection, error) {
	if name == "" {
		pix := earth.NewPixelation(equator)
		return ranges.New(pix), nil
	}

	f, err := os.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		pix := earth.NewPixelation(equator)
		return ranges.New(pix), nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	coll, err := ranges.ReadTSV(f, nil)
	if err != nil {
		return nil, fmt.Errorf("when reading %q: %v", name, err)
	}
	return coll, nil
}

// MillionYears is used to set a flag age
// (in million years)
// to pixel ages (in years).
const millionYears = 1_000_000

var headerFields = []string{
	"species",
	"latitude",
	"longitude",
}

func readTextData(r io.Reader, name string, c *ranges.Collection) error {
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

	tab := csv.NewReader(r)
	tab.Comma = '\t'
	tab.Comment = '#'

	head, err := tab.Read()
	if err != nil {
		return fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range headerFields {
		if _, ok := fields[h]; !ok {
			return fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	age := int64(ageFlag * millionYears)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "species"
		tax := row[fields[f]]

		f = "latitude"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "longitude"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		if tp := c.Type(tax); tp != "" && tp != ranges.Points {
			return fmt.Errorf("taxon %q: has defined a %q map", tax, tp)
		}

		c.Add(tax, age, lat, lon)
	}
	return nil
}

var gbifFields = []string{
	"species",
	"decimallatitude",
	"decimallongitude",
}

func readGBIFData(r io.Reader, name string, c *ranges.Collection) error {
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

	tab := tsv.NewReader(r)
	if format != "csv" {
		tab.Comma = '\t'
	}

	head, err := tab.Read()
	if err != nil {
		return fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range gbifFields {
		if _, ok := fields[h]; !ok {
			return fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	age := int64(ageFlag * millionYears)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "species"
		tax := row[fields[f]]

		f = "decimallatitude"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "decimallongitude"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		if tp := c.Type(tax); tp != "" && tp != ranges.Points {
			return fmt.Errorf("taxon %q: has defined a %q map", tax, tp)
		}

		c.Add(tax, age, lat, lon)
	}

	return nil
}

var pbdbFields = []string{
	"accepted_name",
	"lat",
	"lng",
}

func readPaleoDBData(r io.Reader, name string, c *ranges.Collection) error {
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

	br := bufio.NewReader(r)
	metaLines := 0
	for {
		ln, err := br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("on file %q: %v", name, err)
		}
		metaLines++
		if strings.HasPrefix(ln, "Records:") {
			break
		}
	}

	tab := tsv.NewReader(br)
	tab.Comma = '\t'

	head, err := tab.Read()
	if err != nil {
		return fmt.Errorf("on file %q: while reading header: %v", name, err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range pbdbFields {
		if _, ok := fields[h]; !ok {
			return fmt.Errorf("on file %q: expecting field %q", name, h)
		}
	}

	age := int64(ageFlag * millionYears)
	for {
		row, err := tab.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := tab.FieldPos(0)
		ln += metaLines
		if err != nil {
			return fmt.Errorf("on file %q: row %d: %v", name, ln, err)
		}

		f := "accepted_name"
		tax := row[fields[f]]

		f = "lat"
		lat, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lat < -90 || lat > 90 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid latitude %.6f", name, ln, f, lat)
		}

		f = "lng"
		lon, err := strconv.ParseFloat(row[fields[f]], 64)
		if err != nil {
			return fmt.Errorf("on file %q: row %d: field %q: %v", name, ln, f, err)
		}
		if lon < -180 || lon > 180 {
			return fmt.Errorf("on file %q: row %d: field %q: invalid longitude %.6f", name, ln, f, lon)
		}

		if tp := c.Type(tax); tp != "" && tp != ranges.Points {
			return fmt.Errorf("taxon %q: has defined a %q map", tax, tp)
		}

		c.Add(tax, age, lat, lon)
	}

	return nil
}
