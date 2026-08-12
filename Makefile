SHELL := /bin/sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS := -s -w -X caiyun/internal/version.Version=$(VERSION) -X caiyun/internal/version.Commit=$(COMMIT) -X caiyun/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: test vet backend-build frontend-build frontend-test frontend-e2e frontend-audit build redis-integration deploy-smoke package-release sync-k8s-sql check-k8s-sql check-no-local-docs openapi-local openapi-check asyncapi-local asyncapi-check api-types api-types-check k8s-runtime-check otel-runtime-check compose-runtime-check legacy-list-drain chaos-drill capacity-drill operation-integration frontend-contract-check slo-snapshot

test:
	cd backend && go test ./...
	cd frontend && npm run test:unit

vet:
	cd backend && go vet ./...

# 单一后端制品通过 api/worker/migrate/reencrypt/all 子命令按角色运行。
backend-build:
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(GO_LDFLAGS)" -o caiyun-linux ./cmd/caiyun
	cd backend && sha256sum caiyun-linux > SHA256SUMS

frontend-build:
	cd frontend && npm run build
	cd frontend && npm run bundle:budget

frontend-test:
	cd frontend && npm run test:unit

frontend-e2e:
	cd frontend && npm run e2e

frontend-audit:
	cd frontend && npm audit --audit-level=high

build: backend-build frontend-build

redis-integration:
	cd backend && CAIYUN_REDIS_INTEGRATION=1 go test ./internal/queue -run RedisIntegration -count=1

# 校验部署直接使用版本化迁移；Kubernetes 由 Migration Job 执行同镜像迁移。
sync-k8s-sql:
	python scripts/sync_k8s_sql.py --check

check-k8s-sql:
	python scripts/sync_k8s_sql.py --check

deploy-smoke:
	bash scripts/deploy-smoke-test.sh

package-release: build
	bash scripts/package-release.sh

check-no-local-docs:
	bash scripts/check-no-local-docs.sh

openapi-local: check-no-local-docs
	python scripts/generate-openapi-local.py

openapi-check: openapi-local
	python scripts/validate-openapi-local.py

asyncapi-local: check-no-local-docs
	python scripts/generate-asyncapi-local.py

asyncapi-check: asyncapi-local
	python scripts/validate-asyncapi-local.py

# Generated TypeScript is local build output; its schema inputs remain .local-only.
api-types: openapi-local asyncapi-local
	python scripts/generate-api-client-types.py

api-types-check: openapi-local asyncapi-local
	python scripts/generate-api-client-types.py --check

frontend-contract-check: api-types-check
	python scripts/validate-frontend-contract-usage.py

k8s-runtime-check:
	bash scripts/verify-k8s-runtime.sh

otel-runtime-check:
	bash scripts/verify-otel-runtime.sh

# Target-environment release gates. They are read-only unless the script's
# explicit --execute option is supplied directly by an operator.
compose-runtime-check:
	bash scripts/verify-compose-runtime.sh all

legacy-list-drain:
	bash scripts/verify-legacy-list-drain.sh

chaos-drill:
	bash scripts/chaos-drill.sh worker-recovery

capacity-drill:
	bash scripts/capacity-drill.sh

slo-snapshot:
	bash scripts/collect-slo-snapshot.sh

operation-integration:
	cd backend && CAIYUN_OPERATION_INTEGRATION=1 go test ./internal/services -run OperationMySQLRedisIntegration -count=1
