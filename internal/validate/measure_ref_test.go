// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

func TestValidateMeasureRefSelectors(t *testing.T) {
	tests := []struct {
		name                  string
		ref                   model.MeasureRef
		wantPath, wantMessage string
	}{
		{name: "named", ref: model.MeasureRef{MeasureSyncID: "m"}},
		{name: "subject", ref: model.MeasureRef{SubjectTypeName: "Customer"}},
		{name: "each record", ref: model.MeasureRef{Kind: "each_record"}},
		{name: "missing selector", wantMessage: "exactly one"},
		{name: "named and subject", ref: model.MeasureRef{MeasureSyncID: "m", SubjectTypeName: "Customer"}, wantMessage: "exactly one"},
		{name: "named and kind", ref: model.MeasureRef{MeasureSyncID: "m", Kind: "each_record"}, wantMessage: "exactly one"},
		{name: "subject and kind", ref: model.MeasureRef{SubjectTypeName: "Customer", Kind: "each_record"}, wantMessage: "exactly one"},
		{name: "all selectors", ref: model.MeasureRef{MeasureSyncID: "m", SubjectTypeName: "Customer", Kind: "each_record"}, wantMessage: "exactly one"},
		{name: "unknown kind", ref: model.MeasureRef{Kind: "row"}, wantPath: ".kind", wantMessage: "must be each_record"},
		{name: "blank measure", ref: model.MeasureRef{MeasureSyncID: " "}, wantPath: ".measure_sync_id", wantMessage: "must not be blank"},
		{name: "blank subject", ref: model.MeasureRef{SubjectTypeName: " "}, wantPath: ".subject_type_name", wantMessage: "must not be blank"},
		{name: "raw ID", ref: model.MeasureRef{WarehouseMetricMeasureID: "123"}, wantPath: ".warehouse_metric_measure_id", wantMessage: "not supported by the sync API"},
		{name: "raw ID with selector", ref: model.MeasureRef{WarehouseMetricMeasureID: "123", Kind: "each_record"}, wantPath: ".warehouse_metric_measure_id", wantMessage: "not supported by the sync API"},
	}
	for _, tag := range []string{"", "checkout", "another-tag"} {
		for _, test := range tests {
			t.Run(tag+"/"+test.name, func(t *testing.T) {
				ref := test.ref
				ref.WarehouseMetricSourceSyncID = "src"
				ref.WarehouseMetricSourceSyncTag = tag
				config := measureRefConfig(ref, "firstValue")
				issues := ValidateFiles([]model.FileConfig{{Path: "metrics.yaml", Config: config}})
				checkMeasureIssues(t, issues, "metrics[0].simple_metric_aggregation.measure"+test.wantPath, test.wantMessage)
			})
		}
	}
}

func TestValidateMeasureRefResolution(t *testing.T) {
	tests := []struct {
		name, source, tag, measure, subject, kind, wantField string
	}{
		{name: "missing source", kind: "each_record", wantField: "warehouse_metric_source_sync_id"},
		{name: "blank source", source: " ", kind: "each_record", wantField: "warehouse_metric_source_sync_id"},
		{name: "external missing source", tag: "other", kind: "each_record", wantField: "warehouse_metric_source_sync_id"},
		{name: "unknown source", source: "missing", kind: "each_record", wantField: "warehouse_metric_source_sync_id"},
		{name: "unknown measure", source: "src", measure: "missing", wantField: "measure_sync_id"},
		{name: "unknown subject", source: "src", subject: "missing", wantField: "subject_type_name"},
		{name: "case sensitive subject", source: "src", subject: "customer", wantField: "subject_type_name"},
		{name: "same tag local subject", source: "src", tag: "checkout", subject: "Customer"},
		{name: "same tag unknown subject", source: "src", tag: "checkout", subject: "missing", wantField: "subject_type_name"},
		{name: "same tag unknown measure", source: "src", tag: "checkout", measure: "missing", wantField: "measure_sync_id"},
		{name: "external subject", source: "src", tag: "other", subject: "missing"},
		{name: "external measure", source: "src", tag: "other", measure: "missing"},
		{name: "external source", source: "missing", tag: "other", kind: "each_record"},
		{name: "explicit same tag server source", source: "missing", tag: "checkout", kind: "each_record"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref := model.MeasureRef{WarehouseMetricSourceSyncID: test.source, WarehouseMetricSourceSyncTag: test.tag, MeasureSyncID: test.measure, SubjectTypeName: test.subject, Kind: test.kind}
			config := measureRefConfig(ref, "firstValue")
			// Sources and metrics can be in separate files, in either order.
			sources := model.SyncConfig{SchemaVersion: 1, SyncTag: config.SyncTag, WarehouseMetricSources: config.WarehouseMetricSources}
			config.WarehouseMetricSources = nil
			files := []model.FileConfig{{Path: "metrics.yaml", Config: config}, {Path: "sources.yaml", Config: sources}}
			for i := 0; i < 2; i++ {
				issues := ValidateFiles(files)
				if test.wantField == "" {
					checkMeasureIssues(t, issues, "", "")
				} else {
					checkMeasureIssues(t, issues, "metrics[0].simple_metric_aggregation.measure."+test.wantField, " ")
				}
				files[0], files[1] = files[1], files[0]
			}
		})
	}
}

