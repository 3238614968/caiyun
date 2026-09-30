package tasks

import (
	"encoding/json"
	"errors"
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
}

// 这些任务在 Go 版本中已有独立模块或仍需专门接口，不适合在通用 clickTask 流程里盲点。
var taskListBuiltinSkipTaskIDs = map[int]string{
	106:  "上传任务需要专门接口流程",
	107:  "云笔记任务需要专门接口流程",
	110:  "上传任务需要真实上传实现",
	113:  "上传任务需要专门接口流程",
	434:  "分享文件任务建议走独立分享/邀请任务",
	585:  "AI 相机任务需要专门接口流程",
	522:  "每月上传任务需要批量上传策略",
	1021: "邮件通知奖励建议走消息推送奖励任务",
}

var taskListReminderSkipTaskIDs = map[int]bool{
	1021: true, // 该任务已有 messagepush 模块处理，避免重复告警
}

// TaskListTask 任务列表任务（含翻倍奖励和常规任务自动执行）
type TaskListTask struct {
	client           *http.Client
	logger           *logger.Logger
	api              *api.CaiyunAPI
	fileAPI          *api.FileAPI
	storage          Storage
	phone            string
	authToken        string
	claimedCloud     int
	expansionPending bool
}

func (t *TaskListTask) ClaimedCloud() int { return t.claimedCloud }

type taskListItem struct {
	MarketName string
	GroupKey   string
	Task       api.Task
}

var taskListV2Groups = []string{"cloudEmail", "time", "day", "month"}

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
	t.claimedCloud = 0

	// 1. 先尝试领取已完成未领取的任务奖励
	_ = t.receiveCompletedTaskRewards()

	// 2. 注册新版任务中心所需 deviceId，避免部分 taskListV2 任务点击失败。
	t.registerTaskDevice()

	// 3. 自动执行可安全点击的常规任务
	runErr := t.runAutomaticTasks()

	// 4. 再次领取任务奖励（覆盖刚执行完成的任务）
	rewardErr := t.receiveCompletedTaskRewards()

	// 5. 先撤销分享链接，再删除临时文件，避免在任务上报前破坏分享行为。
	if t.cleanupTemporaryShareLinks() {
		t.cleanupTemporaryFiles()
	}

	// 6. 输出仍未完成的任务提示
	t.checkIncompleteTasks()

	return errors.Join(runErr, rewardErr)
}

func (t *TaskListTask) registerTaskDevice() {
	resp, err := t.api.DoTaskPost()
	if err != nil {
		t.logger.Debug("新版任务中心 deviceId 注册失败", err)
		return
	}
	if resp != nil && !resp.IsSuccess() {
		t.logger.Debug(fmt.Sprintf("新版任务中心 deviceId 注册返回异常: code=%v msg=%s", resp.Code, resp.MessageText()))
	}
}

