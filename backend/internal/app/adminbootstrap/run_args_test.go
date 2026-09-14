package adminbootstrap

import (
	"context"
	"testing"
)

func TestRunRejectsPositionArgs(t *testing.T) {
	if err := Run(context.Background(), []string{"unexpected"}); err == nil {
		t.Fatal("expected bootstrap-admin to reject positional arguments")
	}
}
