package avatar

import (
	"context"
	"reflect"
	"testing"
)

type processorStub struct {
	calls []string
}

func (s *processorStub) Upload(_ context.Context, _ string, _ []byte, requestID string) (string, error) {
	s.calls = append(s.calls, "upload:"+requestID)
	return "uploaded-image", nil
}

func (s *processorStub) SmartCrop(_ context.Context, image, requestID string) (string, error) {
	s.calls = append(s.calls, "crop:"+image+":"+requestID)
	return "cropped-image", nil
}

func (s *processorStub) Optimize(_ context.Context, image, requestID string) (string, error) {
	s.calls = append(s.calls, "optimize:"+image+":"+requestID)
	return "optimized-image", nil
}

func (s *processorStub) UpdateUserAvatar(_ context.Context, userID, image, requestID string) error {
	s.calls = append(s.calls, "update:"+userID+":"+image+":"+requestID)
	return nil
}

func TestProcessRecordsAvatarAndSelectsAuditNotifications(t *testing.T) {
	tests := []struct {
		name       string
		event      PaymentEvent
		riskAction string
		categories []string
	}{
		{
			name:       "settled payment needs no notice",
			event:      PaymentEvent{ID: "evt-1", Type: "payment.settled", Amount: 2500},
			categories: nil,
		},
		{
			name:       "failed payment is audit visible",
			event:      PaymentEvent{ID: "evt-2", Type: "payment.failed", Amount: 2500},
			categories: []string{"payment-review"},
		},
		{
			name:       "reversal plus account lock records both decisions",
			event:      PaymentEvent{ID: "evt-3", Type: "payment.reversed", Amount: 2500},
			riskAction: "account-lock",
			categories: []string{"payment-review", "risk-action"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &processorStub{}
			result, err := NewService(stub).Process(context.Background(), "usr-42", "avatar.png", []byte("image"), "req-7", tt.event, tt.riskAction)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			if result.State != "recorded" || result.AvatarImageID != "optimized-image" {
				t.Fatalf("unexpected result: %+v", result)
			}
			var got []string
			for _, notice := range result.Notifications {
				got = append(got, notice.Category)
			}
			if !reflect.DeepEqual(got, tt.categories) {
				t.Fatalf("notification categories = %v, want %v", got, tt.categories)
			}
			wantCalls := []string{
				"upload:req-7:upload",
				"crop:uploaded-image:req-7:crop",
				"optimize:cropped-image:req-7:optimize",
				"update:usr-42:optimized-image:req-7:user-update",
			}
			if !reflect.DeepEqual(stub.calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", stub.calls, wantCalls)
			}
		})
	}
}
