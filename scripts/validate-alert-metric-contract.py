#!/usr/bin/env python3
"""Prevent Prometheus alerts from referring to unregistered Worker metrics."""
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
rules = (root / "deploy/monitoring/prometheus-rules.yml").read_text(encoding="utf-8")
metrics = (root / "backend/internal/monitor/metrics.go").read_text(encoding="utf-8")
monitor = (root / "k8s/observability-prometheus-operator.yaml").read_text(encoding="utf-8")
manifest = (root / "k8s/caiyun.yaml").read_text(encoding="utf-8")
snapshot = (root / "scripts/collect-slo-snapshot.sh").read_text(encoding="utf-8")
failures = []

# Alert rules refer to Prometheus names; keep their source definitions in the
# same contract so a future refactor cannot silently sever a non-Worker alert.
metric_sources = {
    'caiyun_exchange_recent_total': ('exchange', 'recent_total'),
    'caiyun_exchange_success_rate': ('exchange', 'success_rate'),
    'caiyun_exchange_failed_total': ('exchange', 'failed_total'),
    'caiyun_exchange_scheduler_skipped_total': ('exchange_scheduler', 'skipped_total'),
    'caiyun_exchange_duration_seconds': ('exchange', 'duration_seconds'),
    'caiyun_security_rate_limit_rejected_total': ('security', 'rate_limit_rejected_total'),
    'caiyun_audit_dropped_total': ('audit', 'dropped_total'),
    'caiyun_http_request_duration_seconds': ('http', 'request_duration_seconds'),
    'caiyun_http_requests_total': ('http', 'requests_total'),
    'caiyun_queue_pending': ('queue', 'pending'),
    'caiyun_queue_dead_letter': ('queue', 'dead_letter'),
    'caiyun_queue_delayed': ('queue', 'delayed'),
    'caiyun_worker_up': ('worker', 'up'),
    'caiyun_worker_heartbeat_unix': ('worker', 'heartbeat_unix'),
    'caiyun_operation_transitions_total': ('operation', 'transitions_total'),
    'caiyun_history_archive_runs_total': ('history_archive', 'runs_total'),
    'caiyun_history_archive_hit_batch_limit_total': ('history_archive', 'hit_batch_limit_total'),
    'caiyun_history_archive_last_success_unix': ('history_archive', 'last_success_unix'),
}
for metric, (subsystem, name) in metric_sources.items():
    if metric not in rules:
        failures.append(f"missing alert metric reference: {metric}")
    if f'Subsystem: "{subsystem}"' not in metrics or f'Name:      "{name}"' not in metrics:
        failures.append(f"missing registered metric source: {metric}")
for token in ('absent(caiyun_worker_up{job="caiyun-worker"})', 'caiyun_worker_heartbeat_unix{job="caiyun-worker"}'):
    if token not in rules:
        failures.append(f"missing worker scoping contract token: {token}")
if 'caiyun_worker_running' in rules:
    failures.append('stale unregistered metric caiyun_worker_running in alert rules')
if 'caiyun_worker_heartbeat_unix{job=\\"caiyun-worker\\"}' not in snapshot:
    failures.append('SLO heartbeat query must scope to the caiyun-worker job')
if monitor.count('jobLabel: monitoring-job') != 2:
    failures.append('both ServiceMonitors must set jobLabel: monitoring-job')
for token in ('monitoring-job: caiyun-api', 'monitoring-job: caiyun-worker'):
    if token not in manifest:
        failures.append(f"service label missing: {token}")
if failures:
    print('\n'.join(failures), file=sys.stderr)
    raise SystemExit(1)
print('Prometheus Worker metric/alert contract valid')
