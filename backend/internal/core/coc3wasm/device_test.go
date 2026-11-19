package coc3wasm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDeviceTokenExchangeUsesOfficialWASMAndRejectsUnconfirmedResponse(t *testing.T) {
	data, err := os.ReadFile("testdata/device-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		WasmSHA256    string `json:"wasmSHA256"`
		Clock         int64  `json:"clock"`
		BusinessToken string `json:"businessToken"`
		Request       struct {
			URL, Method, Body string
			Headers           map[string]string
		} `json:"request"`
		Response struct {
			Status int
			Body   string
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	wasm := wheelFixture(t)
	if fmt.Sprintf("%x", sha256.Sum256(wasm)) != reference.WasmSHA256 {
		t.Fatal("official reference module changed")
	}
	entropy := &sequenceReader{}
	requests := 0
	device := &DeviceContext{BusinessToken: reference.BusinessToken, Exchange: func(ctx context.Context, r DeviceRequest) (DeviceResponse, error) {
		requests++
		if r.URL != reference.Request.URL || r.Method != reference.Request.Method {
			t.Fatal("device exchange target changed")
		}
		if r.Body == nil || string(r.Body) != reference.Request.Body || r.Headers["coc-aurora"] != reference.Request.Headers["coc-aurora"] {
			t.Errorf("official device request mismatch: body=%v header=%v bytes=%d/%d", string(r.Body) == reference.Request.Body, r.Headers["coc-aurora"] == reference.Request.Headers["coc-aurora"], len(r.Body), len(reference.Request.Body))
		}
		return DeviceResponse{Status: reference.Response.Status, Body: []byte(reference.Response.Body)}, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	env := environment{now: func() time.Time { return time.UnixMilli(reference.Clock).In(time.FixedZone("CST", 8*3600)) }, random: entropy, device: device}
	value, err := generate(ctx, wasm, env)
	if err == nil || value != "" || requests != 1 || !strings.Contains(err.Error(), "设备上报未确认") {
		t.Fatalf("requests=%d entropy=%d err=%v", requests, entropy.count, err)
	}
}

func TestDeviceFailuresNeverYieldLotteryParams(t *testing.T) {
	for _, scenario := range []string{"network", "http", "malformed", "business", "oversized", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			requests := 0
			value, err := GenerateWithDevice(ctx, wheelFixture(t), DeviceContext{BusinessToken: "fixture-business-token", Exchange: func(ctx context.Context, r DeviceRequest) (DeviceResponse, error) {
				requests++
				if r.URL != DeviceTokenURL {
					t.Fatal("unrestricted device target")
				}
				response := DeviceResponse{Status: 200, Body: []byte(`{"code":"0","success":true,"data":"fixture-device-token"}`)}
				switch scenario {
				case "network":
					return DeviceResponse{}, errors.New("transport error")
				case "http":
					response.Status = 503
				case "malformed":
					response.Body = []byte(`{"code":`)
				case "business":
					response.Body = []byte(`{"code":"-1","success":false,"data":"fixture-device-token"}`)
				case "oversized":
					response.Body = make([]byte, (64<<10)+1)
				case "cancel":
					cancel()
					return DeviceResponse{}, ctx.Err()
				}
				return response, nil
			}})
			if err == nil || value != "" || requests != 1 {
				t.Fatalf("requests=%d params=%v err=%v", requests, value != "", err)
			}
		})
	}
}

func TestDeviceConfirmationRequiresOfficialWrapperSemantics(t *testing.T) {
	for _, value := range []interface{}{nil, undefined{}, false, "", math.NaN()} {
		if deviceTokenConfirmed(value) {
			t.Fatal("empty device result confirmed")
		}
	}
	for _, value := range []interface{}{float64(0), float64(1), true, "fixture"} {
		if !deviceTokenConfirmed(value) {
			t.Fatal("official accepted result rejected")
		}
	}
	for _, value := range []DeviceContext{{BusinessToken: "fixture"}, {BusinessToken: "bad;cookie", Exchange: func(context.Context, DeviceRequest) (DeviceResponse, error) {
		t.Fatal("invalid context performed network exchange")
		return DeviceResponse{}, nil
	}}} {
		if params, err := GenerateWithDevice(context.Background(), wheelFixture(t), value); err == nil || params != "" {
			t.Fatal("invalid device context accepted")
		}
	}
}

func TestDevicePreflightAndLotteryParamsMatchOfficialReference(t *testing.T) {
	data, err := os.ReadFile("testdata/device-confirmed-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Clock         int64
		BusinessToken string
		Params        []string
		Response      struct {
			Status int
			Body   string
		}
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	exchanges := 0
	device := &DeviceContext{BusinessToken: reference.BusinessToken, Exchange: func(context.Context, DeviceRequest) (DeviceResponse, error) {
		exchanges++
		return DeviceResponse{Status: reference.Response.Status, Body: []byte(reference.Response.Body)}, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	value, err := generate(ctx, wheelFixture(t), environment{now: func() time.Time { return time.UnixMilli(reference.Clock).In(time.FixedZone("CST", 8*3600)) }, random: &sequenceReader{}, device: device})
	var params []string
	if err != nil || json.Unmarshal([]byte(value), &params) != nil || len(params) != 2 || exchanges != 1 {
		t.Fatalf("exchanges=%d err=%v", exchanges, err)
	}
	if params[0] != reference.Params[0] || params[1] != reference.Params[1] {
		t.Fatal("post-preflight parameters differ from the official JS reference")
	}
}
