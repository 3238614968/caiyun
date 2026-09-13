package tasks

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// activityActions 汇总各 WebView 活动任务可复用的“真实动作”：上传文件/
// 上传照片、创建云笔记、分享文件、灵犀对话、AI 相机。所有方法只依赖账号
// 级 HTTP 客户端（含 Cookie/JWT/userDomainId），天然支持多账号隔离。
type activityActions struct {
	logger    *logger.Logger
	api       *api.CaiyunAPI
	fileAPI   *api.FileAPI
	storage   Storage
	phone     string
	authToken string
}

func newActivityActions(client *http.Client, log *logger.Logger) *activityActions {
	return &activityActions{
		logger:  log,
		api:     api.NewCaiyunAPI(client),
		fileAPI: api.NewFileAPI(client),
	}
}

func (a *activityActions) setStorage(store Storage) {
	a.storage = store
}

func (a *activityActions) setAccountContext(phone, authToken string) {
	a.phone = strings.TrimSpace(phone)
	a.authToken = strings.TrimSpace(authToken)
}

// uploadTextFile 上传一个临时文本文件（供上传类/分享类任务计数）。
func (a *activityActions) uploadTextFile(prefix string) (string, string, error) {
	name := fmt.Sprintf("%s_%d.txt", prefix, time.Now().Unix())
	resp, err := a.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         name,
		Content:      []byte("0"),
		ChannelSrc:   "10000023",
		Ext:          ".txt",
	})
	if err != nil {
		return "", "", err
	}
	if resp == nil || resp.FileID == "" {
		return "", "", fmt.Errorf("上传任务文件失败：未返回 fileId")
	}
	_ = AppendStringList(a.storage, KeyTempFiles, resp.FileID)
	return resp.FileID, name, nil
}

// uploadPhotoFile 上传一张小图片（照片类任务要求 category=image）。
func (a *activityActions) uploadPhotoFile(prefix string) (string, error) {
	content, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(api.AICameraSampleBase64, "data:image/jpg;base64,"))
	if err != nil {
		return "", fmt.Errorf("解码示例图片失败: %w", err)
	}
	name := fmt.Sprintf("%s_%d.jpg", prefix, time.Now().Unix())
	resp, err := a.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         name,
		Content:      content,
		ChannelSrc:   "10000023",
		Ext:          ".jpg",
	})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.FileID == "" {
		return "", fmt.Errorf("上传照片失败：未返回 fileId")
	}
	_ = AppendStringList(a.storage, KeyTempFiles, resp.FileID)
	return resp.FileID, nil
}

// createTempNote 创建一个云笔记后立即删除（创建笔记类任务）。
func (a *activityActions) createTempNote() error {
	if a.authToken == "" || a.phone == "" {
		return fmt.Errorf("缺少账号 token 或手机号")
	}
	noteAuth, err := a.api.GetNoteAuthToken(a.authToken, a.phone)
	if err != nil {
		return err
	}
	if noteAuth == nil || noteAuth.Headers["app_auth"] == "" {
		return fmt.Errorf("未获取到云笔记 app_auth")
	}
	noteID := utils.RandomString(32)
	title := utils.RandomString(3)
	if err := a.api.CreateNote(noteID, title, a.phone, noteAuth.Headers, nil); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	return a.api.DeleteNote(noteID, noteAuth.Headers)
}

// shareNewFile 上传文件并创建外链（分享文件类任务），链接登记到临时资源
// 存储，由收尾清理任务统一删除。
func (a *activityActions) shareNewFile() error {
	if a.phone == "" {
		return fmt.Errorf("缺少手机号，无法创建分享链接")
	}
	fileID, fileName, err := a.uploadTextFile("activity_share")
	if err != nil {
		return err
	}
	resp, err := a.api.GetOutLink(a.phone, []string{fileID}, fileName)
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("分享接口返回为空")
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
	_ = AppendStringList(a.storage, KeyTempLinks, linkIDs...)
	// 分享文件本身不再计入待清理列表，删除文件可能导致分享失效。
	_ = RemoveStringList(a.storage, KeyTempFiles, fileID)
	return nil
}

// performAICamera 走真实的“上传真图 -> fileId -> aiRecognize(sendType=3) ->
// 带 command 的灵犀对话”链路，用于完成拍照问 AI / AI 相机体验类任务。
func (a *activityActions) performAICamera() error {
	return a.api.CompleteAICameraTask()
}

// performAlbumBackup 复刻 App 手动备份的真实信号：先通过备份通道上传一张
// 真图（op-type=backup，落到相册备份目录），再提交 createSnapshot/completeSnapshot
// 快照，服务端据此判定“成功备份一次文件”。
func (a *activityActions) performAlbumBackup() error {
	content, err := api.GenerateSampleJPEG(600, 800)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("backup_%d.jpg", time.Now().Unix())
	uploaded, err := a.fileAPI.UploadRandomFile(&api.UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         name,
		Content:      content,
		ChannelSrc:   "10000023",
		OpType:       "backup",
		Ext:          ".jpg",
	})
	if err != nil {
		return err
	}
	if uploaded != nil && uploaded.FileID != "" {
		_ = AppendStringList(a.storage, KeyTempFiles, uploaded.FileID)
	}
	return a.fileAPI.TriggerAlbumBackup()
}
