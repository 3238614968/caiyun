package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/core/tasks"
	"fmt"
	"strings"
)

// runAfterTaskCleanup 收尾清理（删除临时上传文件和遗留分享链接）
func (r *TaskRunner) runAfterTaskCleanup() error {
	if r.storage == nil {
		return nil
	}

	var errs []string
	if err := r.cleanupTempShareLinks(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := r.cleanupTempFiles(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (r *TaskRunner) cleanupTempFiles() error {
	fileIDs, err := tasks.LoadStringList(r.storage, tasks.KeyTempFiles)
	if err != nil || len(fileIDs) == 0 {
		return nil
	}

	r.logger.Debug("收尾清理临时文件", strings.Join(fileIDs, ","))
	fileAPI := api.NewFileAPI(r.httpClient)
	resp, err := fileAPI.DeleteFiles(fileIDs)
	if err != nil {
		r.logger.Error("收尾清理临时文件失败", err)
		return err
	}
	if resp != nil && resp.Success {
		_ = tasks.SaveStringList(r.storage, tasks.KeyTempFiles, nil)
		r.logger.Success("收尾清理临时文件成功")
		return nil
	}
	if resp != nil {
		err = fmt.Errorf("code=%s msg=%s", resp.Code, resp.Message)
		r.logger.Error("收尾清理临时文件失败", err)
		return err
	}
	return fmt.Errorf("删除临时文件返回为空")
}

func (r *TaskRunner) cleanupTempShareLinks() error {
	if strings.TrimSpace(r.account.Phone) == "" {
		return nil
	}

	linkIDs, err := tasks.LoadStringList(r.storage, tasks.KeyTempLinks)
	if err != nil || len(linkIDs) == 0 {
		return nil
	}

	r.logger.Debug("收尾清理分享链接", strings.Join(linkIDs, ","))
	resp, err := r.api.DelOutLink(r.account.Phone, linkIDs)
	if err != nil {
		r.logger.Error("收尾清理分享链接失败", err)
		return err
	}
	if resp != nil && resp.IsSuccess() {
		_ = tasks.SaveStringList(r.storage, tasks.KeyTempLinks, nil)
		r.logger.Success("收尾清理分享链接成功")
		return nil
	}
	if resp != nil {
		err = fmt.Errorf("code=%v msg=%s", resp.Code, resp.MessageText())
	} else {
		err = fmt.Errorf("删除分享链接返回为空")
	}
	r.logger.Error("收尾清理分享链接失败", err)
	return err
}

// getRawAccountToken 获取账号原始 token（优先数据库 token，其次从 Auth 解析）
func (r *TaskRunner) getRawAccountToken() string {
	if r.account == nil {
		return ""
	}
	if strings.TrimSpace(r.account.Token) != "" {
		return strings.TrimSpace(r.account.Token)
	}
	if strings.TrimSpace(r.account.Auth) == "" {
		return ""
	}

	info, err := auth.ParseToken(r.account.Auth)
	if err != nil || info == nil {
		return ""
	}
	return strings.TrimSpace(info.Token)
}
