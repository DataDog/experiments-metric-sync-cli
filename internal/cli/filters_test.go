// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const filterYAML = `
schema_version: 1
sync_tag: filters
warehouse_connection_id: connection
warehouse_metric_sources:
  - sync_id: source
    name: Events
    sql: SELECT * FROM events
    timestamp_column: ts
    measures:
      - sync_id: value
        name: Revenue
        column_name: value
        column_type: FLOAT
    properties:
      - sync_id: country
        name: Country
        column_name: country
        column_type: STRING
metrics:
  - sync_id: simple
    name: Simple
    metric_type: simple
    simple_metric_aggregation: &aggregation
      operation: sum
      measure: &measure
        warehouse_metric_source_sync_id: source
        measure_sync_id: value
      property_filters: &filters
        - - property: &property
              warehouse_metric_source_sync_id: source
              property_sync_id: country
            operation: IS
            values: [US, CA]
          - property: *property
            operator: IS_NOT
            values: [FR]
        - - property: *property
            operation: IS
            values: [GB]
  - sync_id: ratio
    name: Ratio
    metric_type: ratio
    ratio_metric_aggregation:
      numerator_aggregation: *aggregation
      denominator_aggregation: *aggregation
  - sync_id: percentile
    name: Percentile
    metric_type: percentile
    percentile_metric_aggregation:
      percentile: 0.95
      measure: *measure
      property_filters: *filters
`

func TestFilterRequestBody(t *testing.T) {
	for _, command := range []string{"validate", "plan", "execute"} {
		t.Run(command, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "filters.yaml")
			if err := os.WriteFile(path, []byte(filterYAML), 0600); err != nil {
				t.Fatal(err)
			}
			var received map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/unstable/ffe/metric-syncs" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(500)
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				writeOperation(t, w, "operation-id", "filters", command, "queued")
			}))
			defer server.Close()
			t.Setenv("DD_API_KEY", "api")
			t.Setenv("DD_APP_KEY", "app")
			var stdout, stderr bytes.Buffer
			args := []string{command, path, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}
			if command != "validate" {
				args = append(args, "--no-poll")
			}
			if code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr); code != 0 {
				t.Fatalf("exit %d: %s %s", code, &stdout, &stderr)
			}
			if command == "validate" {
				if received != nil {
					t.Fatal("validate called API")
				}
				return
			}
			property := received["warehouse_metric_sources"].([]any)[0].(map[string]any)["properties"].([]any)[0]
			wantProperty := map[string]any{"sync_id": "country", "property_name": "Country", "property_column": "country", "property_type": "STRING"}
			if !reflect.DeepEqual(property, wantProperty) {
				t.Fatalf("property = %#v", property)
			}
			ref := map[string]any{"warehouse_metric_source_sync_id": "source", "property_sync_id": "country"}
			wantFilters := []any{
				[]any{map[string]any{"property": ref, "operation": "IS", "values": []any{"US", "CA"}}, map[string]any{"property": ref, "operation": "IS_NOT", "values": []any{"FR"}}},
				[]any{map[string]any{"property": ref, "operation": "IS", "values": []any{"GB"}}},
			}
			metrics := received["metrics"].([]any)
			simple := metrics[0].(map[string]any)["simple_metric_aggregation"].(map[string]any)
			ratio := metrics[1].(map[string]any)["ratio_metric_aggregation"].(map[string]any)
			percentile := metrics[2].(map[string]any)["percentile_metric_aggregation"].(map[string]any)
			for _, agg := range []map[string]any{simple, ratio["numerator_aggregation"].(map[string]any), ratio["denominator_aggregation"].(map[string]any), percentile} {
				if !reflect.DeepEqual(agg["property_filters"], wantFilters) {
					t.Errorf("filters = %#v", agg["property_filters"])
				}
			}
		})
	}
}

func TestInvalidFiltersDoNotCallAPI(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"flat list", "        - - property: &property", "        - property: &property", "parse"},
		{"unknown filter field", "operation: IS\n", "operatoin: IS\n", "property_filters"},
		{"unknown reference field", "property_sync_id: country", "property_snyc_id: country", "property_filters"},
		{"conflicting aliases", "operator: IS_NOT", "operator: IS_NOT\n            operation: IS", "not both"},
		{"missing property", "property_sync_id: country", "property_sync_id: missing", "does not match a property"},
		{"raw property ID", "property_sync_id: country", "warehouse_metric_property_id: raw", "not supported"},
		{"wrong source", "property_sync_id: country", "warehouse_metric_source_sync_tag: other\n              property_sync_id: country", "same source"},
		{"invalid operation", "operation: IS\n", "operation: equals\n", "operation"},
		{"missing values", "values: [US, CA]", "values: []", "at least one value"},
		{"null with values", "operation: IS\n", "operation: IS_NULL\n", "must be empty"},
		{"percentile measure filter", "property_filters: *filters", "property_filters: [[{measure: *measure, operation: '=', values: ['2']}]]", "not supported for percentile"},
		{"empty group", "property_filters: *filters", "property_filters: [[]]", "at least one filter"},
	} {
		for _, command := range []string{"validate", "plan", "execute"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "invalid.yaml")
				if err := os.WriteFile(path, []byte(strings.Replace(filterYAML, tc.old, tc.replacement, 1)), 0600); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid filter called API"); w.WriteHeader(500) }))
				defer server.Close()
				t.Setenv("DD_API_KEY", "api")
				t.Setenv("DD_APP_KEY", "app")
				var stdout, stderr bytes.Buffer
				code := runWithIO([]string{command, path, "--site", server.URL, "--log-file", filepath.Join(t.TempDir(), "test.log")}, VersionInfo{Version: "test"}, &stdout, &stderr)
				output := stdout.String() + stderr.String()
				if code != 1 || !strings.Contains(output, path) || !strings.Contains(output, tc.want) {
					t.Fatalf("exit %d, want %q and filename: %s", code, tc.want, output)
				}
			})
		}
	}
}
