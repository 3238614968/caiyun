package api

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// AI Store（aiTools）作品保存与授权。
//
// 保存链路：`accredit/get` 确认已授权 -> `createUploadTask` 把作品写入
// `/myfaverapp/<AI工具箱>` 下的对应模块目录。
//
// 逆向实测：`module` 1–14 的保存路径可用；`module=15`（朋友圈9图 / AI九宫格）
// 恒回 `30101 云盘目录创建失败` —— 该分支要求 App WebView 的 JSBridge 上下文，
// 外部调用无法复现。因此本实现只对显式指定的模块号尝试保存。
const aiStoreBase = AIYunURL + "/aitools"

// AIStoreAccredit 查询模块授权状态，返回服务端记录的云盘目录 path。
func (api *CaiyunAPI) AIStoreAccredit(module int) (*CaiyunResponse, error) {
	if module <= 0 {
		return nil, fmt.Errorf("AI Store 模块号无效: %d", module)
	}
	return api.marketJSONRequest(
		"POST",
		AIYunURL+"/api/outer/assistant/accredit/get",
		map[string]interface{}{"sourceBusiness": 1, "module": module},
		api.funaiImageHeaders(),
	)
}

// AIStoreAccreditProtocol 读取授权协议（authStatus=-1 时前端会先拉协议）。
func (api *CaiyunAPI) AIStoreAccreditProtocol(module int) (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"POST",
		AIYunURL+"/api/outer/assistant/accredit/protocol/get",
		map[string]interface{}{"sourceBusiness": 1, "module": module},
		api.funaiImageHeaders(),
	)
}

// AIStoreAccreditSubmit 提交授权。仅在用户已明确同意授权时调用。
func (api *CaiyunAPI) AIStoreAccreditSubmit(module int) (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"POST",
		AIYunURL+"/api/outer/assistant/accredit/submit",
		map[string]interface{}{"sourceBusiness": 1, "module": module},
		api.funaiImageHeaders(),
	)
}

// AIStoreAccreditPath 从授权响应里取出云盘目录 path。
func AIStoreAccreditPath(resp *CaiyunResponse) string {
	result, ok := resultMap(resp)
	if !ok {
		return ""
	}
	if data, ok := result["data"].(map[string]interface{}); ok {
		return mapString(data, "path")
	}
	return mapString(result, "path")
}

// AIStoreSaveTask 通过 AI Store 保存接口写入作品。
// imageBase64 支持带或不带 `data:image/...;base64,` 前缀。
func (api *CaiyunAPI) AIStoreSaveTask(module int, imageBase64 string) (*CaiyunResponse, error) {
	if module <= 0 {
		return nil, fmt.Errorf("AI Store 模块号无效: %d", module)
	}
	payload, err := normalizeDataURL(imageBase64)
	if err != nil {
		return nil, err
	}
	headers := api.funaiImageHeaders()
	// 适配器保存分支要求大写头名 + 值 3，与扫描文档保存同源。
	delete(headers, "x-yun-api-version")
	headers["X-YUN-API-VERSION"] = "3"
	headers["X-Yun-Client-Info"] = headers["x-yun-client-info"]
	headers["Content-Type"] = "application/json; charset=UTF-8"
	return api.marketJSONRequest(
		"POST",
		aiStoreBase+"/uop/user/createUploadTask",
		map[string]interface{}{"module": module, "taskId": "", "imageBase64": payload},
		headers,
	)
}

// AIStoreSaveTaskV2 新版保存接口（头 x-yun-tid）。
func (api *CaiyunAPI) AIStoreSaveTaskV2(module int, imageBase64 string) (*CaiyunResponse, error) {
	if module <= 0 {
		return nil, fmt.Errorf("AI Store 模块号无效: %d", module)
	}
	payload, err := normalizeDataURL(imageBase64)
	if err != nil {
		return nil, err
	}
	headers := api.funaiImageHeaders()
	delete(headers, "x-yun-api-version")
	headers["X-YUN-API-VERSION"] = "3"
	headers["Content-Type"] = "application/json; charset=UTF-8"
	return api.marketJSONRequest(
		"POST",
		aiStoreBase+"/uop/user/createUploadTaskV2",
		map[string]interface{}{"module": module, "taskId": "", "imageBase64": payload},
		headers,
	)
}

// normalizeDataURL 校验并规范化 data URL 形式的图片载荷。
func normalizeDataURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("AI Store 保存需要图片数据")
	}
	payload := trimmed
	if comma := strings.Index(trimmed, ","); strings.HasPrefix(trimmed, "data:") && comma > 0 {
		payload = trimmed[comma+1:]
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		return "", fmt.Errorf("AI Store 图片数据不是合法 base64: %w", err)
	}
	return "data:image/jpeg;base64," + payload, nil
}

// ParseAIStoreError 从保存响应里提取服务端业务码与文案。
// AI Store 的错误信封可能是 `{code,body:{code,msg}}`，也可能是 `{code,msg}`。
func ParseAIStoreError(resp *CaiyunResponse) (string, string) {
	if resp == nil {
		return "", ""
	}
	if body, ok := resp.Body.(map[string]interface{}); ok {
		return mapString(body, "code"), mapString(body, "msg")
	}
	if result, ok := resultMap(resp); ok {
		if body, ok := result["body"].(map[string]interface{}); ok {
			return mapString(body, "code"), mapString(body, "msg")
		}
	}
	return fmt.Sprint(resp.Code), resp.MessageText()
}
