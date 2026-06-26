// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

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
