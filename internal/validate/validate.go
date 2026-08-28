// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
)

var syncIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)

type Issue struct {
	File    string `json:"file,omitempty"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (i Issue) Error() string {
	if i.File == "" {
		return fmt.Sprintf("%s: %s", i.Path, i.Message)
	}
	return fmt.Sprintf("%s: %s: %s", i.File, i.Path, i.Message)
}

func ValidateFiles(files []model.FileConfig) []Issue {
	var issues []Issue
	for _, file := range files {
		issues = append(issues, validateConfig(file.Path, file.Config)...)
	}
	if len(issues) > 0 {
		return issues
	}
	return append(issues, validateCombined(files)...)
}

func validateConfig(file string, config model.SyncConfig) []Issue {
	var issues []Issue
	add := func(path, message string) {
		issues = append(issues, Issue{File: file, Path: path, Message: message})
	}

	if config.SchemaVersion != 1 {
		add("schema_version", "must be 1")
	}
	if config.SyncTag == "" {
		add("sync_tag", "is required")
	} else if !syncIDPattern.MatchString(config.SyncTag) {
		add("sync_tag", "must start with an alphanumeric character and contain only alphanumeric, underscore, dot, colon, or dash characters")
	}
	for i, source := range config.WarehouseMetricSources {
		validateSource(add, fmt.Sprintf("warehouse_metric_sources[%d]", i), source)
	}
	for i, metric := range config.Metrics {
		validateMetric(add, fmt.Sprintf("metrics[%d]", i), metric)
	}
	return issues
}

func validateSource(add func(string, string), path string, source model.WarehouseMetricSource) {
	requireSyncID(add, path+".sync_id", source.SyncID)
	require(add, path+".name", source.Name)
	require(add, path+".sql", source.SQL)
	require(add, path+".timestamp_column", source.TimestampColumn)

	for i, subject := range source.SubjectTypes {
		require(add, fmt.Sprintf("%s.subject_types[%d].name", path, i), subject.Name)
		require(add, fmt.Sprintf("%s.subject_types[%d].column_name", path, i), subject.ColumnName)
	}
	for i, measure := range source.Measures {
		prefix := fmt.Sprintf("%s.measures[%d]", path, i)
		requireSyncID(add, prefix+".sync_id", measure.SyncID)
		require(add, prefix+".name", measure.Name)
		require(add, prefix+".column_name", measure.ColumnName)
		require(add, prefix+".column_type", measure.ColumnType)
	}
	for i, property := range source.Properties {
		prefix := fmt.Sprintf("%s.properties[%d]", path, i)
		requireSyncID(add, prefix+".sync_id", property.SyncID)
		require(add, prefix+".name", property.Name)
		require(add, prefix+".column_name", property.ColumnName)
		require(add, prefix+".column_type", property.ColumnType)
	}
}

func validateMetric(add func(string, string), path string, metric model.Metric) {
	requireSyncID(add, path+".sync_id", metric.SyncID)
	require(add, path+".name", metric.Name)
	require(add, path+".metric_type", metric.MetricType)
	if metric.DesiredChange != "" && !oneOf(metric.DesiredChange, "METRIC_INCREASES", "METRIC_DECREASES") {
		add(path+".desired_change", "must be METRIC_INCREASES or METRIC_DECREASES")
	}
	switch metric.MetricType {
	case "simple":
		if metric.SimpleMetricAggregation == nil {
			add(path+".simple_metric_aggregation", "is required when metric_type is simple")
		} else {
			validateAggregation(add, path+".simple_metric_aggregation", metric.SimpleMetricAggregation)
		}
	case "ratio":
		if metric.RatioMetricAggregation == nil {
			add(path+".ratio_metric_aggregation", "is required when metric_type is ratio")
		} else {
			validateAggregation(add, path+".ratio_metric_aggregation.numerator_aggregation", &metric.RatioMetricAggregation.NumeratorAggregation)
			validateAggregation(add, path+".ratio_metric_aggregation.denominator_aggregation", &metric.RatioMetricAggregation.DenominatorAggregation)
		}
	case "percentile":
		if metric.PercentileMetricAggregation == nil {
			add(path+".percentile_metric_aggregation", "is required when metric_type is percentile")
		}
	default:
		add(path+".metric_type", "must be simple, ratio, or percentile")
	}
}

func validateAggregation(add func(string, string), path string, agg *model.SimpleMetricAggregation) {
	if agg == nil {
		return
	}
	if agg.Operation != "threshold" {
		validateNoThresholdFields(add, path, agg)
		return
	}
	require(add, path+".threshold_aggregation_type", agg.ThresholdAggregationType)
	if agg.ThresholdAggregationType != "" && !oneOf(agg.ThresholdAggregationType, "count", "sum") {
		add(path+".threshold_aggregation_type", "must be count or sum")
	}
	require(add, path+".threshold_comparison_operator", agg.ThresholdComparisonOperator)
	if agg.ThresholdComparisonOperator != "" && !oneOf(agg.ThresholdComparisonOperator, "gt", "gte", "lt", "lte", "eq", "neq") {
		add(path+".threshold_comparison_operator", "must be gt, gte, lt, lte, eq, or neq")
	}
	requireFloat(add, path+".threshold_breach_value", agg.ThresholdBreachValue)
	if agg.ThresholdTimeframeValue == nil && agg.ThresholdTimeframeDimension != "" {
		add(path+".threshold_timeframe_value", "must be set with threshold_timeframe_dimension")
	}
	if agg.ThresholdTimeframeValue != nil && strings.TrimSpace(agg.ThresholdTimeframeDimension) == "" {
		add(path+".threshold_timeframe_dimension", "must be set with threshold_timeframe_value")
	}
	if agg.TimeframeStartValue != nil && *agg.TimeframeStartValue != 0 {
		add(path+".timeframe_start_value", "must be 0 or omitted when operation is threshold")
	}
	if agg.TimeframeEndValue != nil {
		add(path+".timeframe_end_value", "must be omitted when operation is threshold")
	}
	if agg.TimeframeUnit != "" {
		add(path+".timeframe_unit", "must be omitted when operation is threshold")
	}
	if agg.WinsorLowerPercentile != nil {
		add(path+".winsor_lower_percentile", "must be omitted when operation is threshold")
	}
	if agg.WinsorUpperPercentile != nil {
		add(path+".winsor_upper_percentile", "must be omitted when operation is threshold")
	}
	if agg.WinsorLowerFixedValue != nil {
		add(path+".winsor_lower_fixed_value", "must be omitted when operation is threshold")
	}
	if agg.WinsorUpperFixedValue != nil {
		add(path+".winsor_upper_fixed_value", "must be omitted when operation is threshold")
	}
}

func validateNoThresholdFields(add func(string, string), path string, agg *model.SimpleMetricAggregation) {
	if agg.ThresholdAggregationType != "" {
		add(path+".threshold_aggregation_type", "can only be set when operation is threshold")
	}
	if agg.ThresholdComparisonOperator != "" {
		add(path+".threshold_comparison_operator", "can only be set when operation is threshold")
	}
	if agg.ThresholdBreachValue != nil {
		add(path+".threshold_breach_value", "can only be set when operation is threshold")
	}
	if agg.ThresholdTimeframeValue != nil {
		add(path+".threshold_timeframe_value", "can only be set when operation is threshold")
	}
	if agg.ThresholdTimeframeDimension != "" {
		add(path+".threshold_timeframe_dimension", "can only be set when operation is threshold")
	}
}

func validateCombined(files []model.FileConfig) []Issue {
	var issues []Issue
	add := func(file, path, message string) {
		issues = append(issues, Issue{File: file, Path: path, Message: message})
	}

	var syncTag string
	var schemaVersion int
	sourceIDs := map[string]string{}
	metricIDs := map[string]string{}
	measuresBySource := map[string]map[string]struct{}{}

	for _, file := range files {
		config := file.Config
		if syncTag == "" {
			syncTag = config.SyncTag
		} else if config.SyncTag != syncTag {
			add(file.Path, "sync_tag", fmt.Sprintf("must match %q from other files in the same operation", syncTag))
		}
		if schemaVersion == 0 {
			schemaVersion = config.SchemaVersion
		} else if config.SchemaVersion != schemaVersion {
			add(file.Path, "schema_version", fmt.Sprintf("must match %d from other files in the same operation", schemaVersion))
		}

		for i, source := range config.WarehouseMetricSources {
			if previous, ok := sourceIDs[source.SyncID]; ok {
				add(file.Path, fmt.Sprintf("warehouse_metric_sources[%d].sync_id", i), fmt.Sprintf("duplicates source sync_id from %s", previous))
			} else if source.SyncID != "" {
				sourceIDs[source.SyncID] = file.Path
			}
			sourceMeasures := map[string]struct{}{}
			for j, measure := range source.Measures {
				if _, ok := sourceMeasures[measure.SyncID]; ok {
					add(file.Path, fmt.Sprintf("warehouse_metric_sources[%d].measures[%d].sync_id", i, j), "duplicates measure sync_id in the same source")
				}
				sourceMeasures[measure.SyncID] = struct{}{}
			}
			measuresBySource[source.SyncID] = sourceMeasures
		}
	}

	for _, file := range files {
		config := file.Config
		for i, metric := range config.Metrics {
			if previous, ok := metricIDs[metric.SyncID]; ok {
				add(file.Path, fmt.Sprintf("metrics[%d].sync_id", i), fmt.Sprintf("duplicates metric sync_id from %s", previous))
			} else if metric.SyncID != "" {
				metricIDs[metric.SyncID] = file.Path
			}
			validateMetricRefs(add, file.Path, fmt.Sprintf("metrics[%d]", i), metric, measuresBySource)
		}
	}
	return issues
}

func validateMetricRefs(add func(string, string, string), file string, path string, metric model.Metric, measuresBySource map[string]map[string]struct{}) {
	switch {
	case metric.SimpleMetricAggregation != nil:
		validateMeasureRef(add, file, path+".simple_metric_aggregation.measure", metric.SimpleMetricAggregation.Measure, measuresBySource)
	case metric.RatioMetricAggregation != nil:
		validateMeasureRef(add, file, path+".ratio_metric_aggregation.numerator_aggregation.measure", metric.RatioMetricAggregation.NumeratorAggregation.Measure, measuresBySource)
		validateMeasureRef(add, file, path+".ratio_metric_aggregation.denominator_aggregation.measure", metric.RatioMetricAggregation.DenominatorAggregation.Measure, measuresBySource)
	case metric.PercentileMetricAggregation != nil:
		validateMeasureRef(add, file, path+".percentile_metric_aggregation.measure", metric.PercentileMetricAggregation.Measure, measuresBySource)
	}
}

func validateMeasureRef(add func(string, string, string), file string, path string, ref model.MeasureRef, measuresBySource map[string]map[string]struct{}) {
	if ref.WarehouseMetricSourceSyncTag != "" || ref.WarehouseMetricMeasureID != "" {
		return
	}
	if ref.WarehouseMetricSourceSyncID == "" || ref.MeasureSyncID == "" {
		add(file, path, "must set warehouse_metric_source_sync_id and measure_sync_id, or use an external/id-based measure reference")
		return
	}
	measures, ok := measuresBySource[ref.WarehouseMetricSourceSyncID]
	if !ok {
		add(file, path+".warehouse_metric_source_sync_id", "does not match a source sync_id in this operation")
		return
	}
	if _, ok := measures[ref.MeasureSyncID]; !ok {
		add(file, path+".measure_sync_id", "does not match a measure sync_id on the referenced source")
	}
}

func require(add func(string, string), path string, value string) {
	if strings.TrimSpace(value) == "" {
		add(path, "is required")
	}
}

func requireFloat(add func(string, string), path string, value *float64) {
	if value == nil {
		add(path, "is required")
	}
}

func requireSyncID(add func(string, string), path string, value string) {
	require(add, path, value)
	if value != "" && !syncIDPattern.MatchString(value) {
		add(path, "must start with an alphanumeric character and contain only alphanumeric, underscore, dot, colon, or dash characters")
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
