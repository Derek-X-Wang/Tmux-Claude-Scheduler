package main

import (
	"github.com/derekxwang/tcs/cmd"
)

// Version information (set by goreleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Set version info for cobra
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
