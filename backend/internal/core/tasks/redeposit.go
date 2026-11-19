package tasks

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// redepositMarkets 云朵中心两个渠道的任务表都可能出现转存类任务槽。
var redepositMarkets = []string{"sign_in_3", "newsign_139mail"}

// RedepositTask 云朵中心转存类任务槽（机制式跟进轮换）：
// 对两渠道任务表里的每个 WAIT 任务探测 getLinkInfoList，非转存槽回空列表
// 直接跳过（无副作用）；有链接则走 点击 -> saveLinkFile -> 延时 ->
// finishRedepositTask 的完整链路。
type RedepositTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
	pending     bool
}

func NewRedepositTask(client *http.Client, log *logger.Logger) *RedepositTask {
	return &RedepositTask{api: api.NewCaiyunAPI(client), logger: log}
}

func (t *RedepositTask) Message() string { return strings.TrimSpace(t.lastMessage) }

func (t *RedepositTask) Pending() bool { return t.pending }

func (t *RedepositTask) Run() error {
	t.pending = false
	var issues []error
	completed, saved, unfinished, probed, skipped := 0, 0, 0, 0, 0

	for _, market := range redepositMarkets {
		table, err := t.api.GetTaskListV3(market)
		if err != nil {
			issues = append(issues, fmt.Errorf("%s任务表查询: %w", market, err))
			continue
		}
		for _, task := range table {
			if task.ID <= 0 || !redepositTaskIsWait(task) || isTaskDisabledByServer(task) {
				continue
			}
			taskID := strconv.Itoa(task.ID)
			probed++
			listResp, listErr := t.api.RedepositLinkList(market, taskID, 1, 10)
			if listErr != nil {
				issues = append(issues, fmt.Errorf("%s/%s转存链接查询: %w", market, taskID, listErr))
				continue
			}
			if !responseCodeIs(listResp, 0) {
				if respErr := rewardResponseError("转存链接查询", listResp, 602); respErr != nil {
					issues = append(issues, fmt.Errorf("%s/%s: %w", market, taskID, respErr))
				}
				continue
			}
			linkIDs := redepositLinkIDs(listResp)
			if len(linkIDs) == 0 {
				// 非转存槽：空列表即跳过，不产生任何点击/转存副作用。
				skipped++
				continue
			}
			if t.logger != nil {
				t.logger.Debug(fmt.Sprintf("转存槽命中：%s/%s(%s) 链接%d个", market, task.Name, taskID, len(linkIDs)))
			}
			// 前置点击（key=task），与 App 内打开任务槽的行为一致。
			if clickErr := t.api.DoTaskWithMarket(market, "task", taskID); clickErr != nil {
				issues = append(issues, fmt.Errorf("%s/%s转存前置点击: %w", market, taskID, clickErr))
				continue
			}
			for _, linkID := range linkIDs {
				saveResp, saveErr := t.api.RedepositSaveFile(market, linkID)
				if saveErr != nil {
					issues = append(issues, fmt.Errorf("%s/%s转存文件: %w", market, taskID, saveErr))
					continue
				}
				if !responseCodeIs(saveResp, 0) {
					if respErr := rewardResponseError("转存文件", saveResp); respErr != nil {
						issues = append(issues, fmt.Errorf("%s/%s: %w", market, taskID, respErr))
					}
					continue
				}
				saved++
				// 服务端需要短暂时间落库转存结果，实测约 600ms。
				time.Sleep(600 * time.Millisecond)
				finishResp, finishErr := t.api.RedepositFinishTask(taskID, linkID)
				if finishErr != nil {
					unfinished++
					issues = append(issues, fmt.Errorf("%s/%s转存完成上报: %w", market, taskID, finishErr))
					continue
				}
				if responseCodeIs(finishResp, 0) {
					completed++
					if t.logger != nil {
						t.logger.Success(fmt.Sprintf("转存任务完成：%s/%s(%s)", market, task.Name, taskID))
					}
				} else {
					unfinished++
					if respErr := rewardResponseError("转存完成上报", finishResp); respErr != nil {
						issues = append(issues, fmt.Errorf("%s/%s: %w", market, taskID, respErr))
					}
				}
			}
		}
	}

	t.pending = unfinished > 0
	parts := []string{fmt.Sprintf("探测任务槽%d个（非转存槽%d个）", probed, skipped)}
	if saved > 0 || completed > 0 {
		parts = append(parts, fmt.Sprintf("转存成功%d次、完成任务%d项", saved, completed))
	}
	if unfinished > 0 {
		parts = append(parts, fmt.Sprintf("%d次转存已保存但完成上报未确认", unfinished))
	}
	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

// redepositTaskIsWait 只处理 WAIT 状态的任务槽：V3 表以 state 字段为准，
// 旧形态（无 state）用 status=0 表示未完成。
func redepositTaskIsWait(task api.Task) bool {
	if state := strings.TrimSpace(task.State); state != "" {
		return strings.EqualFold(state, "WAIT")
	}
	return task.Status == 0
}

// redepositLinkIDs 从 {result:{result:[{linkid:"..."}]}} 信封提取链接 ID。
// 服务端字段为全小写 linkid，兼容个别渠道回 linkId 的形态。
func redepositLinkIDs(resp *api.CaiyunResponse) []string {
	if resp == nil {
		return nil
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil
	}
	var ids []string
	for _, item := range responseArray(result["result"]) {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		linkID := strings.TrimSpace(fmt.Sprint(entry["linkid"]))
		if linkID == "" || linkID == "<nil>" {
			linkID = strings.TrimSpace(fmt.Sprint(entry["linkId"]))
		}
		if linkID != "" && linkID != "<nil>" {
			ids = append(ids, linkID)
		}
	}
	return ids
}
