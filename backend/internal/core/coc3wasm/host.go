package coc3wasm

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/api"
)

type undefined struct{}
type isolatedGlobal struct{}
type wheelNavigator struct{}
type wheelLocation struct{}

type wheelHost struct {
	env         environment
	heap        []interface{}
	live        []bool
	free        []uint32
	err         error
	deviceState *deviceHost
	resultTypes map[string][]api.ValueType
}

func newWheelHost(env environment) *wheelHost {
	heap := make([]interface{}, 1028)
	for i := range heap {
		heap[i] = undefined{}
	}
	heap[1025], heap[1026], heap[1027] = nil, true, false
	return &wheelHost{env: env, heap: heap, live: make([]bool, len(heap)), resultTypes: make(map[string][]api.ValueType)}
}

func (h *wheelHost) fail(err error) { h.err = err; panic(err) }

func (h *wheelHost) get(index uint32) interface{} {
	if uint64(index) >= uint64(len(h.heap)) || index >= 1028 && !h.live[index] {
		h.fail(fmt.Errorf("wheel 宿主句柄越界或已释放"))
	}
	return h.heap[index]
}

func (h *wheelHost) add(value interface{}) uint64 {
	if last := len(h.free) - 1; last >= 0 {
		index := h.free[last]
		h.free = h.free[:last]
		h.heap[index] = value
		h.live[index] = true
		return uint64(index)
	}
	if len(h.heap) >= 4096 {
		h.fail(fmt.Errorf("wheel 宿主句柄数量超过限制"))
	}
	index := len(h.heap)
	h.heap = append(h.heap, value)
	h.live = append(h.live, true)
	return uint64(index)
}

func (h *wheelHost) drop(index uint32) {
	if index < 1028 {
		return
	}
	h.get(index)
	h.heap[index] = undefined{}
	h.live[index] = false
	h.free = append(h.free, index)
}

func (h *wheelHost) readString(mod api.Module, ptr, length uint32) string {
	if length > 64<<10 {
		h.fail(fmt.Errorf("wheel 宿主字符串超过限制"))
	}
	data, ok := mod.Memory().Read(ptr, length)
	if !ok || !utf8.Valid(data) {
		h.fail(fmt.Errorf("wheel 宿主字符串内存无效"))
	}
	return string(data)
}

