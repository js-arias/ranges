// Copyright © 2022 J. Salvador Arias <jsalarias@gmail.com>
// All rights reserved.
// Distributed under BSD2 license that can be found in the LICENSE file.

// TaxRange is a tool to dealt with pixelated range maps.
package main

import (
	"github.com/js-arias/command"
	"github.com/js-arias/ranges/cmd/taxrange/imppoints"
	"github.com/js-arias/ranges/cmd/taxrange/taxa"
)

var app = &command.Command{
	Usage: "taxrange <command> [<argument>...]",
	Short: "a tool to dealt with pixelated range maps",
}

func init() {
	app.Add(imppoints.Command)
	app.Add(taxa.Command)
}

func main() {
	app.Main()
}
