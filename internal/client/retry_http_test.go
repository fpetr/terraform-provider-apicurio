package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRaw_RetriesDeadlock(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"MySQLTransactionRollbackException: Deadlock found when trying to get lock; try restarting transaction"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := &v2Client{endpoint: srv.URL, httpClient: srv.Client(), cfg: ClientConfig{}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, rerr := c.doRaw(ctx, http.MethodDelete, srv.URL+"/whatever", nil, nil)
	if rerr != nil {
		t.Fatalf("unexpected error: %v", rerr)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body=%q)", resp.StatusCode, resp.Body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}
