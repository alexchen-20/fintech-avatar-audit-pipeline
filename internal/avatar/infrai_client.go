package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxAttempts = 4

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type InfraiClient struct {
	baseURL string
	key     string
	http    HTTPDoer
	sleep   func(context.Context, time.Duration) error
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewInfraiClient(baseURL, key string, httpClient HTTPDoer) *InfraiClient {
	return &InfraiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *InfraiClient) Upload(ctx context.Context, filename string, contents []byte, requestID string) (string, error) {
	return c.multipartImage(ctx, http.MethodPost, "/v1/image/upload", requestID, func(w *multipart.Writer) error {
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return err
		}
		if _, err := part.Write(contents); err != nil {
			return err
		}
		return w.WriteField("filename", filename)
	})
}

func (c *InfraiClient) SmartCrop(ctx context.Context, image, requestID string) (string, error) {
	return c.multipartImage(ctx, http.MethodPost, "/v1/image/smart_crop", requestID, func(w *multipart.Writer) error {
		if err := w.WriteField("image", image); err != nil {
			return err
		}
		return w.WriteField("aspect", "1:1")
	})
}

func (c *InfraiClient) Optimize(ctx context.Context, image, requestID string) (string, error) {
	return c.multipartImage(ctx, http.MethodPost, "/v1/image/resize", requestID, func(w *multipart.Writer) error {
		fields := map[string]string{
			"image":   image,
			"width":   "512",
			"height":  "512",
			"fit":     "cover",
			"enlarge": "false",
			"format":  "webp",
			"store":   "true",
		}
		for name, value := range fields {
			if err := w.WriteField(name, value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *InfraiClient) UpdateUserAvatar(ctx context.Context, userID, image, requestID string) error {
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]string{"avatar_image_id": image},
	})
	if err != nil {
		return err
	}
	path := "/v1/auth/user/update/" + url.PathEscape(userID)
	_, err = c.call(ctx, http.MethodPatch, path, "application/json", body, requestID)
	return err
}

func (c *InfraiClient) multipartImage(ctx context.Context, method, path, requestID string, write func(*multipart.Writer) error) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := write(writer); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	data, err := c.call(ctx, method, path, writer.FormDataContentType(), body.Bytes(), requestID)
	if err != nil {
		return "", err
	}
	return imageReference(data)
}

func imageReference(data json.RawMessage) (string, error) {
	var value struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("decode image data: %w", err)
	}
	if value.ID == "" {
		return "", errors.New("image response did not include an id")
	}
	return value.ID, nil
}

func (c *InfraiClient) call(ctx context.Context, method, path, contentType string, body []byte, requestID string) (json.RawMessage, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Idempotency-Key", requestID)

		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send Infrai request: %w", err)
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(payload, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt+1 < maxAttempts {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", decodeErr)
		}
		if !env.OK {
			apiErr := &InfraiError{HTTPStatus: res.StatusCode, Message: http.StatusText(res.StatusCode)}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return nil, apiErr
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, &InfraiError{HTTPStatus: res.StatusCode, Message: "upstream service error"}
		}
		return env.Data, nil
	}
	return nil, &InfraiError{HTTPStatus: http.StatusTooManyRequests, Message: "request rate exceeded"}
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
