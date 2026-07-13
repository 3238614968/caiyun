package sms

import (
	"context"
	"errors"
	"testing"
)

func TestSMSContextAPIsStopBeforeNetworkWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := SendCodeContext(ctx, "13800138000"); !errors.Is(err, context.Canceled) {
		t.Fatalf("SendCodeContext() error = %v, want context.Canceled", err)
	}
	if _, err := GetCodeStatusContext(ctx, "13800138000"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetCodeStatusContext() error = %v, want context.Canceled", err)
	}
	if _, err := VerifyCodeContext(ctx, "13800138000", "123456", "task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("VerifyCodeContext() error = %v, want context.Canceled", err)
	}
	if _, err := SolveSlideContext(ctx, "puzzle", "picture"); !errors.Is(err, context.Canceled) {
		t.Fatalf("SolveSlideContext() error = %v, want context.Canceled", err)
	}
}