// receiveTaskExpansion 领取翻倍奖励（参考原版 taskExpansionTask，支持一次自动备份重试）
func (t *TaskListTask) receiveTaskExpansion() {
	t.expansionPending = false
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
			t.expansionPending = true
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
func (t *TaskListTask) runAutomaticTasks() error {
	items, err := t.fetchAllTaskItems()
	if err != nil {
		t.logger.Error("获取任务列表失败", err)
		return err
	}

	skipIDs := t.loadSkipTaskIDs()
	sharedUploadProcessed := false
	var actionErrors []error
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
		if task.ID == 522 && sharedUploadProcessed {
			continue
		}
		if handled, actionErr := t.handleSpecialTask(item); handled {
			if actionErr != nil {
				actionErrors = append(actionErrors, fmt.Errorf("%s/%d: %w", item.MarketName, task.ID, actionErr))
			}
			if task.ID == 522 && actionErr == nil {
				sharedUploadProcessed = true
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if reason, ok := taskListBuiltinSkipTaskIDs[task.ID]; ok {
			t.logger.Debug(fmt.Sprintf("保留手动/独立处理：%s(%d)，原因：%s", task.Name, task.ID, reason))
			continue
		}

		clickKeys := t.getTaskClickKeys(task)
		if len(clickKeys) == 0 {
			continue
		}

		var runErr error
		for _, key := range clickKeys {
			if err := t.api.DoTaskWithMarket(item.MarketName, key, strconv.Itoa(task.ID)); err != nil {
				runErr = err
				break
			}
		}
		if runErr != nil {
			t.logger.Debug(fmt.Sprintf("自动执行任务失败（保留手动处理）：%s(%d)：%v", task.Name, task.ID, runErr))
			actionErrors = append(actionErrors, fmt.Errorf("%s/%d: %w", item.MarketName, task.ID, runErr))
			continue
		}

		t.logger.Debug(fmt.Sprintf("已尝试自动执行任务：%s(%d)", task.Name, task.ID))
		time.Sleep(500 * time.Millisecond)
	}
	return errors.Join(actionErrors...)
}

// receiveCompletedTaskRewards 领取已完成未领取的任务奖励（status=1）
func (t *TaskListTask) receiveCompletedTaskRewards() error {
	items, err := t.fetchAllTaskItems()
	if err != nil {
		t.logger.Debug("刷新任务列表失败，跳过自动领奖", err)
		return err
	}

	var claimErrors []error
	for _, item := range items {
		task := item.Task
		// V3 task IDs are not cloud reward IDs. Completed V3 tasks use the
		// receiveList bubbles below, including those without canReceive flags.
		if task.State != "" || task.Status != 1 {
			continue
		}

		if err := t.api.ReceiveTaskRewardForMarket(item.MarketName, strconv.Itoa(task.ID)); err != nil {
			t.logger.Debug(fmt.Sprintf("自动领取任务奖励失败：%s(%d)：%v", task.Name, task.ID, err))
			claimErrors = append(claimErrors, fmt.Errorf("%s/%d: %w", item.MarketName, task.ID, err))
			continue
		}

		t.logger.Success(fmt.Sprintf("领取任务奖励成功：%s(%d)", task.Name, task.ID))
		time.Sleep(300 * time.Millisecond)
	}
	response, claimErr := t.api.ReceivePendingCloudRewards()
	if response != nil {
		if payload, ok := response.Result.(map[string]interface{}); ok {
			t.claimedCloud += responseNumber(payload["receivedCloud"])
		}
		t.logger.Info(response.MessageText())
	}
	if claimErr != nil {
		claimErrors = append(claimErrors, claimErr)
	}
	return errors.Join(claimErrors...)
}

func taskHasClaimableReward(task api.Task) bool {
	if !strings.EqualFold(task.State, "FINISH") {
		return false
	}
	for _, button := range task.Button {
		if toBool(button["canReceive"]) {
			return true
		}
	}
	return false
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

		t.logger.Warn(fmt.Sprintf("服务端仍显示待完成：%s：%s(%d)，可能需要继续等待、账号资格或 App 内操作", groupName, taskName, task.ID))
	}
}

// handleSpecialTask executes tasks that require a real cloud, AI or sharing action.
func (t *TaskListTask) handleSpecialTask(item taskListItem) (bool, error) {
	task := item.Task
	if task.ID == 1057 || task.ID == 1028 || (item.MarketName == "newsign_139mail" && strings.Contains(task.Name, "AI工作台")) {
		return true, t.api.CompleteAIWorkbenchTask()
	}
	var action func() error
	switch task.ID {
	case 106:
		action = func() error { return t.handleUploadTask(1, "", "") }
	case 107:
		action = t.handleNoteTask
	case 110:
		action = func() error { return t.handleUploadTask(1, "10000023", "") }
	case 113:
		action = func() error { return t.handleUploadTask(1, "10200153", "") }
	case 614, 615, 1080, 1081:
		action = t.handlePhotoUploadTask
	case 585, 616, 618, 1082, 1084:
		action = t.api.CompleteAICameraTask
	case 617, 1083:
		action = func() error { return t.api.CompleteFunAIImageTask("006") }
	case 609:
		action = t.api.CompleteLingxiChat
	case 547:
		action = t.api.CreateTaskKnowledgeBase
	case 319, 409, 431, 604, 1003, 1005, 1017, 1055:
		// These sign-in center tasks are driven by a real cloud file action.
		action = func() error { return t.handleUploadTask(1, "10000023", "") }
	case 434:
		action = t.handleShareFileTask
	case 522:
		action = func() error { return t.handleMonthlyUploadTask(item) }
	default:
		if hasTaskStep(task, "email") || hasTaskStep(task, "interfacecallback") || hasTaskStep(task, "firsttaskcheck") {
			return false, nil
		}
		switch {
		case strings.Contains(task.Name, "智能消除"):
			action = func() error { return t.api.CompleteFunAIImageTask("006") }
		case strings.Contains(task.Name, "AI相机"), strings.Contains(task.Name, "拍照问AI"):
			action = t.api.CompleteAICameraTask
		case strings.Contains(task.Name, "AI助手对话"), strings.Contains(task.Name, "灵犀对话"):
			action = t.api.CompleteLingxiChat
		case strings.Contains(task.Name, "欢乐瞬间"), strings.Contains(task.Name, "团圆照"):
			action = t.handlePhotoUploadTask
		default:
			return false, nil
		}
	}
	maxRounds := 1
	switch task.ID {
	case 106, 113, 614, 615, 1080, 1081, 319, 409, 431, 604, 1003, 1005, 1017, 1055, 585, 609, 616, 617, 618, 1082, 1083, 1084:
		maxRounds = 4
	}
	for round := 0; round < maxRounds; round++ {
		if err := action(); err != nil {
			t.logger.Debug(fmt.Sprintf("任务真实行为失败：%s(%d)：%v", task.Name, task.ID, err))
			return true, err
		}
		item.Task = task
		if err := t.reportTaskAction(item); err != nil {
			t.logger.Debug(fmt.Sprintf("任务步骤登记失败：%s(%d)：%v", task.Name, task.ID, err))
			return true, err
		}
		t.logger.Success(fmt.Sprintf("已完成行为并登记任务步骤：%s(%d)", task.Name, task.ID))
		if round+1 == maxRounds {
			break
		}
		time.Sleep(1500 * time.Millisecond)
		refreshed, err := t.queryTaskByMarket(item.MarketName, item.GroupKey, task.ID)
		if err != nil || refreshed == nil || isTaskCompleted(*refreshed) ||
			(refreshed.CurrStep <= task.CurrStep && refreshed.Process <= task.Process) {
			break
		}
		task = *refreshed
	}
	return true, nil
}

func (t *TaskListTask) reportTaskAction(item taskListItem) error {
	response, err := t.api.DoTaskPostForTask(item.MarketName, item.Task.ID)
	if err != nil {
		return err
	}
	if response == nil || !response.IsSuccess() {
		if response == nil {
			return fmt.Errorf("doTaskPost 返回空响应")
		}
		return fmt.Errorf("doTaskPost code=%v msg=%s", response.Code, response.MessageText())
	}
	for _, key := range taskActionRegistrationKeys(item.Task) {
		if err := t.api.DoTaskWithMarket(item.MarketName, key, strconv.Itoa(item.Task.ID)); err != nil {
			return err
		}
	}
	return nil
}

func taskActionRegistrationKeys(task api.Task) []string {
	switch task.ID {
	case 1003:
		return []string{"openAppPush"}
	case 1005:
		return []string{"task2"}
	case 409:
		if task.CurrStep > 0 {
			return []string{"task2"}
		}
		return []string{"task", "task2"}
	default:
		return []string{"task"}
	}
}

func hasTaskStep(task api.Task, step string) bool {
	for _, item := range task.StepTypeSet {
		if strings.EqualFold(item, step) {
			return true
		}
	}
	return false
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
		name := fmt.Sprintf("auto_upload_%d_%d.txt", time.Now().UnixNano(), i)
		resp, err := t.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
			ParentFileID: "/",
			Name:         name,
			ChannelSrc:   channelSrc,
			OpType:       opType,
			Ext:          ".txt",
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

func (t *TaskListTask) handlePhotoUploadTask() error {
	if t.fileAPI == nil {
		return fmt.Errorf("文件 API 未初始化")
	}
	content, err := api.GenerateUniqueSampleJPEG(600, 800)
	if err != nil {
		return err
	}
	resp, err := t.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         fmt.Sprintf("auto_photo_%d.jpg", time.Now().UnixNano()),
		Content:      content,
		ContentType:  "image/jpeg",
		ChannelSrc:   "10000023",
		Ext:          ".jpg",
	})
	if err != nil {
		return err
	}
	if resp == nil || resp.FileID == "" {
		return fmt.Errorf("照片上传未返回 fileId")
	}
	return AppendStringList(t.storage, KeyTempFiles, resp.FileID)
}

