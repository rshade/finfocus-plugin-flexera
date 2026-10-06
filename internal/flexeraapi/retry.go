package flexeraapi

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultMaxAttempts   = 3
	defaultBaseDelay     = 200 * time.Millisecond
	defaultMaxDelay      = 2 * time.Second
	defaultMaxRetryAfter = 5 * time.Second
	jitterFraction       = 4
	httpStatusMax        = 599
)

// retryPolicy controls retries of HTTP 429 and 5xx responses.
type retryPolicy struct {
	maxAttempts   int
	baseDelay     time.Duration
	maxDelay      time.Duration
	maxRetryAfter time.Duration
	// jitterN, when set, replaces the global random source. n is exclusive.
	jitterN func(n int64) int64
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		maxAttempts:   defaultMaxAttempts,
		baseDelay:     defaultBaseDelay,
		maxDelay:      defaultMaxDelay,
		maxRetryAfter: defaultMaxRetryAfter,
	}
}

func (c *clientImpl) retryPolicy() retryPolicy {
	if c == nil || c.retry.maxAttempts <= 0 {
		return defaultRetryPolicy()
	}
	return c.retry
}

func retryableStatus(code int) bool {
	if code == http.StatusTooManyRequests {
		return true
	}
	return code >= http.StatusInternalServerError && code <= httpStatusMax
}

func doWithRetry[T any](
	ctx context.Context,
	policy retryPolicy,
	call func(context.Context) (T, int, http.Header, error),
) (T, error) {
	var zero T
	attempts := policy.maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		value, status, header, err := call(ctx)
		if err != nil || !retryableStatus(status) || attempt == attempts {
			return value, err
		}
		if sleepErr := sleepCtx(ctx, retryDelay(header, attempt, policy)); sleepErr != nil {
			return zero, sleepErr
		}
	}
	return zero, nil
}

func retryDelay(header http.Header, attempt int, policy retryPolicy) time.Duration {
	if header != nil {
		if delay, ok := parseRetryAfter(header.Get("Retry-After")); ok {
			return capDelay(delay, policy.maxRetryAfter)
		}
	}
	delay := policy.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if policy.maxDelay > 0 && delay > policy.maxDelay {
			delay = policy.maxDelay
			break
		}
	}
	return applyJitter(delay, policy)
}

func capDelay(delay, maxDelay time.Duration) time.Duration {
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	if delay < 0 {
		return 0
	}
	return delay
}

func applyJitter(delay time.Duration, policy retryPolicy) time.Duration {
	if delay <= 0 {
		return 0
	}
	span := int64(delay) / jitterFraction
	if span == 0 {
		return delay
	}
	next := policy.jitterN
	if next == nil {
		next = rand.Int64N
	}
	delta := next(span*2+1) - span
	out := time.Duration(int64(delay) + delta)
	if out < 0 {
		return 0
	}
	return out
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = trimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := time.Until(when)
	if delay < 0 {
		return 0, true
	}
	return delay, true
}

func trimSpace(value string) string {
	start := 0
	end := len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t') {
		end--
	}
	return value[start:end]
}

func sleepCtx(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
