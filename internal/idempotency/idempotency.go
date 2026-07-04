// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package idempotency

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"github.com/DataDog/experiments-metric-sync-cli/internal/payload"
)

type Input struct {
	Operation string
	Payload   model.SyncConfig
	Options   payload.SubmitOptions
}

func Derive(input Input) (string, error) {
	identity, err := idempotencyIdentity()
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		Operation  string                `json:"operation"`
		SyncTag    string                `json:"sync_tag"`
		Payload    model.SyncConfig      `json:"payload"`
		Options    payload.SubmitOptions `json:"options"`
		CIIdentity string                `json:"ci_identity,omitempty"`
	}{
		Operation:  input.Operation,
		SyncTag:    input.Payload.SyncTag,
		Payload:    input.Payload,
		Options:    input.Options,
		CIIdentity: identity,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return strings.Join([]string{
		"metric-sync",
		input.Operation,
		sanitize(input.Payload.SyncTag),
		hex.EncodeToString(sum[:])[:24],
	}, "-"), nil
}

func idempotencyIdentity() (string, error) {
	if identity := ciIdentity(); identity != "" {
		return identity, nil
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	return "local_nonce=" + nonce, nil
}

func ciIdentity() string {
	candidates := []string{
		"GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT",
		"CI_PIPELINE_ID", "CI_JOB_ID",
		"BUILDKITE_BUILD_ID",
		"CIRCLE_WORKFLOW_ID",
		"BUILD_BUILDID",
		"BUILD_ID",
	}
	var parts []string
	for _, key := range candidates {
		if value := os.Getenv(key); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, ";")
}

func randomNonce() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate idempotency nonce: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func sanitize(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
