package tasks

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"caiyun/internal/core/api"
)

// The reference's task_seed uses a live task list instead of the discontinued
// 2999 promotion. These rewards are sunlight, never cloud balance gains.
func (t *Coc3Task) runSeed() []string {
	before, err := t.api.Coc3GrowthValue(t.tokenValue)
	if err != nil {
		if coc3Unavailable(err) {
			return []string{"AI豆苗活动已结束"}
		}
		return []string{t.failure("AI豆苗状态", err)}
	}
	parts := t.runSeedSign()
	initial, err := t.api.Coc3GrowthTasks(t.tokenValue)
	if err != nil {
		parts = append(parts, t.failure("AI豆苗任务清单", err))
	} else {
		for _, task := range initial {
			t.progressTaskIDs = append(t.progressTaskIDs, task.ID)
		}
		seen := map[int]bool{}
		for _, task := range initial {
			if task.Finished || seen[task.ID] {
				continue
			}
			seen[task.ID] = true
			label := fmt.Sprintf("%s(%d)", task.Title, task.ID)
			if task.Type != 1 && task.Type != 2 && task.Type != 4 && task.Type != 5 {
				t.pending = true
				parts = append(parts, "AI豆苗 "+label+"需真实订购、好友或App操作，已跳过")
				continue
			}
			page := ""
			if u, err := url.Parse(task.RedirectURL); err == nil {
				if _, tail, ok := strings.Cut(u.Path, "/online/"); ok {
					page = strings.Split(tail, "/")[0]
				}
			}
			if task.Type != 2 && page == "" {
				t.pending = true
				parts = append(parts, "AI豆苗 "+label+"未下发目标页面，未提交")
				continue
			}
			resp, err := t.api.ReportCoc3GrowthTask(t.tokenValue, task, page)
			if err != nil {
				parts = append(parts, t.failure("AI豆苗 "+label, err))
			} else if strings.Contains(string(resp.Data), "频繁") {
				t.pending = true
				parts = append(parts, "AI豆苗 "+label+"限频，留待下次复查")
			}
			t.pause(time.Second)
		}
		latest, err := t.api.Coc3GrowthTasks(t.tokenValue)
		if err != nil {
			parts = append(parts, t.failure("AI豆苗完成状态复查", err))
		} else {
			states := map[int]bool{}
			for _, task := range latest {
				states[task.ID] = task.Finished
			}
			done, total := 0, 0
			seen = map[int]bool{}
			var incomplete []string
			for _, task := range initial {
				if seen[task.ID] {
					continue
				}
				seen[task.ID] = true
				total++
				if states[task.ID] {
					done++
				} else {
					incomplete = append(incomplete, fmt.Sprint(task.ID))
				}
			}
			parts = append(parts, fmt.Sprintf("AI豆苗服务端确认完成%d/%d项", done, total))
			if total == 0 {
				t.pending = true
				parts = append(parts, "AI豆苗未下发可核验任务")
			}
			if len(incomplete) > 0 {
				t.pending = true
				parts = append(parts, "AI豆苗待完成："+strings.Join(incomplete, "、"))
			}
		}
	}
	return append(parts, t.claimSeedBubbles(before)...)
}

func (t *Coc3Task) runSeedSign() []string {
	info, err := t.api.Coc3SeedSignInfo(t.tokenValue)
	if err != nil {
		return []string{t.failure("AI豆苗签到查询", err)}
	}
	if info.Unavailable {
		return []string{"AI豆苗签到活动已结束"}
	}
	var parts []string
	if !info.SignToday {
		t.pause(time.Second)
		resp, err := t.api.SignCoc3Seed(t.tokenValue)
		if err != nil {
			return []string{t.failure("AI豆苗签到提交", err)}
		}
		if !resp.Success && resp.ResultCode != -9014 && resp.ResultCode != -1108 && !strings.Contains(resp.Message, "已签") && !strings.Contains(resp.Message, "重复") {
			return []string{t.failure("AI豆苗签到", resp.BusinessError("签到提交"))}
		}
		t.pause(time.Second)
		info, err = t.api.Coc3SeedSignInfo(t.tokenValue)
		if err != nil {
			return []string{t.failure("AI豆苗签到复查", err)}
		}
	}
	if info.SignToday {
		parts = append(parts, fmt.Sprintf("AI豆苗签到已由服务端确认（累签%d天）", info.AccumulateCount))
	} else {
		t.pending = true
		parts = append(parts, "AI豆苗签到已提交，尚未查到完成记录")
	}
	if count, err := t.api.Coc3SignAwardCount(t.tokenValue, api.Coc3SeedSignID); err != nil {
		parts = append(parts, t.failure("AI豆苗签到奖励记录", err))
	} else {
		parts = append(parts, fmt.Sprintf("AI豆苗签到奖励记录%d条", count))
	}
	return parts
}

func (t *Coc3Task) claimSeedBubbles(before int) []string {
	bubbles, err := t.api.Coc3GrowthBubbles(t.tokenValue)
	if err != nil {
		return []string{t.failure("AI豆苗待领阳光值", err)}
	}
	var parts []string
	submitted := map[string]int{}
	seen := map[string]bool{}
	for _, bubble := range bubbles {
		if seen[bubble.ID] {
			continue
		}
		if len(seen) >= 20 {
			t.pending = true
			break
		}
		seen[bubble.ID] = true
		if _, err := t.api.ClaimCoc3GrowthBubble(t.tokenValue, bubble); err != nil {
			parts = append(parts, t.failure("AI豆苗气泡 "+bubble.ID, err))
		} else {
			submitted[bubble.ID] = bubble.Amount
		}
		t.pause(500 * time.Millisecond)
	}
	remaining, err := t.api.Coc3GrowthBubbles(t.tokenValue)
	if err != nil {
		return append(parts, t.failure("AI豆苗气泡复查", err))
	}
	for _, bubble := range remaining {
		delete(submitted, bubble.ID)
	}
	if len(remaining) > 0 {
		t.pending = true
		parts = append(parts, fmt.Sprintf("AI豆苗仍有%d项阳光值待领，未自动重提", len(remaining)))
	}
	after, err := t.api.Coc3GrowthValue(t.tokenValue)
	if err != nil {
		return append(parts, t.failure("AI豆苗阳光值复查", err))
	}
	claimed := 0
	for _, amount := range submitted {
		claimed += amount
	}
	if after < before+claimed {
		t.pending = true
		parts = append(parts, "AI豆苗领取回执与阳光值变化尚未一致，到账待确认")
	} else {
		parts = append(parts, fmt.Sprintf("AI豆苗确认领取阳光值%d（%d→%d，不计入云朵）", claimed, before, after))
	}
	return parts
}
