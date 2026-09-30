// Package wanikani is a minimal client for the parts of the WaniKani API v2
// that durtle-tui uses.
package wanikani

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// BaseURL is the production API root.
const BaseURL = "https://api.wanikani.com/v2/"

const maxRateLimitRetries = 3

// ErrUnauthorized means WaniKani rejected the token (HTTP 401).
var ErrUnauthorized = errors.New("wanikani: API token was rejected")

// APIError is any other non-2xx response.
type APIError struct {
	Status      int
	Method, URL string
	Body        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wanikani: %s %s: HTTP %d: %s", e.Method, e.URL, e.Status, e.Body)
}

// Client talks to the WaniKani API with one personal access token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for baseURL (normally BaseURL; tests pass a local server).
func NewClient(baseURL, token string) *Client {
	// No client-wide timeout: every call carries a context deadline, and a
	// fixed 30 s limit also cut off large pages on slow connections.
	return &Client{base: baseURL, token: token, http: &http.Client{}}
}

// do sends one request, waiting and retrying while rate limited, and decodes
// the JSON response into out unless out is nil.
func (c *Client) do(ctx context.Context, method, url string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Wanikani-Revision", "20170710")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRateLimitRetries {
			resp.Body.Close()
			if err := waitUntil(ctx, resetTime(resp.Header)); err != nil {
				return err
			}
			continue
		}
		return decode(resp, method, url, out)
	}
}

func decode(resp *http.Response, method, url string, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &APIError{Status: resp.StatusCode, Method: method, URL: url, Body: string(bytes.TrimSpace(msg))}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("wanikani: decoding %s: %w", url, err)
	}
	return nil
}

// resetTime reads when the rate limit window resets. Without the header,
// wait a few seconds and try again.
func resetTime(h http.Header) time.Time {
	secs, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64)
	if err != nil {
		return time.Now().Add(5 * time.Second)
	}
	return time.Unix(secs, 0)
}

// waitUntil blocks until t or until ctx is cancelled, whichever comes first.
func waitUntil(ctx context.Context, t time.Time) error {
	timer := time.NewTimer(time.Until(t)) // a past time fires immediately
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
