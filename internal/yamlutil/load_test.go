// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileDefaultsCertified(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Options.Certified() {
		t.Fatal("expected omitted options.is_certified to default to true")
	}
}

func TestLoadFilePreservesExplicitUncertified(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
options:
  is_certified: false
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Options.Certified() {
		t.Fatal("expected options.is_certified=false to be preserved")
	}
}

func TestLoadFileThresholdMetric(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
warehouse_metric_sources:
  - sync_id: src
    name: src
    sql: "select 1"
    timestamp_column: ts
    subject_types:
      - name: User
        column_name: user_id
    measures:
      - sync_id: m
        name: m
        column_name: v
        column_type: FLOAT
metrics:
  - sync_id: met
    name: met
    metric_type: simple
    simple_metric_aggregation:
      operation: threshold
      threshold_aggregation_type: count
      threshold_comparison_operator: gt
      threshold_breach_value: 10
      threshold_timeframe_value: 7
      threshold_timeframe_dimension: days
      measure:
        warehouse_metric_source_sync_id: src
        measure_sync_id: m
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	agg := config.Metrics[0].SimpleMetricAggregation
	if agg == nil {
		t.Fatal("expected simple_metric_aggregation")
	}
	if agg.Operation != "threshold" {
		t.Fatalf("operation = %q, want threshold", agg.Operation)
	}
	if agg.ThresholdAggregationType != "count" {
		t.Fatalf("threshold_aggregation_type = %q, want count", agg.ThresholdAggregationType)
	}
	if agg.ThresholdComparisonOperator != "gt" {
		t.Fatalf("threshold_comparison_operator = %q, want gt", agg.ThresholdComparisonOperator)
	}
	if agg.ThresholdBreachValue == nil || *agg.ThresholdBreachValue != 10 {
		t.Fatalf("threshold_breach_value = %v, want 10", agg.ThresholdBreachValue)
	}
	if agg.ThresholdTimeframeValue == nil || *agg.ThresholdTimeframeValue != 7 {
		t.Fatalf("threshold_timeframe_value = %v, want 7", agg.ThresholdTimeframeValue)
	}
	if agg.ThresholdTimeframeDimension != "days" {
		t.Fatalf("threshold_timeframe_dimension = %q, want days", agg.ThresholdTimeframeDimension)
	}
}

func TestLoadFileThresholdRatioUsesCanonicalAggregationKeys(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
metrics:
  - sync_id: met
    name: met
    metric_type: ratio
    ratio_metric_aggregation:
      numerator_aggregation:
        operation: threshold
        threshold_aggregation_type: sum
        threshold_comparison_operator: gte
        threshold_breach_value: 10
        measure:
          warehouse_metric_source_sync_id: src
          measure_sync_id: m
      denominator_aggregation:
        operation: count
        measure:
          warehouse_metric_source_sync_id: src
          measure_sync_id: m
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ratio := config.Metrics[0].RatioMetricAggregation
	if ratio == nil {
		t.Fatal("expected ratio_metric_aggregation")
	}
	if ratio.NumeratorAggregation.Operation != "threshold" {
		t.Fatalf("numerator operation = %q, want threshold", ratio.NumeratorAggregation.Operation)
	}
	if ratio.DenominatorAggregation.Operation != "count" {
		t.Fatalf("denominator operation = %q, want count", ratio.DenominatorAggregation.Operation)
	}
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metric-sync.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
