package tasks

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// MakeWishTask 全网许愿赢好礼（National_MakeWish）。
type MakeWishTask struct {
	*activityActions
	lastMessage string
	pending     bool
}

func NewMakeWishTask(client *http.Client, log *logger.Logger) *MakeWishTask {
	return &MakeWishTask{activityActions: newActivityActions(client, log)}
}

func (t *MakeWishTask) SetStorage(store Storage) *MakeWishTask {
	t.setStorage(store)
	return t
}

func (t *MakeWishTask) SetAccountContext(phone, authToken string) *MakeWishTask {
	t.setAccountContext(phone, authToken)
	return t
}

// makewishManualTasks 需要真实好友或人工体验入口的进阶任务。
var makewishManualTasks = map[string]string{
	"MOMENTS_9PIC":   "体验朋友圈9图",
	"CLOUD_EXCHANGE": "AI豆兑换抽奖码",
}

func (t *MakeWishTask) Run() error {
	t.pending = false
	var issues []error
	attempted := make(map[int]bool)
	t.logger.Start("------【许愿赢好礼】------")
	t.api.PrepareActivitySession(api.MakeWishMarketName)

	home, err := t.api.MakeWishHome()
	if err != nil {
		t.logger.Error("获取许愿活动首页失败", err)
		return err
	}
	if !home.Config.CanEarnCodes {
		t.lastMessage = fmt.Sprintf("本期攒码时间已结束或未开放，已有抽奖码%d张", home.RaffleCodes.TotalCount)
		return nil
	}

	// 1. 未许愿时先选奖品许愿（首次许愿即得一张抽奖码）。
	if home.Wish == nil && home.Config.CanEarnCodes {
		if err := t.performWish(); err != nil {
			t.logger.Error("许愿失败", err)
			issues = append(issues, err)
		}
		home, err = t.api.MakeWishHome()
		if err != nil {
			return err
		}
	}

	// 2. 推进基础任务（开启通知/成功备份，服务端按真实行为判定）。
	basicAdvanced, err := t.api.MakeWishAutoAdvance(nil)
	if err != nil {
		t.logger.Debug(fmt.Sprintf("推进许愿基础任务失败: %v", err))
		issues = append(issues, err)
	}
	for _, advanced := range basicAdvanced {
		t.logger.Debug(fmt.Sprintf("许愿基础任务推进接口返回: %s（以任务列表状态为准）", advanced.TaskName))
	}

	// 3. 与 AI 灵犀对话完成 AI_LINGXI 后，按任务编号推进。
	tasks, _, err := t.api.MakeWishTaskList()
	if err != nil {
		return fmt.Errorf("获取许愿任务列表失败: %w", err)
	}
	var advanceIDs []int
	manualNames := make([]string, 0, 3)
	for _, task := range tasks {
		if strings.EqualFold(task.TaskState, "FINISH") {
			continue
		}
		performedAction := false
		switch task.TaskCode {
		case "AI_CAMERA":
			if err := t.api.CompleteAICameraTask(); err != nil {
				issues = append(issues, fmt.Errorf("许愿 AI 相机: %w", err))
				t.logger.Debug(fmt.Sprintf("拍照问 AI 任务失败: %v", err))
				continue
			}
			performedAction = true
		case "AI_LINGXI":
			if err := t.api.CompleteLingxiChat(); err != nil {
				issues = append(issues, fmt.Errorf("许愿 AI 对话: %w", err))
				t.logger.Debug(fmt.Sprintf("灵犀对话失败: %v", err))
				continue
			}
			advanceIDs = append(advanceIDs, task.TaskID)
			performedAction = true
		case "BACKUP_FILE":
			// 需要一次真实备份快照，服务端才会判定完成。
			if err := t.performAlbumBackup(); err != nil {
				issues = append(issues, fmt.Errorf("许愿备份: %w", err))
				t.logger.Debug(fmt.Sprintf("触发相册备份失败: %v", err))
				continue
			}
			advanceIDs = append(advanceIDs, task.TaskID)
			performedAction = true
		case "UPLOAD_20":
			// 本月累计上传满 20 个文件；一次性补齐差额后由服务端计数。
			if err := t.ensureMonthlyUploads(task.TaskDescription); err != nil {
				issues = append(issues, fmt.Errorf("许愿累计上传: %w", err))
				t.logger.Debug(fmt.Sprintf("补齐上传数量失败: %v", err))
				continue
			}
			advanceIDs = append(advanceIDs, task.TaskID)
			performedAction = true
		case "ENABLE_NOTIFICATION":
			// 通知开关属于设备侧设置，只能尝试推进，通常需账号此前已开启。
			advanceIDs = append(advanceIDs, task.TaskID)
			attempted[task.TaskID] = true
		default:
			if reason, known := makewishManualTasks[task.TaskCode]; known {
				manualNames = append(manualNames, reason)
			} else if task.TaskID != 1007 {
				manualNames = append(manualNames, fmt.Sprintf("%s(%s)", task.TaskName, task.TaskCode))
			}
		}
		if performedAction {
			attempted[task.TaskID] = true
			if err := t.api.MakeWishPageClick(task.TaskID); err != nil {
				issues = append(issues, fmt.Errorf("许愿步骤登记/%d: %w", task.TaskID, err))
				t.logger.Debug(fmt.Sprintf("许愿任务步骤登记失败(%s): %v", task.TaskCode, err))
			}
		}
	}
	if len(advanceIDs) > 0 {
		advanced, err := t.api.MakeWishAutoAdvance(advanceIDs)
		if err != nil {
			t.logger.Debug(fmt.Sprintf("推进许愿进阶任务失败: %v", err))
			issues = append(issues, err)
		}
		for _, task := range advanced {
			t.logger.Debug(fmt.Sprintf("许愿任务推进接口返回: %s（以任务列表状态为准）", task.TaskName))
		}
	}

	// 4. 汇报抽奖码与愿望。
	latest, err := t.api.MakeWishHome()
	if err != nil {
		t.logger.Debug(fmt.Sprintf("复查许愿首页失败: %v", err))
		issues = append(issues, err)
	} else {
		home = latest
	}

	parts := make([]string, 0, 4)
	if home.Wish != nil {
		parts = append(parts, "当前愿望: "+home.Wish.PrizeName)
	} else {
		parts = append(parts, "本期未许愿")
	}
	parts = append(parts, fmt.Sprintf("抽奖码%d张", home.RaffleCodes.TotalCount))
	if len(manualNames) > 0 {
		parts = append(parts, "需手动: "+strings.Join(manualNames, "、"))
	}
	if len(attempted) > 0 {
		latestTasks, _, checkErr := t.api.MakeWishTaskList()
		if checkErr != nil {
			issues = append(issues, fmt.Errorf("复查许愿任务完成位: %w", checkErr))
		} else {
			for _, task := range latestTasks {
				if strings.EqualFold(task.TaskState, "FINISH") {
					delete(attempted, task.TaskID)
				}
			}
		}
		t.pending = len(attempted) > 0
		if t.pending {
			parts = append(parts, fmt.Sprintf("%d项动作已提交，服务端尚未确认完成", len(attempted)))
		}
	}
	t.lastMessage = strings.Join(parts, "; ")
	if err := errors.Join(issues...); err != nil {
		return err
	}
	t.logger.Success("许愿赢好礼: " + t.lastMessage)
	return nil
}

