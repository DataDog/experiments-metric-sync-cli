// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileDefaultsCertified(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Options.Certified() {
		t.Fatal("expected omitted options.is_certified to default to true")
	}
}

func TestLoadFilePreservesExplicitUncertified(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
options:
  is_certified: false
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Options.Certified() {
		t.Fatal("expected options.is_certified=false to be preserved")
	}
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metric-sync.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