// handleShareFileTask 执行分享文件任务（434）
func (t *TaskListTask) handleShareFileTask() error {
	if t.phone == "" {
		return fmt.Errorf("缺少手机号，无法创建分享链接")
	}

	fileName := fmt.Sprintf("auto_share_%d.txt", time.Now().UnixNano())
	uploadResp, err := t.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         fileName,
		ChannelSrc:   "10000023",
		Ext:          ".txt",
	})
	if err != nil {
		return err
	}
	if uploadResp == nil || uploadResp.FileID == "" {
		return fmt.Errorf("分享文件任务创建临时文件失败")
	}
	fileID := uploadResp.FileID
	_ = AppendStringList(t.storage, KeyTempFiles, fileID)

	resp, err := t.api.GetOutLink(t.phone, []string{fileID}, fileName)
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
	return AppendStringList(t.storage, KeyTempLinks, linkIDs...)
}

func (t *TaskListTask) handleMonthlyUploadTask(item taskListItem) error {
	task := item.Task
	currentProcess := task.Process
	target := 100
	if currentProcess >= target || strings.EqualFold(task.State, "FINISH") {
		return nil
	}

	for attempt := 0; attempt < 3; attempt++ {
		remaining := target - currentProcess
		if remaining <= 0 {
			return nil
		}
		if err := t.handleUploadTask(remaining, "10000023", ""); err != nil {
			return err
		}

		refreshedTask, err := t.queryTaskByMarket(item.MarketName, "time", task.ID)
		if err != nil {
			return err
		}
		if refreshedTask == nil {
			return fmt.Errorf("刷新月上传任务进度失败")
		}
		if strings.EqualFold(refreshedTask.State, "FINISH") || refreshedTask.Process >= target {
			t.logger.Success("月上传补传任务执行成功")
			return nil
		}
		if refreshedTask.Process <= currentProcess {
			return fmt.Errorf("月上传进度未推进，当前 %d/%d", refreshedTask.Process, target)
		}
		currentProcess = refreshedTask.Process
	}

	return fmt.Errorf("月上传补传未完成，当前 %d/%d", currentProcess, target)
}