// The default path implements only wheel.roto_simple("{}"). The optional
// device host adds a bounded preflight; arbitrary process, filesystem, eval and
// network imports still fail closed.
func (h *wheelHost) invoke(ctx context.Context, mod api.Module, name string, stack []uint64) {
	if h.env.device != nil && h.invokeDevice(ctx, mod, name, stack) {
		return
	}
	switch name {
	case "__wbg_now_16f0c993d5dd6c27":
		stack[0] = api.EncodeF64(float64(h.env.now().UnixMilli()))
	case "__wbg_static_accessor_SELF_f207c857566db248", "__wbg_static_accessor_GLOBAL_THIS_ad356e0db91c7913", "__wbg_static_accessor_WINDOW_bb9f1ba69d61b386", "__wbg_static_accessor_GLOBAL_8adb955bd33fac2f":
		stack[0] = h.add(isolatedGlobal{})
	case "__wbg_instanceof_Window_23e677d2c6843922":
		_, ok := h.get(uint32(stack[0])).(isolatedGlobal)
		stack[0] = 0
		if ok {
			stack[0] = 1
		}
	case "__wbg_document_c0320cd4183c6d9b", "__wbg_performance_28be169151161678":
		// The simple protocol permits an absent DOM and Performance object.
		stack[0] = 0
	case "__wbg_navigator_9cebf56f28aa719b":
		stack[0] = h.add(wheelNavigator{})
	case "__wbg_location_fc8d47802682dd93":
		stack[0] = h.add(wheelLocation{})
	case "__wbg_userAgent_161a5f2d2a8dee61", "__wbg_platform_89094136c8b1c14d", "__wbg_href_82f7f7056b4e8510":
		value := UserAgent
		if name == "__wbg_platform_89094136c8b1c14d" {
			value = "Linux aarch64"
		} else if name == "__wbg_href_82f7f7056b4e8510" {
			value = CanvasURL
		}
		h.writeString(ctx, mod, uint32(stack[0]), value)
	case "__wbindgen_object_clone_ref":
		stack[0] = h.add(h.get(uint32(stack[0])))
	case "__wbindgen_object_drop_ref":
		h.drop(uint32(stack[0]))
	case "__wbg___wbindgen_is_undefined_52709e72fb9f179c":
		_, missing := h.get(uint32(stack[0])).(undefined)
		stack[0] = 0
		if missing {
			stack[0] = 1
		}
	case "__wbg_get_3ef1eba1850ade27":
		if _, ok := h.get(uint32(stack[0])).(isolatedGlobal); !ok {
			h.fail(fmt.Errorf("wheel 请求了不支持的对象属性"))
		}
		// The Go host is not Bun, Deno, Node or a browser. No process.env or
		// real host global is ever made available to the guest.
		stack[0] = h.add(undefined{})
	case "__wbindgen_cast_0000000000000007":
		stack[0] = h.add(h.readString(mod, uint32(stack[0]), uint32(stack[1])))
	case "__wbg_eval_c311194bb27c7836":
		if h.readString(mod, uint32(stack[0]), uint32(stack[1])) != "new Error().stack" {
			h.fail(fmt.Errorf("wheel 请求了不支持的宿主表达式"))
		}
		stack[0] = h.add(wheelStack)
	case "__wbg___wbindgen_string_get_395e606bd0ee4427":
		out := uint32(stack[0])
		value, ok := h.get(uint32(stack[1])).(string)
		ptr, length := uint32(0), uint32(0)
		if ok {
			length = uint32(len(value))
			allocation, err := mod.ExportedFunction("__wbindgen_export").Call(ctx, uint64(length), 1)
			if err != nil {
				h.fail(fmt.Errorf("wheel 分配字符串: %w", err))
			}
			ptr = uint32(allocation[0])
			if !mod.Memory().Write(ptr, []byte(value)) {
				h.fail(fmt.Errorf("wheel 字符串写入越界"))
			}
		}
		if !mod.Memory().WriteUint32Le(out, ptr) || !mod.Memory().WriteUint32Le(out+4, length) {
			h.fail(fmt.Errorf("wheel 字符串结果写入越界"))
		}
	case "__wbg_getRandomValues_76dfc69825c9c552":
		data, ok := mod.Memory().Read(uint32(stack[0]), uint32(stack[1]))
		if !ok {
			h.fail(fmt.Errorf("wheel 随机数缓冲区越界"))
		}
		if _, err := io.ReadFull(h.env.random, data); err != nil {
			h.fail(fmt.Errorf("wheel 随机数生成失败: %w", err))
		}
	case "__wbg_log_263fa59a7097debd", "__wbg_log_c69146d8a9856709":
		// Suppress the upstream's static "WASM Initialized" console message.
	case "__wbg___wbindgen_throw_6ddd609b62940d55":
		h.fail(fmt.Errorf("wheel: %s", h.readString(mod, uint32(stack[0]), uint32(stack[1]))))
	default:
		h.fail(fmt.Errorf("不支持的 wheel 宿主调用: %s", name))
	}
}

func (h *wheelHost) writeString(ctx context.Context, mod api.Module, out uint32, value string) {
	allocation, err := mod.ExportedFunction("__wbindgen_export").Call(ctx, uint64(len(value)), 1)
	if err != nil {
		h.fail(fmt.Errorf("wheel 分配字符串: %w", err))
	}
	ptr := uint32(allocation[0])
	if !mod.Memory().Write(ptr, []byte(value)) || !mod.Memory().WriteUint32Le(out, ptr) || !mod.Memory().WriteUint32Le(out+4, uint32(len(value))) {
		h.fail(fmt.Errorf("wheel 字符串结果写入越界"))
	}
}

func read32(data []byte, offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }
