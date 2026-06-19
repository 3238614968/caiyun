SHELL := /bin/sh

.PHONY: test vet backend-build frontend-build frontend-test frontend-e2e frontend-audit build redis-integration sync-k8s-sql

test:
	cd backend && go test ./...
	cd frontend && npm run test:unit

vet:
	cd backend && go vet ./...

backend-build:
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o api-linux ./cmd/api
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o worker-linux ./cmd/worker

frontend-build:
	cd frontend && npm run build

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
