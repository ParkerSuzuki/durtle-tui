package wanikani

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL+"/", "tok123")
}

func TestHeadersAndDecode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Wanikani-Revision"); got != "20170710" {
			t.Errorf("Wanikani-Revision = %q", got)
		}
		fmt.Fprint(w, `{"data":{"username":"durtle","level":5}}`)
	})
	var r Resource[User]
	if err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, &r); err != nil {
		t.Fatal(err)
	}
	if r.Data.Username != "durtle" || r.Data.Level != 5 {
		t.Errorf("decoded %+v", r.Data)
	}
}

func TestUnauthorized(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"error":"not available"}`)
	})
	err := c.do(context.Background(), http.MethodPost, c.base+"reviews", map[string]int{"x": 1}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 422 {
		t.Fatalf("err = %v, want *APIError with status 422", err)
	}
}

func TestRetriesAfterRateLimit(t *testing.T) {
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("RateLimit-Reset", fmt.Sprint(time.Now().Add(-time.Second).Unix()))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{}`)
	})
	if err := c.do(context.Background(), http.MethodGet, c.base+"user", nil, nil); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
}
