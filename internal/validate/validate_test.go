// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestValidateThresholdMetricValid(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation:                   "threshold",
				Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				ThresholdAggregationType:    "count",
				ThresholdComparisonOperator: "gt",
				ThresholdBreachValue:        floatPtr(10),
				ThresholdTimeframeValue:     floatPtr(7),
				ThresholdTimeframeDimension: "days",
			},
		}),
	}})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestValidateThresholdMetricWithoutOptionalTimeframe(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation:                   "threshold",
				Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				ThresholdAggregationType:    "count",
				ThresholdComparisonOperator: "gt",
				ThresholdBreachValue:        floatPtr(10),
			},
		}),
	}})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestValidateThresholdMetricMissingFields(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation: "threshold",
				Measure:   model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
			},
		}),
	}})
	want := []string{
		"threshold_aggregation_type",
		"threshold_comparison_operator",
		"threshold_breach_value",
	}
	if len(issues) != len(want) {
		t.Fatalf("expected %d issues, got %d: %v", len(want), len(issues), issues)
	}
	for i, w := range want {
		if !strings.Contains(issues[i].Path, w) {
			t.Errorf("issue %d path = %q, want it to mention %q", i, issues[i].Path, w)
		}
	}
}

func TestValidateThresholdMetricRequiresPairedTimeframeFields(t *testing.T) {
	tests := []struct {
		name      string
		value     *float64
		dimension string
		wantPath  string
	}{
		{name: "missing dimension", value: floatPtr(7), wantPath: "threshold_timeframe_dimension"},
		{name: "missing value", dimension: "days", wantPath: "threshold_timeframe_value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issues := ValidateFiles([]model.FileConfig{{
				Path: "threshold.yaml",
				Config: thresholdConfig(model.Metric{
					SyncID:     "metric",
					Name:       "metric",
					MetricType: "simple",
					SimpleMetricAggregation: &model.SimpleMetricAggregation{
						Operation:                   "threshold",
						Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
						ThresholdAggregationType:    "count",
						ThresholdComparisonOperator: "gt",
						ThresholdBreachValue:        floatPtr(10),
						ThresholdTimeframeValue:     test.value,
						ThresholdTimeframeDimension: test.dimension,
					},
				}),
			}})
			if len(issues) != 1 {
				t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
			}
			if !strings.Contains(issues[0].Path, test.wantPath) {
				t.Fatalf("issue path = %q, want it to mention %q", issues[0].Path, test.wantPath)
			}
		})
	}
}

func TestValidateThresholdMetricBadEnums(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation:                   "threshold",
				Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				ThresholdAggregationType:    "avg",
				ThresholdComparisonOperator: "between",
				ThresholdBreachValue:        floatPtr(10),
				ThresholdTimeframeValue:     floatPtr(7),
				ThresholdTimeframeDimension: "days",
			},
		}),
	}})
	if len(issues) != 2 {
		t.Fatalf("expected 2 enum issues, got %d: %v", len(issues), issues)
	}
	if !strings.Contains(issues[0].Message, "count or sum") {
		t.Errorf("issue 0 = %q, want count or sum", issues[0].Message)
	}
	if !strings.Contains(issues[1].Message, "gt, gte, lt, lte, eq, or neq") {
		t.Errorf("issue 1 = %q, want operator enum message", issues[1].Message)
	}
}

func TestValidateThresholdRatioComponents(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "ratio",
			RatioMetricAggregation: &model.RatioMetricAggregation{
				NumeratorAggregation: model.SimpleMetricAggregation{
					Operation:                   "threshold",
					Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
					ThresholdAggregationType:    "sum",
					ThresholdComparisonOperator: "lte",
					ThresholdBreachValue:        floatPtr(5),
					ThresholdTimeframeValue:     floatPtr(1),
					ThresholdTimeframeDimension: "weeks",
				},
				DenominatorAggregation: model.SimpleMetricAggregation{
					Operation: "count",
					Measure:   model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				},
			},
		}),
	}})
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %v", issues)
	}
}

func TestValidateThresholdRatioComponentMissingFields(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "ratio",
			RatioMetricAggregation: &model.RatioMetricAggregation{
				NumeratorAggregation: model.SimpleMetricAggregation{
					Operation: "threshold",
					Measure:   model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				},
				DenominatorAggregation: model.SimpleMetricAggregation{
					Operation: "count",
					Measure:   model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				},
			},
		}),
	}})
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues for numerator threshold, got %d: %v", len(issues), issues)
	}
	for _, issue := range issues {
		if !strings.Contains(issue.Path, "numerator_aggregation") {
			t.Errorf("issue %q should be on the numerator path", issue.Path)
		}
	}
}

func TestValidateThresholdMetricRejectsStandardTimeframeAndWinsorization(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation:                   "threshold",
				Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				ThresholdAggregationType:    "count",
				ThresholdComparisonOperator: "gt",
				ThresholdBreachValue:        floatPtr(10),
				TimeframeStartValue:         intPtr(1),
				TimeframeEndValue:           intPtr(7),
				TimeframeUnit:               "days",
				WinsorLowerPercentile:       floatPtr(0.01),
				WinsorUpperPercentile:       floatPtr(0.99),
				WinsorLowerFixedValue:       floatPtr(0),
				WinsorUpperFixedValue:       floatPtr(100),
			},
		}),
	}})
	wantPaths := []string{
		"timeframe_start_value",
		"timeframe_end_value",
		"timeframe_unit",
		"winsor_lower_percentile",
		"winsor_upper_percentile",
		"winsor_lower_fixed_value",
		"winsor_upper_fixed_value",
	}
	if len(issues) != len(wantPaths) {
		t.Fatalf("expected %d issues, got %d: %v", len(wantPaths), len(issues), issues)
	}
	for i, wantPath := range wantPaths {
		if !strings.Contains(issues[i].Path, wantPath) {
			t.Errorf("issue %d path = %q, want it to mention %q", i, issues[i].Path, wantPath)
		}
	}
}

func TestValidateNonThresholdMetricRejectsThresholdFields(t *testing.T) {
	issues := ValidateFiles([]model.FileConfig{{
		Path: "threshold.yaml",
		Config: thresholdConfig(model.Metric{
			SyncID:     "metric",
			Name:       "metric",
			MetricType: "simple",
			SimpleMetricAggregation: &model.SimpleMetricAggregation{
				Operation:                   "sum",
				Measure:                     model.MeasureRef{MeasureSyncID: "m", WarehouseMetricSourceSyncID: "src"},
				ThresholdAggregationType:    "count",
				ThresholdComparisonOperator: "gt",
				ThresholdBreachValue:        floatPtr(10),
				ThresholdTimeframeValue:     floatPtr(7),
				ThresholdTimeframeDimension: "days",
			},
		}),
	}})
	if len(issues) != 5 {
		t.Fatalf("expected 5 issues, got %d: %v", len(issues), issues)
	}
	for _, issue := range issues {
		if !strings.Contains(issue.Message, "only be set when operation is threshold") {
			t.Errorf("issue %q should reject threshold fields on a non-threshold operation", issue)
		}
	}
}

func thresholdConfig(metric model.Metric) model.SyncConfig {
	return model.SyncConfig{
		SchemaVersion: 1,
		SyncTag:       "checkout",
		WarehouseMetricSources: []model.WarehouseMetricSource{{
			SyncID:          "src",
			Name:            "src",
			SQL:             "select 1",
			TimestampColumn: "ts",
			Measures:        []model.Measure{{SyncID: "m", Name: "m", ColumnName: "v", ColumnType: "FLOAT"}},
		}},
		Metrics: []model.Metric{metric},
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

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
		{name: "multibyte name", columns: []string{strings.Repeat("時", 100)}},
		{name: "multibyte name at limit", columns: []string{strings.Repeat("時", 255)}},
		{name: "too many", columns: []string{"a", "b", "c", "d", "e"}, path: "additional_timestamp_columns", message: "at most 4"},
		{name: "blank", columns: []string{" \t"}, path: "additional_timestamp_columns[0]", message: "required"},
		{name: "empty name", columns: []string{""}, path: "additional_timestamp_columns[0]", message: "required"},
		{name: "long name", columns: []string{strings.Repeat("a", 256)}, path: "additional_timestamp_columns[0]", message: "at most 255"},
		{name: "long multibyte name", columns: []string{strings.Repeat("時", 256)}, path: "additional_timestamp_columns[0]", message: "at most 255"},
		{name: "duplicate", columns: []string{"first", "first"}, path: "additional_timestamp_columns[1]", message: "unique"},
		{name: "trimmed duplicate", columns: []string{" first ", "first"}, path: "additional_timestamp_columns[1]", message: "unique"},
		{name: "primary", columns: []string{"ts"}, path: "additional_timestamp_columns[0]", message: "cannot include timestamp_column"},
		{name: "trimmed primary", columns: []string{" ts "}, path: "additional_timestamp_columns[0]", message: "cannot include timestamp_column"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := thresholdConfig(model.Metric{})
			config.Metrics = nil
			config.WarehouseMetricSources[0].AdditionalTimestampColumns = tc.columns
			issues := ValidateFiles([]model.FileConfig{{Path: "metric-sync.yaml", Config: config}})
			assertValidationIssue(t, issues, "warehouse_metric_sources[0].", tc.path, tc.message)
		})
	}
}

func TestWinsorizationStrategyAndBounds(t *testing.T) {
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
				issues := ValidateFiles([]model.FileConfig{{Path: "metric-sync.yaml", Config: thresholdConfig(metric)}})
				assertValidationIssue(t, issues, prefix, tc.path, tc.message)
			})
		}
	}
}

func assertValidationIssue(t *testing.T, issues []Issue, prefix, path, message string) {
	t.Helper()
	if path == "" {
		if len(issues) != 0 {
			t.Fatalf("unexpected issues: %v", issues)
		}
		return
	}
	if len(issues) != 1 || issues[0].File != "metric-sync.yaml" || issues[0].Path != prefix+path || !strings.Contains(issues[0].Message, message) {
		t.Fatalf("expected metric-sync.yaml: %s%s: %s, got %v", prefix, path, message, issues)
	}
}
