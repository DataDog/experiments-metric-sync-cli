// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestValidateLocalMeasureFilterOperands(t *testing.T) {
	for _, location := range []string{"simple", "numerator", "denominator"} {
		for _, tc := range []struct {
			name, columnType, operation string
			values                      []string
			path, message               string
		}{
			{name: "signed integer", columnType: "INTEGER", operation: ">", values: []string{"+42"}},
			{name: "integer limits", columnType: "INTEGER", operation: "BETWEEN", values: []string{"-9223372036854775808", "9223372036854775807"}},
			{name: "equal integer bounds", columnType: "INTEGER", operation: "BETWEEN", values: []string{"2", "2"}},
			{name: "float decimal", columnType: "FLOAT", operation: "<", values: []string{"-.125"}},
			{name: "float exponent", columnType: "FLOAT", operation: ">=", values: []string{"1.25e+3"}},
			{name: "float integer literal", columnType: "FLOAT", operation: "=", values: []string{"001"}},
			{name: "float bounds", columnType: "FLOAT", operation: "BETWEEN", values: []string{"-2.5", "2.4"}},
			{name: "equal float bounds", columnType: "FLOAT", operation: "BETWEEN", values: []string{"2.5", "2.50"}},
			{name: "null", columnType: "FLOAT", operation: "IS_NULL"},
			{name: "not null", columnType: "INTEGER", operation: "IS_NOT_NULL", values: []string{}},
			{name: "nonnumeric measure", columnType: "STRING", operation: "=", values: []string{"10"}, path: "measure", message: "INTEGER or FLOAT"},
			{name: "nonnumeric null measure", columnType: "STRING", operation: "IS_NULL", path: "measure", message: "INTEGER or FLOAT"},
			{name: "integer decimal", columnType: "INTEGER", operation: "=", values: []string{"1.0"}, path: "values", message: "must be an integer"},
			{name: "integer exponent", columnType: "INTEGER", operation: "=", values: []string{"1e2"}, path: "values", message: "must be an integer"},
			{name: "integer overflow", columnType: "INTEGER", operation: "=", values: []string{"9223372036854775808"}, path: "values", message: "64-bit integer"},
			{name: "integer underflow", columnType: "INTEGER", operation: "=", values: []string{"-9223372036854775809"}, path: "values", message: "64-bit integer"},
			{name: "empty value", columnType: "INTEGER", operation: "=", values: []string{""}, path: "values", message: "finite base-10"},
			{name: "text", columnType: "FLOAT", operation: "=", values: []string{"abc"}, path: "values", message: "finite base-10"},
			{name: "NaN", columnType: "FLOAT", operation: "=", values: []string{"NaN"}, path: "values", message: "finite base-10"},
			{name: "infinity", columnType: "FLOAT", operation: "=", values: []string{"Inf"}, path: "values", message: "finite base-10"},
			{name: "hexadecimal", columnType: "FLOAT", operation: "=", values: []string{"0x1p2"}, path: "values", message: "finite base-10"},
			{name: "trailing expression", columnType: "FLOAT", operation: "=", values: []string{"1 OR 1=1"}, path: "values", message: "finite base-10"},
			{name: "whitespace", columnType: "FLOAT", operation: "=", values: []string{" 1 "}, path: "values", message: "finite base-10"},
			{name: "float overflow", columnType: "FLOAT", operation: "=", values: []string{"1e999"}, path: "values", message: "finite base-10"},
			{name: "descending integer bounds retain precision", columnType: "INTEGER", operation: "BETWEEN", values: []string{"9223372036854775807", "9223372036854775806"}, path: "values", message: "lower bound"},
			{name: "descending float bounds", columnType: "FLOAT", operation: "BETWEEN", values: []string{"2.5", "2.4"}, path: "values", message: "lower bound"},
			{name: "missing BETWEEN operand", columnType: "FLOAT", operation: "BETWEEN", values: []string{"1"}, path: "values", message: "exactly 2 values"},
			{name: "missing comparison operand", columnType: "FLOAT", operation: "=", path: "values", message: "exactly 1 values"},
		} {
			t.Run(location+"/"+tc.name, func(t *testing.T) {
				config := measureFilterConfig(model.MeasureRef{WarehouseMetricSourceSyncID: "src", MeasureSyncID: "m"}, tc.columnType, tc.operation, tc.values)
				metric := &config.Metrics[0]
				prefix := "metrics[0].simple_metric_aggregation."
				if location != "simple" {
					agg := *metric.SimpleMetricAggregation
					other := agg
					other.PropertyFilters = nil
					metric.MetricType = "ratio"
					metric.SimpleMetricAggregation = nil
					metric.RatioMetricAggregation = &model.RatioMetricAggregation{NumeratorAggregation: other, DenominatorAggregation: other}
					if location == "numerator" {
						metric.RatioMetricAggregation.NumeratorAggregation = agg
					} else {
						metric.RatioMetricAggregation.DenominatorAggregation = agg
					}
					prefix = "metrics[0].ratio_metric_aggregation." + location + "_aggregation."
				}
				issues := ValidateFiles([]model.FileConfig{{Path: "metric-sync.yaml", Config: config}})
				assertValidationIssue(t, issues, prefix+"property_filters[0][0].", tc.path, tc.message)
			})
		}
	}
}

func TestValidateMeasureFilterTypeResolution(t *testing.T) {
	for _, tc := range []struct {
		name, tag string
		local     bool
		wantError bool
	}{
		{name: "implicit current source", local: true, wantError: true},
		{name: "explicit current source", tag: "checkout", local: true, wantError: true},
		{name: "same ID in another tag", tag: "other", local: true},
		{name: "external source", tag: "other"},
		{name: "absent source with explicit current tag", tag: "checkout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := model.MeasureRef{WarehouseMetricSourceSyncID: "src", WarehouseMetricSourceSyncTag: tc.tag, MeasureSyncID: "m"}
			config := measureFilterConfig(ref, "INTEGER", "=", []string{"1.5"})
			sources := model.SyncConfig{SchemaVersion: 1, SyncTag: config.SyncTag}
			if tc.local {
				sources.WarehouseMetricSources = config.WarehouseMetricSources
			}
			config.WarehouseMetricSources = nil
			// Local sources can be declared in another file. External types must
			// not use a source from this payload with the same ID and another tag.
			files := []model.FileConfig{{Path: "metric-sync.yaml", Config: config}, {Path: "sources.yaml", Config: sources}}
			issues := ValidateFiles(files)
			path := ""
			if tc.wantError {
				path = "values"
			}
			assertValidationIssue(t, issues, "metrics[0].simple_metric_aggregation.property_filters[0][0].", path, "must be an integer")
		})
	}
}

func measureFilterConfig(ref model.MeasureRef, columnType, operation string, values []string) model.SyncConfig {
	aggregationRef := ref
	aggregationRef.MeasureSyncID = ""
	aggregationRef.Kind = "each_record"
	config := measureRefConfig(aggregationRef, "count")
	config.WarehouseMetricSources[0].Measures[0].ColumnType = columnType
	config.Metrics[0].SimpleMetricAggregation.PropertyFilters = model.PropertyFilters{{{
		Measure: &ref, Operation: operation, Values: values,
	}}}
	return config
}
