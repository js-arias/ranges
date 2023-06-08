// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// Package mapcmd implements a command to draw
// the geographic range of taxon in a map.
package mapcmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/js-arias/blind"
	"github.com/js-arias/command"
	"github.com/js-arias/earth"
	"github.com/js-arias/earth/model"
	"github.com/js-arias/ranges"
)

var Command = &command.Command{
	Usage: `map [-c|--columns] [-t|--taxon <name>]
	[--bg <image>]
	[--timepix <time-pixelation>] [--gray] [--key <key-file>]
	-o|--output <out-img-file> [<rng-file>...]`,
	Short: "draw a map of a taxon geographic range",
	Long: `
Package map draws the geographic range of the indicated taxon using a plate
carrée (equirectangular) projection.

One or more range files can be given as arguments. If no file is given, the
ranges will be read from the standard input.

Flag --output, or -o, is required and sets the name of the output image. If
multiple taxa are used, the taxon name, taxon age and type of range will be
append to the name of the image. By default the background image will be empty,
if the flag --bg is given, the indicated image will be used as the background,
or if the flag --timepix is defined, the indicated time pixelation will be used
as background. This alternative is useful if the taxa have different ages. Keys
for the time pixelation values can be defined with the flag --key, and flag
--gray uses gray colors (so ranges will be easier to see). By default the
output image will be 3600 pixels wide, use the flag --columns, or -c, to define
a different number of image columns.

By default maps for all taxa will be produced. Use the flag -taxon to define a
particular taxon to be mapped.
	`,
	SetFlags: setFlags,
	Run:      run,
}

var grayFlag bool
var colsFlag int
var bgFile string
var keyFlag string
var modelFile string
var taxFlag string
var output string

func setFlags(c *command.Command) {
	c.Flags().BoolVar(&grayFlag, "gray", false, "")
	c.Flags().IntVar(&colsFlag, "columns", 3600, "")
	c.Flags().IntVar(&colsFlag, "c", 3600, "")
	c.Flags().StringVar(&bgFile, "bg", "", "")
	c.Flags().StringVar(&keyFlag, "key", "", "")
	c.Flags().StringVar(&modelFile, "timepix", "", "")
	c.Flags().StringVar(&taxFlag, "taxon", "", "")
	c.Flags().StringVar(&taxFlag, "t", "", "")
	c.Flags().StringVar(&output, "output", "", "")
	c.Flags().StringVar(&output, "o", "", "")
}

