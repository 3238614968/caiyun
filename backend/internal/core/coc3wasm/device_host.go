package coc3wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/tetratelabs/wazero/api"
)

type deviceObject map[string]interface{}
type deviceStorage struct{}
type deviceDate struct{}
type deviceAbort struct{}
type deviceHeaders map[string]string
type deviceRequest struct {
	url, method, body string
	headers           deviceHeaders
}
type deviceResponse struct {
	status int
	body   string
}

func (h *wheelHost) deviceException(ctx context.Context, mod api.Module, message string) {
	_, err := mod.ExportedFunction("__wbindgen_export3").Call(ctx, h.add(message))
	if err != nil {
		h.fail(fmt.Errorf("设备上报异常传递失败"))
	}
}

func (h *wheelHost) deviceNumber(name string, stack []uint64, value float64) {
	if len(h.resultTypes[name]) > 0 && h.resultTypes[name][0] == api.ValueTypeF64 {
		stack[0] = api.EncodeF64(value)
	} else {
		if math.IsNaN(value) {
			value = 0
		}
		stack[0] = uint64(int64(value))
	}
}

func (h *wheelHost) invokeDevice(ctx context.Context, mod api.Module, name string, stack []uint64) bool {
	get := func(i int) interface{} { return h.get(uint32(stack[i])) }
	text := func(i int) string { return h.readString(mod, uint32(stack[i]), uint32(stack[i+1])) }
	add := func(v interface{}) { stack[0] = h.add(v) }
	write := func(out uint32, value string) { h.writeString(ctx, mod, out, value) }
	boolean := func(v bool) {
		stack[0] = 0
		if v {
			stack[0] = 1
		}
	}
	switch name {
	case "__wbg_eval_c311194bb27c7836":
		if text(0) == "debugger;" {
			add(undefined{})
			return true
		}
		return false
	case "__wbg_localStorage_51c38b3b222e1ed2":
		add(deviceStorage{})
	case "__wbg_getItem_a7cc1d4f154f2e6f":
		if text(2) == "canvasworkbenchweb_auth" {
			value, _ := json.Marshal(map[string]interface{}{"user_info": map[string]string{"phone": h.env.device.BusinessToken}})
			write(uint32(stack[0]), string(value))
		} else {
			mod.Memory().WriteUint32Le(uint32(stack[0]), 0)
			mod.Memory().WriteUint32Le(uint32(stack[0])+4, 0)
		}
	case "__wbg_language_bc1369032c30bc3e":
		write(uint32(stack[0]), "zh-CN")
	case "__wbg_languages_58f4de7e1d7f5b31":
		add([]interface{}{"zh-CN"})
	case "__wbg_onLine_e150233649c2627d":
		boolean(true)
	case "__wbg_hardwareConcurrency_a8e3f7c0d77df6d3":
		h.deviceNumber(name, stack, 4)
	case "__wbg_get_3ef1eba1850ade27":
		object, key := get(0), get(1)
		if _, ok := object.(wheelNavigator); ok && key == "maxTouchPoints" {
			add(float64(0))
		} else if m, ok := object.(deviceObject); ok {
			if v, ok := m[fmt.Sprint(key)]; ok {
				add(v)
			} else {
				add(undefined{})
			}
		} else {
			add(undefined{})
		}
	case "__wbg_length_8632bd8b5ab30449", "__wbg_length_b3416cf66a5452c8":
		n := 0
		switch v := get(0).(type) {
		case string:
			n = len(v)
		case []interface{}:
			n = len(v)
		}
		h.deviceNumber(name, stack, float64(n))
	case "__wbg_get_unchecked_329cfe50afab7352":
		v, ok := get(0).([]interface{})
		index := int(stack[1])
		if ok && index >= 0 && index < len(v) {
			add(v[index])
		} else {
			add(undefined{})
		}
	case "__wbg___wbindgen_number_get_34bb9d9dcfa21373":
		out := uint32(stack[0])
		value, ok := get(1).(float64)
		flag := uint32(0)
		if ok {
			flag = 1
		}
		mod.Memory().WriteUint32Le(out, flag)
		mod.Memory().WriteUint64Le(out+8, api.EncodeF64(value))
	case "__wbg___wbindgen_boolean_get_c0f3f60bac5a78d1":
		value, ok := get(0).(bool)
		stack[0] = 0xFFFFFF
		if ok {
			stack[0] = 0
			if value {
				stack[0] = 1
			}
		}
	case "__wbg___wbindgen_is_null_0b605fc6b167c56f":
		boolean(get(0) == nil)
	case "__wbg___wbindgen_is_function_3c846841762788c1":
		_, ok := get(0).(*deviceFunction)
		boolean(ok)
	case "__wbg_has_926ef2ff40b308cf":
		boolean(false)
	case "__wbg_screen_596be4cb34f5852d", "__wbg_crypto_ed4c4da5b2e2eae1":
		add(undefined{})
	case "__wbg_devicePixelRatio_c36a5fab28da634e", "__wbg_innerWidth_e6af2d66d3b06991", "__wbg_innerHeight_ba245c3eff70b2a1":
		h.deviceNumber(name, stack, math.NaN())
	case "__wbg_width_0f53f9bb8b240a4e", "__wbg_height_3234dfa2a4da7b4a", "__wbg_colorDepth_2dca0de60c4d2c0a", "__wbg_pixelDepth_93f89789f54502c6":
		h.deviceException(ctx, mod, "absent screen")
		h.deviceNumber(name, stack, 0)
	case "__wbg_new_0_1dcafdf5e786e876":
		add(deviceDate{})
	case "__wbg_getTimezoneOffset_639bcf2dde21158b":
		_, offset := h.env.now().Zone()
		h.deviceNumber(name, stack, float64(-offset/60))
	case "__wbg_hostname_a30ece22df1c8b63":
		write(uint32(stack[0]), "dev.coc.10086.cn")
	case "__wbg_protocol_2c6bffc6b0fafa07":
		write(uint32(stack[0]), "https:")
	case "__wbg_port_6ebe920b4e947aa4":
		write(uint32(stack[0]), "")
	case "__wbg_frameElement_a00324fcb3f8f3f5":
		stack[0] = 0
	case "__wbg_parent_8078622065f6a717":
		add(isolatedGlobal{})
	case "__wbg_is_a166b9958c2438ad":
		_, a := get(0).(isolatedGlobal)
		_, b := get(1).(isolatedGlobal)
		boolean(a && b)
	case "__wbg_isSecureContext_b78081a385656549":
		boolean(true)
	case "__wbg_instanceof_Object_be1962063fcc0c9f":
		_, ok := get(0).(deviceObject)
		boolean(ok)
	case "__wbg_getPrototypeOf_adf5ea1c7561d551":
		add(undefined{})
	case "__wbg_queueMicrotask_0c399741342fb10f":
		add(undefined{})
	case "__wbg_queueMicrotask_a082d78ce798393e":
		f, ok := get(0).(*deviceFunction)
		if !ok {
			h.fail(fmt.Errorf("设备微任务不是受限闭包"))
		}
		h.deviceState.queue = append(h.deviceState.queue, func() error { _, err := f.invoke(); return err })
	case "__wbg_resolve_ae8d83246e5bcc12":
		if p, ok := get(0).(*devicePromise); ok {
			add(p)
		} else {
			add(&devicePromise{state: 1, value: get(0)})
		}
	case "__wbg_new_typed_aaaeaf29cf802876":
		a, b := stack[0], stack[1]
		p := &devicePromise{}
		yes := &deviceFunction{invoke: func(args ...interface{}) (interface{}, error) {
			v := interface{}(undefined{})
			if len(args) > 0 {
				v = args[0]
			}
			h.settleDevice(p, 1, v)
			return undefined{}, nil
		}}
		no := &deviceFunction{invoke: func(args ...interface{}) (interface{}, error) {
			v := interface{}(undefined{})
			if len(args) > 0 {
				v = args[0]
			}
			h.settleDevice(p, 2, v)
			return undefined{}, nil
		}}
		if _, err := mod.ExportedFunction("__wasm_bindgen_func_elem_803").Call(ctx, a, b, h.add(yes), h.add(no)); err != nil {
			h.fail(err)
		}
		add(p)
	case "__wbindgen_cast_0000000000000001":
		add(h.deviceClosure(ctx, mod, stack[0], stack[1], "__wasm_bindgen_func_elem_821"))
	case "__wbindgen_cast_0000000000000005":
		add(h.deviceClosure(ctx, mod, stack[0], stack[1], "__wasm_bindgen_func_elem_511"))
	case "__wbindgen_cast_0000000000000006":
		add(api.DecodeF64(stack[0]))
	case "__wbg__wbg_cb_unref_6b5b6b8576d35cb1":
		if f, ok := get(0).(*deviceFunction); ok && f.unref != nil {
			if err := f.unref(); err != nil {
				h.fail(err)
			}
		}
	case "__wbg_then_098abe61755d12f6", "__wbg_then_9e335f6dd892bc11":
		p, ok := get(0).(*devicePromise)
		if !ok {
			h.fail(fmt.Errorf("设备异步链不是受限 Promise"))
		}
		no := interface{}(undefined{})
		if len(stack) > 2 && name == "__wbg_then_9e335f6dd892bc11" {
			no = get(2)
		}
		add(h.thenDevice(p, get(1), no))
	case "__wbg_call_2d781c1f4d5c0ef8", "__wbg_call_e133b57c9155d22c":
		f, ok := get(0).(*deviceFunction)
		if !ok {
			h.fail(fmt.Errorf("设备调用不是受限闭包"))
		}
		var args []interface{}
		if name == "__wbg_call_2d781c1f4d5c0ef8" {
			args = append(args, get(2))
		}
		v, err := f.invoke(args...)
		if err != nil {
			h.deviceException(ctx, mod, err.Error())
			stack[0] = 0
		} else {
			add(v)
		}
	case "__wbg_new_c518c60af666645b":
		add(&deviceAbort{})
	case "__wbg_signal_166e1da31adcac18":
		add(&deviceAbort{})
	case "__wbg_abort_5ef96933660780b7":
	case "__wbg_setTimeout_ef24d2fc3ad97385":
		f, ok := get(0).(*deviceFunction)
		if !ok {
			h.fail(fmt.Errorf("设备计时器不是受限闭包"))
		}
		h.deviceState.nextTimer++
		h.deviceState.timers[h.deviceState.nextTimer] = f
		stack[0] = uint64(h.deviceState.nextTimer)
	case "__wbg_clearTimeout_113b1cde814ec762":
		delete(h.deviceState.timers, int(stack[0]))
	case "__wbg_new_ab79df5bd7c26067":
		add(deviceObject{})
	case "__wbg_new_0837727332ac86ba":
		add(deviceHeaders{})
	case "__wbg_new_5415f704ce1c4eda":
		add(url.Values{})
	case "__wbg_set_method_8c015e8bcafd7be1":
		get(0).(deviceObject)["method"] = text(1)
	case "__wbg_set_body_a3d856b097dfda04":
		get(0).(deviceObject)["body"] = get(1)
	case "__wbg_set_signal_0cebecb698f25d21":
		get(0).(deviceObject)["signal"] = get(1)
	case "__wbg_set_headers_3c8fecc693b75327":
		get(0).(deviceObject)["headers"] = get(1)
	case "__wbg_set_e09648bea3f1af1e":
		switch v := get(0).(type) {
		case deviceHeaders:
			v[strings.ToLower(text(1))] = text(3)
		case url.Values:
			v.Set(text(1), text(3))
		default:
			h.fail(fmt.Errorf("设备请求字段类型无效"))
		}
	case "__wbg_new_with_str_4c859c3e69e6cb15":
		add(&deviceRequest{url: text(0), method: "GET"})
	case "__wbg_new_with_str_and_init_b4b54d1a819bc724":
		m := get(2).(deviceObject)
		r := &deviceRequest{url: text(0), method: "GET", headers: deviceHeaders{}}
		if v, ok := m["method"].(string); ok {
			r.method = v
		}
		if v, ok := m["body"].(string); ok {
			r.body = v
		}
		if v, ok := m["headers"].(deviceHeaders); ok {
			r.headers = v
		}
		add(r)
	case "__wbg_url_b6f96880b733816c":
		write(uint32(stack[0]), get(1).(*deviceRequest).url)
	case "__wbg_new_bb1018d527df73cb":
		u, err := url.Parse(text(0))
		if err != nil {
			h.fail(fmt.Errorf("设备目标 URL 无效"))
		}
		add(u)
	case "__wbg_search_35617fb7936183df":
		u := get(1).(*url.URL)
		s := ""
		if u.RawQuery != "" {
			s = "?" + u.RawQuery
		}
		write(uint32(stack[0]), s)
	case "__wbg_toString_3272fa0dfd05dd87":
		var s string
		switch v := get(0).(type) {
		case *url.URL:
			s = v.String()
		case url.Values:
			s = v.Encode()
		default:
			h.fail(fmt.Errorf("设备 URL 转换类型无效"))
		}
		add(s)
	case "__wbg_set_search_bd09fe57b201bac5":
		get(0).(*url.URL).RawQuery = strings.TrimPrefix(text(1), "?")
	case "__wbg_fetch_8d9b732df7467c44":
		r := get(0).(*deviceRequest)
		var body struct {
			Phone string `json:"benefit_phone"`
			Text  string `json:"encryptedText"`
		}
		if h.deviceState.exchanges != 0 || r.url != DeviceTokenURL || r.method != "POST" || len(r.body) > 32<<10 || json.Unmarshal([]byte(r.body), &body) != nil || body.Phone != h.env.device.BusinessToken || body.Text == "" {
			h.fail(fmt.Errorf("设备上报未满足固定目标、单次和当前会话限制"))
		}
		h.deviceState.exchanges++
		response, err := h.env.device.Exchange(ctx, DeviceRequest{URL: r.url, Method: r.method, Headers: r.headers, Body: []byte(r.body)})
		if err != nil {
			h.fail(fmt.Errorf("COC3 设备上报传输失败，未生成抽奖参数"))
		} else if len(response.Body) > 64<<10 {
			h.fail(fmt.Errorf("设备上报回执过大"))
		} else if !validDeviceReceipt(response) {
			// The guest can resolve even when an error envelope contains data.
			// Independently require the official HTTP/business receipt first.
			h.fail(fmt.Errorf("官方设备上报未确认，不生成抽奖提交令牌"))
		} else {
			add(&devicePromise{state: 1, value: &deviceResponse{status: response.Status, body: string(response.Body)}})
		}
	case "__wbg_instanceof_Response_9b4d9fd451e051b1":
		_, ok := get(0).(*deviceResponse)
		boolean(ok)
	case "__wbg_ok_7ec8b94facac7704":
		r := get(0).(*deviceResponse)
		boolean(r.status >= 200 && r.status < 300)
	case "__wbg_text_372f5b91442c50f9":
		add(&devicePromise{state: 1, value: get(0).(*deviceResponse).body})
	default:
		return false
	}
	return true
}
