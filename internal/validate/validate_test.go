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