func (t *MakeWishTask) performWish() error {
	prizes, err := t.api.MakeWishPrizeList()
	if err != nil {
		return err
	}
	selected := api.SelectMakeWishPrize(prizes, os.Getenv("CAIYUN_MAKEWISH_PRIZE"))
	if selected == nil {
		return fmt.Errorf("许愿奖品列表为空")
	}
	if err := t.api.MakeWish(selected.MakeWishPrizeID); err != nil {
		return err
	}
	t.logger.Success(fmt.Sprintf("许愿成功: %s", selected.PrizeName))
	return nil
}

func (t *MakeWishTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func (t *MakeWishTask) Pending() bool { return t.pending }

// makewishMonthlyUploadTarget 与 ensureMonthlyUploads 共同实现“本月累计上传
// 文件满 N 个”任务的差额补齐，只上传缺口数量，避免堆积。
const makewishMonthlyUploadTarget = 20

func (t *MakeWishTask) ensureMonthlyUploads(description string) error {
	uploaded := parseWishUploadCount(description)
	missing := makewishMonthlyUploadTarget - uploaded
	if missing < 1 {
		return nil
	}
	if missing > makewishMonthlyUploadTarget {
		missing = makewishMonthlyUploadTarget
	}
	for i := 0; i < missing; i++ {
		if _, _, err := t.uploadTextFile("makewish_upload"); err != nil {
			return err
		}
	}
	t.logger.Success(fmt.Sprintf("补齐本月上传文件 %d 个（原 %d/%d）", missing, uploaded, makewishMonthlyUploadTarget))
	return nil
}

var wishUploadCountPattern = regexp.MustCompile(`(\d+)\s*$`)

// parseWishUploadCount 从“本月累计上传文件X个”中提取 X。
func parseWishUploadCount(description string) int {
	text := strings.TrimSpace(description)
	if text == "" {
		return 0
	}
	// 去掉结尾的“个”等非数字单位后取尾部连续数字。
	text = strings.TrimRight(text, "个 ")
	match := wishUploadCountPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value < 0 {
		return 0
	}
	return value
}
