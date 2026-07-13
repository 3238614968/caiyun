# 数据加密密钥轮换 / 重加密操作

统一后端制品使用版本化 AES-GCM 密钥环。`APP_ENV=production` 时 API/Worker 会拒绝读取明文敏感字段；只有显式 `reencrypt` 子命令允许读取并迁移历史明文。

## 1. 同时配置新旧密钥

```env
APP_ENV=production
DATA_ENCRYPTION_KEYS=v1=0123456789abcdef0123456789abcdef,v2=abcdef0123456789abcdef0123456789
DATA_ENCRYPTION_CURRENT_VERSION=v2
```

`v1` 用于解密历史数据，`v2` 用于新写入。确认轮换完成前不要删除旧密钥。

## 2. 先 dry-run

```bash
cd /www/wwwroot/caiyun
./caiyun-linux reencrypt --table all --batch-size 200
```

输出包含扫描行数、待更新行/字段、明文字段、旧版本字段和因并发更新而跳过的行。dry-run 不写数据库。

## 3. 备份并 apply

先完成数据库备份，建议维护窗口运行：

```bash
./caiyun-linux reencrypt --table all --batch-size 200 --apply
./caiyun-linux reencrypt --table all --batch-size 200
```

单表执行：

```bash
./caiyun-linux reencrypt --table accounts --apply
./caiyun-linux reencrypt --table exchange_rules --apply
```

`exchange_accounts` 是 `exchange_rules` 的兼容别名。更新使用原值 CAS；扫描后被 API/Worker 刷新的凭据会安全跳过，不会被旧值覆盖，后续重跑即可。

## 4. 验证和移除旧密钥

1. 验证账号登录、Token 刷新和抢兑规则。
2. 再次 dry-run，确认无明文或旧版本字段。
3. 稳定观察一个发布周期。
4. 从密钥环移除旧版本并重启 API/Worker。

辅助脚本：

```bash
bash rotate-encryption.sh --target /www/wwwroot/caiyun --apply
```

脚本读取目标目录 `.env`，然后调用 `caiyun-linux reencrypt`。
