package tasks

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

const (
	taskListDefaultBackupRetryCount = 1
	taskListDefaultBackupWaitSecond = 20
)

var taskListRandomCloudTaskIDs = map[int]bool{
	478: true,
	481: true,
}

// 这些任务在 Go 版本中已有独立模块或仍需专门接口，不适合在通用 clickTask 流程里盲点。
var taskListBuiltinSkipTaskIDs = map[int]string{
	106:  "上传任务需要专门接口流程",
	107:  "云笔记任务需要专门接口流程",
	110:  "上传任务需要真实上传实现",
	113:  "上传任务需要专门接口流程",
	434:  "分享文件任务建议走独立分享/邀请任务",
	522:  "每月上传任务需要批量上传策略",
	1021: "邮件通知奖励建议走消息推送奖励任务",
}

var taskListReminderSkipTaskIDs = map[int]bool{
	1021: true, // 该任务已有 messagepush 模块处理，避免重复告警
}

// TaskListTask 任务列表任务（含翻倍奖励和常规任务自动执行）
type TaskListTask struct {
	client    *http.Client
	logger    *logger.Logger
	api       *api.CaiyunAPI
	fileAPI   *api.FileAPI
	storage   Storage
	phone     string
	authToken string
}

type taskListItem struct {
	MarketName string
	GroupKey   string
	Task       api.Task
}

// NewTaskListTask 创建任务列表任务
func NewTaskListTask(client *http.Client, logger *logger.Logger) *TaskListTask {
	return &TaskListTask{
		client:  client,
		logger:  logger,
		api:     api.NewCaiyunAPI(client),
		fileAPI: api.NewFileAPI(client),
	}
}

// SetStorage 设置任务存储（记录临时资源，供收尾清理）
func (t *TaskListTask) SetStorage(store Storage) *TaskListTask {
	t.storage = store
	return t
}

// SetAccountContext 设置账号上下文（手机号和原始 token）
func (t *TaskListTask) SetAccountContext(phone, authToken string) *TaskListTask {
	t.phone = strings.TrimSpace(phone)
	t.authToken = strings.TrimSpace(authToken)
	return t
}

// Run 执行任务列表任务
func (t *TaskListTask) Run() error {
	t.logger.Start("------【任务列表】------")

	// 1. 先尝试领取已完成未领取的任务奖励
	t.receiveCompletedTaskRewards()

	// 2. 自动执行可安全点击的常规任务
	t.runAutomaticTasks()

	// 3. 再次领取任务奖励（覆盖刚执行完成的任务）
	t.receiveCompletedTaskRewards()

	// 4. 输出仍未完成的任务提示
	t.checkIncompleteTasks()

	return nil
}

// receiveTaskExpansion 领取翻倍奖励（参考原版 taskExpansionTask，支持一次自动备份重试）
func (t *TaskListTask) receiveTaskExpansion() {
	retryCount := t.getEnvInt("CAIYUN_TASK_BACKUP_RETRY_COUNT", taskListDefaultBackupRetryCount)
	if retryCount < 0 {
		retryCount = 0
	}
	waitSecond := t.getEnvInt("CAIYUN_TASK_BACKUP_WAIT_SECONDS", taskListDefaultBackupWaitSecond)
	if waitSecond <= 0 {
		waitSecond = taskListDefaultBackupWaitSecond
	}

	for attempt := 0; attempt <= retryCount; attempt++ {
		resp, err := t.api.GetTaskExpansion()
		if err != nil {
			t.logger.Error("获取备份额外奖励失败", err)
			return
		}

		if parseCaiyunCode(resp.Code) != 0 {
			return
		}
		if resp.Result == nil {
			return
		}

		resultMap, ok := resp.Result.(map[string]interface{})
		if !ok {
			return
		}

		curMonthBackup := toBool(resultMap["curMonthBackup"])
		if !curMonthBackup {
			if attempt < retryCount {
				t.logger.Warn("本月未开启备份，尝试自动备份后重试翻倍奖励检查")
				t.tryBackupForTaskExpansion()
				t.logger.Debug(fmt.Sprintf("等待 %d 秒后重试翻倍奖励检查", waitSecond))
				time.Sleep(time.Duration(waitSecond) * time.Second)
				continue
			}

			t.logger.Warn("本月未开启备份，将无法获取翻倍奖励，需要手动开启")
			return
		}

		curMonthTaskRecordCount := toInt(resultMap["curMonthTaskRecordCount"])
		acceptDate := toString(resultMap["acceptDate"])
		if curMonthTaskRecordCount > 0 && acceptDate != "" {
			receiveResp, err := t.api.ReceiveTaskExpansion(acceptDate)
			if err != nil {
				t.logger.Error("领取翻倍奖励失败", err)
				return
			}

			if receiveResp.Result != nil {
				if receiveResultMap, ok := receiveResp.Result.(map[string]interface{}); ok {
					if cloudCount, ok := receiveResultMap["cloudCount"]; ok {
						t.logger.Success(fmt.Sprintf("领取到%v个云朵", cloudCount))
					}
				}
			}
		}

		nextMonthTaskRecordCount := toInt(resultMap["nextMonthTaskRecordCount"])
		if nextMonthTaskRecordCount > 0 {
			t.logger.Debug(fmt.Sprintf("下月可领取%d个云朵", nextMonthTaskRecordCount))
		}
		return
	}
}

