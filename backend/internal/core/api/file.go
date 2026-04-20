package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FileAPI 文件操作 API
type FileAPI struct {
	client HTTPClient
}

// HTTPClient HTTP 客户端接口
type HTTPClient interface {
	Get(url string, headers map[string]string) (*http.Response, error)
	Post(url string, headers map[string]string, body interface{}) (*http.Response, error)
	ReadResponseBody(resp *http.Response) (string, error)
}

// NewFileAPI 创建文件 API
func NewFileAPI(client HTTPClient) *FileAPI {
	return &FileAPI{client: client}
}

// FileInfo 文件信息
type FileInfo struct {
	FileID      string `json:"fileId"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	CreateTime  int64  `json:"createTime"`
	UpdateTime  int64  `json:"updateTime"`
	Type        string `json:"type"`
}

// FileListResp 文件列表响应
type FileListResp struct {
	Files []FileInfo `json:"files"`
}

// GetFileList 获取文件列表
func (f *FileAPI) GetFileList(parentFileID string) (*FileListResp, error) {
	if parentFileID == "" {
		parentFileID = "/"
	}

	headers := map[string]string{
		"Content-Type":      "application/json",
		"x-yun-api-version": "v1",
		"x-yun-app-channel": "10000023",
		"x-yun-client-info": "6|127.0.0.1|1|12.5.4|realme|RMX5060|BCFF2BBA6881DD8E4971803C63DDB5E4|02-00-00-00-00-00|android 15|1264X2592|zh||||032|0|",
	}

	body := map[string]interface{}{
		"pageInfo": map[string]interface{}{
			"pageSize":   100,
			"pageCursor": nil,
		},
		"orderBy":                 "updated_at",
		"orderDirection":          "DESC",
		"parentFileId":            parentFileID,
		"imageThumbnailStyleList": []string{"Small", "Large"},
	}

	resp, err := f.client.Post(
		"https://personal-kd-njs.yun.139.com/hcy/file/list",
		headers,
		body,
	)

	if err != nil {
		return nil, fmt.Errorf("获取文件列表失败: %w", err)
	}

	responseBody, err := f.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []FileInfo `json:"items"`
		} `json:"data"`
		Files []FileInfo `json:"files"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	files := result.Files
	if len(result.Data.Items) > 0 {
		files = result.Data.Items
	}

	return &FileListResp{Files: files}, nil
}

// UploadRandomFileRequest 上传随机文件请求
type UploadRandomFileRequest struct {
	ParentFileID string
	Name         string
	Content      []byte
	ChannelSrc   string
	OpType       string
	Ext          string
}

// UploadRandomFileResp 上传文件响应
type UploadRandomFileResp struct {
	FileID string `json:"fileId"`
}

