package monitor

import (
	"testing"
	"time"
)

func TestMetricsWorkerHeartbeatAndState(t *testing.T) {
	metrics := NewMetrics()
	heartbeat := time.Unix(1_719_451_200, 0)

	metrics.SetWorkerState(false)
	metrics.TouchWorkerHeartbeat(heartbeat)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	values := map[string]float64{}
	for _, family := range families {
		if len(family.Metric) == 0 || family.Metric[0].Gauge == nil {
			continue
		}
		values[family.GetName()] = family.Metric[0].Gauge.GetValue()
	}

	if got := values["caiyun_worker_up"]; got != 1 {
		t.Fatalf("worker_up = %v, want 1", got)
	}
	if got := values["caiyun_worker_heartbeat_unix"]; got != float64(heartbeat.Unix()) {
		t.Fatalf("worker_heartbeat_unix = %v, want %d", got, heartbeat.Unix())
	}
}

func TestMetricsSetAuditDroppedUsesCounterDelta(t *testing.T) {
	metrics := NewMetrics()
	metrics.SetAuditDropped(2)
	metrics.SetAuditDropped(5)
	metrics.SetAuditDropped(5)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "caiyun_audit_dropped_total" {
			continue
		}
		if len(family.Metric) == 0 || family.Metric[0].Counter == nil {
			t.Fatalf("metric type = %#v, want counter", family.Type)
		}
		if got := family.Metric[0].Counter.GetValue(); got != 5 {
			t.Fatalf("audit dropped total = %v, want 5", got)
		}
		return
	}
	t.Fatal("caiyun_audit_dropped_total not found")
}

func TestMetricsIncAuditDroppedStaysInSyncWithPolling(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncAuditDropped()
	metrics.SetAuditDropped(1)
	metrics.SetAuditDropped(2)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "caiyun_audit_dropped_total" {
			continue
		}
		if len(family.Metric) == 0 || family.Metric[0].Counter == nil {
			t.Fatalf("metric type = %#v, want counter", family.Type)
		}
		if got := family.Metric[0].Counter.GetValue(); got != 2 {
			t.Fatalf("audit dropped total = %v, want 2", got)
		}
		return
	}
	t.Fatal("caiyun_audit_dropped_total not found")
}

func TestMetricsRecordHistoryArchiveRun(t *testing.T) {
	metrics := NewMetrics()
	finishedAt := time.Unix(1_719_451_800, 0)
	metrics.RecordHistoryArchiveRunAt("success", finishedAt, 2*time.Second, 3, 5, true)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	foundRuns := false
	foundHitLimit := false
	foundLastRun := false
	foundLastSuccess := false
	for _, family := range families {
		switch family.GetName() {
		case "caiyun_history_archive_runs_total":
			for _, metric := range family.Metric {
				if len(metric.Label) > 0 && metric.Label[0].GetValue() == "success" && metric.Counter != nil && metric.Counter.GetValue() == 1 {
					foundRuns = true
				}
			}
		case "caiyun_history_archive_hit_batch_limit_total":
			if len(family.Metric) > 0 && family.Metric[0].Counter != nil && family.Metric[0].Counter.GetValue() == 1 {
				foundHitLimit = true
			}
		case "caiyun_history_archive_last_run_unix":
			if len(family.Metric) > 0 && family.Metric[0].Gauge != nil && family.Metric[0].Gauge.GetValue() == float64(finishedAt.Unix()) {
				foundLastRun = true
			}
		case "caiyun_history_archive_last_success_unix":
			if len(family.Metric) > 0 && family.Metric[0].Gauge != nil && family.Metric[0].Gauge.GetValue() == float64(finishedAt.Unix()) {
				foundLastSuccess = true
			}
		}
	}
	if !foundRuns || !foundHitLimit || !foundLastRun || !foundLastSuccess {
		t.Fatalf("archive run metrics missing: runs=%t hitLimit=%t lastRun=%t lastSuccess=%t", foundRuns, foundHitLimit, foundLastRun, foundLastSuccess)
	}
}

func TestMetricsRecordHistoryArchiveBatch(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordHistoryArchiveBatch("task_logs", 3, 150*time.Millisecond)
	metrics.RecordHistoryArchiveBatch("exchange_records", 5, 200*time.Millisecond)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	foundTaskRows := false
	foundRecordRows := false
	foundTaskBatch := false
	foundRecordBatch := false
	for _, family := range families {
		switch family.GetName() {
		case "caiyun_history_archive_moved_rows_total":
			for _, metric := range family.Metric {
				if len(metric.Label) == 0 || metric.Counter == nil {
					continue
				}
				switch metric.Label[0].GetValue() {
				case "task_logs":
					if metric.Counter.GetValue() == 3 {
						foundTaskRows = true
					}
				case "exchange_records":
					if metric.Counter.GetValue() == 5 {
						foundRecordRows = true
					}
				}
			}
		case "caiyun_history_archive_batches_total":
			for _, metric := range family.Metric {
				if len(metric.Label) == 0 || metric.Counter == nil {
					continue
				}
				switch metric.Label[0].GetValue() {
				case "task_logs":
					if metric.Counter.GetValue() == 1 {
						foundTaskBatch = true
					}
				case "exchange_records":
					if metric.Counter.GetValue() == 1 {
						foundRecordBatch = true
					}
				}
			}
		}
	}
	if !foundTaskRows || !foundRecordRows || !foundTaskBatch || !foundRecordBatch {
		t.Fatalf("archive batch metrics missing: taskRows=%t recordRows=%t taskBatch=%t recordBatch=%t", foundTaskRows, foundRecordRows, foundTaskBatch, foundRecordBatch)
	}
}

func TestMetricsRecordExchangeAttempt(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordExchangeAttempt(true, "ignored", 250*time.Millisecond)
	metrics.RecordExchangeAttempt(false, "auth", 500*time.Millisecond)

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	foundAttemptsSuccess := false
	foundAttemptsFailed := false
	foundSuccessTotal := false
	foundFailedTotal := false
	for _, family := range families {
		switch family.GetName() {
		case "caiyun_exchange_attempts_total":
			for _, metric := range family.Metric {
				if len(metric.Label) < 2 || metric.Counter == nil {
					continue
				}
				labels := map[string]string{}
				for _, label := range metric.Label {
					labels[label.GetName()] = label.GetValue()
				}
				if labels["result"] == "success" && labels["reason"] == "success" && metric.Counter.GetValue() == 1 {
					foundAttemptsSuccess = true
				}
				if labels["result"] == "failed" && labels["reason"] == "auth" && metric.Counter.GetValue() == 1 {
					foundAttemptsFailed = true
				}
			}
		case "caiyun_exchange_success_total":
			if len(family.Metric) > 0 && family.Metric[0].Counter != nil && family.Metric[0].Counter.GetValue() == 1 {
				foundSuccessTotal = true
			}
		case "caiyun_exchange_failed_total":
			if len(family.Metric) > 0 && family.Metric[0].Counter != nil && family.Metric[0].Counter.GetValue() == 1 {
				foundFailedTotal = true
			}
		}
	}
	if !foundAttemptsSuccess || !foundAttemptsFailed || !foundSuccessTotal || !foundFailedTotal {
		t.Fatalf("exchange attempt metrics missing: successAttempt=%t failedAttempt=%t successTotal=%t failedTotal=%t", foundAttemptsSuccess, foundAttemptsFailed, foundSuccessTotal, foundFailedTotal)
	}
}