// tryBackupForTaskExpansion 尝试触发一次备份（用于满足翻倍奖励条件）
func (t *TaskListTask) tryBackupForTaskExpansion() {
	if t.fileAPI == nil {
		return
	}

	name := fmt.Sprintf("caiyun-backup-%d.txt", time.Now().Unix())
	uploadResp, err := t.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "",
		Name:         name,
		Content:      []byte("caiyun-backup-probe"),
		ChannelSrc:   "10200153",
		OpType:       "backup",
	})
	if err != nil {
		t.logger.Debug("自动备份尝试失败", err)
		return
	}
	if uploadResp != nil && uploadResp.FileID != "" {
		_ = AppendStringList(t.storage, KeyTempFiles, uploadResp.FileID)
	}

	// 说明：当前上传接口仍为简化实现，实际是否触发成功以平台状态为准。
	t.logger.Debug("已尝试执行一次自动备份，用于触发翻倍奖励条件")
}

// runAutomaticTasks 自动执行通用 clickTask 任务
func (t *TaskListTask) runAutomaticTasks() {
	items, err := t.fetchAllTaskItems()
	if err != nil {
		t.logger.Error("获取任务列表失败", err)
		return
	}

	skipIDs := t.loadSkipTaskIDs()
	for _, item := range items {
		task := item.Task

		if task.ID <= 0 {
			continue
		}
		if isTaskCompleted(task) {
			continue
		}
		if skipIDs[task.ID] {
			t.logger.Debug(fmt.Sprintf("按配置跳过任务：%s(%d)", task.Name, task.ID))
			continue
		}
		if isTaskDisabledByServer(task) {
			continue
		}
		if t.handleSpecialTask(item) {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if reason, ok := taskListBuiltinSkipTaskIDs[task.ID]; ok {
			t.logger.Debug(fmt.Sprintf("保留手动/独立处理：%s(%d)，原因：%s", task.Name, task.ID, reason))
			continue
		}

		key := "task"
		if taskListRandomCloudTaskIDs[task.ID] {
			key = "randomCloudTask"
		}

		if err := t.api.DoTask(key, strconv.Itoa(task.ID)); err != nil {
			t.logger.Debug(fmt.Sprintf("自动执行任务失败（保留手动处理）：%s(%d)：%v", task.Name, task.ID, err))
			continue
		}

		t.logger.Debug(fmt.Sprintf("已尝试自动执行任务：%s(%d)", task.Name, task.ID))
		time.Sleep(500 * time.Millisecond)
	}
}

// receiveCompletedTaskRewards 领取已完成未领取的任务奖励（status=1）
func (t *TaskListTask) receiveCompletedTaskRewards() {
	items, err := t.fetchAllTaskItems()
	if err != nil {
		t.logger.Debug("刷新任务列表失败，跳过自动领奖", err)
		return
	}

	for _, item := range items {
		task := item.Task
		if task.Status != 1 {
			continue
		}

		if err := t.api.ReceiveTaskReward(strconv.Itoa(task.ID)); err != nil {
			t.logger.Debug(fmt.Sprintf("自动领取任务奖励失败：%s(%d)：%v", task.Name, task.ID, err))
			continue
		}

		t.logger.Success(fmt.Sprintf("领取任务奖励成功：%s(%d)", task.Name, task.ID))
		time.Sleep(300 * time.Millisecond)
	}
}

// checkIncompleteTasks 检查未完成任务
func (t *TaskListTask) checkIncompleteTasks() {
	items, err := t.fetchAllTaskItems()
	if err != nil {
		t.logger.Error("获取任务列表失败:", err)
		return
	}

	skipIDs := t.loadSkipTaskIDs()
	for _, item := range items {
		task := item.Task
		if task.ID <= 0 {
			continue
		}
		if taskListReminderSkipTaskIDs[task.ID] {
			continue
		}
		if skipIDs[task.ID] {
			continue
		}
		if isTaskCompleted(task) {
			continue
		}
		if isTaskDisabledByServer(task) {
			continue
		}

		groupName := getGroupName(item.GroupKey)
		taskName := getTaskName(task.ID)
		if taskName == "" {
			taskName = task.Name
		}

		t.logger.Fail(fmt.Sprintf("未完成：请前往移动云盘手动完成%s：%s(%d)", groupName, taskName, task.ID))
	}
}

// handleSpecialTask 处理 mjs 中需要专门流程的任务（107/110/434/522）
func (t *TaskListTask) handleSpecialTask(item taskListItem) bool {
	task := item.Task
	switch task.ID {
	case 106:
		t.tryClickTask(task)
		if err := t.handleUploadTask(1, "", ""); err != nil {
			t.logger.Debug(fmt.Sprintf("上传任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	case 107:
		t.tryClickTask(task)
		if err := t.handleNoteTask(); err != nil {
			t.logger.Debug(fmt.Sprintf("云笔记任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	case 110:
		t.tryClickTask(task)
		if err := t.handleUploadTask(1, "10000023", ""); err != nil {
			t.logger.Debug(fmt.Sprintf("上传任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	case 113:
		t.tryClickTask(task)
		t.tryRefreshNoteAuthToken()
		if err := t.handleUploadTask(1, "10200153", ""); err != nil {
			t.logger.Debug(fmt.Sprintf("上传任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	case 434:
		t.tryClickTask(task)
		if err := t.handleShareFileTask(); err != nil {
			t.logger.Debug(fmt.Sprintf("分享文件任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	case 522:
		t.tryClickTask(task)
		limit := t.getEnvInt("CAIYUN_TASK_MONTHLY_UPLOAD_DAILY_COUNT", 5)
		if limit < 0 {
			limit = 0
		}
		need := limit
		if task.Process >= 100 {
			need = 0
		}
		if err := t.handleUploadTask(need, "10000023", ""); err != nil {
			t.logger.Debug(fmt.Sprintf("每月上传任务执行失败（%d）：%v", task.ID, err))
		}
		return true
	}
	return false
}

// tryClickTask 尝试点击任务入口（失败仅记录调试日志）
func (t *TaskListTask) tryClickTask(task api.Task) {
	if err := t.api.DoTask("task", strconv.Itoa(task.ID)); err != nil {
		t.logger.Debug(fmt.Sprintf("点击任务失败（继续后续流程）：%s(%d)：%v", task.Name, task.ID, err))
	}
}

func (t *TaskListTask) tryRefreshNoteAuthToken() {
	if t.authToken == "" || t.phone == "" {
		return
	}
	if _, err := t.api.GetNoteAuthToken(t.authToken, t.phone); err != nil {
		t.logger.Debug("刷新 token 失败", err)
	}
}

// handleUploadTask 执行上传类任务（110/522）
func (t *TaskListTask) handleUploadTask(times int, channelSrc, opType string) error {
	if times <= 0 {
		return nil
	}
	if t.fileAPI == nil {
		return fmt.Errorf("文件 API 未初始化")
	}

	for i := 0; i < times; i++ {
		resp, err := t.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
			ParentFileID: "/",
			ChannelSrc:   channelSrc,
			OpType:       opType,
		})
		if err != nil {
			return err
		}
		if resp != nil && resp.FileID != "" {
			_ = AppendStringList(t.storage, KeyTempFiles, resp.FileID)
			t.logger.Debug(fmt.Sprintf("上传临时文件成功：%s", resp.FileID))
		}
		if i < times-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

// handleShareFileTask 执行分享文件任务（434）
func (t *TaskListTask) handleShareFileTask() error {
	if t.phone == "" {
		return fmt.Errorf("缺少手机号，无法创建分享链接")
	}

	fileID, fileName, err := t.selectShareTargetFile()
	if err != nil {
		return err
	}
	if fileID == "" {
		return fmt.Errorf("没有可用于分享的文件")
	}

	resp, err := t.api.GetOutLink(t.phone, []string{fileID}, "")
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("分享接口返回为空")
	}

	codeStr := strings.TrimSpace(fmt.Sprint(resp.Code))
	if !resp.Success && codeStr != "" && codeStr != "0" {
		return fmt.Errorf("创建分享链接失败: code=%v, msg=%s", resp.Code, resp.Message)
	}

	var linkIDs []string
	for _, item := range resp.Data.GetOutLinkRes.GetOutLinkResSet {
		if item.LinkID != "" {
			linkIDs = append(linkIDs, item.LinkID)
		}
	}
	if len(linkIDs) == 0 {
		return fmt.Errorf("创建分享链接成功但未拿到 linkID")
	}

	t.logger.Success(fmt.Sprintf("分享文件成功：%s(%s)", fileName, fileID))

	delResp, err := t.api.DelOutLink(t.phone, linkIDs)
	if err != nil || delResp == nil || !delResp.IsSuccess() {
		_ = AppendStringList(t.storage, KeyTempLinks, linkIDs...)
		if err != nil {
			return fmt.Errorf("删除分享链接失败，已登记收尾清理: %w", err)
		}
		return fmt.Errorf("删除分享链接失败，已登记收尾清理: %v", delResp)
	}

	return nil
}

// handleNoteTask 执行云笔记任务（107）
func (t *TaskListTask) handleNoteTask() error {
	if t.authToken == "" || t.phone == "" {
		return fmt.Errorf("缺少账号 token 或手机号")
	}

	noteAuth, err := t.api.GetNoteAuthToken(t.authToken, t.phone)
	if err != nil {
		return err
	}
	if noteAuth == nil || noteAuth.Headers["app_auth"] == "" {
		return fmt.Errorf("未获取到云笔记 app_auth")
	}

	noteID := utils.RandomString(32)
	title := utils.RandomString(3)
	if err := t.api.CreateNote(noteID, title, t.phone, noteAuth.Headers, nil); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	if err := t.api.DeleteNote(noteID, noteAuth.Headers); err != nil {
		return err
	}

	t.logger.Success("云笔记任务执行成功")
	return nil
}

// selectShareTargetFile 选择一个可分享的文件（优先临时上传文件）
func (t *TaskListTask) selectShareTargetFile() (string, string, error) {
	if ids, err := LoadStringList(t.storage, KeyTempFiles); err == nil {
		for _, id := range ids {
			if id != "" {
				return id, "temp", nil
			}
		}
	}

	if t.fileAPI == nil {
		return "", "", fmt.Errorf("文件 API 未初始化")
	}

	listResp, err := t.fileAPI.GetFileList("/")
	if err != nil {
		return "", "", err
	}
	if listResp == nil || len(listResp.Files) == 0 {
		return "", "", nil
	}

	for _, file := range listResp.Files {
		if file.FileID != "" {
			return file.FileID, file.Name, nil
		}
	}
	return "", "", nil
}

// fetchAllTaskItems 获取主任务列表和邮箱任务列表（参考原版 Qm）
func (t *TaskListTask) fetchAllTaskItems() ([]taskListItem, error) {
	marketNames := []string{"sign_in_3", "newsign_139mail"}
	items := make([]taskListItem, 0, 64)

	for _, marketName := range marketNames {
		taskList, err := t.api.GetTaskList(marketName)
		if err != nil {
			// 邮箱任务列表在部分账号上可能不可用，不中断主流程。
			if marketName == "newsign_139mail" {
				t.logger.Debug("获取邮箱任务列表失败，已跳过", err)
				continue
			}
			return nil, fmt.Errorf("获取任务列表失败(%s): %w", marketName, err)
		}

		if taskList == nil {
			continue
		}
		if taskList.Code != 0 {
			if marketName == "newsign_139mail" {
				t.logger.Debug(fmt.Sprintf("邮箱任务列表返回失败，已跳过：%s", taskList.Message))
				continue
			}
			return nil, fmt.Errorf("获取任务列表失败(%s): %s", marketName, taskList.Message)
		}

		for group, tasks := range taskList.Result {
			for _, task := range tasks {
				if task.GroupID == "" {
					task.GroupID = group
				}
				if task.MarketName == "" {
					task.MarketName = marketName
				}

				items = append(items, taskListItem{
					MarketName: marketName,
					GroupKey:   group,
					Task:       task,
				})
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Task.ID == items[j].Task.ID {
			return items[i].MarketName < items[j].MarketName
		}
		return items[i].Task.ID < items[j].Task.ID
	})

	return items, nil
}

// loadSkipTaskIDs 从环境变量读取跳过任务列表（格式：1,2,3）
func (t *TaskListTask) loadSkipTaskIDs() map[int]bool {
	result := make(map[int]bool)
	raw := strings.TrimSpace(os.Getenv("CAIYUN_TASKLIST_SKIP_IDS"))
	if raw == "" {
		return result
	}

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		result[id] = true
	}
	return result
}

// getEnvInt 读取环境变量整数，读取失败时返回默认值
func (t *TaskListTask) getEnvInt(key string, defaultVal int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return val
}

// isTaskCompleted 判断任务是否已完成（含已领取）
func isTaskCompleted(task api.Task) bool {
	if task.Status == 1 || task.Status == 2 {
		return true
	}
	return strings.EqualFold(task.State, "FINISH")
}

// isTaskDisabledByServer 判断任务是否被服务端禁用
func isTaskDisabledByServer(task api.Task) bool {
	// 仅在 state 字段存在时信任 enable，避免 sign_in_3 任务因缺省值被误判。
	if task.State == "" {
		return false
	}
	return task.Enable != 1
}

// parseCaiyunCode 兼容解析 code 字段
func parseCaiyunCode(code interface{}) int {
	switch v := code.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if v == "0" {
			return 0
		}
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return -1
}

func toBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "true" || x == "1" || x == "yes"
	case float64:
		return x != 0
	case int:
		return x != 0
	}
	return false
}

func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
	}
	return 0
}

func toString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}

// getGroupName 获取任务组名称
func getGroupName(group string) string {
	names := map[string]string{
		"day":        "每日任务",
		"month":      "每月任务",
		"new":        "新用户任务",
		"time":       "热门任务",
		"hidden":     "隐藏任务",
		"hiddenabc":  "隐藏任务",
		"beiyong1":   "临时任务",
		"cloudEmail": "邮箱任务",
	}

	if name, ok := names[group]; ok {
		return name
	}
	return "未知任务"
}

// getTaskName 获取任务名称
func getTaskName(taskID int) string {
	names := map[int]string{
		472:  "去体验139邮箱",
		447:  "去中国移动APP领好礼",
		409:  "从固定入口访问云朵中心",
		1004: "给好友发邮件",
		1014: "体验“PDF转换”功能",
		1015: "体验“文件收集”功能",
		1020: "完成一次邮箱资产备份",
		1028: "体验AI工作台",
		1029: "查看“我的账单”",
	}

	if name, ok := names[taskID]; ok {
		return name
	}
	return ""
}
