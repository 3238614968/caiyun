package handlers

import (
	"caiyun/internal/models"
	apiresponse "caiyun/pkg/response"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type GetExchangeRecordsResponse struct {
	Records []*models.ExchangeRecord `json:"records"`
	Total   int64                    `json:"total"`
	Stats   RecordStats              `json:"stats"`
}

type RecordStats struct {
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
}

// GetExchangeRecords 获取抢兑记录列表

// GetExchangeRecords 获取抢兑记录列表
func (h *ExchangeHandler) GetExchangeRecords(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取筛选参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, limit = normalizePageLimit(page, limit, 20, 100)
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32) // 查询参数，0 表示不筛选
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	records, total, err := h.exchangeService.GetExchangeRecordsContext(
		c.Request.Context(),
		userID,
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		page,
		limit,
	)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 获取统计信息（最近 30 天）
	startTime := time.Now().AddDate(0, 0, -30)
	endTime := time.Now()
	successCount, failCount, err := h.exchangeService.GetRecordStatsContext(c.Request.Context(), userID, startTime, endTime)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, GetExchangeRecordsResponse{
		Records: records,
		Total:   total,
		Stats: RecordStats{
			Success: successCount,
			Failed:  failCount,
		},
	})
}

// ExportExchangeRecords 导出抢兑记录

// ExportExchangeRecords 导出抢兑记录
func (h *ExchangeHandler) ExportExchangeRecords(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取筛选参数
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32) // 查询参数，0 表示不筛选
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	exportFormat := c.DefaultQuery("format", "csv") // csv 或 json

	// 获取所有记录（不分页）
	records, _, err := h.exchangeService.GetExchangeRecordsContext(
		c.Request.Context(),
		userID,
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		1,
		10000, // 最多导出10000条
	)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 根据格式导出
	if exportFormat == "json" {
		// JSON格式
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", "attachment; filename=exchange_records.json")
		c.JSON(http.StatusOK, gin.H{
			"records":     records,
			"total":       len(records),
			"exported_at": time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}

	// CSV格式（默认）
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=exchange_records.csv")

	// 写入BOM以支持中文
	c.Writer.Write([]byte("\xEF\xBB\xBF"))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// 写入表头
	_ = writer.Write([]string{"记录ID", "用户ID", "账号ID", "商品ID", "商品名称", "状态", "消息", "执行时长(ms)", "创建时间"})

	// 写入数据
	for _, record := range records {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(record.ID), 10),
			strconv.FormatUint(uint64(record.UserID), 10),
			strconv.FormatUint(uint64(record.ExchangeAccountID), 10),
			strconv.FormatUint(uint64(record.ProductID), 10),
			escapeCSVFormula(record.PrizeName),
			escapeCSVFormula(record.Status),
			escapeCSVFormula(record.Message),
			strconv.Itoa(record.ExecutionTimeMs),
			record.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

func escapeCSVFormula(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
