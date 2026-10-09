// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestAdditionalTimestampColumns(t *testing.T) {
	for _, tc := range []struct {
		name    string
		columns []string
		path    string
		message string
	}{
		{name: "omitted"},
		{name: "empty", columns: []string{}},
		{name: "four", columns: []string{"first", "second", "third", strings.Repeat("a", 255)}},
		{name: "too many", columns: []string{"a", "b", "c", "d", "e"}, path: "additional_timestamp_columns", message: "at most 4"},
		{name: "blank", columns: []string{" \t"}, path: "additional_timestamp_columns[0]", message: "required"},
		{name: "empty name", columns: []string{""}, path: "additional_timestamp_columns[0]", message: "required"},
		{name: "long name", columns: []string{strings.Repeat("a", 256)}, path: "additional_timestamp_columns[0]", message: "at most 255"},
		{name: "duplicate", columns: []string{"first", "first"}, path: "additional_timestamp_columns[1]", message: "unique"},
		{name: "trimmed duplicate", columns: []string{" first ", "first"}, path: "additional_timestamp_columns[1]", message: "unique"},
		{name: "primary", columns: []string{"ts"}, path: "additional_timestamp_columns[0]", message: "cannot include timestamp_column"},
		{name: "trimmed primary", columns: []string{" ts "}, path: "additional_timestamp_columns[0]", message: "cannot include timestamp_column"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := thresholdConfig(model.Metric{})
			config.Metrics = nil
			config.WarehouseMetricSources[0].AdditionalTimestampColumns = tc.columns
			issues := ValidateFiles([]model.FileConfig{{Path: "settings.yaml", Config: config}})
			assertSettingsIssue(t, issues, "warehouse_metric_sources[0].", tc.path, tc.message)
		})
	}
}

func TestWinsorizationSettings(t *testing.T) {
	str := func(value string) *string { return &value }
	for _, location := range []string{"simple", "numerator", "denominator"} {
		for _, tc := range []struct {
			name          string
			strategy      *string
			operation     string
			lower, upper  *float64
			path, message string
		}{
			{name: "omitted"},
			{name: "all subjects", strategy: str("all_assigned_subjects")},
			{name: "nonzero", strategy: str("nonzero"), upper: floatPtr(0.99)},
			{name: "boundary percentiles", lower: floatPtr(0), upper: floatPtr(1)},
			{name: "invalid", strategy: str("other"), path: "winsorization_strategy", message: "all_assigned_subjects or nonzero"},
			{name: "empty", strategy: str(""), path: "winsorization_strategy", message: "all_assigned_subjects or nonzero"},
			{name: "whitespace", strategy: str(" nonzero "), path: "winsorization_strategy", message: "all_assigned_subjects or nonzero"},
			{name: "threshold omitted", operation: "threshold"},
			{name: "threshold default", operation: "threshold", strategy: str("all_assigned_subjects")},
			{name: "threshold nonzero", operation: "threshold", strategy: str("nonzero"), path: "winsorization_strategy", message: "when operation is threshold"},
			{name: "lower below zero", lower: floatPtr(-0.01), path: "winsor_lower_percentile", message: "between 0 and 1"},
			{name: "lower above one", lower: floatPtr(1.01), path: "winsor_lower_percentile", message: "between 0 and 1"},
			{name: "upper below zero", upper: floatPtr(-0.01), path: "winsor_upper_percentile", message: "between 0 and 1"},
			{name: "upper above one", upper: floatPtr(1.01), path: "winsor_upper_percentile", message: "between 0 and 1"},
			{name: "equal bounds", lower: floatPtr(0.5), upper: floatPtr(0.5), path: "winsor_lower_percentile", message: "less than winsor_upper_percentile"},
			{name: "reversed bounds", lower: floatPtr(0.9), upper: floatPtr(0.1), path: "winsor_lower_percentile", message: "less than winsor_upper_percentile"},
		} {
			t.Run(location+"/"+tc.name, func(t *testing.T) {
				agg := model.SimpleMetricAggregation{
					Operation: "sum", Measure: model.MeasureRef{WarehouseMetricSourceSyncID: "src", MeasureSyncID: "m"},
					WinsorizationStrategy: tc.strategy, WinsorLowerPercentile: tc.lower, WinsorUpperPercentile: tc.upper,
				}
				if tc.operation == "threshold" {
					agg.Operation = "threshold"
					agg.ThresholdAggregationType = "sum"
					agg.ThresholdComparisonOperator = "gt"
					agg.ThresholdBreachValue = floatPtr(100)
				}
				metric := model.Metric{SyncID: "metric", Name: "Metric", MetricType: "simple", SimpleMetricAggregation: &agg}
				prefix := "metrics[0].simple_metric_aggregation."
				if location != "simple" {
					metric.MetricType = "ratio"
					metric.SimpleMetricAggregation = nil
					other := model.SimpleMetricAggregation{Operation: "count", Measure: agg.Measure}
					metric.RatioMetricAggregation = &model.RatioMetricAggregation{NumeratorAggregation: other, DenominatorAggregation: other}
					if location == "numerator" {
						metric.RatioMetricAggregation.NumeratorAggregation = agg
					} else {
						metric.RatioMetricAggregation.DenominatorAggregation = agg
					}
					prefix = "metrics[0].ratio_metric_aggregation." + location + "_aggregation."
				}
				issues := ValidateFiles([]model.FileConfig{{Path: "settings.yaml", Config: thresholdConfig(metric)}})
				assertSettingsIssue(t, issues, prefix, tc.path, tc.message)
			})
		}
	}
}

func assertSettingsIssue(t *testing.T, issues []Issue, prefix, path, message string) {
	t.Helper()
	if path == "" {
		if len(issues) != 0 {
			t.Fatalf("unexpected issues: %v", issues)
		}
		return
	}
	if len(issues) != 1 || issues[0].File != "settings.yaml" || issues[0].Path != prefix+path || !strings.Contains(issues[0].Message, message) {
		t.Fatalf("expected settings.yaml: %s%s: %s, got %v", prefix, path, message, issues)
	}
}
