// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"os"
	"path/filepath"
	"strings"
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
        winsorization_strategy: all_assigned_subjects
        threshold_aggregation_type: sum
        threshold_comparison_operator: gte
        threshold_breach_value: 10
        measure:
          warehouse_metric_source_sync_id: src
          measure_sync_id: m
      denominator_aggregation:
        operation: count
        winsorization_strategy: nonzero
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
	if ratio.NumeratorAggregation.WinsorizationStrategy == nil || *ratio.NumeratorAggregation.WinsorizationStrategy != "all_assigned_subjects" || ratio.DenominatorAggregation.WinsorizationStrategy == nil || *ratio.DenominatorAggregation.WinsorizationStrategy != "nonzero" {
		t.Fatalf("unexpected ratio strategies: %#v", ratio)
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

func TestLoadFileDocumentBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"leading marker", "---\nschema_version: 1\nsync_tag: test\n", ""},
		{"trailing comments", "schema_version: 1\nsync_tag: test\n# comment\n  \n", ""},
		{"end marker", "schema_version: 1\nsync_tag: test\n...\n# comment\n", ""},
		{"second document", "schema_version: 1\nsync_tag: test\n---\nmetrics: []\n", "only one YAML document"},
		{"empty second document", "schema_version: 1\nsync_tag: test\n---\n", "only one YAML document"},
		{"comment second document", "schema_version: 1\nsync_tag: test\n---\n# comment\n", "only one YAML document"},
		{"malformed second document", "schema_version: 1\nsync_tag: test\n---\nmetrics: [\n", "after the first YAML document"},
		{"malformed trailing content", "schema_version: 1\nsync_tag: test\n...\ninvalid\n", "after the first YAML document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeYAML(t, tc.body)
			_, err := LoadFile(path)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected filename and %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadFilesKeepsSeparateDocuments(t *testing.T) {
	files, err := LoadFiles([]string{writeYAML(t, "schema_version: 1\nsync_tag: test\n"), writeYAML(t, "---\nschema_version: 1\nsync_tag: test\n")})
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v, error=%v", files, err)
	}
}

func TestLoadFileAgingAndFractionalTimeframe(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
metrics:
  - sync_id: met
    name: met
    metric_type: simple
    simple_metric_aggregation:
      operation: sum
      timeframe_start_value: 0.5
      timeframe_end_value: 1.5
      timeframe_unit: weeks
      enable_aging_subject_filter: true
      aging_threshold_days: 14
      measure:
        warehouse_metric_source_sync_id: src
        measure_sync_id: m
`)

	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	agg := config.Metrics[0].SimpleMetricAggregation
	if agg.TimeframeStartValue == nil || *agg.TimeframeStartValue != 0.5 || agg.TimeframeEndValue == nil || *agg.TimeframeEndValue != 1.5 {
		t.Fatalf("timeframe = %v..%v, want 0.5..1.5", agg.TimeframeStartValue, agg.TimeframeEndValue)
	}
	if agg.EnableAgingSubjectFilter == nil || !*agg.EnableAgingSubjectFilter || agg.AgingThresholdDays == nil || *agg.AgingThresholdDays != 14 {
		t.Fatalf("aging = %v/%v, want true/14", agg.EnableAgingSubjectFilter, agg.AgingThresholdDays)
	}
}

func TestLoadFileRejectsPercentileTimeframe(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
metrics:
  - sync_id: met
    name: met
    metric_type: percentile
    percentile_metric_aggregation:
      percentile: 0.9
      timeframe_end_value: 7
      measure:
        warehouse_metric_source_sync_id: src
        measure_sync_id: m
`)

	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "timeframe_end_value") {
		t.Fatalf("expected unknown timeframe_end_value error, got %v", err)
	}
}

