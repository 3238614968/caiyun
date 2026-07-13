SHELL := /bin/sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GO_LDFLAGS := -s -w -X caiyun/internal/version.Version=$(VERSION) -X caiyun/internal/version.Commit=$(COMMIT) -X caiyun/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: test vet backend-build frontend-build frontend-test frontend-e2e frontend-audit build redis-integration deploy-smoke package-release sync-k8s-sql check-no-local-docs openapi-local openapi-check

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

# 从 migrations/init.sql 同步 backend/scripts 与 k8s/caiyun.yaml，避免手动维护多份 SQL。
sync-k8s-sql:
	python scripts/sync_k8s_sql.py

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
