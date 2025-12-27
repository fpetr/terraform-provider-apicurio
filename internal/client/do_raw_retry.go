package client

import (
	"context"
	"io"
	"net/http"
)

type replayableBody struct {
	ra io.ReaderAt
	sz int64
}

func newReplayableBody(body io.Reader) (*replayableBody, bool) {
	if body == nil {
		return &replayableBody{ra: nil, sz: 0}, true
	}
	// Only retry when we can reliably replay the body without consuming it.
	rs, ok := body.(io.ReadSeeker)
	if !ok {
		return nil, false
	}
	ra, ok := body.(io.ReaderAt)
	if !ok {
		return nil, false
	}
	cur, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, false
	}
	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		_, _ = rs.Seek(cur, io.SeekStart)
		return nil, false
	}
	_, _ = rs.Seek(cur, io.SeekStart)
	return &replayableBody{ra: ra, sz: end}, true
}

func (b *replayableBody) reader() io.Reader {
	if b == nil {
		return nil
	}
	if b.ra == nil {
		return nil
	}
	return io.NewSectionReader(b.ra, 0, b.sz)
}

func doRawWithDeadlockRetry(ctx context.Context, httpClient *http.Client, cfg ClientConfig, method, urlStr string, headers map[string]string, body io.Reader) (*rawResp, *ResponseError) {
	replay, replayable := newReplayableBody(body)
	if !replayable {
		// Not safely replayable -> do a single attempt without retry.
		req, rerr := http.NewRequestWithContext(ctx, method, urlStr, body)
		if rerr != nil {
			return nil, &ResponseError{Err: rerr}
		}
		applyAuth(req, cfg)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, rerr2 := httpClient.Do(req)
		if rerr2 != nil {
			return nil, &ResponseError{Err: rerr2}
		}
		b, _ := ReadBodyLimited(resp.Body)
		_ = resp.Body.Close()
		return &rawResp{StatusCode: resp.StatusCode, Body: b, Header: resp.Header}, nil
	}

	policy := defaultDeadlockRetryPolicy()
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		req, rerr := http.NewRequestWithContext(ctx, method, urlStr, replay.reader())
		if rerr != nil {
			return nil, &ResponseError{Err: rerr}
		}
		applyAuth(req, cfg)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, rerr := httpClient.Do(req)
		if rerr != nil {
			return nil, &ResponseError{Err: rerr}
		}
		b, _ := ReadBodyLimited(resp.Body)
		_ = resp.Body.Close()
		r := &rawResp{StatusCode: resp.StatusCode, Body: b, Header: resp.Header}
		if !isRetryableDeadlock(r.StatusCode, r.Body) {
			return r, nil
		}
		if attempt == policy.MaxAttempts-1 {
			return r, nil
		}
		if serr := sleepWithContext(ctx, deadlockRetryDelay(policy, attempt)); serr != nil {
			return nil, &ResponseError{Err: serr}
		}
	}

	return nil, &ResponseError{Err: context.Canceled}
}
