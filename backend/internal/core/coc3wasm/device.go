package coc3wasm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/tetratelabs/wazero/api"
)

const DeviceTokenURL = "https://dev.coc.10086.cn/coc3/gr/risk-ability/wasm/gettoken"

type DeviceRequest struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
}

type DeviceResponse struct {
	Status int
	Body   []byte
}

func validDeviceReceipt(response DeviceResponse) bool {
	var receipt struct {
		Code    json.RawMessage `json:"code"`
		Success *bool           `json:"success"`
		Data    string          `json:"data"`
	}
	if response.Status != 200 || len(response.Body) > 64<<10 || json.Unmarshal(response.Body, &receipt) != nil {
		return false
	}
	code := strings.TrimSpace(string(receipt.Code))
	return (code == "0" || code == `"0"`) && receipt.Success != nil && *receipt.Success && strings.TrimSpace(receipt.Data) != "" && len(receipt.Data) <= 4096 && !strings.ContainsAny(receipt.Data, "\r\n")
}

// DeviceContext permits exactly one official device-token exchange. Browser
// features absent in Go remain absent; this host never fabricates canvas/GPU,
// sensors or an App verification result. Server rejection stops generation.
type DeviceContext struct {
	BusinessToken string
	Exchange      func(context.Context, DeviceRequest) (DeviceResponse, error)
}

func GenerateWithDevice(ctx context.Context, wasm []byte, device DeviceContext) (string, error) {
	if device.BusinessToken == "" || len(device.BusinessToken) > 512 || strings.ContainsAny(device.BusinessToken, ";\r\n") || device.Exchange == nil {
		return "", fmt.Errorf("COC3 设备上报缺少当前业务会话或受限传输")
	}
	return generate(ctx, wasm, environment{now: time.Now, random: rand.Reader, device: &device})
}

type deviceHost struct {
	queue     []func() error
	exchanges int
	timers    map[int]*deviceFunction
	nextTimer int
	lastError error
}

func (h *wheelHost) runDeviceToken(ctx context.Context, mod api.Module, wheel uint64) error {
	h.deviceState = &deviceHost{timers: make(map[int]*deviceFunction)}
	fn := mod.ExportedFunction("wheel_g_dt_with_custom_url")
	if fn == nil || !sameTypes(fn.Definition().ParamTypes(), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}) || !sameTypes(fn.Definition().ResultTypes(), []api.ValueType{api.ValueTypeI32}) {
		return fmt.Errorf("wheel 缺少官方设备上报入口")
	}
	ptr, err := mod.ExportedFunction("__wbindgen_export").Call(ctx, uint64(len(DeviceTokenURL)), 1)
	if err != nil || !mod.Memory().Write(uint32(ptr[0]), []byte(DeviceTokenURL)) {
		return fmt.Errorf("设备上报入口分配失败")
	}
	value, err := fn.Call(ctx, wheel, ptr[0], uint64(len(DeviceTokenURL)), 10000)
	if err != nil {
		if h.err != nil {
			return h.err
		}
		return fmt.Errorf("设备上报入口执行失败: %w", err)
	}
	promise, ok := h.get(uint32(value[0])).(*devicePromise)
	if !ok {
		return fmt.Errorf("设备上报未返回受限异步结果")
	}
	for operations := 0; promise.state == 0; operations++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if operations > 10000 || len(h.deviceState.queue) == 0 {
			if h.err != nil {
				return h.err
			}
			if h.deviceState.lastError != nil {
				return fmt.Errorf("设备异步任务失败: %w", h.deviceState.lastError)
			}
			return fmt.Errorf("设备上报异步流程未能完成")
		}
		task := h.deviceState.queue[0]
		h.deviceState.queue = h.deviceState.queue[1:]
		if err := task(); err != nil {
			return err
		}
	}
	if promise.state != 1 || !deviceTokenConfirmed(promise.value) || h.deviceState.exchanges != 1 {
		return fmt.Errorf("官方设备上报未确认，不生成抽奖提交令牌")
	}
	return nil
}