func (t *TaskListTask) queryTaskByMarket(marketName, group string, taskID int) (*api.Task, error) {
	current, err := t.api.GetTaskListV3(marketName)
	if err == nil {
		for _, task := range current {
			if task.ID == taskID {
				return &task, nil
			}
		}
		return nil, nil
	}
	if marketName == "sign_in_3" {
		return t.queryTaskV2ByGroup(group, taskID)
	}
	return nil, err
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
		if currentTasks, err := t.api.GetTaskListV3(marketName); err == nil {
			for _, task := range currentTasks {
				if task.GroupID == "" {
					task.GroupID = task.Group
				}
				task.MarketName = marketName
				items = append(items, taskListItem{
					MarketName: marketName,
					GroupKey:   task.GroupID,
					Task:       task,
				})
			}
			continue
		} else {
			t.logger.Debug(fmt.Sprintf("V3 任务列表不可用，回退旧版(%s): %v", marketName, err))
		}
		if marketName == "sign_in_3" {
			for _, group := range taskListV2Groups {
				taskList, err := t.api.GetTaskListV2(group)
				if err != nil {
					return nil, fmt.Errorf("获取新版任务列表失败(%s): %w", group, err)
				}
				if taskList == nil {
					continue
				}
				if taskList.Code != 0 {
					return nil, fmt.Errorf("获取新版任务列表失败(%s): %s", group, taskList.MessageText())
				}

				for _, task := range taskList.Result[group] {
					if task.GroupID == "" {
						task.GroupID = group
					}
					task.MarketName = marketName
					items = append(items, taskListItem{
						MarketName: marketName,
						GroupKey:   group,
						Task:       task,
					})
				}
			}
			continue
		}

		taskList, err := t.api.GetTaskList(marketName)
		if err != nil {
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
			return nil, fmt.Errorf("获取任务列表失败(%s): %s", marketName, taskList.MessageText())
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
	if strings.TrimSpace(task.State) != "" {
		return strings.EqualFold(task.State, "FINISH")
	}
	if task.Status == 1 || task.Status == 2 {
		return true
	}
	return false
}

// isTaskDisabledByServer 判断任务是否被服务端禁用
func isTaskDisabledByServer(task api.Task) bool {
	// enable 是可选字段；缺省不能按禁用处理。
	return task.Enable != nil && *task.Enable != 1
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
	case json.Number:
		n, err := x.Int64()
		return err == nil && n != 0
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
	case json.Number:
		n, _ := x.Int64()
		return int(n)
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
		434:  "分享文件",
		522:  "当月上传文件满100个",
		585:  "AI相机",
		1004: "给好友发邮件",
		1014: "体验“PDF转换”功能",
		1015: "体验“文件收集”功能",
		1020: "完成一次邮件转存",
		1028: "体验AI工作台",
		1057: "体验AI工作台",
		1029: "查看“我的账单”",
	}

	if name, ok := names[taskID]; ok {
		return name
	}
	return ""
}

