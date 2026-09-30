package services

import (
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/tasks"
)

// selectCyclicAssistPeer gives every account one different helper when all
// accounts run in a batch. This avoids using one helper for every target.
func selectCyclicAssistPeer(peers []tasks.MailPeer, targetID uint) *tasks.MailPeer {
	var next, first *tasks.MailPeer
	for i := range peers {
		peer := &peers[i]
		if peer.AccountID == 0 || peer.AccountID == targetID {
			continue
		}
		if first == nil || peer.AccountID < first.AccountID {
			first = peer
		}
		if peer.AccountID > targetID && (next == nil || peer.AccountID < next.AccountID) {
			next = peer
		}
	}
	if next != nil {
		return next
	}
	return first
}

func (r *TaskRunner) runMutualAssistTask() *TaskResult {
	started := time.Now()
	peer := selectCyclicAssistPeer(r.mailPeers, r.account.ID)
	if peer == nil {
		return taskResultFromErr("mutual_assist", started, nil, "同一用户下没有其他活跃账号")
	}
	if r.assistTokenMgr == nil || r.assistAccountRepo == nil {
		return taskResultFromErr("mutual_assist", started, fmt.Errorf("互助账号服务未初始化"), "")
	}
	peerAccount, err := r.assistAccountRepo.FindByID(peer.AccountID)
	if err != nil {
		return taskResultFromErr("mutual_assist", started, err, "")
	}
	if peerAccount.UserID != r.account.UserID || !peerAccount.IsActive || peerAccount.ID == r.account.ID {
		return taskResultFromErr("mutual_assist", started, fmt.Errorf("助力账号已不可用或不属于当前用户"), "")
	}
	peerToken, err := r.assistTokenMgr.GetToken(peerAccount.ID)
	if err != nil || peerToken == nil || peerToken.JWTToken == "" {
		return taskResultFromErr("mutual_assist", started, fmt.Errorf("助力账号 Token 不可用: %v", err), "")
	}
	client := corehttp.NewClient()
	client.SetMarketAccount(peerAccount.Phone)
	if peerToken.Auth != "" {
		client.SetAuth(peerToken.Auth)
	} else {
		client.SetAuth(peerAccount.Auth)
	}
	client.SetJWTToken(peerToken.JWTToken)
	if peerToken.SSOToken != "" {
		client.SetSSOToken(peerToken.SSOToken)
	}
	helperAPI := api.NewCaiyunAPI(client)

	var reports []string
	var errors []string
	pending := false
	if outcome, confirmed, err := r.assistMakeWish(helperAPI); err != nil {
		errors = append(errors, "许愿助力: "+err.Error())
	} else {
		reports = append(reports, outcome)
		pending = pending || !confirmed
	}
	if outcome, confirmed, err := r.assistTokenPK(helperAPI); err != nil {
		errors = append(errors, "算力助力: "+err.Error())
	} else {
		reports = append(reports, outcome)
		pending = pending || !confirmed
	}
	if outcome, confirmed, err := r.assistFunAI(helperAPI); err != nil {
		errors = append(errors, "趣玩 AI 助力: "+err.Error())
	} else {
		reports = append(reports, outcome)
		pending = pending || !confirmed
	}
	if len(errors) > 0 {
		return taskResultFromErr("mutual_assist", started, fmt.Errorf("%s；%s", strings.Join(reports, "；"), strings.Join(errors, "；")), "")
	}
	result := taskResultFromErr("mutual_assist", started, nil, strings.Join(reports, "；"))
	if pending {
		result.Status = "pending"
	}
	return result
}

func (r *TaskRunner) assistFunAI(helper *api.CaiyunAPI) (string, bool, error) {
	module, err := r.api.FunaiPageModule()
	if err != nil {
		return "", false, err
	}
	var pendingIDs []string
	for _, task := range module.TaskIDs {
		if task.TaskSign == 1 && task.IsComplete != 1 {
			pendingIDs = append(pendingIDs, task.ID)
		}
	}
	if len(pendingIDs) == 0 {
		return "趣玩 AI 邀请已完成或未下发", true, nil
	}
	code, err := r.api.FunaiInviteCode()
	if err != nil {
		return "", false, err
	}
	helper.PrepareActivitySession(api.FunAIMarketName)
	if err := helper.FunaiAcceptInvite(code); err != nil {
		var business *api.ActivityBusinessError
		if stderrors.As(err, &business) && business.Code == "513" {
			return "趣玩 AI 助力方不满足服务端资格（513）", false, nil
		}
		return "", false, err
	}
	updated, err := r.api.FunaiPageModule()
	if err != nil {
		return "", false, fmt.Errorf("复查趣玩 AI 邀请完成位: %w", err)
	}
	completed := map[string]bool{}
	for _, task := range updated.TaskIDs {
		if task.IsComplete == 1 {
			completed[task.ID] = true
		}
	}
	for _, id := range pendingIDs {
		if !completed[id] {
			return "趣玩 AI 助力已接受，任务待服务端确认", false, nil
		}
	}
	return "趣玩 AI 邀请助力已完成", true, nil
}

func (r *TaskRunner) assistMakeWish(helper *api.CaiyunAPI) (string, bool, error) {
	r.api.PrepareActivitySession(api.MakeWishMarketName)
	tasks, _, err := r.api.MakeWishTaskList()
	if err != nil {
		return "", false, err
	}
	pending := false
	for _, task := range tasks {
		if task.TaskID == 1007 {
			pending = !strings.EqualFold(task.TaskState, "FINISH")
			break
		}
	}
	if !pending {
		return "许愿分享助力已完成或未下发", true, nil
	}
	code, err := r.api.MakeWishGenerateShareCode()
	if err != nil {
		return "", false, err
	}
	helper.PrepareActivitySession(api.MakeWishMarketName)
	eligible, err := helper.MakeWishCanAssist(code)
	if err != nil {
		return "", false, err
	}
	if !eligible {
		return "许愿助力方当前不符合资格", false, nil
	}
	if err := helper.MakeWishAssist(code); err != nil {
		return "", false, err
	}
	time.Sleep(2 * time.Second)
	updated, _, err := r.api.MakeWishTaskList()
	if err != nil {
		return "许愿助力请求已接受，任务状态待复查", false, nil
	}
	for _, task := range updated {
		if task.TaskID == 1007 && strings.EqualFold(task.TaskState, "FINISH") {
			return "许愿分享助力已完成", true, nil
		}
	}
	return "许愿助力请求已接受，任务仍待服务端确认", false, nil
}

func (r *TaskRunner) assistTokenPK(helper *api.CaiyunAPI) (string, bool, error) {
	r.api.PrepareActivitySession(api.TokenPKMarketName)
	tasks, err := r.api.TokenPKTaskList()
	if err != nil {
		return "", false, err
	}
	state := ""
	for _, task := range tasks {
		if task.ID == 7 {
			state = strings.ToUpper(strings.TrimSpace(task.State))
			break
		}
	}
	if state == "" || state == "FINISH" {
		return "算力分享助力已完成或未下发", true, nil
	}
	if state != "SUCCESS" {
		code, err := r.api.TokenPKGenerateInviteCode()
		if err != nil {
			return "", false, err
		}
		helper.PrepareActivitySession(api.TokenPKMarketName)
		if err := helper.TokenPKAcceptInvite(code); err != nil {
			return "", false, err
		}
		time.Sleep(2 * time.Second)
	}
	if err := r.api.TokenPKReceivePrize(7); err != nil {
		return "算力助力已尝试，领奖待服务端确认", false, nil
	}
	return "算力分享助力已领奖", true, nil
}
