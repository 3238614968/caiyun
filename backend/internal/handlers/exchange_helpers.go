package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

const maxExchangeScheduleListItems = 100

func normalizeExchangeTime(value, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	for _, layout := range []string{"15:04:05", "15:04"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("15:04:05"), nil
		}
	}
	return "", fmt.Errorf("格式错误，应为 HH:MM 或 HH:MM:SS")
}

func normalizeExchangeTaskType(taskType string) (string, error) {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return string(models.ExchangeTaskFixed), nil
	}
	switch taskType {
	case string(models.ExchangeTaskFixed), string(models.ExchangeTaskLongTerm):
		return taskType, nil
	default:
		return "", fmt.Errorf("任务类型不合法")
	}
}

func normalizeMaxAttempts(maxAttempts int) (int, error) {
	if maxAttempts <= 0 {
		return 1, nil
	}
	if maxAttempts > 100 {
		return 0, fmt.Errorf("最大抢兑次数不能超过 100")
	}
	return maxAttempts, nil
}

var exchangeTaskCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

func parseExchangeTaskFilter(c *gin.Context) repository.ExchangeTaskFilter {
	filter := repository.ExchangeTaskFilter{
		AccountKeyword: strings.TrimSpace(c.Query("account_keyword")),
		Remark:         strings.TrimSpace(c.Query("remark")),
		Status:         strings.TrimSpace(c.Query("status")),
		RestockCycle:   strings.TrimSpace(c.Query("restock_cycle")),
	}
	if minCloud, ok := parseOptionalIntQuery(c.Query("min_cloud")); ok {
		filter.MinCloud = &minCloud
	}
	if maxCloud, ok := parseOptionalIntQuery(c.Query("max_cloud")); ok {
		filter.MaxCloud = &maxCloud
	}
	if active := strings.TrimSpace(c.Query("active")); active != "" {
		value := active == "1" || strings.EqualFold(active, "true") || active == "可用"
		filter.OnlyActive = &value
	}
	return filter
}

func parseOptionalIntQuery(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil
}

func normalizeExchangeScheduleExtras(restockTimesValue any, customCron, calendarPolicy string, holidayDatesValue any, workdayDatesValue any) (string, string, string, string, string, error) {
	restockTimes, err := normalizeTimeList(restockTimesValue)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("补货时间点%s", err.Error())
	}
	customCron = strings.TrimSpace(customCron)
	if customCron != "" {
		if _, err := exchangeTaskCronParser.Parse(customCron); err != nil {
			return "", "", "", "", "", fmt.Errorf("自定义 cron 格式错误: %v", err)
		}
	}
	calendarPolicy = strings.ToLower(strings.TrimSpace(calendarPolicy))
	if calendarPolicy == "" {
		calendarPolicy = "all"
	}
	switch calendarPolicy {
	case "all", "workday", "holiday":
	default:
		return "", "", "", "", "", fmt.Errorf("日历策略不合法")
	}
	holidayDates, err := normalizeDateList(holidayDatesValue)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("节假日日期%s", err.Error())
	}
	workdayDates, err := normalizeDateList(workdayDatesValue)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("调休工作日日期%s", err.Error())
	}
	return restockTimes, customCron, calendarPolicy, holidayDates, workdayDates, nil
}

func normalizeTimeList(value any) (string, error) {
	items := normalizeLooseStringList(value)
	if len(items) == 0 {
		return "", nil
	}
	if len(items) > maxExchangeScheduleListItems {
		return "", fmt.Errorf("数量不能超过 %d 项", maxExchangeScheduleListItems)
	}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		timeValue, err := normalizeExchangeTime(item, "")
		if err != nil {
			return "", err
		}
		normalized = append(normalized, timeValue)
	}
	return strings.Join(uniqueStrings(normalized), ","), nil
}

func normalizeDateList(value any) (string, error) {
	items := normalizeLooseStringList(value)
	if len(items) == 0 {
		return "", nil
	}
	if len(items) > maxExchangeScheduleListItems {
		return "", fmt.Errorf("数量不能超过 %d 项", maxExchangeScheduleListItems)
	}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		parsed, err := time.Parse("2006-01-02", item)
		if err != nil {
			return "", fmt.Errorf("格式错误，应为 YYYY-MM-DD")
		}
		normalized = append(normalized, parsed.Format("2006-01-02"))
	}
	return strings.Join(uniqueStrings(normalized), ","), nil
}

func normalizeLooseStringList(value any) []string {
	var rawItems []string
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		rawItems = strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
		})
	case []string:
		rawItems = v
	case []any:
		for _, item := range v {
			rawItems = append(rawItems, fmt.Sprint(item))
		}
	default:
		rawItems = strings.FieldsFunc(fmt.Sprint(v), func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
		})
	}
	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeRestockConfig(cycle string, weekday *int, dayOfMonth *int) (string, *int, *int, error) {
	cycle = strings.ToLower(strings.TrimSpace(cycle))
	if cycle == "" {
		cycle = "daily"
	}
	switch cycle {
	case "daily":
		return cycle, nil, nil, nil
	case "weekly":
		if weekday == nil {
			currentWeekday := int(time.Now().Weekday())
			weekday = &currentWeekday
		}
		if *weekday < 0 || *weekday > 6 {
			return "", nil, nil, fmt.Errorf("补货周期为 weekly 时星期必须在 0-6 之间")
		}
		return cycle, weekday, nil, nil
	case "monthly":
		if dayOfMonth == nil {
			currentDay := time.Now().Day()
			dayOfMonth = &currentDay
		}
		if *dayOfMonth < 1 || *dayOfMonth > 31 {
			return "", nil, nil, fmt.Errorf("补货周期为 monthly 时日期必须在 1-31 之间")
		}
		return cycle, nil, dayOfMonth, nil
	case "once":
		return cycle, nil, nil, nil
	default:
		return "", nil, nil, fmt.Errorf("补货周期不合法")
	}
}

func normalizePageLimit(page, limit, defaultLimit, maxLimit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}
