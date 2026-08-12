#!/usr/bin/env python3
"""Validate deployment invariants that are not covered by a YAML parser alone."""
from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
BASE = (ROOT / "k8s" / "caiyun.yaml").read_text(encoding="utf-8")
OBSERVABILITY = (ROOT / "k8s" / "observability-prometheus-operator.yaml").read_text(encoding="utf-8")
KEDA = (ROOT / "k8s" / "autoscaling-keda.yaml").read_text(encoding="utf-8")
DEPLOY = (ROOT / "scripts" / "deploy-k8s.sh").read_text(encoding="utf-8")
RUNTIME_CHECK = (ROOT / "scripts" / "verify-k8s-runtime.sh").read_text(encoding="utf-8")
OTEL_RUNTIME_CHECK = (ROOT / "scripts" / "verify-otel-runtime.sh").read_text(encoding="utf-8")
OTEL_COLLECTOR = (ROOT / "k8s" / "observability-otel-collector.yaml").read_text(encoding="utf-8")
CHAOS_DRILL = (ROOT / "scripts" / "chaos-drill.sh").read_text(encoding="utf-8")
CAPACITY_DRILL = (ROOT / "scripts" / "capacity-drill.sh").read_text(encoding="utf-8")
SLO_SNAPSHOT = (ROOT / "scripts" / "collect-slo-snapshot.sh").read_text(encoding="utf-8")
RULES = (ROOT / "deploy" / "monitoring" / "prometheus-rules.yml").read_text(encoding="utf-8")
OVERLAYS = ROOT / "k8s" / "overlays"


def require(content: str, token: str, location: str) -> None:
    if token not in content:
        raise SystemExit(f"{location} is missing required token: {token}")


