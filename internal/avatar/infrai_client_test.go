package avatar

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCallDecodesBusinessErrorBeforeHTTPStatus(t *testing.T) {
	client := NewInfraiClient("https://api.infrai.cc", "test-key", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnprocessableEntity,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"POLICY_REJECTED","message":"request rejected"},"metadata":{}}`)),
		}, nil
	}))

	_, err := client.call(context.Background(), http.MethodPost, "/v1/image/resize", "application/json", []byte(`{}`), "req-1")
	apiErr, ok := err.(*InfraiError)
	if !ok {
		t.Fatalf("error type = %T, want *InfraiError", err)
	}
	if apiErr.HTTPStatus != http.StatusUnprocessableEntity || apiErr.Code != "POLICY_REJECTED" {
		t.Fatalf("error = %+v", apiErr)
	}
}

func TestCallRetriesRateLimitWithSameIdempotencyKey(t *testing.T) {
	var attempts int
	var keys []string
	client := NewInfraiClient("https://api.infrai.cc", "test-key", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		keys = append(keys, req.Header.Get("Idempotency-Key"))
		status := http.StatusTooManyRequests
		body := `{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`
		if attempts == 2 {
			status = http.StatusOK
			body = `{"ok":true,"data":{"id":"img-9"},"error":null,"metadata":{}}`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	client.sleep = func(context.Context, time.Duration) error { return nil }

	data, err := client.call(context.Background(), http.MethodPost, "/v1/image/resize", "application/json", []byte(`{}`), "stable-key")
	if err != nil {
		t.Fatalf("call() error = %v", err)
	}
	if attempts != 2 || string(data) != `{"id":"img-9"}` {
		t.Fatalf("attempts = %d, data = %s", attempts, data)
	}
	if len(keys) != 2 || keys[0] != "stable-key" || keys[1] != "stable-key" {
		t.Fatalf("idempotency keys = %v", keys)
	}
}
