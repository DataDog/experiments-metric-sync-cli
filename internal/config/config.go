package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	EnvAPIKey     = "DD_API_KEY"
	EnvAppKey     = "DD_APP_KEY"
	EnvSite       = "DD_SITE"
	EnvAuthDomain = "DD_AUTH_DOMAIN"
)

type Config struct {
	APIKey  string
	AppKey  string
	Site    string
	BaseURL string
}

type Options struct {
	SiteOverride string
}

func Load(options Options) (Config, error) {
	apiKey := os.Getenv(EnvAPIKey)
	appKey := os.Getenv(EnvAppKey)
	site := firstNonEmpty(options.SiteOverride, os.Getenv(EnvSite), "datadoghq.com")

	if (apiKey == "" || appKey == "") && os.Getenv(EnvAuthDomain) != "" {
		authConfig, err := loadFromDDAuth(os.Getenv(EnvAuthDomain))
		if err != nil {
			return Config{}, err
		}
		apiKey = firstNonEmpty(apiKey, authConfig.APIKey)
		appKey = firstNonEmpty(appKey, authConfig.AppKey)
		site = firstNonEmpty(options.SiteOverride, os.Getenv(EnvSite), authConfig.Site, site)
	}

	if apiKey == "" {
		return Config{}, fmt.Errorf("%s is required", EnvAPIKey)
	}
	if appKey == "" {
		return Config{}, fmt.Errorf("%s is required", EnvAppKey)
	}

	baseURL, err := NormalizeBaseURL(site)
	if err != nil {
		return Config{}, err
	}

	return Config{
		APIKey:  apiKey,
		AppKey:  appKey,
		Site:    site,
		BaseURL: baseURL,
	}, nil
}

func NormalizeBaseURL(site string) (string, error) {
	site = strings.TrimSpace(site)
	if site == "" {
		site = "datadoghq.com"
	}
	if strings.HasPrefix(site, "http://") || strings.HasPrefix(site, "https://") {
		parsed, err := url.Parse(site)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("invalid Datadog site URL %q", site)
		}
		return strings.TrimRight(site, "/"), nil
	}
	if strings.Contains(site, "/") {
		return "", fmt.Errorf("invalid Datadog site %q", site)
	}
	if strings.HasPrefix(site, "api.") {
		return "https://" + site, nil
	}
	return "https://api." + site, nil
}

func Redacted(config Config) map[string]string {
	return map[string]string{
		"site":     config.Site,
		"base_url": config.BaseURL,
		"api_key":  redact(config.APIKey),
		"app_key":  redact(config.AppKey),
	}
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
