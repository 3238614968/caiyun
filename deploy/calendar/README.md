# 节假日表导入说明

调度器已接入 `calendar_dates` 表：

- `day_type=holiday`：节假日/休息日，工作日策略会跳过，节假日策略会执行。
- `day_type=workday`：调休工作日，覆盖周末默认休息判断。
- 表中未覆盖日期：回退到周六/周日为休息日。

导入方式：

```bash
DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=caiyun_app DB_PASSWORD='***' DB_NAME=caiyun \
  bash scripts/import-calendar.sh deploy/calendar/zh-cn-2026.csv
```

`deploy/calendar/zh-cn-2026.csv` 内置的是 2026 法定节日兜底种子；生产建议按国务院办公厅当年正式放假通知补齐完整连休和调休工作日后再导入。
