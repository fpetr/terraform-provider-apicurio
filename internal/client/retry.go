package client

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type deadlockRetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func defaultDeadlockRetryPolicy() deadlockRetryPolicy {
	return deadlockRetryPolicy{
		MaxAttempts: 8,
		BaseDelay:   150 * time.Millisecond,
		MaxDelay:    2 * time.Second,
	}
}

var (
	retryRandMu sync.Mutex
	retryRand   = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func deadlockRetryDelay(p deadlockRetryPolicy, attempt int) time.Duration {
	// attempt is 0-based for the first failure.
	base := float64(p.BaseDelay)
	exp := base * math.Pow(2, float64(attempt))
	d := time.Duration(exp)
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	// Add up to 100ms jitter to reduce thundering herd.
	retryRandMu.Lock()
	j := time.Duration(retryRand.Int63n(int64(100 * time.Millisecond)))
	retryRandMu.Unlock()
	return d + j
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isRetryableDeadlock(statusCode int, body string) bool {
	// Keep this strict: only retry when it very clearly looks like a transient DB deadlock.
	if statusCode < 500 || statusCode >= 600 {
		return false
	}

	trim := strings.TrimSpace(body)
	if trim == "" {
		return false
	}

	lower := strings.ToLower(trim)
	if strings.Contains(lower, "deadlock found") ||
		strings.Contains(lower, "mysqltransactionrollbackexception") ||
		strings.Contains(lower, "try restarting transaction") ||
		strings.Contains(lower, "deadlock detected") ||
		strings.Contains(lower, "could not serialize access") ||
		strings.Contains(lower, "serialization failure") ||
		strings.Contains(lower, "sqlstate 40p01") ||
		strings.Contains(lower, "sqlstate 40001") {
		return true
	}

	// Best-effort: Apicurio often returns JSON like:
	// {"message": "...", "detail": "...", "name": "RuntimeSqlException"}
	var obj struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal([]byte(trim), &obj); err != nil {
		return false
	}

	msg := strings.ToLower(obj.Message + " " + obj.Detail + " " + obj.Name)
	return strings.Contains(msg, "deadlock") ||
		strings.Contains(msg, "mysqltransactionrollbackexception") ||
		strings.Contains(msg, "could not serialize access") ||
		strings.Contains(msg, "serialization failure") ||
		strings.Contains(msg, "40p01") ||
		strings.Contains(msg, "40001")
}
