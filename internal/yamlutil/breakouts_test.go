// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package yamlutil

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"github.com/DataDog/experiments-metric-sync-cli/internal/payload"
	"github.com/DataDog/experiments-metric-sync-cli/internal/validate"
)

func TestBreakoutExampleToPayload(t *testing.T) {
	config, err := LoadFile("../../examples/breakouts.yaml")
	if err != nil {
		t.Fatal(err)
	}
	files := []model.FileConfig{{Path: "breakouts.yaml", Config: config}}
	if issues := validate.ValidateFiles(files); len(issues) != 0 {
		t.Fatalf("validation: %v", issues)
	}
	request, _, err := payload.Build(files)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"property_breakouts":[{"property":{"property_sync_id":"country","warehouse_metric_source_sync_id":"purchases"},"values":["US","CA"]}]`) {
		t.Fatalf("breakouts lost: %s", raw)
	}
	if strings.Contains(string(raw), `"property_filters"`) {
		t.Fatalf("breakouts became filters: %s", raw)
	}
}

func TestBreakoutOmittedVersusExplicitClear(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		present    bool
	}{
		{"omitted", "", false},
		{"clear", "    property_breakouts: []\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := LoadFile(writeYAML(t, "schema_version: 1\nsync_tag: test\nmetrics:\n  - sync_id: revenue\n    name: Revenue\n    metric_type: simple\n"+tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(config.Metrics[0])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"property_breakouts":[]`) != tc.present {
				t.Fatalf("presence lost: %s", raw)
			}
		})
	}
}
