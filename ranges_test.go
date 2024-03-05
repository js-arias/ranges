// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package ranges_test

import (
	"reflect"
	"testing"

	"github.com/js-arias/earth"
	"github.com/js-arias/ranges"
)

func TestNew(t *testing.T) {
	coll := makeCollection(t)
	testCollection(t, coll)
}

func makeCollection(t testing.TB) *ranges.Collection {
	t.Helper()

	coll := ranges.New(earth.NewPixelation(360))
	data := []struct {
		name   string
		age    int64
		latLon [][2]float64
	}{
		{
			name: "Brontostoma discus",
			latLon: [][2]float64{
				{4.27, -72.54},
				{8.67, -83.56},
			},
		},
		{
			name: "Rhododendron ericoides",
			latLon: [][2]float64{
				{4.08, 118.52},
				{3.86, 115.55},
				{6.08, 116.55},
				{6.15, 116.65},
			},
		},
		{
			name: "Megazostrodon rudnerae",
			age:  201_600_000,
			latLon: [][2]float64{
				{-44.1, -1.4},
			},
		},
	}

	for _, d := range data {
		for _, p := range d.latLon {
			coll.Add(d.name, d.age, p[0], p[1])
		}
	}

	rng := map[int]float64{
		34661: 0.0833333,
		34662: 0.2083333,
		34663: 0.4166667,
		34664: 0.2083333,
		34665: 0.0833333,
	}
	coll.Set("Eoraptor lunensis", 230_000_000, rng)

	return coll
}

func testCollection(t testing.TB, coll *ranges.Collection) {
	t.Helper()

	if eq := coll.Pixelation().Equator(); eq != 360 {
		t.Errorf("pixelation: got %d pixels, want %d", eq, 360)
	}

	taxa := []string{"Brontostoma discus", "Eoraptor lunensis", "Megazostrodon rudnerae", "Rhododendron ericoides"}
	if ls := coll.Taxa(); !reflect.DeepEqual(ls, taxa) {
		t.Errorf("taxa: got %v, want %v", ls, taxa)
	}
	for _, nm := range taxa {
		if !coll.HasTaxon(nm) {
			t.Errorf("hasTaxon: taxon %q not found", nm)
		}
	}

	tests := map[string]struct {
		age int64
		tp  ranges.Type
		rng map[int]float64
	}{
		"Brontostoma discus": {
			tp: ranges.Points,
			rng: map[int]float64{
				17319: 1,
				19117: 1,
			},
		},
		"Rhododendron ericoides": {
			tp: ranges.Points,
			rng: map[int]float64{
				18588: 1,
				19305: 1,
				19308: 1,
			},
		},
		"Megazostrodon rudnerae": {
			tp:  ranges.Points,
			age: 201_600_000,
			rng: map[int]float64{
				34957: 1,
			},
		},
	}
	for name, test := range tests {
		rng := coll.Range(name)
		if !reflect.DeepEqual(rng, test.rng) {
			t.Errorf("taxon %q range map: got %v, want %v", name, rng, test.rng)
		}
		tp := coll.Type(name)
		if tp != test.tp {
			t.Errorf("taxon %q range type: got %q, want %q", name, tp, test.tp)
		}
		if age := coll.Age(name); age != test.age {
			t.Errorf("taxon %q age: got %d, want %d", name, age, test.age)
		}
	}

	// Eoraptor
	nm := "Eoraptor lunensis"
	eoAge := 230_000_000
	if age := coll.Age(nm); age != int64(eoAge) {
		t.Errorf("taxon %q age: got %d, want %d", nm, age, eoAge)
	}
	eoRng := map[int]float64{
		34661: 0.2,
		34662: 0.5,
		34663: 1,
		34664: 0.5,
		34665: 0.2,
	}
	tp := coll.Type(nm)
	if tp != ranges.Range {
		t.Errorf("taxon %q range type: got %q, want %q", nm, tp, ranges.Range)
	}
	rng := coll.Range(nm)
	for px, p := range rng {
		diff := p - eoRng[px]
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.001 {
			t.Errorf("taxon %q: pixel %d: got %.6f, want %.6f", nm, px, p, eoRng[px])
		}
	}
	for px := range eoRng {
		if _, ok := rng[px]; !ok {
			t.Errorf("taxon %q: pixel %d: not in range", nm, px)
		}
	}
}

