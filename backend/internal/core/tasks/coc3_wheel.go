package tasks

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"caiyun/internal/core/coc3wasm"
)

//go:embed assets/coc3/wheel_bg.wasm
var coc3WheelWASM []byte

func getCoc3Env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// Preserve the explicit static-header override; otherwise generate a fresh
// header inside Go. No executable, browser or JavaScript file is needed.
func (t *Coc3Task) resolveWasmParams() string {
	if value := getCoc3Env("CAIYUN_COC3_WASM_PARAMS", ""); value != "" {
		return value
	}
	value, err := coc3GenerateWasmParams(getCoc3Env("CAIYUN_COC3_WASM_DIR", ""), coc3wasm.DeviceContext{
		BusinessToken: t.tokenValue,
		Exchange: func(ctx context.Context, request coc3wasm.DeviceRequest) (coc3wasm.DeviceResponse, error) {
			return t.api.ExchangeCoc3DeviceToken(ctx, t.tokenValue, request)
		},
	})
	if err != nil {
		t.wasmError = err.Error()
		t.logger.Debug("coc3 wasmParams Go 生成失败", err)
		return ""
	}
	if value == "" {
		t.wasmError = "生成器未返回令牌"
	}
	return value
}

var coc3GenerateWasmParams = generateCoc3WasmParamsWithDevice

func (t *Coc3Task) resolveRiskToken() string {
	if value := t.setting("CAIYUN_COC3_RISK_TOKEN"); value != "" {
		return value
	}
	if t.riskToken != "" || t.riskTokenError != "" {
		return t.riskToken
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	value, err := t.api.Coc3ServiceRiskToken(ctx)
	if err != nil {
		t.riskTokenError = err.Error()
		return ""
	}
	t.riskToken = value
	return value
}

func (t *Coc3Task) resolveFingerprint() string {
	if value := t.setting("CAIYUN_COC3_DEVICE_ID"); value != "" {
		return value
	}
	if t.fingerprint != "" || t.fingerprintError != "" {
		return t.fingerprint
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	value, err := t.api.Coc3Fingerprint(ctx, t.tokenValue)
	if err != nil {
		t.fingerprintError = err.Error()
		return ""
	}
	t.fingerprint = value
	return value
}

func loadCoc3Wheel(dir string) ([]byte, error) {
	wasm := coc3WheelWASM
	if dir != "" {
		file, err := os.Open(filepath.Join(dir, "wheel_bg.wasm"))
		if err != nil {
			return nil, fmt.Errorf("读取 wheel_bg.wasm: %w", err)
		}
		defer file.Close()
		wasm, err = io.ReadAll(io.LimitReader(file, coc3wasm.MaxModuleSize+1))
		if err != nil {
			return nil, fmt.Errorf("读取 wheel_bg.wasm: %w", err)
		}
	}
	return wasm, nil
}

func generateCoc3WasmParamsWithDevice(dir string, device coc3wasm.DeviceContext) (string, error) {
	wasm, err := loadCoc3Wheel(dir)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return coc3wasm.GenerateWithDevice(ctx, wasm, device)
}

// Kept for offline reference-vector checks, never used by a production draw.
func generateCoc3WasmParams(dir string) (string, error) {
	wasm, err := loadCoc3Wheel(dir)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return coc3wasm.Generate(ctx, wasm)
}
