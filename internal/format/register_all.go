// Package format — register_all.go
//
// Go runs every init() in a package in file-name order, so by the time
// this file's init() runs, every formatter file has already called
// register() from its own init(). Here we just sanity-check the result.
package format

import "log"

func init() {
	if len(registry) == 0 {
		panic("format: no formatters registered — check that " +
			"cmd/server/main.go imports file-converter/internal/format")
	}
	log.Printf("[INFO] format: %d formatter(s) registered", len(registry))
}