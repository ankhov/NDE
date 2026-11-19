// Package main is the Network Diagnostics Engine (NDE) entry point.
//
// main() delegates to runApp(), whose implementation is selected by build tag:
//   - default (headless): headless.go — runs the Application Core until signal.
//   - -tags=gui:          gui.go     — runs the Wails v3 desktop application.
//
// Both implementations share the same bootstrap (config, logging, storage,
// migration, Application Core) so the domain is identical in either mode.
package main

import (
	"fmt"
	"os"
)

// buildVersion is overridden at link time with -ldflags "-X main.buildVersion=...".
var buildVersion = "dev"

func main() {
	if err := runApp(); err != nil {
		fmt.Fprintf(os.Stderr, "nde: %v\n", err)
		os.Exit(1)
	}
}
