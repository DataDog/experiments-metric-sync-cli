// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type schemaNode struct {
	Ref        string                     `json:"$ref"`
	Properties map[string]json.RawMessage `json:"properties"`
	AllOf      []schemaNode               `json:"allOf"`
}

// The YAML loader rejects unknown fields, so a schema property missing from the model
// makes the CLI reject configs that the API accepts.
func TestModelMatchesSchema(t *testing.T) {
	raw, err := os.ReadFile("../../schemas/metric_sync_v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		schemaNode
		Defs map[string]schemaNode `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	// properties maps each field name to its JSON type, or "" when the type is a $ref.
	var properties func(node schemaNode, into map[string]string)
	properties = func(node schemaNode, into map[string]string) {
		if node.Ref != "" {
			properties(schema.Defs[strings.TrimPrefix(node.Ref, "#/$defs/")], into)
		}
		for name, raw := range node.Properties {
			var property struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(raw, &property)
			if property.Type != "" || into[name] == "" {
				into[name] = property.Type
			}
		}
		for _, part := range node.AllOf {
			properties(part, into)
		}
	}

	for _, tc := range []struct {
		def string
		typ any
		// cliOnly fields are parsed so validation can explain what to use instead.
		cliOnly []string
	}{
		{def: "", typ: SyncConfig{}},
		{def: "warehouse_metric_source", typ: WarehouseMetricSource{}},
		{def: "source_subject_type", typ: SourceSubjectType{}},
		{def: "measure", typ: Measure{}},
		{def: "property", typ: Property{}},
		{def: "metric", typ: Metric{}},
		{def: "simple_metric_aggregation", typ: SimpleMetricAggregation{}},
		{def: "ratio_component_aggregation", typ: SimpleMetricAggregation{}},
		{def: "ratio_metric_aggregation", typ: RatioMetricAggregation{}},
		{def: "percentile_metric_aggregation", typ: PercentileMetricAggregation{}},
		{def: "measure_ref", typ: MeasureRef{}, cliOnly: []string{"warehouse_metric_measure_id"}},
		{def: "property_ref", typ: PropertyRef{}, cliOnly: []string{"warehouse_metric_property_id"}},
		{def: "property_filter", typ: PropertyFilter{}},
	} {
		name := reflect.TypeOf(tc.typ).Name()
		t.Run(name+"/"+tc.def, func(t *testing.T) {
			node := schema.schemaNode
			if tc.def != "" {
				var ok bool
				if node, ok = schema.Defs[tc.def]; !ok {
					t.Fatalf("schema has no $defs.%s", tc.def)
				}
			}
			want := map[string]string{}
			properties(node, want)
			got := modelFields(reflect.TypeOf(tc.typ), tc.cliOnly)
			for field, jsonType := range want {
				kind, ok := got[field]
				if !ok {
					t.Errorf("schema field %q is missing from %s", field, name)
				} else if (jsonType == "number" && kind != reflect.Float64) || (jsonType == "integer" && kind != reflect.Int) {
					t.Errorf("%s field %q is %s but the schema type is %s", name, field, kind, jsonType)
				}
			}
			for field := range got {
				if _, ok := want[field]; !ok {
					t.Errorf("%s field %q is not in the schema", name, field)
				}
			}
		})
	}
}

// modelFields maps YAML keys sent to the API to their kind. Fields tagged json:"-" are CLI options.
func modelFields(typ reflect.Type, cliOnly []string) map[string]reflect.Kind {
	fields := map[string]reflect.Kind{}
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Tag.Get("json") == "-" {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if slices.Contains(cliOnly, name) {
			continue
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		fields[name] = fieldType.Kind()
	}
	return fields
}
