// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestAgingSubjectFilter(t *testing.T) {
	enabled, disabled := true, false
	days := func(v int) *int { return &v }
	for _, tc := range []struct {
		name          string
		enable        *bool
		days          *int
		end           *float64
		unit          string
		operation     string
		path, message string
	}{
		{name: "omitted"},
		{name: "disabled", enable: &disabled},
		{name: "window cutoff", enable: &enabled, end: floatPtr(7), unit: "days"},
		{name: "fractional window cutoff", enable: &enabled, end: floatPtr(1.5), unit: "weeks"},
		{name: "explicit days", enable: &enabled, days: days(14)},
		{name: "zero days", enable: &enabled, days: days(0)},
		{name: "negative days", enable: &enabled, days: days(-1), path: "aging_threshold_days", message: "0 or greater"},
		{name: "days without enable", days: days(14), path: "aging_threshold_days", message: "no effect unless enable_aging_subject_filter"},
		{name: "days with disabled", enable: &disabled, days: days(14), path: "aging_threshold_days", message: "no effect unless enable_aging_subject_filter"},
		{name: "missing end", enable: &enabled, unit: "days", path: "timeframe_end_value", message: "required when enable_aging_subject_filter"},
		{name: "missing unit", enable: &enabled, end: floatPtr(7), path: "timeframe_unit", message: "required when enable_aging_subject_filter"},
		{name: "unsupported unit", enable: &enabled, end: floatPtr(7), unit: "months", path: "timeframe_unit", message: "when enable_aging_subject_filter"},
		{name: "threshold with days", enable: &enabled, days: days(7), operation: "threshold"},
		{name: "threshold without days", enable: &enabled, operation: "threshold", path: "aging_threshold_days", message: "operation is threshold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := model.SimpleMetricAggregation{
				Operation: "sum", Measure: model.MeasureRef{WarehouseMetricSourceSyncID: "src", MeasureSyncID: "m"},
				TimeframeEndValue: tc.end, TimeframeUnit: tc.unit,
				EnableAgingSubjectFilter: tc.enable, AgingThresholdDays: tc.days,
			}
			if tc.operation == "threshold" {
				agg.Operation = "threshold"
				agg.ThresholdAggregationType = "sum"
				agg.ThresholdComparisonOperator = "gt"
				agg.ThresholdBreachValue = floatPtr(100)
			}
			metric := model.Metric{SyncID: "metric", Name: "Metric", MetricType: "simple", SimpleMetricAggregation: &agg}
			issues := ValidateFiles([]model.FileConfig{{Path: "metric-sync.yaml", Config: thresholdConfig(metric)}})
			assertValidationIssue(t, issues, "metrics[0].simple_metric_aggregation.", tc.path, tc.message)
		})
	}
}

func TestRatioAgingSubjectFilter(t *testing.T) {
	enabled := true
	days := func(v int) *int { return &v }
	aging := func(end float64, unit string, cutoff *int) model.SimpleMetricAggregation {
		return model.SimpleMetricAggregation{
			Operation: "count", Measure: model.MeasureRef{WarehouseMetricSourceSyncID: "src", Kind: "each_record"},
			TimeframeEndValue: &end, TimeframeUnit: unit, EnableAgingSubjectFilter: &enabled, AgingThresholdDays: cutoff,
		}
	}
	plain := model.SimpleMetricAggregation{Operation: "count", Measure: model.MeasureRef{WarehouseMetricSourceSyncID: "src", Kind: "each_record"}}
	for _, tc := range []struct {
		name          string
		num, den      model.SimpleMetricAggregation
		path, message string
	}{
		{name: "both disabled", num: plain, den: plain},
		{name: "same window", num: aging(7, "days", nil), den: aging(7, "days", nil)},
		{name: "same explicit days", num: aging(7, "days", days(14)), den: aging(1, "weeks", days(14))},
		{name: "explicit days match window", num: aging(7, "days", days(7)), den: aging(1, "weeks", nil)},
		{name: "only numerator", num: aging(7, "days", nil), den: plain, path: "denominator_aggregation.enable_aging_subject_filter", message: "must match numerator"},
		{name: "only denominator", num: plain, den: aging(7, "days", nil), path: "denominator_aggregation.enable_aging_subject_filter", message: "must match numerator"},
		{name: "different unit", num: aging(1, "days", nil), den: aging(1, "calendar_days", nil), path: "denominator_aggregation.timeframe_unit", message: "must match numerator"},
		{name: "different end", num: aging(7, "days", nil), den: aging(14, "days", nil), path: "denominator_aggregation.timeframe_end_value", message: "must match numerator"},
		{name: "different explicit days", num: aging(7, "days", days(7)), den: aging(7, "days", days(14)), path: "denominator_aggregation", message: "same aging cutoff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metric := model.Metric{SyncID: "metric", Name: "Metric", MetricType: "ratio", RatioMetricAggregation: &model.RatioMetricAggregation{NumeratorAggregation: tc.num, DenominatorAggregation: tc.den}}
			issues := ValidateFiles([]model.FileConfig{{Path: "metric-sync.yaml", Config: thresholdConfig(metric)}})
			assertValidationIssue(t, issues, "metrics[0].ratio_metric_aggregation.", tc.path, tc.message)
		})
	}
}
