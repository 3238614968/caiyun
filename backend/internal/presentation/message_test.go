package presentation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageHidesDiagnosticsAndPreservesBusinessResult(t *testing.T) {
	const solver = "滑块识别offset=376 confidence=0.8511 method=内置多特征NCC peak_margin=0.5681 solver=builtin solve_ms=93.76 session=bound-h5 资格 exchangeCode=0"
	const build = "build=20261112-085058-current commit=ded8d5487b04-dirty built_at=2026-11-12T00:50:58Z"
	tests := []struct {
		name, taskType, raw, want string
	}{
		{"reported detail", "exchange", "您的云朵数不足；" + solver + "；" + build, "云朵不足，兑换失败"},
		{"reported notification", "exchange_complete", `小海 · QQ音乐绿钻会员月卡：您的云朵数不足 | http_status=200 | code=1001 | body={"code":1001,"msg":"您的云朵数不足"} | ` + solver + " | " + build, "小海 · QQ音乐绿钻会员月卡：云朵不足，兑换失败"},
		{"structured history", "exchange", "商品: QQ音乐绿钻会员月卡 | 兑换账号: 小海 | 结果: 您的云朵数不足；" + solver, "商品: QQ音乐绿钻会员月卡 | 兑换账号: 小海 | 结果: 云朵不足，兑换失败"},
		{"success", "exchange", "兑换成功，" + solver + " | " + build, "兑换成功"},
		{"sold out", "exchange", `奖品单日已耗尽 | http_status=200 | code=412 | trace_id=trace-123 | body={}`, "库存已抢完"},
		{"eligibility action", "exchange", "兑换资格校验未通过：exchangeCode=2303（需开启自动备份） | " + build, "兑换条件未满足：需开启自动备份"},
		{"gateway refusal", "exchange", "活动异常，请稍后重试！Error Code：GK | http_status=200 | code=610 | body={}", "活动验证未通过，请稍后重试"},
		{"gateway code only", "exchange", "code=610 | body={}", "活动验证未通过，请稍后重试"},
		{"solver is not success", "exchange", solver + " | " + build, "执行结果待确认"},
		{"diagnostics only", "exchange", "http_status=200 | code=0 | body={}", "执行结果待确认"},
		{"empty pending task", "exchange", "", ""},
		{"account session expired", "exchange", "账号已失效，请重新登录 | http_status=401 | " + build, "登录状态已失效，请重新登录"},
		{"captcha failure", "exchange", "滑块验证码识别失败：滑块识别置信度不足，未提交兑换", "验证未通过，请重试"},
		{"network error hides request", "exchange", `兑换请求失败：Post "https://example.invalid/?jwtToken=fixture": context deadline exceeded | ` + build, "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"},
		{"uncertain result", "exchange", "兑换结果待确认，请勿重复提交 | http_status=502", "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"},
		{"response parsing error", "exchange", "解析响应失败：unexpected EOF | body=<html>", "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"},
		{"no auth false positive", "exchange", `您的云朵数不足 | body={"token":"fixture"}`, "云朵不足，兑换失败"},
		{"body cannot inject visible result", "exchange", `您的云朵数不足 | body={"msg":"兑换成功 | 已完成","code":0} | ` + solver, "云朵不足，兑换失败"},
		{"partial task progress", "coc3", "COC3：服务端完成记录 0/3，抽奖次数未减少 | " + build, "COC3：服务端完成记录 0/3，抽奖次数未减少"},
		{"ordinary task result", "sign_in_3", "小云互动礼已完成，获得 20 云朵 | " + build, "小云互动礼已完成，获得 20 云朵"},
		{"product context is not a result", "exchange", "商品: 云朵不足补偿礼包 | 兑换账号: 小海 | 结果: 兑换成功", "商品: 云朵不足补偿礼包 | 兑换账号: 小海 | 结果: 兑换成功"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Message(tt.taskType, tt.raw)
			if got != tt.want {
				t.Fatalf("display = %q, want %q", got, tt.want)
			}
			if twice := Message(tt.taskType, got); twice != got {
				t.Fatalf("formatting a replay changed result: %q -> %q", got, twice)
			}
		})
	}
}

func TestMessageJSONPreservesNumbersStatusAndNonMessageFields(t *testing.T) {
	raw := json.RawMessage(`{"task_type":"exchange","id":9007199254740993,"success":false,"status":"failed","retry_count":2,"product_name":"QQ音乐绿钻会员月卡","message":"您的云朵数不足；滑块识别offset=376 confidence=0.8511；build=current","items":[{"last_result":"奖品已兑完 | code=412 | body={}"}],"error":{"code":1001}}`)
	before := string(raw)
	cleaned, err := MessageJSON("notification", raw)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID          uint64 `json:"id"`
		Success     bool   `json:"success"`
		Status      string `json:"status"`
		RetryCount  int    `json:"retry_count"`
		ProductName string `json:"product_name"`
		Message     string `json:"message"`
		Items       []struct {
			LastResult string `json:"last_result"`
		} `json:"items"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(cleaned, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 9007199254740993 || got.Success || got.Status != "failed" || got.RetryCount != 2 || got.Error.Code != 1001 || got.ProductName != "QQ音乐绿钻会员月卡" {
		t.Fatalf("non-message event fields changed: %s", cleaned)
	}
	if got.Message != "云朵不足，兑换失败" || len(got.Items) != 1 || got.Items[0].LastResult != "库存已抢完" {
		t.Fatalf("diagnostics remain in event: %s", cleaned)
	}
	if string(raw) != before {
		t.Fatal("stored/raw event was mutated")
	}
	for _, invalid := range []json.RawMessage{[]byte(`{"message":`), []byte(`[{`)} {
		if _, err := MessageJSON("exchange", invalid); err == nil {
			t.Fatal("invalid event JSON accepted")
		}
	}
}

func TestMessageJSONKeepsNonDiagnosticOperationEvents(t *testing.T) {
	input := json.RawMessage(`{"operation_id":"op-42","status":"running","error_summary":"","progress":{"completed":1,"total":3,"message":"正在执行第 2 项"}}`)
	cleaned, err := MessageJSON("operation.updated", input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cleaned), `"error_summary":""`) || !strings.Contains(string(cleaned), "正在执行第 2 项") {
		t.Fatalf("operation progress changed: %s", cleaned)
	}
}

func TestMessageJSONKeepsNullFieldsAndExchangeEventContext(t *testing.T) {
	input := json.RawMessage(`{"task_type":"once","success":false,"message":"您的云朵数不足 | code=1001","last_result":null,"skip_reason":null,"error_summary":null}`)
	cleaned, err := MessageJSON("exchange_complete", input)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(cleaned, &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"last_result", "skip_reason", "error_summary"} {
		if string(result[field]) != "null" {
			t.Fatalf("nullable field %s changed: %s", field, cleaned)
		}
	}
	var message string
	if err := json.Unmarshal(result["message"], &message); err != nil || message != "云朵不足，兑换失败" {
		t.Fatalf("exchange event lost its context: %s", cleaned)
	}
}
