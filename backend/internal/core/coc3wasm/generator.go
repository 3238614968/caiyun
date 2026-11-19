// Package coc3wasm executes the official wheel module inside the Go process.
// Its host exposes a clock, cryptographic entropy and isolated values. The
// optional device preflight can call only the caller's restricted transport;
// the module cannot inspect process state, open files or choose a network target.
package coc3wasm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	MaxModuleSize = 8 << 20
	maxParamsSize = 32 << 10
	wheelStack    = "Error\n    at wheel.roto_simple (coc3-wheel.wasm)"
	// The simple wheel entry uses these page values; it does not need a DOM.
	UserAgent = "Mozilla/5.0 (Linux; Android 16; RMQ) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0.0.0 Mobile Safari/537.36 leadeon/12.5.4/CMCCIT"
	CanvasURL = "https://dev.coc.10086.cn/coc3/canvas/rightsmarket-h5-canvas/online/home"
)

// Compilation is shared; guest memories, globals and host handles are not.
var compilationCache = wazero.NewCompilationCache()

type environment struct {
	now    func() time.Time
	random io.Reader
	device *DeviceContext
}

// Generate returns JSON.stringify([aurora, content]) for the protocol's "{}"
// input. Each call owns an instance and releases it on success or failure.
func Generate(ctx context.Context, wasm []byte) (string, error) {
	return generate(ctx, wasm, environment{now: time.Now, random: rand.Reader})
}

func generate(ctx context.Context, wasm []byte, env environment) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(wasm) == 0 || len(wasm) > MaxModuleSize {
		return "", fmt.Errorf("wheel WASM 大小无效")
	}
	// 256 pages = 16 MiB. Context cancellation also interrupts guest loops.
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(256).
		WithCloseOnContextDone(true).WithCompilationCache(compilationCache)
	runtime := wazero.NewRuntimeWithConfig(ctx, config)
	defer runtime.Close(context.Background())
	compiled, err := runtime.CompileModule(ctx, wasm)
	if err != nil {
		return "", fmt.Errorf("编译 wheel WASM: %w", err)
	}
	host := newWheelHost(env)
	builder := runtime.NewHostModuleBuilder("./wheel_bg.js")
	for _, imported := range compiled.ImportedFunctions() {
		module, name, _ := imported.Import()
		if module != "./wheel_bg.js" {
			return "", fmt.Errorf("不支持的 wheel 导入模块: %s", module)
		}
		host.resultTypes[name] = imported.ResultTypes()
		builder.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			host.invoke(ctx, mod, name, stack)
		}), imported.ParamTypes(), imported.ResultTypes()).Export(name)
	}
	if _, err := builder.Instantiate(ctx); err != nil {
		return "", fmt.Errorf("初始化 wheel 宿主: %w", err)
	}
	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return "", fmt.Errorf("初始化 wheel WASM: %w", err)
	}
	for name, signature := range map[string]struct{ params, results []api.ValueType }{
		"__wbindgen_start":                {},
		"wheel_create_wheel":              {results: []api.ValueType{api.ValueTypeI32}},
		"__wbindgen_export":               {params: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, results: []api.ValueType{api.ValueTypeI32}},
		"__wbindgen_add_to_stack_pointer": {params: []api.ValueType{api.ValueTypeI32}, results: []api.ValueType{api.ValueTypeI32}},
		"wheel_roto_simple":               {params: []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}},
	} {
		fn := module.ExportedFunction(name)
		if fn == nil || !sameTypes(fn.Definition().ParamTypes(), signature.params) || !sameTypes(fn.Definition().ResultTypes(), signature.results) {
			return "", fmt.Errorf("wheel WASM 接口不兼容: %s", name)
		}
	}
	call := func(name string, args ...uint64) ([]uint64, error) {
		values, err := module.ExportedFunction(name).Call(ctx, args...)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if host.err != nil {
				return nil, host.err
			}
			return nil, fmt.Errorf("wheel/%s: %w", name, err)
		}
		return values, nil
	}
	if _, err := call("__wbindgen_start"); err != nil {
		return "", err
	}
	wheel, err := call("wheel_create_wheel")
	if err != nil {
		return "", err
	}
	if env.device != nil {
		if err := host.runDeviceToken(ctx, module, wheel[0]); err != nil {
			return "", err
		}
	}
	input, err := call("__wbindgen_export", 2, 1)
	if err != nil {
		return "", err
	}
	if module.Memory() == nil || !module.Memory().Write(uint32(input[0]), []byte("{}")) {
		return "", fmt.Errorf("wheel 输入内存无效")
	}
	result, err := call("__wbindgen_add_to_stack_pointer", uint64(^uint32(15)))
	if err != nil {
		return "", err
	}
	resultPtr := uint32(result[0])
	if _, err := call("wheel_roto_simple", uint64(resultPtr), wheel[0], input[0], 2); err != nil {
		return "", err
	}
	frame, ok := module.Memory().Read(resultPtr, 16)
	if !ok {
		return "", fmt.Errorf("wheel 返回内存无效")
	}
	arrayPtr, arrayLen, exception, failed := read32(frame, 0), read32(frame, 4), read32(frame, 8), read32(frame, 12)
	if failed != 0 {
		if uint64(exception) >= uint64(len(host.heap)) {
			return "", fmt.Errorf("wheel 返回无效异常句柄")
		}
		return "", fmt.Errorf("wheel 生成失败: %v", host.heap[exception])
	}
	if arrayLen != 2 {
		return "", fmt.Errorf("wheel 未返回 aurora/content 对")
	}
	refs, ok := module.Memory().Read(arrayPtr, 8)
	if !ok {
		return "", fmt.Errorf("wheel 返回数组无效")
	}
	var pair [2]string
	for i := range pair {
		index := read32(refs, i*4)
		if uint64(index) >= uint64(len(host.heap)) || index >= 1028 && !host.live[index] {
			return "", fmt.Errorf("wheel 返回无效令牌句柄")
		}
		value, ok := host.heap[index].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("wheel 返回空或无效令牌")
		}
		pair[i] = value
	}
	encoded, err := json.Marshal(pair)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxParamsSize {
		return "", fmt.Errorf("wheel 令牌超过请求头大小限制")
	}
	return string(encoded), nil
}

func sameTypes(left, right []api.ValueType) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
