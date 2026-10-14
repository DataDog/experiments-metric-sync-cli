// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// PropertyFilters is an OR of groups. Each group is an AND of filters.
// YAML also accepts one filter or a flat list, which becomes one AND group.
type PropertyFilters [][]PropertyFilter

// filterShape captures the node without decoding its values.
type filterShape struct {
	node *yaml.Node
}

func (s *filterShape) UnmarshalYAML(node *yaml.Node) error {
	s.node = node
	return nil
}

// Use the decoder callback to retain strict field checks, scalar text, and
// alias handling from the caller's YAML decoder.
func (f *PropertyFilters) UnmarshalYAML(unmarshal func(any) error) error {
	type filterInput struct {
		Property  *PropertyRef `yaml:"property"`
		Measure   *MeasureRef  `yaml:"measure"`
		Operation *string      `yaml:"operation"`
		Operator  *string      `yaml:"operator"`
		Values    []string     `yaml:"values"`
	}
	// Inspect only the shape. Decode values through the caller's callback so
	// strict field checks, scalar text, and alias checks remain in effect.
	var shape filterShape
	if err := unmarshal(&shape); err != nil {
		return err
	}
	var groups [][]filterInput
	var err error
	switch shape.node.Kind {
	case yaml.MappingNode:
		var single filterInput
		err = unmarshal(&single)
		groups = [][]filterInput{{single}}
	case yaml.SequenceNode:
		flat := false
		if len(shape.node.Content) > 0 {
			first := shape.node.Content[0]
			if first.Kind == yaml.AliasNode {
				first = first.Alias
			}
			flat = first.Kind == yaml.MappingNode
		}
		if flat {
			var filters []filterInput
			err = unmarshal(&filters)
			groups = [][]filterInput{filters}
		} else {
			err = unmarshal(&groups)
		}
	default:
		return fmt.Errorf("property_filters: expected one filter, a flat AND list, or nested AND/OR groups")
	}
	if err != nil {
		return fmt.Errorf("property_filters: %w; expected one filter, a flat AND list, or nested AND/OR groups", err)
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
