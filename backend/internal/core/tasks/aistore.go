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
// `CAIYUN_AISTORE_SAVE_MODULE` 时尝试。module=15 走新版原生作品上传、
// 审核和 AI 动态复查；其他模块保留原保存接口。
// 保存任务的接受回执不构成作品落盘或活动完成的证明。
type AIStoreTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
	pending     bool
}

// NewAIStoreTask 创建 AI Store 任务。
func NewAIStoreTask(client *http.Client, log *logger.Logger) *AIStoreTask {
	return &AIStoreTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *AIStoreTask) Message() string { return strings.TrimSpace(t.lastMessage) }
func (t *AIStoreTask) Pending() bool   { return t.pending }

// aiStoreProbeModules 默认探测授权状态的模块（对应各 AI 工具）。
var aiStoreProbeModules = []int{7, 4, 9, 13, 1, 26, 19}

// Run 探测授权状态，并在配置了模块号时尝试保存一张样图。
func (t *AIStoreTask) Run() error {
	t.pending = false
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
		t.pending = true
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
	if module == 15 {
		result, err := t.api.CompleteNineGridTask()
		if err != nil {
			issues = append(issues, fmt.Errorf("九宫格作品保存: %w", err))
		} else {
			parts = append(parts, fmt.Sprintf("九宫格封面审核通过，云端已核验%d个文件，AI动态已保存；活动完成位需单独复查", len(result.FileIDs)))
		}
		t.pending = true
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
			t.pending = true
			parts = append(parts, fmt.Sprintf("AI Store 保存任务已接受(module=%d)，作品保存结果仍需核验", module))
		case code == "30101":
			t.pending = true
			parts = append(parts, fmt.Sprintf("AI Store 保存(module=%d)目录创建失败(30101)，未确认保存，请在 App 检查授权与目录", module))
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
