package distance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type httpStatusError struct {
	Code int
	Body string
}

func (o *ORSDistanceProvider) newRequest(
	ctx context.Context,
	method string,
	url string,
	body io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", o.apiKey)
	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (o *ORSDistanceProvider) do(req *http.Request) (*http.Response, error) {
	resp, err := o.session.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &httpStatusError{
			Code: resp.StatusCode,
			Body: strings.TrimSpace(string(b)),
		}
	}
	return resp, nil
}

// doWithRetry retires transient failures (network errors, 5xx responses)
// using exponential backoff while respecting context cancellation.
func (o *ORSDistanceProvider) doWithRetry(
	ctx context.Context,
	makeReq func() (*http.Request, error),
) (*http.Response, error) {
	const maxAttepts = 4
	backoff := 200 * time.Millisecond

	var lastErr error

	for attempt := 1; attempt <= maxAttepts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		req, err := makeReq()
		if err != nil {
			return nil, fmt.Errorf("make request: %w", err)
		}

		resp, err := o.do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		retry := false
		var he *httpStatusError
		if errors.As(err, &he) {
			switch he.Code {
			case 429, 500, 502, 503, 504:
				retry = true
			}
		}

		var netErr net.Error
		if !retry && errors.As(err, &netErr) {
			retry = true
		}

		if !retry || attempt == maxAttepts {
			return nil, lastErr
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		backoff *= 2
	}

	return nil, lastErr
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("Code %d: %s", e.Code, e.Body)
}
