// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"gopkg.in/yaml.v3"
)

func LoadFiles(paths []string) ([]model.FileConfig, error) {
	files := make([]model.FileConfig, 0, len(paths))
	for _, path := range paths {
		config, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, model.FileConfig{
			Path:   path,
			Config: config,
		})
	}
	return files, nil
}

func LoadFile(path string) (model.SyncConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.SyncConfig{}, fmt.Errorf("read %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config model.SyncConfig
	if err := decoder.Decode(&config); err != nil {
		return model.SyncConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return model.SyncConfig{}, fmt.Errorf("parse %s after the first YAML document: %w", path, err)
		}
		return model.SyncConfig{}, fmt.Errorf("parse %s: only one YAML document is allowed per file; move additional documents to separate files", path)
	}
	config.Options = config.Options.WithDefaults()
	return config, nil
}
