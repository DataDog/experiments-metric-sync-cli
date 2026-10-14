// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package validate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

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
	if len(source.AdditionalTimestampColumns) > 4 {
		add(path+".additional_timestamp_columns", "must contain at most 4 columns")
	}
	timestamps := map[string]bool{strings.TrimSpace(source.TimestampColumn): true}
	for i, column := range source.AdditionalTimestampColumns {
		columnPath := fmt.Sprintf("%s.additional_timestamp_columns[%d]", path, i)
		require(add, columnPath, column)
		if utf8.RuneCountInString(column) > 255 {
			add(columnPath, "must contain at most 255 characters")
		}
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		if timestamps[column] {
			add(columnPath, "must be unique and cannot include timestamp_column")
		}
		timestamps[column] = true
	}

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
	if agg.WinsorizationStrategy != nil {
		if !oneOf(*agg.WinsorizationStrategy, "all_assigned_subjects", "nonzero") {
			add(path+".winsorization_strategy", "must be all_assigned_subjects or nonzero")
		} else if agg.Operation == "threshold" && *agg.WinsorizationStrategy != "all_assigned_subjects" {
			add(path+".winsorization_strategy", "must be all_assigned_subjects or omitted when operation is threshold")
		}
	}
	if agg.WinsorLowerPercentile != nil && (*agg.WinsorLowerPercentile < 0 || *agg.WinsorLowerPercentile > 1) {
		add(path+".winsor_lower_percentile", "must be between 0 and 1")
	}
	if agg.WinsorUpperPercentile != nil && (*agg.WinsorUpperPercentile < 0 || *agg.WinsorUpperPercentile > 1) {
		add(path+".winsor_upper_percentile", "must be between 0 and 1")
	}
	if agg.WinsorLowerPercentile != nil && agg.WinsorUpperPercentile != nil && *agg.WinsorLowerPercentile >= *agg.WinsorUpperPercentile {
		add(path+".winsor_lower_percentile", "must be less than winsor_upper_percentile")
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
	sourcesByID := map[string]model.WarehouseMetricSource{}

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
			sourcesByID[source.SyncID] = source
		}
	}

	for _, file := range files {
		config := file.Config
		addForFile := func(path, message string) {
			add(file.Path, path, message)
		}
		for i, metric := range config.Metrics {
			if previous, ok := metricIDs[metric.SyncID]; ok {
				add(file.Path, fmt.Sprintf("metrics[%d].sync_id", i), fmt.Sprintf("duplicates metric sync_id from %s", previous))
			} else if metric.SyncID != "" {
				metricIDs[metric.SyncID] = file.Path
			}
			validateMetricRefs(addForFile, fmt.Sprintf("metrics[%d]", i), metric, syncTag, sourcesByID)
		}
	}
	return issues
}

func validateMetricRefs(add func(string, string), path string, metric model.Metric, syncTag string, sourcesByID map[string]model.WarehouseMetricSource) {
	validateAgg := func(path string, agg model.SimpleMetricAggregation) {
		validateFilters(add, path+".property_filters", agg.PropertyFilters, agg.Measure, true, syncTag, sourcesByID)
		if validateMeasureRef(add, path+".measure", agg.Measure, syncTag, sourcesByID) {
			validateOperationAgainstMeasure(add, path+".operation", agg.Operation, agg.Measure)
		}
	}
	if metric.SimpleMetricAggregation != nil {
		validateAgg(path+".simple_metric_aggregation", *metric.SimpleMetricAggregation)
	}
	if metric.RatioMetricAggregation != nil {
		validateAgg(path+".ratio_metric_aggregation.numerator_aggregation", metric.RatioMetricAggregation.NumeratorAggregation)
		validateAgg(path+".ratio_metric_aggregation.denominator_aggregation", metric.RatioMetricAggregation.DenominatorAggregation)
	}
	if metric.PercentileMetricAggregation != nil {
		validateMeasureRef(add, path+".percentile_metric_aggregation.measure", metric.PercentileMetricAggregation.Measure, syncTag, sourcesByID)
		validateFilters(add, path+".percentile_metric_aggregation.property_filters", metric.PercentileMetricAggregation.PropertyFilters, metric.PercentileMetricAggregation.Measure, false, syncTag, sourcesByID)
	}
}

func validateMeasureRef(add func(string, string), path string, ref model.MeasureRef, syncTag string, sourcesByID map[string]model.WarehouseMetricSource) bool {
	if ref.WarehouseMetricMeasureID != "" {
		add(path+".warehouse_metric_measure_id", "is not supported by the sync API; use warehouse_metric_source_sync_id with measure_sync_id, subject_type_name, or kind: each_record")
		return false
	}
	if strings.TrimSpace(ref.WarehouseMetricSourceSyncID) == "" {
		add(path+".warehouse_metric_source_sync_id", "is required; set it to the source sync_id")
		return false
	}
	selectorCount := 0
	for _, selector := range []string{ref.MeasureSyncID, ref.Kind, ref.SubjectTypeName} {
		if selector != "" {
			selectorCount++
		}
	}
	if selectorCount != 1 {
		add(path, "must select exactly one of measure_sync_id, subject_type_name, or kind: each_record")
		return false
	}
	if ref.Kind != "" && ref.Kind != "each_record" {
		add(path+".kind", "must be each_record; use measure_sync_id or subject_type_name for a column measure")
		return false
	}
	if ref.MeasureSyncID != "" && strings.TrimSpace(ref.MeasureSyncID) == "" {
		add(path+".measure_sync_id", "must not be blank")
		return false
	}
	if ref.SubjectTypeName != "" && strings.TrimSpace(ref.SubjectTypeName) == "" {
		add(path+".subject_type_name", "must not be blank")
		return false
	}

	source, ok := sourcesByID[ref.WarehouseMetricSourceSyncID]
	// Match backend resolution: an explicit tag is local only when it matches
	// this operation and the source is in the payload. Plan resolves other sources.
	if ref.WarehouseMetricSourceSyncTag != "" && (ref.WarehouseMetricSourceSyncTag != syncTag || !ok) {
		return true
	}
	if !ok {
		add(path+".warehouse_metric_source_sync_id", "does not match a source sync_id in this operation; include the source or set warehouse_metric_source_sync_tag for a server reference and run plan")
		return false
	}
	if ref.MeasureSyncID != "" {
		for _, measure := range source.Measures {
			if measure.SyncID == ref.MeasureSyncID {
				return true
			}
		}
		add(path+".measure_sync_id", "does not match a measure sync_id on the referenced source")
		return false
	}
	if ref.SubjectTypeName != "" {
		for _, subject := range source.SubjectTypes {
			if subject.Name == ref.SubjectTypeName {
				return true
			}
		}
		add(path+".subject_type_name", "does not match subject_types[].name on the referenced source; add the subject mapping or use a mapped name")
		return false
	}
	return true
}

func validateOperationAgainstMeasure(add func(string, string), path, operation string, ref model.MeasureRef) {
	switch {
	case operation == "count" && ref.Kind != "each_record":
		add(path, "count requires measure.kind: each_record")
	case oneOf(operation, "sum", "average") && ref.MeasureSyncID == "":
		add(path, operation+" requires a user-defined measure; set measure.measure_sync_id")
	case operation == "countDistinctValue" && ref.Kind == "each_record":
		add(path, "countDistinctValue cannot use kind: each_record; set measure.measure_sync_id or measure.subject_type_name")
	case operation == "uniqueSubjects" && ref.SubjectTypeName == "":
		add(path, "uniqueSubjects requires measure.subject_type_name; use a subject mapped on the source")
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
