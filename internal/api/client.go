// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/experiments-metric-sync-cli/internal/config"
	"github.com/DataDog/experiments-metric-sync-cli/internal/model"
	"github.com/DataDog/experiments-metric-sync-cli/internal/payload"
)

type Client struct {
	baseURL    string
	apiKey     string
	appKey     string
	httpClient *http.Client
	logger     DebugLogger
}

type DebugLogger interface {
	Info(message string, attrs ...slog.Attr)
	Error(message string, attrs ...slog.Attr)
}

func NewClient(config config.Config) *Client {
	return NewClientWithLogger(config, nil)
}

func NewClientWithLogger(config config.Config, logger DebugLogger) *Client {
	return &Client{
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		apiKey:  config.APIKey,
		appKey:  config.AppKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

func (c *Client) Submit(ctx context.Context, operation string, request model.SyncConfig, options payload.SubmitOptions, idempotencyKey string) (*Operation, error) {
	values := url.Values{}
	values.Set("plan", strconv.FormatBool(operation == "plan"))
	values.Set("is_certified", strconv.FormatBool(options.IsCertified))
	if options.UpgradeMode != "" {
		values.Set("upgrade_mode", options.UpgradeMode)
	}
	values.Set("force_delete", strconv.FormatBool(options.ForceDelete))

	endpoint := c.baseURL + "/api/unstable/ffe/metric-syncs?" + values.Encode()
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	c.info("submit request",
		slog.String("operation", operation),
		slog.String("endpoint", endpoint),
		slog.String("idempotency_key", idempotencyKey),
	)
	c.info("submit payload", slog.String("payload", string(body)))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.addAuthHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	var operationResponse Operation
	if err := c.do(httpReq, &operationResponse); err != nil {
		return nil, err
	}
	return &operationResponse, nil
}

func (c *Client) GetStatus(ctx context.Context, metricSyncID string) (*Operation, error) {
	endpoint := c.baseURL + "/api/unstable/ffe/metric-syncs/" + url.PathEscape(metricSyncID)
	c.info("status request", slog.String("endpoint", endpoint))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.addAuthHeaders(httpReq)
	httpReq.Header.Set("Accept", "application/json")

	var operation Operation
	if err := c.do(httpReq, &operation); err != nil {
		return nil, err
	}
	return &operation, nil
}

func (c *Client) GetResult(ctx context.Context, metricSyncID string) (*Result, error) {
	endpoint := c.baseURL + "/api/unstable/ffe/metric-syncs/" + url.PathEscape(metricSyncID) + "/result"
	c.info("result request", slog.String("endpoint", endpoint))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.addAuthHeaders(httpReq)
	httpReq.Header.Set("Accept", "application/json")

	var result Result
	if err := c.do(httpReq, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListWarehouseConnections(ctx context.Context) ([]WarehouseConnection, error) {
	endpoint := c.baseURL + "/api/unstable/ffe/warehouse-connections"
	c.info("warehouse connections request", slog.String("endpoint", endpoint))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.addAuthHeaders(httpReq)
	httpReq.Header.Set("Accept", "application/json")

	body, err := c.doRaw(httpReq)
	if err != nil {
		return nil, err
	}
	return decodeWarehouseConnections(body)
}

func (c *Client) addAuthHeaders(req *http.Request) {
	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.appKey)
}

func (c *Client) do(req *http.Request, output any) error {
	body, err := c.doRaw(req)
	if err != nil {
		return err
	}
	return decodeData(body, output)
}

func (c *Client) doRaw(req *http.Request) ([]byte, error) {
	start := time.Now()
	c.info("http request",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
	)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.error("http request failed",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()),
			slog.Any("error", err),
		)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.error("http response read failed",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()),
			slog.Int("status", resp.StatusCode),
			slog.Any("error", err),
		)
		return nil, err
	}
	c.info("http response",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.Int("status", resp.StatusCode),
		slog.Duration("duration", time.Since(start)),
		slog.String("body", string(body)),
	)

	if resp.StatusCode == http.StatusAccepted && req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/result") {
		pendingMessage := decodePendingMessage(body)
		return nil, &ResultNotReadyError{RetryAfter: resp.Header.Get("Retry-After"), Message: pendingMessage}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp.StatusCode, body)
	}

	return body, nil
}

func (c *Client) info(message string, attrs ...slog.Attr) {
	if c.logger == nil {
		return
	}
	c.logger.Info(message, attrs...)
}

func (c *Client) error(message string, attrs ...slog.Attr) {
	if c.logger == nil {
		return
	}
	c.logger.Error(message, attrs...)
}

func decodeData(body []byte, output any) error {
	var envelope struct {
		Data struct {
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode response envelope: %w", err)
	}
	if len(envelope.Data.Attributes) == 0 {
		return fmt.Errorf("response missing data.attributes")
	}
	if err := json.Unmarshal(envelope.Data.Attributes, output); err != nil {
		return fmt.Errorf("decode response attributes: %w", err)
	}
	switch typed := output.(type) {
	case *Operation:
		if typed.MetricSyncID == "" {
			typed.MetricSyncID = envelope.Data.ID
		}
	case *Result:
		if typed.MetricSyncID == "" {
			typed.MetricSyncID = envelope.Data.ID
		}
	}
	return nil
}

func decodeWarehouseConnections(body []byte) ([]WarehouseConnection, error) {
	var envelope struct {
		Data []struct {
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode response envelope: %w", err)
	}
	connections := make([]WarehouseConnection, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		var connection WarehouseConnection
		if len(item.Attributes) > 0 {
			if err := json.Unmarshal(item.Attributes, &connection); err != nil {
				return nil, fmt.Errorf("decode warehouse connection attributes: %w", err)
			}
		}
		if connection.ID == "" {
			connection.ID = item.ID
		}
		connections = append(connections, connection)
	}
	return connections, nil
}

func decodeAPIError(statusCode int, body []byte) error {
	if strings.TrimSpace(string(body)) == "" {
		return &APIError{StatusCode: statusCode}
	}
	var errorEnvelope struct {
		Errors []ErrorItem `json:"errors"`
		Data   struct {
			Attributes struct {
				Errors []Issue `json:"errors"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &errorEnvelope); err != nil {
		return &APIError{StatusCode: statusCode, Errors: []ErrorItem{{Title: string(body)}}}
	}
	if len(errorEnvelope.Data.Attributes.Errors) > 0 {
		items := make([]ErrorItem, 0, len(errorEnvelope.Data.Attributes.Errors))
		for _, issue := range errorEnvelope.Data.Attributes.Errors {
			items = append(items, ErrorItem{
				Status: strconv.Itoa(statusCode),
				Title:  issue.Message,
				Detail: issue.Remediation,
			})
		}
		return &APIError{StatusCode: statusCode, Errors: items}
	}
	if len(errorEnvelope.Errors) == 0 {
		return &APIError{StatusCode: statusCode}
	}
	return &APIError{StatusCode: statusCode, Errors: errorEnvelope.Errors}
}

func decodePendingMessage(body []byte) string {
	var envelope struct {
		Data struct {
			Attributes struct {
				Message string `json:"message"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Data.Attributes.Message
}
