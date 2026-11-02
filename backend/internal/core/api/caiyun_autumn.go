package api

import (
	"fmt"
	"net/url"
)

const GPMarketURL = "https://gp.mcloud.139.com/ycloud"

func (api *CaiyunAPI) RedeemActivityCode(code string) (*CaiyunResponse, error) {
	if code == "" {
		return nil, fmt.Errorf("缺少兑换码")
	}
	return api.marketJSONRequest("POST", MobileMarketURL+"/redeemcode-service/diamond/obtain", map[string]string{"redeemCode": code}, nil)
}

func (api *CaiyunAPI) BillTasks() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", GPMarketURL+"/bill/toDoTask/taskInfos", nil, api.signInMarketHeaders("sign_in_3"))
}
func (api *CaiyunAPI) BillExecute(code string, feedback map[string]interface{}) (*CaiyunResponse, error) {
	body := map[string]interface{}{"taskCode": code}
	switch code {
	case "BROWSE_REPORT", "MEMORY_ALBUM":
	case "AI_PHOTO":
		if api.client.GetUserDomainID() == "" {
			return nil, fmt.Errorf("年度账单缺少 userDomainId")
		}
		body["param"] = api.client.GetUserDomainID()
	case "FEEDBACK":
		if len(feedback) == 0 {
			return nil, fmt.Errorf("年度账单未配置问卷答案")
		}
		body["param"] = feedback
	default:
		return nil, fmt.Errorf("不支持的年度账单任务: %s", code)
	}
	return api.marketJSONRequest("POST", GPMarketURL+"/bill/toDoTask/executeTask", body, api.signInMarketHeaders("sign_in_3"))
}
func (api *CaiyunAPI) BillLotteryCount() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", GPMarketURL+"/bill/taskLottery/queryCount", nil, nil)
}
func (api *CaiyunAPI) BillDraw() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", GPMarketURL+"/bill/taskLottery/prize?channel=app", nil, nil)
}
func (api *CaiyunAPI) LuckybagInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", GPMarketURL+"/luckybag/common/user/info", nil, nil)
}
func (api *CaiyunAPI) LuckybagSubscribe() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", GPMarketURL+"/luckybag/common/user/subscribe", map[string]interface{}{}, nil)
}
func (api *CaiyunAPI) LuckybagDraw() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", GPMarketURL+"/luckybag/blindbox/lottery", map[string]string{"source": "app"}, nil)
}
func (api *CaiyunAPI) RedCavalryProgress() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", MobileMarketURL+"/simple/redcavalry/progress", nil, nil)
}
func (api *CaiyunAPI) RedCavalryPrepare(station string, game bool) (*CaiyunResponse, error) {
	step := "readCard"
	if game {
		step = "completeGame"
	}
	return api.marketJSONRequest("POST", MobileMarketURL+"/simple/redcavalry/"+step, map[string]string{"stationCode": station}, nil)
}
func (api *CaiyunAPI) RedCavalryQuestion(station string) (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", MobileMarketURL+"/simple/redcavalry/question?stationCode="+url.QueryEscape(station), nil, nil)
}
func (api *CaiyunAPI) RedCavalryAnswer(station string, id interface{}, answer string) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/simple/redcavalry/submitAnswer", map[string]interface{}{"stationCode": station, "questionId": id, "answer": answer}, nil)
}
func (api *CaiyunAPI) RedCavalryReview() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/simple/redcavalry/completeReview", map[string]interface{}{}, nil)
}
func (api *CaiyunAPI) TokenPKSmartAlbumOpen() error {
	body, err := api.activityGetBody(TokenPKMarketName, MobileMarketURL+"/tokenpk/task/smartAlbum/open")
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("开启算力智能相册", body)
	return err
}
func (api *CaiyunAPI) TokenPKStageReceive(phase int) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/tokenpk/toplist/progress/receiveReward", map[string]int{"phaseNo": phase}, api.buildActivityHeaders(TokenPKMarketName, "", nil))
}
func (api *CaiyunAPI) TokenPKRank(month string) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/tokenpk/toplist/rank/queryRankHome", map[string]string{"monthKey": month}, api.buildActivityHeaders(TokenPKMarketName, "", nil))
}
func (api *CaiyunAPI) FunaiTimedLottery(module, task string) (*CaiyunResponse, error) {
	if module == "" || task == "" {
		return nil, fmt.Errorf("缺少限时抽奖模块或任务 ID")
	}
	return api.marketJSONRequest("POST", MobileMarketURL+"/funai/api/timed/lottery", map[string]string{"moduleId": module, "taskId": task}, api.buildActivityHeaders(FunAIMarketName, "", nil))
}
func (api *CaiyunAPI) RafflecodeAssist(code string) (*CaiyunResponse, error) {
	if code == "" {
		return nil, fmt.Errorf("缺少欢乐透邀请码")
	}
	return api.marketJSONRequest("POST", rafflecodeBase+"/shareInvite?data="+url.QueryEscape(code), nil, nil)
}
