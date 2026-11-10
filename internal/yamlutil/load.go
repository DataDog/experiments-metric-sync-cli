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
		file, err := loadFileConfig(path)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func LoadFile(path string) (model.SyncConfig, error) {
	file, err := loadFileConfig(path)
	return file.Config, err
}

func loadFileConfig(path string) (model.FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.FileConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	config, err := decodeConfig(path, data)
	if err != nil {
		return model.FileConfig{}, err
	}
	return model.FileConfig{
		Path:                  path,
		Config:                config,
		MetricCollectionsNull: hasNullTopLevelKey(data, "metric_collections"),
	}, nil
}

// hasNullTopLevelKey reports whether the document sets key to an explicit null.
// The typed decode turns null, ~, and an empty value into the same nil slice as an
// omitted key, so only the node tree can tell them apart.
func hasNullTopLevelKey(data []byte, key string) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			value := root.Content[i+1]
			return value.Kind == yaml.ScalarNode && value.ShortTag() == "!!null"
		}
	}
	return false
}

func decodeConfig(path string, data []byte) (model.SyncConfig, error) {
	if err := rejectNullListItems(path, data); err != nil {
		return model.SyncConfig{}, err
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

// nullCheckedLists are the top-level lists whose items decode into structs.
var nullCheckedLists = []string{"warehouse_metric_sources", "metrics", "metric_collections"}

// rejectNullListItems fails on list items that are null, such as an entry whose
// fields are all commented out. The typed decode silently drops those items, and
// because the YAML is the desired state, a dropped collection or member would be
// deleted on the server.
func rejectNullListItems(path string, data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		// The typed decode reports syntax errors with better context.
		return nil
	}
	root := doc.Content[0]
	for _, key := range nullCheckedLists {
		items := mappingValue(root, key)
		if err := rejectNullItems(path, key, items); err != nil {
			return err
		}
		if key != "metric_collections" || items == nil || items.Kind != yaml.SequenceNode {
			continue
		}
		for i, collection := range items.Content {
			field := fmt.Sprintf("metric_collections[%d].metrics", i)
			if err := rejectNullItems(path, field, mappingValue(collection, "metrics")); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectNullItems(path, field string, items *yaml.Node) error {
	if items == nil || items.Kind != yaml.SequenceNode {
		return nil
	}
	for i, item := range items.Content {
		if item.Kind == yaml.ScalarNode && item.ShortTag() == "!!null" {
			return fmt.Errorf("parse %s: line %d: %s[%d] is empty; remove the list item or fill in its fields", path, item.Line, field, i)
		}
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
