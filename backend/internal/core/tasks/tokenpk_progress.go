package tasks

import (
	"caiyun/internal/core/api"
	"errors"
)

func tokenStageClaimed(stage api.TokenPKRewardStage) bool {
	if stage.ReceiveStatus != nil {
		return *stage.ReceiveStatus != 0
	}
	return stage.Status != 0
}
func (t *TokenPKTask) receiveProgressRewards(used int64, stages []api.TokenPKRewardStage) (int, error) {
	claimed := map[int]bool{}
	var issues []error
	for _, stage := range stages {
		if stage.PhaseNo <= 0 || int64(stage.TokenThreshold) > used || tokenStageClaimed(stage) || claimed[stage.PhaseNo] {
			continue
		}
		resp, err := t.api.TokenPKStageReceive(stage.PhaseNo)
		if err != nil {
			issues = append(issues, err)
			continue
		}
		if responseCodeIs(resp, 20003, 9001) {
			t.pending = true
			continue
		}
		if err = rewardResponseError("领取算力阶段奖励", resp); err != nil {
			issues = append(issues, err)
			continue
		}
		claimed[stage.PhaseNo] = true
	}
	if len(claimed) > 0 {
		_, latest, err := t.api.TokenPKProgressQueryHome()
		if err != nil {
			return 0, errors.Join(append(issues, err)...)
		}
		confirmed := 0
		for _, stage := range latest {
			if claimed[stage.PhaseNo] && tokenStageClaimed(stage) {
				confirmed++
				delete(claimed, stage.PhaseNo)
			}
		}
		if len(claimed) > 0 {
			t.pending = true
		}
		return confirmed, errors.Join(issues...)
	}
	return 0, errors.Join(issues...)
}
