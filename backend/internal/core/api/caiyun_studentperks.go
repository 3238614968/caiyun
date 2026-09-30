package api

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// National_StudentPerks（全网学生认证福利）。
//
// 认证本身在微信侧完成，服务端只提供「同步认证结果」与「领奖」两个动作。
// cert/sync 请求体为空：它把权威源已存在的认证结果同步进来，
// 不会凭空产生学生身份 —— 未认证账号同步后仍是 certStatus=0。
const studentPerksBase = MobileMarketURL + "/simple/studentperks"

// StudentPerksStatus 学生认证与领奖状态。
type StudentPerksStatus struct {
	CertStatus      int    `json:"certStatus"`
	CertStatusDesc  string `json:"certStatusDesc"`
	CanCert         bool   `json:"canCert"`
	CanClaim        bool   `json:"canClaim"`
	PrizeStatus     int    `json:"prizeStatus"`
	PrizeStatusDesc string `json:"prizeStatusDesc"`
	StockStatus     int    `json:"stockStatus"`
	StockStatusDesc string `json:"stockStatusDesc"`
}

// StudentPerksUserStatus 查询认证/领奖状态。
func (api *CaiyunAPI) StudentPerksUserStatus() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", studentPerksBase+"/user/status", nil, nil)
}

// StudentPerksCertSync 触发一次认证结果同步（如实拉取权威源状态）。
func (api *CaiyunAPI) StudentPerksCertSync() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", studentPerksBase+"/cert/sync", map[string]interface{}{}, nil)
}

// StudentPerksPrizeCheck 按 openid 查询领奖资格。
func (api *CaiyunAPI) StudentPerksPrizeCheck(openID string) (*CaiyunResponse, error) {
	endpoint := studentPerksBase + "/prize/check"
	if openID = strings.TrimSpace(openID); openID != "" {
		endpoint += "?openid=" + url.QueryEscape(openID)
	}
	return api.marketJSONRequest("GET", endpoint, nil, nil)
}

// StudentPerksPrizeClaim 领取学生认证奖品。页面提交的是短验码的 MD5。
func (api *CaiyunAPI) StudentPerksPrizeClaim(smsCode string) (*CaiyunResponse, error) {
	if strings.TrimSpace(smsCode) == "" {
		return nil, fmt.Errorf("学生认证领奖需要短信验证码")
	}
	return api.marketJSONRequest(
		"POST",
		studentPerksBase+"/prize/claim",
		map[string]string{"smsCode": md5HexLower(smsCode)},
		nil,
	)
}

// ParseStudentPerksStatus 把响应 result 解析成结构化状态。
func ParseStudentPerksStatus(resp *CaiyunResponse) (StudentPerksStatus, bool) {
	result, ok := resultMap(resp)
	if !ok {
		return StudentPerksStatus{}, false
	}
	return StudentPerksStatus{
		CertStatus:      mapInt(result, "certStatus"),
		CertStatusDesc:  mapString(result, "certStatusDesc"),
		CanCert:         mapBool(result, "canCert"),
		CanClaim:        mapBool(result, "canClaim"),
		PrizeStatus:     mapInt(result, "prizeStatus"),
		PrizeStatusDesc: mapString(result, "prizeStatusDesc"),
		StockStatus:     mapInt(result, "stockStatus"),
		StockStatusDesc: mapString(result, "stockStatusDesc"),
	}, true
}

// StudentPerksReadyToClaim 判断是否处于「已认证、可领奖」状态。
func StudentPerksReadyToClaim(status StudentPerksStatus) bool {
	return status.CanClaim && status.CertStatus != 2
}

func md5HexLower(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
