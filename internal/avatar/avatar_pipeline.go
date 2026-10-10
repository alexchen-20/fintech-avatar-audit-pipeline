package avatar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxAvatarBytes = 8 << 20

type AvatarProcessor interface {
	Upload(ctx context.Context, filename string, contents []byte, requestID string) (string, error)
	SmartCrop(ctx context.Context, image, requestID string) (string, error)
	Optimize(ctx context.Context, image, requestID string) (string, error)
	UpdateUserAvatar(ctx context.Context, userID, image, requestID string) error
}

type Service struct {
	processor AvatarProcessor
}

type PaymentEvent struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Amount int64  `json:"amount_minor"`
}

type AuditNotification struct {
	Category string `json:"category"`
	Subject  string `json:"subject"`
	EventID  string `json:"event_id,omitempty"`
}

type UploadResult struct {
	UserID        string              `json:"user_id"`
	AvatarImageID string              `json:"avatar_image_id"`
	State         string              `json:"state"`
	Notifications []AuditNotification `json:"notifications"`
}

func NewService(processor AvatarProcessor) *Service {
	return &Service{processor: processor}
}

func (s *Service) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes)
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart request"})
		return
	}

	userID := strings.TrimSpace(r.FormValue("user_id"))
	requestID := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if userID == "" || requestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and Idempotency-Key are required"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read file"})
		return
	}

	event := PaymentEvent{
		ID:   strings.TrimSpace(r.FormValue("payment_event_id")),
		Type: strings.TrimSpace(r.FormValue("payment_event_type")),
	}
	riskAction := strings.TrimSpace(r.FormValue("risk_action"))
	result, err := s.Process(r.Context(), userID, header.Filename, contents, requestID, event, riskAction)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Service) Process(ctx context.Context, userID, filename string, contents []byte, requestID string, event PaymentEvent, riskAction string) (UploadResult, error) {
	uploaded, err := s.processor.Upload(ctx, filename, contents, requestID+":upload")
	if err != nil {
		return UploadResult{}, err
	}
	cropped, err := s.processor.SmartCrop(ctx, uploaded, requestID+":crop")
	if err != nil {
		return UploadResult{}, err
	}
	optimized, err := s.processor.Optimize(ctx, cropped, requestID+":optimize")
	if err != nil {
		return UploadResult{}, err
	}
	if err := s.processor.UpdateUserAvatar(ctx, userID, optimized, requestID+":user-update"); err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		UserID:        userID,
		AvatarImageID: optimized,
		State:         "recorded",
		Notifications: auditNotifications(userID, event, riskAction),
	}, nil
}

func auditNotifications(userID string, event PaymentEvent, riskAction string) []AuditNotification {
	var notices []AuditNotification
	if event.ID != "" && (event.Type == "payment.failed" || event.Type == "payment.reversed") {
		notices = append(notices, AuditNotification{
			Category: "payment-review",
			Subject:  userID,
			EventID:  event.ID,
		})
	}
	if riskAction != "" {
		notices = append(notices, AuditNotification{
			Category: "risk-action",
			Subject:  userID,
		})
	}
	return notices
}

func writeServiceError(w http.ResponseWriter, err error) {
	var apiErr *InfraiError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status := apiErr.HTTPStatus
		if status == http.StatusTooManyRequests {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "avatar processing failed"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("encode response: %v\n", err)
	}
}