// UploadRandomFile 上传随机文件
func (f *FileAPI) UploadRandomFile(req *UploadRandomFileRequest) (*UploadRandomFileResp, error) {
	if req == nil {
		req = &UploadRandomFileRequest{}
	}

	parentFileID := req.ParentFileID
	if parentFileID == "" {
		parentFileID = "/"
	}

	content := req.Content
	if len(content) == 0 {
		content = []byte(randomUploadContent())
	}

	name := req.Name
	if name == "" {
		ext := req.Ext
		if ext == "" {
			ext = ".txt"
		}
		name = "auto_upload_" + randomHex(8) + ext
	}

	channelSrc := req.ChannelSrc
	if channelSrc == "" {
		channelSrc = "10000023"
	}

	contentHash := sha256Hex(content)
	size := len(content)

	createHeaders := buildUploadHeaders(channelSrc, req.OpType)
	createBody := map[string]interface{}{
		"parentFileId":         parentFileID,
		"name":                 name,
		"type":                 "file",
		"size":                 size,
		"fileRenameMode":       "auto_rename",
		"contentHash":          contentHash,
		"contentHashAlgorithm": "SHA256",
		"contentType":          "application/oct-stream",
		"parallelUpload":       false,
		"partInfos": []map[string]interface{}{
			{
				"parallelHashCtx": map[string]interface{}{"partOffset": 0},
				"partNumber":      1,
				"partSize":        size,
			},
		},
	}

	createResp, err := f.client.Post("https://personal-kd-njs.yun.139.com/hcy/file/create", createHeaders, createBody)
	if err != nil {
		return nil, fmt.Errorf("上传文件创建请求失败: %w", err)
	}

	createRespBody, err := f.client.ReadResponseBody(createResp)
	if err != nil {
		return nil, fmt.Errorf("读取创建响应失败: %w", err)
	}

	var createResult struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Exist       bool   `json:"exist"`
			RapidUpload bool   `json:"rapidUpload"`
			UploadID    string `json:"uploadId"`
			FileID      string `json:"fileId"`
			PartInfos   []struct {
				UploadURL string `json:"uploadUrl"`
			} `json:"partInfos"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(createRespBody), &createResult); err != nil {
		return nil, fmt.Errorf("解析创建响应失败: %w", err)
	}

	if !createResult.Success {
		return nil, fmt.Errorf("上传文件创建失败: code=%s, msg=%s", createResult.Code, createResult.Message)
	}
	if createResult.Data.FileID == "" {
		return nil, fmt.Errorf("上传文件创建失败: fileId 为空")
	}

	// 文件已存在或秒传成功时无需走 PUT 和 complete
	if createResult.Data.Exist || createResult.Data.RapidUpload {
		return &UploadRandomFileResp{FileID: createResult.Data.FileID}, nil
	}

	if len(createResult.Data.PartInfos) == 0 || createResult.Data.PartInfos[0].UploadURL == "" {
		return nil, fmt.Errorf("上传文件创建失败: uploadUrl 为空")
	}

	if err := putBinaryToUploadURL(createResult.Data.PartInfos[0].UploadURL, content); err != nil {
		return nil, fmt.Errorf("上传文件内容失败: %w", err)
	}

	completeHeaders := buildUploadHeaders(channelSrc, req.OpType)
	completeBody := map[string]interface{}{
		"fileId":               createResult.Data.FileID,
		"uploadId":             createResult.Data.UploadID,
		"contentHash":          contentHash,
		"contentHashAlgorithm": "SHA256",
	}

	completeResp, err := f.client.Post("https://personal-kd-njs.yun.139.com/hcy/file/complete", completeHeaders, completeBody)
	if err != nil {
		return nil, fmt.Errorf("上传文件完成请求失败: %w", err)
	}

	completeRespBody, err := f.client.ReadResponseBody(completeResp)
	if err != nil {
		return nil, fmt.Errorf("读取完成响应失败: %w", err)
	}

	var completeResult struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(completeRespBody), &completeResult); err != nil {
		return nil, fmt.Errorf("解析完成响应失败: %w", err)
	}
	if !completeResult.Success {
		return nil, fmt.Errorf("上传文件完成失败: code=%s, msg=%s", completeResult.Code, completeResult.Message)
	}

	return &UploadRandomFileResp{FileID: createResult.Data.FileID}, nil
}

// DeleteFilesResp 删除文件响应
type DeleteFilesResp struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DeleteFiles 删除文件
func (f *FileAPI) DeleteFiles(fileIDs []string) (*DeleteFilesResp, error) {
	if len(fileIDs) == 0 {
		return &DeleteFilesResp{Success: true}, nil
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	for k, v := range buildUploadHeaders("10000023", "") {
		headers[k] = v
	}

	body := map[string]interface{}{
		"fileIds": fileIDs,
	}

	resp, err := f.client.Post(
		"https://personal-kd-njs.yun.139.com/hcy/recyclebin/batchTrash",
		headers,
		body,
	)

	if err != nil {
		return nil, fmt.Errorf("删除文件失败: %w", err)
	}

	responseBody, err := f.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result DeleteFilesResp
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// buildUploadHeaders 构建上传/文件操作请求头（参考 mjs hu()）
func buildUploadHeaders(channelSrc, opType string) map[string]string {
	if channelSrc == "" {
		channelSrc = "10000023"
	}

	headers := map[string]string{
		"x-yun-api-version": "v1",
		"x-yun-app-channel": channelSrc,
		"x-yun-op-type":     "1",
	}

	switch channelSrc {
	case "10200153": // PC
		headers["x-yun-market-source"] = "001"
		headers["x-yun-module-type"] = "100"
		headers["x-yun-svc-type"] = "1"
		headers["x-yun-client-info"] = "||11|8.3.3.20250521|PC|TGktMjAyNDA5MTcxMDA2||| Windows 11 (11.0)|1920X1020|Q2hpbmVzZSAoU2ltcGxpZmllZCk=|||"
		headers["User-Agent"] = "Mozilla/5.0"
	case "10000023": // Android
		op := "1"
		subOp := "100"
		if strings.EqualFold(opType, "backup") {
			op = "2"
			subOp = "200"
		}
		headers["x-huawei-channelSrc"] = channelSrc
		headers["x-yun-client-info"] = "6|127.0.0.1|1|12.5.4|realme|RMX5060|BCFF2BBA6881DD8E4971803C63DDB5E4|02-00-00-00-00-00|android 15|1264X2592|zh||||032|0|"
		headers["x-yun-op-type"] = op
		headers["x-yun-sub-op-type"] = subOp
		headers["User-Agent"] = "okhttp/4.12.0"
		headers["Connection"] = "Keep-Alive"
	default: // Web
		headers["x-huawei-channelSrc"] = channelSrc
		headers["x-yun-client-info"] = "||9|12.5.4|Chrome|143.0.7499.146|codextestshare||Windows 10||zh-CN|||Q2hyb21l||"
		headers["x-yun-device-id"] = "||9|12.5.4|Chrome|143.0.7499.146|codextestshare||Windows 10||zh-CN|||Q2hyb21l||"
	}

	return headers
}

func putBinaryToUploadURL(uploadURL string, content []byte) error {
	req, err := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	const chars = "0123456789abcdef"
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	seed := time.Now().UnixNano()
	for i := range b {
		b[i] = chars[(seed+int64(i)*17)%int64(len(chars))]
	}
	return string(b)
}

func randomUploadContent() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomHex(24))
}

// API 通用结构

// CommonResponse 通用 API 响应
type CommonResponse struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Success bool   `json:"success"`
}

// AI 云朵相关 API

// AIRecordIdResp AI 云朵获取记录ID响应（getPreCloudReward）
type AIRecordIdResp struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
	Result  struct {
		CloudNum int    `json:"cloudNum"`
		RecordID string `json:"recordId"`
	} `json:"result"`
}

// AIExchangeResp AI 云朵兑换响应（getCloudReward）
type AIExchangeResp struct {
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Result  interface{} `json:"result"` // 云朵数量
}

// AIClaimResp AI 云朵领取响应
type AIClaimResp struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}