func (t *TaskListTask) getTaskClickKeys(task api.Task) []string {
	if task.ID == 549 {
		seenAvailability := false
		canReceive := false
		for _, button := range task.Button {
			if value, exists := button["canReceive"]; exists {
				seenAvailability = true
				canReceive = canReceive || toBool(value)
			}
		}
		if seenAvailability && !canReceive {
			return nil
		}
	}
	if task.ID == 409 {
		if task.CurrStep > 0 {
			return []string{"task2"}
		}
		return []string{"task", "task2"}
	}
	if taskListRandomCloudTaskIDs[task.ID] {
		if task.CurrStep == 0 {
			return []string{"randomCloudTask"}
		}
		return []string{"task"}
	}
	for _, stepType := range task.StepTypeSet {
		if !strings.EqualFold(stepType, "click") {
			// 多步骤任务必须先完成真实行为；通用点击不能代替上传、邮箱或 AI 回调。
			return nil
		}
	}
	if len(task.StepTypeSet) == 1 {
		return []string{"task"}
	}
	return nil
}

func (t *TaskListTask) queryTaskV2ByGroup(group string, taskID int) (*api.Task, error) {
	taskList, err := t.api.GetTaskListV2(group)
	if err != nil {
		return nil, err
	}
	if taskList == nil || taskList.Code != 0 {
		return nil, fmt.Errorf("获取任务列表失败: %s", group)
	}
	for _, task := range taskList.Result[group] {
		if task.ID == taskID {
			taskCopy := task
			return &taskCopy, nil
		}
	}
	return nil, nil
}

func (t *TaskListTask) cleanupTemporaryFiles() {
	if t.fileAPI == nil {
		return
	}

	fileIDs, _ := LoadStringList(t.storage, KeyTempFiles)
	if len(fileIDs) == 0 {
		return
	}

	uniq := make(map[string]bool, len(fileIDs))
	finalIDs := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		if strings.TrimSpace(fileID) == "" || uniq[fileID] {
			continue
		}
		uniq[fileID] = true
		finalIDs = append(finalIDs, fileID)
	}
	if len(finalIDs) == 0 {
		return
	}

	delResp, err := t.fileAPI.DeleteFiles(finalIDs)
	if err != nil {
		t.logger.Debug("清理临时上传文件失败", err)
		return
	}
	if delResp != nil && delResp.Success {
		_ = SaveStringList(t.storage, KeyTempFiles, nil)
		t.logger.Debug(fmt.Sprintf("已清理临时上传/分享文件 %d 个", len(finalIDs)))
	}
}

func (t *TaskListTask) cleanupTemporaryShareLinks() bool {
	linkIDs, err := LoadStringList(t.storage, KeyTempLinks)
	if err != nil {
		t.logger.Debug("读取临时分享链接失败", err)
		return false
	}
	if len(linkIDs) == 0 {
		return true
	}
	if t.phone == "" {
		return false
	}
	resp, err := t.api.DelOutLink(t.phone, linkIDs)
	if err != nil || resp == nil || !resp.IsSuccess() {
		t.logger.Debug("清理临时分享链接失败，保留给收尾任务重试", err)
		return false
	}
	_ = SaveStringList(t.storage, KeyTempLinks, nil)
	return true
}
