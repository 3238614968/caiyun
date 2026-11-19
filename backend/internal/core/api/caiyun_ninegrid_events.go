package api

import (
	"encoding/json"
	"fmt"
	"time"
)

const nineGridResultPage = "https://yun.139.com/aiTools/#/wechat9picEditOffice"

// VerifyAndReportNineGridResult resumes the same real saved output after an
// eventual-consistency delay. It creates no files or dynamic, and must be
// explicitly invoked because successful readback is followed by SDK events.
func (api *CaiyunAPI) VerifyAndReportNineGridResult(result *NineGridResult) error {
	if result == nil || len(result.FileIDs) != 11 || result.DynamicID == "" {
		return fmt.Errorf("缺少本次九宫格真实作品回执")
	}
	if err := api.verifyNineGridFiles(result.FileIDs); err != nil {
		return err
	}
	result.FilesVerified = true
	data, err := api.nineGridRequest("/file/getAuditResult", map[string]string{"fileId": result.FileIDs[0]})
	if err != nil {
		return err
	}
	var audit struct {
		Result *struct {
			Status *int `json:"status"`
		} `json:"contentAuditResult"`
	}
	if json.Unmarshal(data, &audit) != nil || audit.Result == nil || audit.Result.Status == nil || *audit.Result.Status != 2 {
		return fmt.Errorf("九宫格封面复查未确认审核通过")
	}
	result.AuditVerified = true
	if err := api.waitNineGridRecord(result.DynamicID, time.Sleep); err != nil {
		return err
	}
	result.DynamicVerified = true
	if result.EventAccepted {
		return nil
	}
	if err := api.reportNineGridSaved(result, time.Now()); err != nil {
		return err
	}
	result.EventAccepted = true
	return nil
}

// The current official 6524 chunk emits aitool_success on next/batch-save and
// aitool_picsave_result after cloud persistence. Report only verified output.
func (api *CaiyunAPI) reportNineGridSaved(result *NineGridResult, now time.Time) error {
	if result == nil || !result.FilesVerified || !result.AuditVerified || !result.DynamicVerified || result.DynamicID == "" || len(result.FileIDs) != 11 {
		return fmt.Errorf("九宫格真实作品未核验，不能上报生成成功")
	}
	err := api.reportNativeAIEvents(now, nineGridResultPage, "5.1.9_feature_2", map[string]string{
		"source_channel_h5": "AI工具", "source_id_h5": "1", "app_platform_h5": "APP",
		"hw_request_h5": "10805", "user_domain_id": api.client.GetUserDomainID(),
	}, []nativeAIEvent{
		{Name: "aitool_success", Params: map[string]interface{}{
			"module": "AI工具", "sub_module": "朋友圈9图", "submodule_id": "026", "aitool_id": 15,
			"page_title": "朋友圈9图", "aimoment_styleid": 1, "aitool_traceid": "",
		}},
		{Name: "aitool_picsave_result", Params: map[string]interface{}{
			"module": "AI工具", "sub_module": "朋友圈9图", "page_title": "朋友圈9图",
			"save_type": "保存到云盘", "save_result": "全部保存成功",
		}},
	})
	if err != nil {
		return fmt.Errorf("九宫格官方事件未确认: %w", err)
	}
	return nil
}
