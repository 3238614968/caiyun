package tasks

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// AIStoreTask（AI Store / aiTools 作品保存与授权）。
//
// 授权查询是只读动作，默认执行；保存作品只在显式配置
// `CAIYUN_AISTORE_SAVE_MODULE` 时尝试，因为逆向实测只有 module 1–14
// 的保存路径可用，module=15（朋友圈9图）恒回 `30101 云盘目录创建失败`，
// 要求 App WebView 的 JSBridge 上下文。
type AIStoreTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewAIStoreTask 创建 AI Store 任务。
func NewAIStoreTask(client *http.Client, log *logger.Logger) *AIStoreTask {
	return &AIStoreTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *AIStoreTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// aiStoreProbeModules 默认探测授权状态的模块（对应各 AI 工具）。
var aiStoreProbeModules = []int{7, 4, 9, 13, 1, 26, 19}

// Run 探测授权状态，并在配置了模块号时尝试保存一张样图。
func (t *AIStoreTask) Run() error {
	var issues []error
	var parts []string

	authorized := 0
	for _, module := range aiStoreProbeModules {
		resp, err := t.api.AIStoreAccredit(module)
		if err != nil {
			issues = append(issues, fmt.Errorf("AI Store 授权查询(module=%d): %w", module, err))
			continue
		}
		if responseCodeIs(resp, 0) && api.AIStoreAccreditPath(resp) != "" {
			authorized++
		} else if !responseCodeIs(resp, 0) {
			issues = append(issues, rewardResponseError("AI Store 授权查询", resp, 404, 602))
		}
	}
	parts = append(parts, fmt.Sprintf("AI Store 已授权模块%d/%d", authorized, len(aiStoreProbeModules)))

	moduleRaw := t.setting("CAIYUN_AISTORE_SAVE_MODULE")
	if moduleRaw == "" {
		parts = append(parts, "未配置 CAIYUN_AISTORE_SAVE_MODULE，仅探测授权")
		t.lastMessage = strings.Join(parts, "；")
		if t.logger != nil {
			t.logger.Info(t.lastMessage)
		}
		return errors.Join(issues...)
	}

	module, err := strconv.Atoi(moduleRaw)
	if err != nil || module <= 0 {
		issues = append(issues, fmt.Errorf("CAIYUN_AISTORE_SAVE_MODULE 无效: %q", moduleRaw))
		t.lastMessage = strings.Join(parts, "；")
		return errors.Join(issues...)
	}

	content, err := api.GenerateUniqueSampleJPEG(600, 800)
	if err != nil {
		issues = append(issues, fmt.Errorf("生成 AI Store 样图: %w", err))
		t.lastMessage = strings.Join(parts, "；")
		return errors.Join(issues...)
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(content)

	resp, err := t.api.AIStoreSaveTask(module, dataURL)
	if err != nil {
		issues = append(issues, fmt.Errorf("AI Store 保存(module=%d): %w", module, err))
	} else {
		code, message := api.ParseAIStoreError(resp)
		switch {
		case code == "0" || code == "0000":
			parts = append(parts, fmt.Sprintf("AI Store 作品已保存(module=%d)", module))
		case code == "30101":
			parts = append(parts, fmt.Sprintf("AI Store 保存(module=%d)被拒：需 App WebView 上下文（30101）", module))
		default:
			issues = append(issues, fmt.Errorf("AI Store 保存(module=%d)失败: code=%s %s", module, code, message))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
