package api

import (
	"fmt"
	"net/url"
	"strings"
)

// 云朵中心转存三件套（Python 侧实测报文）：
//  1. POST /redeposit/getLinkInfoList 查询任务槽可用转存链接，
//     非转存槽回空列表（无副作用，可安全探测）；
//  2. GET /signin/task/click?key=task&id=<taskId> 前置点击（DoTaskWithMarket）；
//  3. POST /redeposit/saveLinkFile 转存链接文件，code=0 即转存成功；
//  4. 延时约 600ms 后 GET /signin/redeposit/finishRedepositTask 完成任务。

// redepositHeaders 转存接口挂在云朵中心渠道下，沿用签到中心的渠道头；
// 非签到中心渠道回退到活动层默认鉴权头。
func (api *CaiyunAPI) redepositHeaders(marketName string) map[string]string {
	if isSignInMarket(strings.TrimSpace(marketName)) {
		return api.signInMarketHeaders(marketName)
	}
	return nil
}

// RedepositLinkList 查询任务槽的转存链接列表。
// 信封为 {code:0,result:{result:[{linkid:"..."}]}}（linkid 全小写）；
// 非转存槽返回空列表，调用方据此跳过。
func (api *CaiyunAPI) RedepositLinkList(marketName, taskID string, pageNum, pageSize int) (*CaiyunResponse, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("转存链接查询缺少 taskId")
	}
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	return api.marketJSONRequest("POST", MobileMarketURL+"/redeposit/getLinkInfoList", map[string]interface{}{
		"marketName": marketName,
		"linkType":   taskID,
		"pageNum":    pageNum,
		"pageSize":   pageSize,
	}, api.redepositHeaders(marketName))
}

// RedepositSaveFile 转存链接内的文件，code=0 表示转存成功。
func (api *CaiyunAPI) RedepositSaveFile(marketName, linkID string) (*CaiyunResponse, error) {
	if strings.TrimSpace(linkID) == "" {
		return nil, fmt.Errorf("转存文件缺少 linkId")
	}
	return api.marketJSONRequest("POST", MobileMarketURL+"/redeposit/saveLinkFile", map[string]interface{}{
		"marketName": marketName,
		"linkId":     linkID,
	}, api.redepositHeaders(marketName))
}

// RedepositFinishTask 上报转存任务完成（需在 saveLinkFile 之后延时约 600ms 调用）。
func (api *CaiyunAPI) RedepositFinishTask(taskID, linkID string) (*CaiyunResponse, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(linkID) == "" {
		return nil, fmt.Errorf("转存完成上报缺少 taskId 或 linkId")
	}
	endpoint := fmt.Sprintf("%s/signin/redeposit/finishRedepositTask?taskId=%s&linkId=%s",
		MobileMarketURL, url.QueryEscape(taskID), url.QueryEscape(linkID))
	return api.marketJSONRequest("GET", endpoint, nil, nil)
}
