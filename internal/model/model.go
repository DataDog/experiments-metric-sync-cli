// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

type Options struct {
	IsCertified bool   `json:"-" yaml:"is_certified,omitempty"`
	UpgradeMode string `json:"-" yaml:"upgrade_mode,omitempty"`
	ForceDelete bool   `json:"-" yaml:"force_delete,omitempty"`
}

func (o Options) WithDefaults() Options {
	if o.UpgradeMode == "" {
		o.UpgradeMode = "none"
	}
	return o
}

type SyncConfig struct {
	SchemaVersion          int                     `json:"schema_version" yaml:"schema_version"`
	SyncTag                string                  `json:"sync_tag" yaml:"sync_tag"`
	ReferenceURL           string                  `json:"reference_url,omitempty" yaml:"reference_url,omitempty"`
	WarehouseConnectionID  string                  `json:"warehouse_connection_id,omitempty" yaml:"warehouse_connection_id,omitempty"`
	WarehouseMetricSources []WarehouseMetricSource `json:"warehouse_metric_sources,omitempty" yaml:"warehouse_metric_sources,omitempty"`
	Metrics                []Metric                `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	Options                Options                 `json:"-" yaml:"options,omitempty"`
}

type WarehouseMetricSource struct {
	SyncID                string              `json:"sync_id" yaml:"sync_id"`
	ExistingID            string              `json:"existing_id,omitempty" yaml:"existing_id,omitempty"`
	Name                  string              `json:"name" yaml:"name"`
	Description           string              `json:"description,omitempty" yaml:"description,omitempty"`
	SQL                   string              `json:"sql" yaml:"sql"`
	TimestampColumn       string              `json:"timestamp_column" yaml:"timestamp_column"`
	DatePartitionColumn   string              `json:"date_partition_column,omitempty" yaml:"date_partition_column,omitempty"`
	ReferenceURL          string              `json:"reference_url,omitempty" yaml:"reference_url,omitempty"`
	WarehouseConnectionID string              `json:"warehouse_connection_id,omitempty" yaml:"warehouse_connection_id,omitempty"`
	ForceRebuildAlways    bool                `json:"force_rebuild_always,omitempty" yaml:"force_rebuild_always,omitempty"`
	SubjectTypes          []SourceSubjectType `json:"subject_types,omitempty" yaml:"subject_types,omitempty"`
	Measures              []Measure           `json:"measures,omitempty" yaml:"measures,omitempty"`
	Properties            []Property          `json:"properties,omitempty" yaml:"properties,omitempty"`
}

type SourceSubjectType struct {
	Name       string `json:"name" yaml:"name"`
	ColumnName string `json:"column_name" yaml:"column_name"`
}

type Measure struct {
	SyncID      string `json:"sync_id" yaml:"sync_id"`
	Name        string `json:"name" yaml:"name"`
	ColumnName  string `json:"column_name" yaml:"column_name"`
	ColumnType  string `json:"column_type" yaml:"column_type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Property struct {
	SyncID      string `json:"sync_id" yaml:"sync_id"`
	Name        string `json:"name" yaml:"name"`
	ColumnName  string `json:"column_name" yaml:"column_name"`
	ColumnType  string `json:"column_type" yaml:"column_type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Metric struct {
	SyncID                      string                       `json:"sync_id" yaml:"sync_id"`
	ExistingID                  string                       `json:"existing_id,omitempty" yaml:"existing_id,omitempty"`
	Name                        string                       `json:"name" yaml:"name"`
	Description                 string                       `json:"description,omitempty" yaml:"description,omitempty"`
	MetricType                  string                       `json:"metric_type" yaml:"metric_type"`
	FormatAsPercent             bool                         `json:"format_as_percent,omitempty" yaml:"format_as_percent,omitempty"`
	ReferenceURL                string                       `json:"reference_url,omitempty" yaml:"reference_url,omitempty"`
	DesiredChange               string                       `json:"desired_change,omitempty" yaml:"desired_change,omitempty"`
	SimpleMetricAggregation     *SimpleMetricAggregation     `json:"simple_metric_aggregation,omitempty" yaml:"simple_metric_aggregation,omitempty"`
	RatioMetricAggregation      *RatioMetricAggregation      `json:"ratio_metric_aggregation,omitempty" yaml:"ratio_metric_aggregation,omitempty"`
	PercentileMetricAggregation *PercentileMetricAggregation `json:"percentile_metric_aggregation,omitempty" yaml:"percentile_metric_aggregation,omitempty"`
	GuardrailCutoffThreshold    *float64                     `json:"guardrail_cutoff_threshold,omitempty" yaml:"guardrail_cutoff_threshold,omitempty"`
}

type SimpleMetricAggregation struct {
	Operation             string           `json:"operation" yaml:"operation"`
	Measure               MeasureRef       `json:"measure" yaml:"measure"`
	TimeframeStartValue   *int             `json:"timeframe_start_value,omitempty" yaml:"timeframe_start_value,omitempty"`
	TimeframeEndValue     *int             `json:"timeframe_end_value,omitempty" yaml:"timeframe_end_value,omitempty"`
	TimeframeUnit         string           `json:"timeframe_unit,omitempty" yaml:"timeframe_unit,omitempty"`
	PropertyFilters       []PropertyFilter `json:"property_filters,omitempty" yaml:"property_filters,omitempty"`
	WinsorLowerPercentile *float64         `json:"winsor_lower_percentile,omitempty" yaml:"winsor_lower_percentile,omitempty"`
	WinsorUpperPercentile *float64         `json:"winsor_upper_percentile,omitempty" yaml:"winsor_upper_percentile,omitempty"`
	WinsorLowerFixedValue *float64         `json:"winsor_lower_fixed_value,omitempty" yaml:"winsor_lower_fixed_value,omitempty"`
	WinsorUpperFixedValue *float64         `json:"winsor_upper_fixed_value,omitempty" yaml:"winsor_upper_fixed_value,omitempty"`
}

type RatioMetricAggregation struct {
	Numerator   SimpleMetricAggregation `json:"numerator" yaml:"numerator"`
	Denominator SimpleMetricAggregation `json:"denominator" yaml:"denominator"`
}

type PercentileMetricAggregation struct {
	Operation           string     `json:"operation" yaml:"operation"`
	Measure             MeasureRef `json:"measure" yaml:"measure"`
	Percentile          float64    `json:"percentile" yaml:"percentile"`
	TimeframeStartValue *int       `json:"timeframe_start_value,omitempty" yaml:"timeframe_start_value,omitempty"`
	TimeframeEndValue   *int       `json:"timeframe_end_value,omitempty" yaml:"timeframe_end_value,omitempty"`
	TimeframeUnit       string     `json:"timeframe_unit,omitempty" yaml:"timeframe_unit,omitempty"`
}

type MeasureRef struct {
	Kind                         string `json:"kind,omitempty" yaml:"kind,omitempty"`
	MeasureSyncID                string `json:"measure_sync_id,omitempty" yaml:"measure_sync_id,omitempty"`
	WarehouseMetricSourceSyncID  string `json:"warehouse_metric_source_sync_id,omitempty" yaml:"warehouse_metric_source_sync_id,omitempty"`
	WarehouseMetricSourceSyncTag string `json:"warehouse_metric_source_sync_tag,omitempty" yaml:"warehouse_metric_source_sync_tag,omitempty"`
	WarehouseMetricMeasureID     string `json:"warehouse_metric_measure_id,omitempty" yaml:"warehouse_metric_measure_id,omitempty"`
	SubjectTypeName              string `json:"subject_type_name,omitempty" yaml:"subject_type_name,omitempty"`
}

type PropertyFilter struct {
	Property PropertyRef `json:"property" yaml:"property"`
	Operator string      `json:"operator" yaml:"operator"`
	Values   []string    `json:"values" yaml:"values"`
}

type PropertyRef struct {
	PropertySyncID               string `json:"property_sync_id,omitempty" yaml:"property_sync_id,omitempty"`
	WarehouseMetricSourceSyncID  string `json:"warehouse_metric_source_sync_id,omitempty" yaml:"warehouse_metric_source_sync_id,omitempty"`
	WarehouseMetricSourceSyncTag string `json:"warehouse_metric_source_sync_tag,omitempty" yaml:"warehouse_metric_source_sync_tag,omitempty"`
	WarehouseMetricPropertyID    string `json:"warehouse_metric_property_id,omitempty" yaml:"warehouse_metric_property_id,omitempty"`
}

type FileConfig struct {
	Path   string
	Config SyncConfig
}
