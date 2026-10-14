// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// PropertyFilters is an OR of groups. Each group is an AND of filters.
type PropertyFilters [][]PropertyFilter

func (f *PropertyFilters) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: property_filters must be a list of filter groups", node.Line)
	}
	for _, group := range node.Content {
		if group.Kind != yaml.SequenceNode {
			return fmt.Errorf("line %d: property_filters requires nested lists; place AND conditions in one group and OR conditions in separate groups", group.Line)
		}
	}
	// Keep strict field checks inside the custom decoder, including references.
	type filterInput struct {
		Property  *PropertyRef `yaml:"property"`
		Measure   *MeasureRef  `yaml:"measure"`
		Operation *string      `yaml:"operation"`
		Operator  *string      `yaml:"operator"`
		Values    []string     `yaml:"values"`
	}
	var resolved any
	if err := node.Decode(&resolved); err != nil {
		return fmt.Errorf("property_filters: %w", err)
	}
	data, err := yaml.Marshal(resolved)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var groups [][]filterInput
	if err := decoder.Decode(&groups); err != nil {
		return fmt.Errorf("property_filters: %w", err)
	}
	result := make(PropertyFilters, len(groups))
	for i, group := range groups {
		result[i] = make([]PropertyFilter, len(group))
		for j, filter := range group {
			if filter.Operation != nil && filter.Operator != nil {
				return fmt.Errorf("property_filters[%d][%d]: use operation or its legacy alias operator, not both", i, j)
			}
			operation := filter.Operation
			if operation == nil {
				operation = filter.Operator
			}
			result[i][j] = PropertyFilter{Property: filter.Property, Measure: filter.Measure, Values: filter.Values}
			if operation != nil {
				result[i][j].Operation = *operation
			}
		}
	}
	*f = result
	return nil
}