func TestLoadFileRejectsFractionalIntegers(t *testing.T) {
	for _, tc := range []struct {
		name, schemaVersion, days string
	}{
		{name: "fractional aging days", schemaVersion: "1", days: "1.5"},
		{name: "negative fractional aging days", schemaVersion: "1", days: "-0.5"},
		{name: "quoted aging days", schemaVersion: "1", days: `"14"`},
		{name: "fractional schema version", schemaVersion: "1.5", days: "14"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeYAML(t, `
schema_version: `+tc.schemaVersion+`
sync_tag: checkout
metrics:
  - sync_id: met
    name: met
    metric_type: simple
    simple_metric_aggregation:
      operation: sum
      enable_aging_subject_filter: true
      aging_threshold_days: `+tc.days+`
      measure:
        warehouse_metric_source_sync_id: src
        measure_sync_id: m
`)
			if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "must be an integer") {
				t.Fatalf("expected integer error, got %v", err)
			}
		})
	}
}

func TestLoadFilesMetricCollectionsNullDetection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		wantNull bool
		wantNil  bool
	}{
		{name: "null", body: "metric_collections: null\n", wantNull: true, wantNil: true},
		{name: "tilde", body: "metric_collections: ~\n", wantNull: true, wantNil: true},
		{name: "empty value", body: "metric_collections:\n", wantNull: true, wantNil: true},
		{name: "empty list", body: "metric_collections: []\n", wantNull: false, wantNil: false},
		{name: "omitted", body: "", wantNull: false, wantNil: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeYAML(t, "schema_version: 1\nsync_tag: checkout\n"+tc.body)
			files, err := LoadFiles([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			file := files[0]
			if file.MetricCollectionsNull != tc.wantNull {
				t.Fatalf("MetricCollectionsNull = %v, want %v", file.MetricCollectionsNull, tc.wantNull)
			}
			if (file.Config.MetricCollections == nil) != tc.wantNil {
				t.Fatalf("MetricCollections nil = %v, want %v", file.Config.MetricCollections == nil, tc.wantNil)
			}
		})
	}
}

func TestLoadFileMetricCollections(t *testing.T) {
	path := writeYAML(t, `
schema_version: 1
sync_tag: checkout
metric_collections:
  - sync_id: checkout_core
    name: Checkout core
    is_guardrail: true
    metrics:
      - metric_sync_id: checkout_revenue
      - metric_sync_tag: growth
        metric_sync_id: signups
`)
	config, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	collection := config.MetricCollections[0]
	if collection.SyncID != "checkout_core" || !collection.IsGuardrail || len(collection.Metrics) != 2 ||
		collection.Metrics[1].MetricSyncTag != "growth" || collection.Metrics[1].MetricSyncID != "signups" {
		t.Fatalf("unexpected collection: %+v", collection)
	}

	unknown := writeYAML(t, `
schema_version: 1
sync_tag: checkout
metric_collections:
  - sync_id: checkout_core
    name: Checkout core
    owner: me
`)
	if _, err := LoadFile(unknown); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected unknown collection field to be rejected, got %v", err)
	}
}

func TestLoadFileRejectsNullListItems(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "warehouse metric source", body: "warehouse_metric_sources: [~]\n", want: "line 3: warehouse_metric_sources[0] is empty"},
		{name: "metric", body: "metrics:\n  -\n", want: "line 4: metrics[0] is empty"},
		{name: "collection", body: "metric_collections: [null]\n", want: "line 3: metric_collections[0] is empty"},
		{
			name: "collection with all fields commented out",
			body: "metric_collections:\n  - sync_id: keep\n    name: Keep\n  -\n    # sync_id: gone\n    # name: Gone\n",
			want: "line 6: metric_collections[1] is empty",
		},
		{
			name: "collection member",
			body: "metric_collections:\n  - sync_id: keep\n    name: Keep\n    metrics:\n      - metric_sync_id: met\n      -\n",
			want: "line 8: metric_collections[0].metrics[1] is empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeYAML(t, "schema_version: 1\nsync_tag: checkout\n"+tc.body)
			_, err := LoadFile(path)
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q and the file path, got %v", tc.want, err)
			}
		})
	}
}
