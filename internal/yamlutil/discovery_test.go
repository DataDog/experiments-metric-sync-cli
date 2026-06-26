// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverRecursesAndSortsYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b.yml"))
	write(t, filepath.Join(dir, "nested", "a.yaml"))
	write(t, filepath.Join(dir, ".hidden", "ignored.yaml"))
	write(t, filepath.Join(dir, "notes.txt"))

	got, err := Discover([]string{dir}, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(dir, "b.yml"),
		filepath.Join(dir, "nested", "a.yaml"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDiscoverCombinesPositionalAndFileFlags(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	write(t, first)
	write(t, second)

	got, err := Discover([]string{first}, []string{second, first})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{first, second}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema_version: 1\nsync_tag: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
