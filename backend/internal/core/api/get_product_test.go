package api

import (
	"context"
	"errors"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestGetProductListContextStopsBeforeNetworkWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := corehttp.NewClient()
	_, err := NewCaiyunAPI(client).GetProductListContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetProductListContext() error = %v, want context.Canceled", err)
	}
}
