package tasks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

type TodayCloudTask struct {
	api                    *api.CaiyunAPI
	logger                 *logger.Logger
	todayCount, totalCloud int
	pending                bool
	now                    func() time.Time
}

func NewTodayCloudTask(client *http.Client, log *logger.Logger) *TodayCloudTask {
	return &TodayCloudTask{api: api.NewCaiyunAPI(client), logger: log, now: time.Now}
}
func (t *TodayCloudTask) TodayCount() int { return t.todayCount }
func (t *TodayCloudTask) TotalCloud() int { return t.totalCloud }
func (t *TodayCloudTask) Pending() bool   { return t.pending }

var cloudRecordZone = time.FixedZone("CST", 8*3600)

func cloudRecordTime(value interface{}) (time.Time, error) {
	text := fmt.Sprint(value)
	for _, format := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999", "2006-01-02"} {
		if stamp, err := time.ParseInLocation(format, text, cloudRecordZone); err == nil {
			return stamp.In(cloudRecordZone), nil
		}
	}
	var n int64
	switch v := value.(type) {
	case float64:
		n = int64(v)
	case int64:
		n = v
	default:
		n, _ = strconv.ParseInt(text, 10, 64)
	}
	if n > 1e12 {
		return time.UnixMilli(n).In(cloudRecordZone), nil
	}
	if n >= 1e9 {
		return time.Unix(n, 0).In(cloudRecordZone), nil
	}
	return time.Time{}, fmt.Errorf("流水时间无效")
}
func cloudRecordDate(row map[string]interface{}) (time.Time, error) {
	for _, key := range []string{"inserttime", "insertTime", "updatetime", "updateTime"} {
		if value, exists := row[key]; exists {
			if stamp, err := cloudRecordTime(value); err == nil {
				return stamp, nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("流水缺少有效时间")
}

// Run reports a daily cumulative statistic. It must never be recorded as this
// execution's new gain. Unknown records or an exceeded page bound stay pending.
func (t *TodayCloudTask) Run() error {
	t.todayCount, t.totalCloud, t.pending = 0, 0, false
	today := t.now().In(cloudRecordZone).Format("2006-01-02")
	const pageSize, maxPages = 300, 20
	seenIDs := map[string]bool{}
	seenPages := map[[32]byte]bool{}
	for page := 1; page <= maxPages; page++ {
		resp, err := t.api.GetCloudRecord(page, pageSize, 1)
		if err != nil {
			return err
		}
		if err := rewardResponseError("获取云朵流水", resp); err != nil {
			return err
		}
		data, ok := resp.Result.(map[string]interface{})
		if !ok {
			return fmt.Errorf("云朵流水缺少有效结果")
		}
		records, ok := data["records"].([]interface{})
		if !ok {
			return fmt.Errorf("云朵流水缺少records数组")
		}
		if len(records) == 0 {
			break
		}
		raw, _ := json.Marshal(records)
		fingerprint := sha256.Sum256(raw)
		if seenPages[fingerprint] {
			t.pending = true
			break
		}
		seenPages[fingerprint] = true
		for _, item := range records {
			row, ok := item.(map[string]interface{})
			if !ok {
				t.pending = true
				continue
			}
			id := ""
			for _, key := range []string{"id", "recordId", "oid"} {
				if v, exists := row[key]; exists && v != nil && fmt.Sprint(v) != "" {
					id = key + ":" + fmt.Sprint(v)
					break
				}
			}
			if id != "" {
				if seenIDs[id] {
					continue
				}
				seenIDs[id] = true
			}
			stamp, err := cloudRecordDate(row)
			if err != nil {
				t.pending = true
				continue
			}
			if stamp.Format("2006-01-02") != today {
				continue
			}
			amount, err := strconv.Atoi(fmt.Sprint(row["num"]))
			if err != nil {
				t.pending = true
				continue
			}
			if amount > 0 {
				if amount > int(^uint(0)>>1)-t.totalCloud {
					return fmt.Errorf("云朵流水金额超出可统计范围")
				}
				t.todayCount++
				t.totalCloud += amount
			}
		}
		stop := len(records) < pageSize
		if pages, exists := data["pages"]; exists {
			n, e := strconv.Atoi(fmt.Sprint(pages))
			if e != nil || n < page {
				return fmt.Errorf("云朵流水分页元数据无效")
			}
			stop = page >= n
		} else if total, exists := data["total"]; exists {
			n, e := strconv.Atoi(fmt.Sprint(total))
			if e != nil || n < 0 {
				return fmt.Errorf("云朵流水总数无效")
			}
			stop = page*pageSize >= n
		}
		if stop {
			break
		}
		if page == maxPages {
			t.pending = true
		}
	}
	if t.logger != nil {
		message := fmt.Sprintf("今日获得%d次云朵，累计%d（仅统计）", t.todayCount, t.totalCloud)
		if t.pending {
			message += "；部分流水无效或分页未读取完整"
		}
		t.logger.Info(strings.TrimSpace(message))
	}
	return nil
}