func TestValidateMeasureOperationCompatibility(t *testing.T) {
	selectors := []model.MeasureRef{{MeasureSyncID: "m"}, {SubjectTypeName: "Customer"}, {Kind: "each_record"}}
	tests := []struct {
		operation string
		allowed   [3]bool
	}{
		{"count", [3]bool{false, false, true}},
		{"sum", [3]bool{true, false, false}},
		{"average", [3]bool{true, false, false}},
		{"countDistinctValue", [3]bool{true, true, false}},
		{"uniqueSubjects", [3]bool{false, true, false}},
		{"firstValue", [3]bool{true, true, true}},
		{"lastValue", [3]bool{true, true, true}},
		{"threshold", [3]bool{true, true, true}},
	}
	for _, tag := range []string{"", "other"} {
		for _, test := range tests {
			for i, ref := range selectors {
				t.Run(tag+"/"+test.operation+"/"+[]string{"named", "subject", "each_record"}[i], func(t *testing.T) {
					ref.WarehouseMetricSourceSyncID = "src"
					ref.WarehouseMetricSourceSyncTag = tag
					config := measureRefConfig(ref, test.operation)
					if test.operation == "threshold" {
						agg := config.Metrics[0].SimpleMetricAggregation
						agg.ThresholdAggregationType = "count"
						agg.ThresholdComparisonOperator = "gt"
						agg.ThresholdBreachValue = floatPtr(1)
					}
					issues := ValidateFiles([]model.FileConfig{{Path: "metrics.yaml", Config: config}})
					if test.allowed[i] {
						checkMeasureIssues(t, issues, "", "")
					} else {
						checkMeasureIssues(t, issues, "metrics[0].simple_metric_aggregation.operation", test.operation)
					}
				})
			}
		}
	}
}

func TestValidateMeasureRefAggregationLocations(t *testing.T) {
	for _, location := range []string{"simple_metric_aggregation", "ratio_metric_aggregation.numerator_aggregation", "ratio_metric_aggregation.denominator_aggregation", "percentile_metric_aggregation"} {
		for _, scenario := range []string{"valid", "unknown subject", "raw ID", "wrong operation"} {
			if scenario == "wrong operation" && location == "percentile_metric_aggregation" {
				continue // Percentile aggregations have no operation field.
			}
			t.Run(location+"/"+scenario, func(t *testing.T) {
				ref := model.MeasureRef{WarehouseMetricSourceSyncID: "src", SubjectTypeName: "Customer"}
				if scenario == "unknown subject" {
					ref.SubjectTypeName = "missing"
				}
				if scenario == "raw ID" {
					ref.WarehouseMetricMeasureID = "unsupported-id"
				}
				config := measureRefConfig(ref, "countDistinctValue")
				metric := &config.Metrics[0]
				if scenario == "wrong operation" {
					metric.SimpleMetricAggregation.Operation = "count"
				}
				agg := *metric.SimpleMetricAggregation
				if strings.HasPrefix(location, "ratio") {
					metric.MetricType = "ratio"
					metric.SimpleMetricAggregation = nil
					good := model.SimpleMetricAggregation{Operation: "count", Measure: model.MeasureRef{WarehouseMetricSourceSyncID: "src", Kind: "each_record"}}
					metric.RatioMetricAggregation = &model.RatioMetricAggregation{NumeratorAggregation: good, DenominatorAggregation: good}
					if strings.Contains(location, "numerator") {
						metric.RatioMetricAggregation.NumeratorAggregation = agg
					} else {
						metric.RatioMetricAggregation.DenominatorAggregation = agg
					}
				} else if strings.HasPrefix(location, "percentile") {
					metric.MetricType = "percentile"
					metric.SimpleMetricAggregation = nil
					metric.PercentileMetricAggregation = &model.PercentileMetricAggregation{Measure: ref}
				}
				issues := ValidateFiles([]model.FileConfig{{Path: "metrics.yaml", Config: config}})
				switch scenario {
				case "valid":
					checkMeasureIssues(t, issues, "", "")
				case "unknown subject":
					checkMeasureIssues(t, issues, "metrics[0]."+location+".measure.subject_type_name", "subject_types[].name")
				case "raw ID":
					checkMeasureIssues(t, issues, "metrics[0]."+location+".measure.warehouse_metric_measure_id", "not supported")
				case "wrong operation":
					checkMeasureIssues(t, issues, "metrics[0]."+location+".operation", "count requires")
				}
			})
		}
	}
}

func measureRefConfig(ref model.MeasureRef, operation string) model.SyncConfig {
	config := thresholdConfig(model.Metric{SyncID: "metric", Name: "Metric", MetricType: "simple", SimpleMetricAggregation: &model.SimpleMetricAggregation{Operation: operation, Measure: ref}})
	config.WarehouseMetricSources[0].SubjectTypes = []model.SourceSubjectType{{Name: "Customer", ColumnName: "customer_id"}}
	return config
}

func checkMeasureIssues(t *testing.T, issues []Issue, path, message string) {
	t.Helper()
	if message == "" {
		if len(issues) != 0 {
			t.Fatalf("expected no issues, got %v", issues)
		}
		return
	}
	if len(issues) != 1 || issues[0].File != "metrics.yaml" || issues[0].Path != path || !strings.Contains(issues[0].Message, message) {
		t.Fatalf("expected one issue at %q containing %q, got %v", path, message, issues)
	}
}
