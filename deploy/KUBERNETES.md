# Kubernetes 部署

本文基于仓库中的 `k8s/caiyun.yaml` 与 `scripts/deploy-k8s.sh`，适用于测试集群、私有云及生产 Kubernetes 集群。清单包含 MySQL、Redis、迁移 Job、API、Worker、前端、持久卷、PDB 和 NetworkPolicy；对数据库可靠性要求较高时，建议改用云数据库与托管 Redis。

## 1. 部署拓扑

| 资源 | 默认名称 | 说明 |
| --- | --- | --- |
| Namespace | `caiyun` | 资源隔离 |
| StatefulSet | `mysql`、`redis` | 内置数据服务 |
| Job | `caiyun-migrate` | 部署前执行数据库迁移 |
| Deployment | `backend-api` | HTTP API，默认 2 副本 |
| Deployment | `backend-worker` | 定时任务与队列消费，默认 2 副本 |
| Deployment | `frontend` | 静态资源与反向代理，默认 2 副本 |
| Service | `mysql`、`redis`、`backend-api`、`backend-worker`、`frontend` | 集群内访问 |

当前清单只创建 `ClusterIP` Service，不会自动暴露公网入口。

## 2. 前置条件

- 可用的 Kubernetes 集群和 `kubectl`。
- 集群存在默认 `StorageClass`，或已在 PVC 中明确指定存储类。
- 节点能够拉取后端与前端镜像。
- 已安装 Ingress Controller（需要域名入口时）。
- DNS、TLS 证书、镜像仓库凭据已经准备完毕。

```bash
kubectl version --client
kubectl cluster-info
kubectl get nodes
kubectl get storageclass
```

## 3. 构建并推送镜像

为每次发布使用不可变版本号，避免使用浮动的 `latest`：

```bash
VERSION=2.1.0
REGISTRY=registry.example.com/caiyun

docker build -t "$REGISTRY/backend:$VERSION" -f backend/Dockerfile .
docker build -t "$REGISTRY/frontend:$VERSION" -f frontend/Dockerfile frontend

docker push "$REGISTRY/backend:$VERSION"
docker push "$REGISTRY/frontend:$VERSION"
```

将 `k8s/caiyun.yaml` 中迁移 Job、API、Worker 和前端的 `image:` 替换为上述完整地址。私有仓库还需创建拉取凭据，并在 Pod 模板中配置 `imagePullSecrets`：

```bash
kubectl create namespace caiyun
kubectl -n caiyun create secret docker-registry registry-credentials \
  --docker-server=registry.example.com \
  --docker-username=REGISTRY_USER \
  --docker-password=REGISTRY_PASSWORD
```

## 4. 创建运行密钥

先创建命名空间，再创建 Secret。生产密钥应由密码管理系统生成并托管，不要写入清单或提交到仓库。

```bash
kubectl create namespace caiyun --dry-run=client -o yaml | kubectl apply -f -

kubectl -n caiyun create secret generic caiyun-secrets \
  --from-literal=mysql-root-password='MYSQL_ROOT_PASSWORD' \
  --from-literal=mysql-password='MYSQL_APP_PASSWORD' \
  --from-literal=redis-password='REDIS_PASSWORD' \
  --from-literal=jwt-secret='JWT_SECRET' \
  --from-literal=data-encryption-keys='v1=32_BYTE_KEY' \
  --from-literal=worker-monitor-token='MONITOR_TOKEN' \
  --from-literal=smtp-password='SMTP_PASSWORD'
```

`DATA_ENCRYPTION_KEYS` 的活动密钥必须能够解密历史数据。轮换方法见 [`../backend/docs/ENCRYPTION_ROTATION.md`](../backend/docs/ENCRYPTION_ROTATION.md)。

## 5. 调整配置与存储

部署前检查 `k8s/caiyun.yaml` 中的 ConfigMap：

- `ALLOWED_ORIGINS`：填写实际 HTTPS 域名。
- `TRUSTED_PROXIES`：仅信任 Ingress 或内部代理网段。
- `DB_HOST`、`REDIS_HOST`：使用外部托管服务时替换地址。
- SMTP、并发数、日志级别：按环境调整。
- CPU、内存 requests/limits：依据压测结果设置。

检查 PVC 状态：

```bash
kubectl -n caiyun get pvc
kubectl describe pvc -n caiyun
```

生产数据库应使用支持快照、扩容和跨可用区恢复的存储。若改用外部 MySQL/Redis，应删除或禁用清单中的相应 StatefulSet、Service 和 PVC，避免误连内部实例。

## 6. 执行部署

仓库脚本会应用清单、重建迁移 Job、等待迁移完成，再等待各 Deployment 滚动完成：

```bash
bash scripts/deploy-k8s.sh
```

也可以手动执行以便逐步观察：

```bash
kubectl apply -f k8s/caiyun.yaml
kubectl -n caiyun logs -f job/caiyun-migrate
kubectl -n caiyun wait --for=condition=complete job/caiyun-migrate --timeout=10m
kubectl -n caiyun rollout status deployment/backend-api --timeout=5m
kubectl -n caiyun rollout status deployment/backend-worker --timeout=5m
kubectl -n caiyun rollout status deployment/frontend --timeout=5m
```

迁移 Job 失败时不要继续放量，先查看日志并修复配置、权限或迁移问题。

## 7. 配置 Ingress 与 TLS

前端容器已将 `/api`、`/events` 和 `/ws` 转发到后端，因此入口可统一指向 `frontend` Service：

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: caiyun
  namespace: caiyun
  annotations:
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [cloud.example.com]
      secretName: caiyun-tls
  rules:
    - host: cloud.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: frontend
                port:
                  number: 80
```

```bash
kubectl apply -f ingress.yaml
kubectl -n caiyun get ingress
```

若使用其他 Ingress Controller，请使用其对应的 SSE、WebSocket 和超时配置。

## 8. 验证部署

```bash
kubectl -n caiyun get pods,svc,pvc,job
kubectl -n caiyun get events --sort-by=.lastTimestamp
kubectl -n caiyun logs deployment/backend-api --tail=200
kubectl -n caiyun logs deployment/backend-worker --tail=200
kubectl -n caiyun port-forward service/frontend 8080:80
```

随后访问 `http://127.0.0.1:8080`，并验证登录、任务创建、实时事件和 Worker 监控端点。确认所有 Pod 的 readiness probe 通过后再切换生产流量。

## 9. 扩缩容与更新

```bash
kubectl -n caiyun scale deployment/backend-api --replicas=4
kubectl -n caiyun scale deployment/backend-worker --replicas=3
kubectl -n caiyun set image deployment/backend-api api=REGISTRY/backend:VERSION
kubectl -n caiyun rollout status deployment/backend-api
kubectl -n caiyun rollout history deployment/backend-api
```

Worker 副本数应结合队列积压、外部接口限流和任务幂等性评估。升级和回滚流程见 [`UPGRADE_ROLLBACK.md`](UPGRADE_ROLLBACK.md)。

## 10. 生产检查

- 镜像使用版本或摘要固定，发布前完成漏洞扫描。
- Secret 不以明文进入 Git、镜像或日志。
- API、Worker 和前端设置 PDB、资源限制及反亲和策略。
- MySQL、Redis 和 PVC 纳入备份与恢复演练。
- NetworkPolicy 与云防火墙仅放行必要链路。
- 对迁移 Job、Pod 重启、队列积压和存储容量建立告警。