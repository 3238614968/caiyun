package errors

import (
	stderrors "errors"
	"testing"
)

func TestAppErrorWrapReturnsNewInstance(t *testing.T) {
	base := NewWithDetails(CodeBadRequest, "参数错误", "phone required")
	causeA := stderrors.New("a")
	causeB := stderrors.New("b")

	wrappedA := base.Wrap(causeA)
	wrappedB := base.Wrap(causeB)

	if wrappedA == wrappedB {
		t.Fatal("Wrap should return a new error instance")
	}
	if !stderrors.Is(wrappedA, causeA) {
		t.Fatal("wrappedA should unwrap causeA")
	}
	if !stderrors.Is(wrappedB, causeB) {
		t.Fatal("wrappedB should unwrap causeB")
	}
	if stderrors.Is(wrappedA, causeB) {
		t.Fatal("wrappedA should not be mutated by later Wrap calls")
	}
}
