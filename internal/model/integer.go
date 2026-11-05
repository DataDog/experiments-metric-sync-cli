// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Integer rejects fractional YAML values. yaml.v3 decodes 1.5 into an int as 1 without an error.
type Integer int

func (i *Integer) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!int" {
		return fmt.Errorf("line %d: %q must be an integer", node.Line, node.Value)
	}
	var value int
	if err := node.Decode(&value); err != nil {
		return err
	}
	*i = Integer(value)
	return nil
}
