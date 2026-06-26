package main

import (
	"os"

	"github.com/DataDog/experiments-metric-sync-cli/internal/cli"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
