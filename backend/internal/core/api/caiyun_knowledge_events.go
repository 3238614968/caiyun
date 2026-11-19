package api

import "time"

const knowledgeCreatePage = "https://yun.139.com/ai-helper/?sourceid=810292#/pages/knowledgeBase/create?skipType=create"

// Called only before a real private creation, after validation and deduplication.
// Reusing a library does not replay a creation event. e=0 is not task completion.
func (api *CaiyunAPI) reportKnowledgeCreateEvent(now time.Time) error {
	return api.reportNativeAIEvents(now, knowledgeCreatePage, "5.1.10.feature_tob", map[string]string{
		"source_channel_h5": "101", "source_id_h5": "810292", "app_platform_h5": "App",
		"app_version_h5": MarketClientVersion, "hw_request_h5": "10252400", "algorithm_source_channel": "101",
	}, []nativeAIEvent{{Name: "aidialogue_click", Params: map[string]interface{}{
		"module": "AI助手小天", "page_title": "创建知识库", "scenario": "知识库", "click_object": "立即创建",
	}}})
}