func run(c *command.Command, args []string) error {
	if output == "" {
		return c.UsageError("undefined output image flag --output")
	}

	if bgFile != "" && modelFile != "" {
		return c.UsageError("both --bg and --timepix flags defined")
	}

	var bgImg image.Image
	if bgFile != "" {
		var err error
		bgImg, err = readBgImage(bgFile)
		if err != nil {
			return err
		}
	}
	var keys *pixKey
	var tPix *model.TimePix
	if modelFile != "" {
		if keyFlag != "" {
			var err error
			keys, err = readKeys(keyFlag)
			if err != nil {
				return err
			}
			if grayFlag && len(keys.gray) == 0 {
				grayFlag = false
			}
		}
		if keys != nil {
			var err error
			tPix, err = readTimePix(modelFile)
			if err != nil {
				return err
			}
		}
	}

	if len(args) == 0 {
		args = append(args, "-")
	}
	for _, a := range args {
		coll, err := readCollection(c.Stdin(), a)
		if err != nil {
			return err
		}
		if err := procCollection(coll, bgImg, tPix, keys); err != nil {
			return err
		}
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

// MillionYears is used to transform age in years
// to million years.
const millionYears = 1_000_000

func procCollection(c *ranges.Collection, bgImg image.Image, tp *model.TimePix, keys *pixKey) error {
	ls := c.Taxa()
	for _, tax := range ls {
		if taxFlag != "" && taxFlag != tax {
			continue
		}
		age := c.Age(tax)
		outImg := newImg(c.Pixelation())
		if bgImg != nil {
			outImg.setBg(bgImg)
		}
		if tp != nil {
			if tp.Pixelation().Equator() != c.Pixelation().Equator() {
				return fmt.Errorf("mismatch range pixelation: got %d pixels, want %d", c.Pixelation().Equator(), tp.Pixelation().Equator())
			}
			outImg.setModel(tp, age, keys)
		}
		rng := c.Range(tax)
		outImg.rng = rng

		tp := c.Type(tax)
		taxName := strings.Join(strings.Fields(tax), "_")
		name := fmt.Sprintf("%s-%s-%.2f-%s.png", output, taxName, float64(age)/millionYears, tp)
		if err := writeImage(name, outImg); err != nil {
			return err
		}
	}
	return nil
}

func readBgImage(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("when decoding image file %q: %v", name, err)
	}
	return img, nil
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

type mapImg struct {
	step  float64
	color map[int]color.RGBA
	pix   *earth.Pixelation
	rng   map[int]float64
}

func (m *mapImg) ColorModel() color.Model { return color.RGBAModel }
func (m *mapImg) Bounds() image.Rectangle { return image.Rect(0, 0, colsFlag, colsFlag/2) }
func (m *mapImg) At(x, y int) color.Color {
	lat := 90 - float64(y)*m.step
	lon := float64(x)*m.step - 180

	pos := m.pix.Pixel(lat, lon).ID()
	if v, ok := m.rng[pos]; ok {
		return blind.Gradient(v)
	}

	c, ok := m.color[pos]
	if !ok {
		return color.RGBA{0, 0, 0, 0}
	}
	return c
}

func newImg(pix *earth.Pixelation) *mapImg {
	return &mapImg{
		step:  360 / float64(colsFlag),
		color: make(map[int]color.RGBA, pix.Len()),
		pix:   pix,
	}
}

func (m *mapImg) setBg(bg image.Image) {
	stepX := float64(360) / float64(bg.Bounds().Dx())
	stepY := float64(180) / float64(bg.Bounds().Dy())
	for id := 0; id < m.pix.Len(); id++ {
		px := m.pix.ID(id).Point()
		x := int((px.Longitude() + 180) / stepX)
		y := int((90 - px.Latitude()) / stepY)
		r, g, b, a := bg.At(x, y).RGBA()
		c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
		m.color[id] = c
	}
}

func (m *mapImg) setModel(tp *model.TimePix, age int64, pix *pixKey) {
	age = tp.ClosestStageAge(age)
	for id := 0; id < m.pix.Len(); id++ {
		v, _ := tp.At(age, id)
		if grayFlag {
			cv, ok := pix.gray[v]
			if !ok {
				continue
			}
			m.color[id] = color.RGBA{cv, cv, cv, 255}
			continue
		}
		c, ok := pix.color[v]
		if !ok {
			continue
		}
		m.color[id] = c
	}
}

func writeImage(name string, m *mapImg) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if e != nil && err == nil {
			err = e
		}
	}()

	if err := png.Encode(f, m); err != nil {
		return fmt.Errorf("when encoding image file %q: %v", name, err)
	}
	return nil
}

// PixKey stores the color values
// for a pixel value.
type pixKey struct {
	color map[int]color.RGBA
	gray  map[int]uint8
}

func readKeys(name string) (*pixKey, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'

	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("while reading header: %v", err)
	}
	fields := make(map[string]int, len(head))
	for i, h := range head {
		h = strings.ToLower(h)
		fields[h] = i
	}
	for _, h := range []string{"key", "color"} {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("expecting field %q", h)
		}
	}

	pk := &pixKey{
		color: make(map[int]color.RGBA),
		gray:  make(map[int]uint8),
	}

	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		ln, _ := r.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("on row %d: %v", ln, err)
		}

		f := "key"
		k, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}

		f = "color"
		vals := strings.Split(row[fields[f]], ",")
		if len(vals) != 3 {
			return nil, fmt.Errorf("on row %d: field %q: found %d values, want 3", ln, f, len(vals))
		}

		red, err := strconv.Atoi(strings.TrimSpace(vals[0]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [red value]: %v", ln, f, err)
		}
		if red > 255 {
			return nil, fmt.Errorf("on row %d: field %q [red value]: invalid value %d", ln, f, red)
		}
		green, err := strconv.Atoi(strings.TrimSpace(vals[1]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [green value]: %v", ln, f, err)
		}
		if green > 255 {
			return nil, fmt.Errorf("on row %d: field %q [green value]: invalid value %d", ln, f, green)
		}
		blue, err := strconv.Atoi(strings.TrimSpace(vals[2]))
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q [blue value]: %v", ln, f, err)
		}
		if blue > 255 {
			return nil, fmt.Errorf("on row %d: field %q [blue value]: invalid value %d", ln, f, blue)
		}

		c := color.RGBA{uint8(red), uint8(green), uint8(blue), 255}
		pk.color[k] = c

		f = "gray"
		if _, ok := fields[f]; !ok {
			continue
		}
		gray, err := strconv.Atoi(row[fields[f]])
		if err != nil {
			return nil, fmt.Errorf("on row %d: field %q: %v", ln, f, err)
		}
		if gray > 255 {
			return nil, fmt.Errorf("on row %d: field %q: invalid value %d", ln, f, gray)
		}

		pk.gray[k] = uint8(gray)
	}
	return pk, nil
}
