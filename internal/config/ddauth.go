package config

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type ddAuthConfig struct {
	APIKey string
	AppKey string
	Site   string
}

func loadFromDDAuth(domain string) (ddAuthConfig, error) {
	cmd := exec.Command("dd-auth", "--domain", domain, "--output")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return ddAuthConfig{}, fmt.Errorf("dd-auth failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	values := parseEnvOutput(string(output))
	return ddAuthConfig{
		APIKey: values[EnvAPIKey],
		AppKey: values[EnvAppKey],
		Site:   values[EnvSite],
	}, nil
}

func parseEnvOutput(output string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "export ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		values[strings.TrimSpace(key)] = value
	}
	return values
}
