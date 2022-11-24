// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

package ranges_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/js-arias/ranges"
)

func TestTSV(t *testing.T) {
	data := makeCollection(t)

	var buf bytes.Buffer
	if err := data.TSV(&buf); err != nil {
		t.Fatalf("while writing data: %v", err)
	}

	c, err := ranges.ReadTSV(strings.NewReader(buf.String()), nil)
	if err != nil {
		t.Fatalf("while reading data: %v", err)
	}

	testCollection(t, c)
}
