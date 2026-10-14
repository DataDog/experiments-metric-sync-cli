// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

import "fmt"

// PropertyFilters is an OR of groups. Each group is an AND of filters.
type PropertyFilters [][]PropertyFilter

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
	var groups [][]filterInput
	if err := unmarshal(&groups); err != nil {
		return fmt.Errorf("property_filters: %w; use nested lists with AND conditions in each group and OR conditions between groups", err)
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