def main() -> int:
    for token in ("caiyun-mysql-init-sql", "/docker-entrypoint-initdb.d/", "001_init_caiyun_database.sql"):
        if token in BASE:
            raise SystemExit(f"base K8s manifest still contains legacy SQL snapshot token: {token}")
    for token in ("kind: Job", "name: caiyun-migrate", "args: [\"migrate\"]", "name: wait-mysql", "name: wait-redis"):
        require(BASE, token, "base K8s manifest")
    for token in ('mysqladmin ping -h "$DB_HOST"', 'redis-cli -h "$REDIS_HOST"'):
        require(BASE, token, "base K8s migration dependency waits")
    if BASE.count("image: caiyun-backend:2.1.0") != 3:
        raise SystemExit("base K8s manifest must have exactly API, Worker and Migration Job backend image placeholders")
    for token in ("kind: HorizontalPodAutoscaler", "name: backend-api", "name: backend-worker", "name: backend-worker\n  namespace", "name: metrics", "REDIS_ADDRESS: \"redis:6379\"", "TASK_QUEUE_STREAM_KEY: \"task:queue:stream\""):
        require(BASE, token, "base K8s manifest")
    for token in ("OTEL_ENABLED: \"false\"", "OTEL_EXPORTER_OTLP_ENDPOINT: \"\"", "OTEL_EXPORTER_OTLP_CERTIFICATE: \"\"", "OTEL_SAMPLE_RATIO: \"0.10\"", "FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD: \"false\""):
        require(BASE, token, "base K8s observability config")
    for token in ("TZ: Asia/Shanghai", "TASK_CONFIG_SYNC_ON_STARTUP: \"false\"", "terminationGracePeriodSeconds: 60", "terminationGracePeriodSeconds: 75"):
        require(BASE, token, "base K8s runtime hardening")
    for workload in ("backend-api", "backend-worker", "frontend"):
        start = BASE.find(f"kind: Deployment\nmetadata:\n  name: {workload}\n")
        if start < 0:
            raise SystemExit(f"base K8s manifest is missing Deployment {workload}")
        end = BASE.find("\n---", start)
        section = BASE[start:] if end < 0 else BASE[start:end]
        require(section, 'lifecycle:\n            preStop:\n              exec:\n                command: ["/bin/sh", "-c", "sleep 5"]', f"{workload} graceful rollout")
    for token in ("kind: ServiceMonitor", "name: caiyun-api", "name: caiyun-worker", "key: api-monitor-token", "key: worker-monitor-token"):
        require(OBSERVABILITY, token, "Prometheus Operator manifest")
    for token in ("kind: TriggerAuthentication", "kind: ScaledObject", "name: backend-worker-redis-streams", "addressFromEnv: REDIS_ADDRESS", "stream: task:queue:stream", "consumerGroup: caiyun-workers", "lagCount: \"25\"", "activationLagCount: \"1\"", "key: redis-password"):
        require(KEDA, token, "KEDA manifest")
    for token in ("BACKEND_IMAGE must be an immutable image digest", "OTEL_COLLECTOR_IMAGE must be an immutable digest", "servicemonitors.monitoring.coreos.com", "OBSERVABILITY_MANIFEST", "scaledobjects.keda.sh", "KEDA_MANIFEST", "delete hpa backend-worker"):
        require(DEPLOY, token, "deploy-k8s.sh")
    for token in ("job/$JOB_NAME", "backend-api", "backend-worker", "metrics.k8s.io", "servicemonitor caiyun-api caiyun-worker", "scaledobject backend-worker-redis-streams", "fallback backend-worker HPA"):
        require(RUNTIME_CHECK, token, "verify-k8s-runtime.sh")
    for token in ("REQUIRE_OTEL_COLLECTOR", "OTEL_COLLECTOR_IMAGE", "caiyun-otel-collector", "OTEL_EXPORTER_OTLP_CERTIFICATE", "caiyun-otel-receiver-ca"):
        require(RUNTIME_CHECK, token, "verify-k8s-runtime.sh")
    for token in ("kubectl", "curl", "port-forward", "caiyun-otel-receiver-tls", "caiyun-otel-exporter", "OTEL_EXPORTER_OTLP_INSECURE"):
        require(OTEL_RUNTIME_CHECK, token, "verify-otel-runtime.sh")
    for token in ("kind: Deployment", "name: caiyun-otel-collector", "kind: Service", "kind: ConfigMap", "tls.crt", "tls.key", "TRACE_BACKEND_OTLP_HTTP_ENDPOINT", "readOnlyRootFilesystem: true"):
        require(OTEL_COLLECTOR, token, "OTel Collector manifest")
    for token in ("worker-recovery", "api-rollout", "dry-run", "verify-k8s-runtime.sh"):
        require(CHAOS_DRILL, token, "chaos-drill.sh")
    for token in ("CAPACITY_JOB_MANIFEST", "activeDeadlineSeconds", "TARGET_WORKER_REPLICAS", "PROMETHEUS_URL", "collect-slo-snapshot.sh", "verify-k8s-runtime.sh"):
        require(CAPACITY_DRILL, token, "capacity-drill.sh")
    for token in ("PROMETHEUS_URL", "caiyun_http_request_duration_seconds_bucket", "caiyun_operation_transitions_total", "caiyun_queue_pending", "caiyun_worker_heartbeat_unix", "SLO_ENFORCE", ".local/slo"):
        require(SLO_SNAPSHOT, token, "collect-slo-snapshot.sh")
    for token in ("CaiyunOperationFailureSpike", "CaiyunOperationFailureRatioHigh", "caiyun_operation_transitions_total"):
        require(RULES, token, "Prometheus alert rules")

    for environment in ("dev", "staging", "production"):
        directory = OVERLAYS / environment
        kustomization = (directory / "kustomization.yaml").read_text(encoding="utf-8")
        config_patch = (directory / "config.patch.yaml").read_text(encoding="utf-8")
        workload_patch = (directory / "workloads.patch.yaml").read_text(encoding="utf-8")
        require(kustomization, "resources:\n  - ../../", f"{environment} overlay")
        require(kustomization, f"namespace: caiyun-{environment}", f"{environment} overlay")
        require(kustomization, "name: caiyun-backend", f"{environment} overlay")
        require(kustomization, "name: caiyun-frontend", f"{environment} overlay")
        require(config_patch, "APP_ENV:", f"{environment} ConfigMap patch")
        for workload in ("name: backend-api", "name: backend-worker", "name: frontend"):
            require(workload_patch, workload, f"{environment} workload patch")

    production_config = (OVERLAYS / "production" / "config.patch.yaml").read_text(encoding="utf-8")
    production_data_patch = (OVERLAYS / "production" / "managed-data-services.patch.yaml").read_text(encoding="utf-8")
    for token in ("MYSQL_MANAGED_ENDPOINT", "REDIS_MANAGED_ENDPOINT"):
        require(production_config, token, "production managed data overlay")
    for token in ("OTEL_ENABLED: \"true\"", "caiyun-otel-collector:4317", "OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_SAMPLE_RATIO: \"0.10\""):
        require(production_config, token, "production OTel overlay")
    production_kustomization = (OVERLAYS / "production" / "kustomization.yaml").read_text(encoding="utf-8")
    production_workloads = (OVERLAYS / "production" / "workloads.patch.yaml").read_text(encoding="utf-8")
    require(production_kustomization, "observability-otel-collector.yaml", "production OTel overlay")
    for token in ("otel-collector-ca", "caiyun-otel-receiver-ca", "/etc/caiyun/otel-ca"):
        require(production_workloads, token, "production OTel workload patch")
    for token in ("$patch: delete", "name: mysql", "name: redis"):
        require(production_data_patch, token, "production managed data overlay")
    print("Kubernetes migration, observability and autoscaling invariants valid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