func TestDelete(t *testing.T) {
	coll := makeCollection(t)
	del := "Rhododendron ericoides"
	coll.Delete(del)

	taxa := []string{"Brontostoma discus", "Eoraptor lunensis", "Megazostrodon rudnerae"}
	if ls := coll.Taxa(); !reflect.DeepEqual(ls, taxa) {
		t.Errorf("taxa: got %v, want %v", ls, taxa)
	}

	if coll.HasTaxon(del) {
		t.Errorf("HasTaxon: taxon %q found", del)
	}
}

func TestSetPixels(t *testing.T) {
	coll := makeCollection(t)

	nm := "Eoraptor lunensis"
	rng := map[int]float64{
		34662: 0.2083333,
		34663: 0.4166667,
		34664: 0.2083333,
		34665: 0.0833333,
	}
	coll.SetPixels(nm, 230_000_000, rng)

	tp := coll.Type(nm)
	if tp != ranges.Points {
		t.Errorf("taxon %q range type: got %q, want %q", nm, tp, ranges.Points)
	}

	got := coll.Range(nm)
	for px, p := range got {
		if _, ok := rng[px]; !ok {
			t.Errorf("taxon %q: pixel %d is true", nm, px)
		}
		if p != 1.0 {
			t.Errorf("taxon %q: pixel %d: probability %.6f, want 1.0", nm, px, p)
		}
	}

	for px := range rng {
		if _, ok := got[px]; !ok {
			t.Errorf("taxon %q: pixel %d is false", nm, px)
		}
	}
}

func TestAddPixel(t *testing.T) {
	pix := earth.NewPixelation(360)
	coll := ranges.New(pix)
	data := []struct {
		name   string
		age    int64
		latLon [][2]float64
	}{
		{
			name: "Brontostoma discus",
			latLon: [][2]float64{
				{4.27, -72.54},
				{8.67, -83.56},
			},
		},
		{
			name: "Rhododendron ericoides",
			latLon: [][2]float64{
				{4.08, 118.52},
				{3.86, 115.55},
				{6.08, 116.55},
				{6.15, 116.65},
			},
		},
		{
			name: "Megazostrodon rudnerae",
			age:  201_600_000,
			latLon: [][2]float64{
				{-44.1, -1.4},
			},
		},
	}

	for _, d := range data {
		for _, p := range d.latLon {
			px := pix.Pixel(p[0], p[1]).ID()
			coll.AddPixel(d.name, d.age, px)
		}
	}

	if eq := coll.Pixelation().Equator(); eq != 360 {
		t.Errorf("pixelation: got %d pixels, want %d", eq, 360)
	}

	taxa := []string{"Brontostoma discus", "Megazostrodon rudnerae", "Rhododendron ericoides"}
	if ls := coll.Taxa(); !reflect.DeepEqual(ls, taxa) {
		t.Errorf("taxa: got %v, want %v", ls, taxa)
	}
	for _, nm := range taxa {
		if !coll.HasTaxon(nm) {
			t.Errorf("hasTaxon: taxon %q not found", nm)
		}
	}

	tests := map[string]struct {
		age int64
		tp  ranges.Type
		rng map[int]float64
	}{
		"Brontostoma discus": {
			tp: ranges.Points,
			rng: map[int]float64{
				17319: 1,
				19117: 1,
			},
		},
		"Rhododendron ericoides": {
			tp: ranges.Points,
			rng: map[int]float64{
				18588: 1,
				19305: 1,
				19308: 1,
			},
		},
		"Megazostrodon rudnerae": {
			tp:  ranges.Points,
			age: 201_600_000,
			rng: map[int]float64{
				34957: 1,
			},
		},
	}
	for name, test := range tests {
		rng := coll.Range(name)
		if !reflect.DeepEqual(rng, test.rng) {
			t.Errorf("taxon %q range map: got %v, want %v", name, rng, test.rng)
		}
		tp := coll.Type(name)
		if tp != test.tp {
			t.Errorf("taxon %q range type: got %q, want %q", name, tp, test.tp)
		}
		if age := coll.Age(name); age != test.age {
			t.Errorf("taxon %q age: got %d, want %d", name, age, test.age)
		}
	}
}