// The official getde wrapper accepts a truthy result or the numeric value zero.
// A fulfilled Promise containing undefined, null or false is not confirmation.
func deviceTokenConfirmed(value interface{}) bool {
	switch v := value.(type) {
	case nil, undefined:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return !math.IsNaN(v)
	default:
		return true
	}
}

// Promise jobs are guest-local and run serially under the generator's context.
// They cannot inspect process state or dispatch arbitrary host operations.
type devicePromise struct {
	state     int
	value     interface{}
	listeners []func() error
}

type deviceFunction struct {
	invoke func(...interface{}) (interface{}, error)
	unref  func() error
}

func (h *wheelHost) settleDevice(p *devicePromise, state int, value interface{}) {
	if p.state != 0 {
		return
	}
	if next, ok := value.(*devicePromise); state == 1 && ok {
		if next == p {
			h.settleDevice(p, 2, "promise cycle")
			return
		}
		follow := func() error { h.settleDevice(p, next.state, next.value); return nil }
		if next.state == 0 {
			next.listeners = append(next.listeners, follow)
		} else {
			h.deviceState.queue = append(h.deviceState.queue, follow)
		}
		return
	}
	p.state, p.value = state, value
	h.deviceState.queue = append(h.deviceState.queue, p.listeners...)
	p.listeners = nil
}

func (h *wheelHost) thenDevice(p *devicePromise, yes, no interface{}) *devicePromise {
	child := &devicePromise{}
	job := func() error {
		fn := yes
		if p.state == 2 {
			fn = no
		}
		if f, ok := fn.(*deviceFunction); ok {
			value, err := f.invoke(p.value)
			if err != nil {
				h.deviceState.lastError = err
				h.settleDevice(child, 2, err.Error())
			} else {
				h.settleDevice(child, 1, value)
			}
		} else {
			h.settleDevice(child, p.state, p.value)
		}
		return nil
	}
	if p.state == 0 {
		p.listeners = append(p.listeners, job)
	} else {
		h.deviceState.queue = append(h.deviceState.queue, job)
	}
	return child
}

func (h *wheelHost) deviceClosure(ctx context.Context, mod api.Module, a, b uint64, adapter string) *deviceFunction {
	refs := 1
	f := &deviceFunction{}
	f.unref = func() error {
		refs--
		if refs == 0 {
			_, err := mod.ExportedFunction("__wasm_bindgen_func_elem_228").Call(ctx, a, b)
			a = 0
			return err
		}
		return nil
	}
	f.invoke = func(args ...interface{}) (interface{}, error) {
		refs++
		defer f.unref()
		if adapter == "__wasm_bindgen_func_elem_511" {
			_, err := mod.ExportedFunction(adapter).Call(ctx, a, b)
			return undefined{}, err
		}
		ret, err := mod.ExportedFunction("__wbindgen_add_to_stack_pointer").Call(ctx, uint64(^uint32(15)))
		if err != nil {
			return nil, err
		}
		defer mod.ExportedFunction("__wbindgen_add_to_stack_pointer").Call(ctx, 16)
		value := interface{}(undefined{})
		if len(args) > 0 {
			value = args[0]
		}
		_, err = mod.ExportedFunction(adapter).Call(ctx, ret[0], a, b, h.add(value))
		if err != nil {
			return nil, err
		}
		frame, ok := mod.Memory().Read(uint32(ret[0]), 8)
		if !ok {
			return nil, fmt.Errorf("设备上报闭包结果越界")
		}
		if read32(frame, 4) != 0 {
			index := read32(frame, 0)
			value := h.get(index)
			h.drop(index)
			return nil, fmt.Errorf("设备上报闭包拒绝: %v", value)
		}
		return undefined{}, nil
	}
	return f
}
