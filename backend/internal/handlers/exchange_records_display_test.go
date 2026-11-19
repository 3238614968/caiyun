package handlers

import (
	"bytes"
	"encoding/csv"
	"reflect"
	"testing"
	"time"

	"caiyun/internal/dto"
	"caiyun/internal/models"
)

func TestExchangeCSVExportsBusinessResultAndPreservesFormulaEscaping(t *testing.T) {
	record := &models.ExchangeRecord{
		ID: 7, UserID: 8, ExchangeAccountID: 9, ProductID: 10,
		PrizeName: "=商品名称", Status: "failed", ExecutionTimeMs: 94,
		Message:   "您的云朵数不足；滑块识别offset=376 confidence=0.8511；build=current",
		CreatedAt: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC),
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(exchangeRecordCSVRow(dto.ToExchangeRecordResponse(record))); err != nil {
		t.Fatal(err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	got, err := csv.NewReader(&buffer).Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"7", "8", "9", "10", "'=商品名称", "failed", "云朵不足，兑换失败", "94", "2026-11-12 09:00:00"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CSV row = %#v, want %#v", got, want)
	}
}
