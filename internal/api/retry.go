// Unless explicitly stated otherwise all files in this repository are licensed under the Apache License, Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026 Datadog, Inc.

package api

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRateLimitAttempts = 8
	maxRateLimitWait     = 3 * time.Minute
)

func (c *Client) requestWithRetries(req *http.Request) (*http.Response, []byte, error) {
	started := time.Now()
	var waited time.Duration
	for attempt := 1; ; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, nil, err
		}
		if attempt > 1 {
			var err error
			req, err = replayRequest(req)
			if err != nil {
				return nil, nil, err
			}
		}
		resp, body, err := c.requestOnce(req)
		if err != nil || resp.StatusCode != http.StatusTooManyRequests {
			return resp, body, err
		}
		delay := rateLimitDelay(resp.Header, attempt, time.Now())
		remaining := min(maxRateLimitWait-waited, maxRateLimitWait-time.Since(started))
		if attempt >= maxRateLimitAttempts || delay > remaining {
			return nil, nil, fmt.Errorf("HTTP 429 retry budget exhausted after %d attempts (maximum %d attempts, %s waiting): %w", attempt, maxRateLimitAttempts, maxRateLimitWait, decodeAPIError(resp.StatusCode, body))
		}
		c.info("rate limited; retrying", slog.Int("attempt", attempt), slog.Duration("delay", delay))
		sleep := c.retrySleep
		if sleep == nil {
			sleep = waitForRetry
		}
		if err := sleep(req.Context(), delay); err != nil {
			return nil, nil, fmt.Errorf("waiting to retry HTTP 429: %w", err)
		}
		waited += delay
	}
}

func replayRequest(req *http.Request) (*http.Request, error) {
	retry := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody {
		if req.GetBody == nil {
			return nil, fmt.Errorf("cannot retry HTTP 429: request body is not replayable")
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("replay request after HTTP 429: %w", err)
		}
		retry.Body = body
	}
	return retry, nil
}

func rateLimitDelay(headers http.Header, attempt int, now time.Time) time.Duration {
	delay, guided := retryAfterDelay(headers.Get("Retry-After"), now)
	// Datadog defines X-RateLimit-Reset as seconds until the next reset.
	if reset, ok := delaySeconds(headers.Get("X-RateLimit-Reset")); ok {
		delay, guided = max(delay, reset), true
	}
	if guided {
		// Spread shards across the reset boundary without retrying before it.
		return delay + time.Duration(rand.Int64N(int64(time.Second)))
	}
	ceiling := min(2*time.Second<<min(attempt-1, 4), 30*time.Second)
	return ceiling/2 + time.Duration(rand.Int64N(int64(ceiling/2)))
}

func retryAfterDelay(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if delay, ok := delaySeconds(value); ok {
		return delay, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return min(max(date.Sub(now), 0), maxRateLimitWait+time.Second), true
	}
	return 0, false
}

func delaySeconds(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, false
	}
	seconds, err := strconv.ParseUint(value, 10, 64)
	if err != nil || seconds > uint64(maxRateLimitWait/time.Second) {
		// Keep an oversized valid delay above budget, including integer overflow.
		return maxRateLimitWait + time.Second, true
	}
	return time.Duration(seconds) * time.Second, true
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
