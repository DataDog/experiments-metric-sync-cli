// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package config

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		site string
		want string
	}{
		{name: "plain site", site: "datadoghq.com", want: "https://api.datadoghq.com"},
		{name: "api host", site: "api.datadoghq.eu", want: "https://api.datadoghq.eu"},
		{name: "explicit url", site: "https://smart-edge-proxy.us1.staging.dog", want: "https://smart-edge-proxy.us1.staging.dog"},
		{name: "trims slash", site: "https://example.com/", want: "https://example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.site)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadRequiresEnvCredentials(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvAppKey, "")
	t.Setenv(EnvAuthDomain, "")

	if _, err := Load(Options{}); err == nil {
		t.Fatal("expected missing credential error")
	}
}
