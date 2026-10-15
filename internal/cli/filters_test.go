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
        property_name: Country
        property_column: country
        property_type: STRING
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

func withFilterBody(input, body string) string {
	start := strings.Index(input, "      property_filters: &filters\n")
	end := strings.Index(input[start:], "  - sync_id: ratio") + start
	return input[:start] + "      property_filters: &filters\n" + body + input[end:]
}

const singleFilterBody = `        property: &property
          warehouse_metric_source_sync_id: source
          property_sync_id: country
        operation: IS
        values: [US, CA]
`

const flatFilterBody = `        - property: &property
            warehouse_metric_source_sync_id: source
            property_sync_id: country
          operation: IS
          values: [US, CA]
        - property: *property
          operator: IS_NOT
          values: [FR]
`

func TestFilterRequestBody(t *testing.T) {
	groupAliases := strings.Replace(filterYAML, "        - - property: &property", "        - &common_group\n          - property: &property", 1)
	groupAliases = strings.Replace(groupAliases, "        - - property: *property", "        - &other_group\n          - property: *property", 1)
	groupAliases = strings.Replace(groupAliases, "property_filters: *filters", "property_filters: [*common_group, *other_group]", 1)
	filterAlias := withFilterBody(filterYAML, strings.Replace(flatFilterBody, "- property: &property", "- &condition\n          property: &property", 1))
	filterAlias = strings.Replace(filterAlias, "property_filters: *filters", "property_filters: [*condition, {property: *property, operation: IS_NOT, values: [FR]}]", 1)

	referenceIDs := strings.ReplaceAll(filterYAML, ": source", ": 001")
	referenceIDs = strings.ReplaceAll(referenceIDs, "sync_id: country", "sync_id: 002")
	scalarValues := strings.Replace(filterYAML, "name: Events", "name: &code 001", 1)
	scalarValues = strings.Replace(scalarValues, "values: [US, CA]", "values: [001, 1.00, 1e3, 2026-10-14, True, *code]", 1)
	for _, tc := range []struct {
		name, input, sourceID, propertyID string
		values                            []any
		shape                             string
	}{
		{"filter alias in flat list", filterAlias, "source", "country", []any{"US", "CA"}, "flat"},
		{"single filter", withFilterBody(filterYAML, singleFilterBody), "source", "country", []any{"US", "CA"}, "single"},
		{"flat AND list", withFilterBody(filterYAML, flatFilterBody), "source", "country", []any{"US", "CA"}, "flat"},
		{"single scalar text", withFilterBody(filterYAML, strings.Replace(singleFilterBody, "[US, CA]", "[001, 1.00, 1e3, 2026-10-14, True]", 1)), "source", "country", []any{"001", "1.00", "1e3", "2026-10-14", "True"}, "single"},
		{"flat scalar text", withFilterBody(filterYAML, strings.Replace(flatFilterBody, "[US, CA]", "[001, 1.00, 1e3, 2026-10-14, True]", 1)), "source", "country", []any{"001", "1.00", "1e3", "2026-10-14", "True"}, "flat"},
		{"standard", filterYAML, "source", "country", []any{"US", "CA"}, "nested"},
		{"reference text", referenceIDs, "001", "002", []any{"US", "CA"}, "nested"},
		{"scalar text", scalarValues, "source", "country", []any{"001", "1.00", "1e3", "2026-10-14", "True", "001"}, "nested"},
		{"group aliases", groupAliases, "source", "country", []any{"US", "CA"}, "nested"},
	} {
		for _, command := range []string{"validate", "plan", "execute"} {
			t.Run(tc.name+"/"+command, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "filters.yaml")
				if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
					t.Fatal(err)
				}
				var received map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/v2/experiments/metric-syncs" {
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
				wantProperty := map[string]any{"sync_id": tc.propertyID, "property_name": "Country", "property_column": "country", "property_type": "STRING"}
				if !reflect.DeepEqual(property, wantProperty) {
					t.Fatalf("property = %#v", property)
				}
				ref := map[string]any{"warehouse_metric_source_sync_id": tc.sourceID, "property_sync_id": tc.propertyID}
				wantFilters := []any{
					[]any{map[string]any{"property": ref, "operation": "IS", "values": tc.values}, map[string]any{"property": ref, "operation": "IS_NOT", "values": []any{"FR"}}},
					[]any{map[string]any{"property": ref, "operation": "IS", "values": []any{"GB"}}},
				}
				if tc.shape != "nested" {
					wantFilters = wantFilters[:1]
					if tc.shape == "single" {
						wantFilters[0] = wantFilters[0].([]any)[:1]
					}
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
}

func TestInvalidFiltersDoNotCallAPI(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"malformed indentation", "        - - property: &property", "        - property: &property", "parse"},
		{"single unknown field", "property_filters: *filters", "property_filters: {property: *property, operation: IS, values: [US], typo: true}", "field typo not found"},
		{"flat unknown field", "property_filters: *filters", "property_filters: [{property: *property, operation: IS, values: [US], typo: true}]", "field typo not found"},
		{"flat conflicting aliases", "property_filters: *filters", "property_filters: [{property: *property, operation: IS, operator: IS, values: [US]}]", "not both"},
		{"mixed filter and group", "property_filters: *filters", "property_filters: [{property: *property, operation: IS, values: [US]}, [{property: *property, operation: IS, values: [CA]}]]", "expected one filter"},
		{"scalar filter", "property_filters: *filters", "property_filters: country", "expected one filter"},
		{"single missing values", "property_filters: *filters", "property_filters: {property: *property, operation: IS, values: []}", "at least one value"},
		{"flat missing property", "property_filters: *filters", "property_filters: [{property: {warehouse_metric_source_sync_id: source, property_sync_id: missing}, operation: IS, values: [US]}]", "does not match a property"},
		{"old property name", "property_name: Country", "name: Country", "field name not found"},
		{"old property column", "property_column: country", "column_name: country", "field column_name not found"},
		{"old property type", "property_type: STRING", "column_type: STRING", "field column_type not found"},
		{"missing property name", "property_name: Country", "property_name: ''", "properties[0].property_name: is required"},
		{"missing property column", "property_column: country", "property_column: ''", "properties[0].property_column: is required"},
		{"missing property type", "property_type: STRING", "property_type: ''", "properties[0].property_type: is required"},
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

func TestInvalidMeasureFilterOperandsDoNotCallAPI(t *testing.T) {
	prefix := filterYAML[:strings.Index(filterYAML, "      property_filters: &filters\n")]
	for _, tc := range []struct{ name, columnType, operation, values, want string }{
		{"nonnumeric measure", "STRING", "=", "[10]", "INTEGER or FLOAT"},
		{"nonnumeric value", "FLOAT", ">=", "[abc]", "finite base-10"},
		{"fractional integer", "INTEGER", "=", "[1.5]", "must be an integer"},
		{"descending bounds", "FLOAT", "BETWEEN", "[10, 1]", "lower bound"},
	} {
		for _, command := range []string{"validate", "plan", "execute"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				input := strings.Replace(prefix, "column_type: FLOAT", "column_type: "+tc.columnType, 1) +
					"      property_filters:\n        measure: *measure\n        operation: '" + tc.operation + "'\n        values: " + tc.values + "\n"
				path := filepath.Join(t.TempDir(), "invalid.yaml")
				if err := os.WriteFile(path, []byte(input), 0600); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Error("invalid measure filter called API")
					w.WriteHeader(500)
				}))
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
func TestNullFilterRequestBody(t *testing.T) {
	input := filterYAML[:strings.Index(filterYAML, "  - sync_id: percentile")]
	for _, operation := range []string{"IS_NULL", "IS_NOT_NULL"} {
		for target, reference := range map[string]string{
			"property": "property: {warehouse_metric_source_sync_id: source, property_sync_id: country}",
			"measure":  "measure: {warehouse_metric_source_sync_id: source, measure_sync_id: value}",
		} {
			for valuesName, values := range map[string]string{"omitted": "", "null": ", values: null", "empty array": ", values: []"} {
				condition := "{" + reference + ", operation: " + operation + values + "}"
				for shape, body := range map[string]string{"single": condition, "flat": "[" + condition + "]", "nested": "[[" + condition + "]]"} {
					for _, command := range []string{"plan", "execute"} {
						t.Run(strings.Join([]string{operation, target, valuesName, shape, command}, "/"), func(t *testing.T) {
							path := filepath.Join(t.TempDir(), "null-filters.yaml")
							if err := os.WriteFile(path, []byte(withFilterBody(input, "        "+body+"\n")), 0600); err != nil {
								t.Fatal(err)
							}
							var received map[string]any
							server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
									t.Error(err)
								}
								writeOperation(t, w, "operation-id", "filters", command, "queued")
							}))
							defer server.Close()
							t.Setenv("DD_API_KEY", "api")
							t.Setenv("DD_APP_KEY", "app")
							var stdout, stderr bytes.Buffer
							args := []string{command, path, "--site", server.URL, "--no-poll", "--log-file", filepath.Join(t.TempDir(), "test.log")}
							if code := runWithIO(args, VersionInfo{Version: "test"}, &stdout, &stderr); code != 0 {
								t.Fatalf("exit %d: %s %s", code, &stdout, &stderr)
							}
							if received == nil {
								t.Fatal("no request received")
							}
							metrics := received["metrics"].([]any)
							simple := metrics[0].(map[string]any)["simple_metric_aggregation"].(map[string]any)
							ratio := metrics[1].(map[string]any)["ratio_metric_aggregation"].(map[string]any)
							for _, agg := range []map[string]any{simple, ratio["numerator_aggregation"].(map[string]any), ratio["denominator_aggregation"].(map[string]any)} {
								filter := agg["property_filters"].([]any)[0].([]any)[0].(map[string]any)
								values, ok := filter["values"].([]any)
								if !ok || len(values) != 0 {
									t.Errorf("values = %#v, want an empty JSON array", filter["values"])
								}
								if filter["operation"] != operation || filter[target] == nil {
									t.Errorf("filter target or operation changed: %#v", filter)
								}
							}
						})
					}
				}
			}
		}
	}
}
