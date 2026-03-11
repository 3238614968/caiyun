# 移动云盘管理系统 API 文档

## 概述

本文档描述了移动云盘管理系统的 RESTful API 接口。

- **基础URL**: `http://localhost:8080`
- **认证方式**: Bearer Token (JWT)
- **内容类型**: `application/json`

---

## 认证

所有需要认证的接口都需要在请求头中包含以下字段：

```
Authorization: Bearer <your-jwt-token>
```

---

## 通用响应格式

### 成功响应

```json
{
  "code": 200,
  "message": "success",
  "data": { ... }
}
```

### 错误响应

```json
{
  "code": 400,
  "message": "错误信息",
  "error": "详细错误描述"
}
```

---

## 接口列表

### 1. 认证接口

#### 1.1 用户注册

**POST** `/api/auth/register`

**请求体:**
```json
{
  "username": "testuser",
  "password": "password123",
  "email": "test@example.com"
}
```

**响应:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": 1704067200,
  "user": {
    "id": 1,
    "username": "testuser",
    "email": "test@example.com",
    "role": "user"
  }
}
```

**错误码:**
- `400` - 请求参数错误
- `409` - 用户名或邮箱已存在

---

#### 1.2 用户登录

**POST** `/api/auth/login`

**请求体:**
```json
{
  "username": "testuser",
  "password": "password123"
}
```

**响应:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": 1704067200,
  "user": {
    "id": 1,
    "username": "testuser",
    "email": "test@example.com",
    "role": "user"
  }
}
```

**错误码:**
- `400` - 请求参数错误
- `401` - 用户名或密码错误

---

#### 1.3 刷新Token

**POST** `/api/auth/refresh`

**请求头:**
```
Authorization: Bearer <current-token>
```

**响应:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": 1704067200
}
```

---

### 2. 账号管理接口

#### 2.1 获取账号列表

**GET** `/api/accounts`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 10 | 每页数量 (1-100) |

**响应:**
```json
{
  "accounts": [
    {
      "id": 1,
      "user_id": 1,
      "phone": "13800138000",
      "platform": "pc",
      "cloud_count": 1000,
      "remark": "主账号",
      "is_active": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "total": 10,
  "page": 1,
  "page_size": 10
}
```

---

#### 2.2 创建账号

**POST** `/api/accounts`

**请求体:**
```json
{
  "phone": "13800138000",
  "auth": "base64_encoded_auth_string",
  "remark": "主账号"
}
```

**响应:**
```json
{
  "id": 1,
  "user_id": 1,
  "phone": "13800138000",
  "platform": "pc",
  "cloud_count": 0,
  "remark": "主账号",
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

**错误码:**
- `400` - 请求参数错误
- `409` - 手机号已存在

---

#### 2.3 获取账号详情

**GET** `/api/accounts/{id}`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**响应:**
```json
{
  "id": 1,
  "user_id": 1,
  "phone": "13800138000",
  "platform": "pc",
  "cloud_count": 1000,
  "remark": "主账号",
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

**错误码:**
- `404` - 账号不存在

---

#### 2.4 更新账号

**PUT** `/api/accounts/{id}`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**请求体:**
```json
{
  "phone": "13800138000",
  "auth": "base64_encoded_auth_string",
  "remark": "更新后的备注"
}
```

**响应:**
```json
{
  "id": 1,
  "user_id": 1,
  "phone": "13800138000",
  "platform": "pc",
  "cloud_count": 1000,
  "remark": "更新后的备注",
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-02T00:00:00Z"
}
```

---

#### 2.5 删除账号

**DELETE** `/api/accounts/{id}`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**响应:**
```json
{
  "message": "删除成功"
}
```

---

#### 2.6 设置账号状态

**PUT** `/api/accounts/{id}/status?is_active=true`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**查询参数:**
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| is_active | bool | 是 | 是否激活 |

**响应:**
```json
{
  "message": "状态更新成功"
}
```

---

#### 2.7 刷新账号Token

**POST** `/api/accounts/{id}/refresh`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**响应:**
```json
{
  "id": 1,
  "user_id": 1,
  "phone": "13800138000",
  "token": "refreshed_token...",
  "expire_at": 1704067200000
}
```

---

#### 2.8 手动触发任务

**POST** `/api/accounts/{id}/trigger`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**响应:**
```json
{
  "message": "任务已提交执行"
}
```

---

### 3. 任务管理接口

#### 3.1 获取任务日志

**GET** `/api/tasks/logs`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| account_id | int | 否 | - | 账号ID过滤 |
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 20 | 每页数量 (1-100) |

**响应:**
```json
{
  "task_logs": [
    {
      "id": 1,
      "user_id": 1,
      "account_id": 1,
      "task_type": "signin",
      "status": "success",
      "message": "签到成功",
      "cloud_gained": 10,
      "execution_time": 500,
      "created_at": "2024-01-01T08:00:00Z"
    }
  ],
  "total": 100,
  "page": 1,
  "page_size": 20
}
```

---

#### 3.2 触发所有任务

**POST** `/api/tasks/trigger-all`

**响应:**
```json
{
  "message": "所有任务已提交执行"
}
```

---

### 4. 数据统计接口

#### 4.1 获取仪表盘数据

**GET** `/api/stats/dashboard`

**响应:**
```json
{
  "data": {
    "total_cloud": 10000,
    "account_count": 5,
    "today_gained": 500,
    "yesterday_diff": 100,
    "week_diff": 500,
    "success_rate": 95.5,
    "trend_data": [
      {
        "date": "2024-01-01",
        "cloud_count": 9500
      }
    ],
    "account_ranking": [
      {
        "account_id": 1,
        "phone": "138****8000",
        "remark": "主账号",
        "cloud_count": 5000
      }
    ]
  }
}
```

---

#### 4.2 获取云朵统计

**GET** `/api/stats/cloud`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| account_id | int | 否 | - | 账号ID过滤 |
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 10 | 每页数量 |

**响应:**
```json
{
  "cloud_stats": [
    {
      "id": 1,
      "user_id": 1,
      "account_id": 1,
      "date": "2024-01-01",
      "cloud_count": 1000,
      "cloud_diff": 100,
      "cloud_diff_week": 500
    }
  ],
  "total": 30,
  "page": 1,
  "page_size": 10
}
```

---

#### 4.3 获取趋势数据

**GET** `/api/stats/trend`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| days | int | 否 | 7 | 天数 (1-365) |

**响应:**
```json
{
  "trend_data": [
    {
      "date": "2024-01-01",
      "cloud_count": 9500
    },
    {
      "date": "2024-01-02",
      "cloud_count": 9600
    }
  ]
}
```

---

#### 4.4 手动计算统计数据

**POST** `/api/stats/calculate`

**响应:**
```json
{
  "message": "统计数据计算完成"
}
```

---

#### 4.5 获取总云朵数

**GET** `/api/stats/total-cloud`

**响应:**
```json
{
  "total_cloud": 10000
}
```

---

### 5. 管理员接口

> ⚠️ **注意**: 以下接口需要管理员权限 (role = "admin")

#### 5.1 获取所有用户

**GET** `/api/admin/users`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| page | int | 否 | 1 | 页码 |
| size | int | 否 | 10 | 每页数量 |

**响应:**
```json
{
  "users": [
    {
      "id": 1,
      "username": "admin",
      "email": "admin@example.com",
      "role": "admin",
      "created_at": "2024-01-01 00:00:00"
    }
  ],
  "total": 10,
  "page": 1,
  "size": 10
}
```

---

#### 5.2 获取所有账号

**GET** `/api/admin/accounts`

**查询参数:**
| 参数 | 类型 | 必填 | 默认值 | 描述 |
|------|------|------|--------|------|
| page | int | 否 | 1 | 页码 |
| page_size | int | 否 | 10 | 每页数量 |

**响应:** 同 `/api/accounts` 列表接口

---

#### 5.3 更新用户角色

**PUT** `/api/admin/users/{id}/role`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 用户ID |

**请求体:**
```json
{
  "role": "admin"
}
```

**响应:**
```json
{
  "message": "角色更新成功"
}
```

---

#### 5.4 更新账号状态

**PUT** `/api/admin/accounts/{id}/status`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**请求体:**
```json
{
  "is_active": true
}
```

**响应:**
```json
{
  "message": "账号状态更新成功"
}
```

---

#### 5.5 删除用户

**DELETE** `/api/admin/users/{id}`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 用户ID |

**响应:**
```json
{
  "message": "用户删除成功"
}
```

**错误码:**
- `400` - 不能删除自己

---

#### 5.6 删除账号

**DELETE** `/api/admin/accounts/{id}`

**路径参数:**
| 参数 | 类型 | 描述 |
|------|------|------|
| id | int | 账号ID |

**响应:**
```json
{
  "message": "账号删除成功"
}
```

---

#### 5.7 获取统计概览

**GET** `/api/admin/stats/overview`

**响应:**
```json
{
  "user_count": 10,
  "account_count": 50,
  "total_cloud": 50000,
  "active_tasks": 45
}
```

---

### 6. 健康检查

#### 6.1 服务健康检查

**GET** `/health`

**响应:**
```json
{
  "status": "ok"
}
```

---

## 错误码说明

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 201 | 创建成功 |
| 400 | 请求参数错误 |
| 401 | 未授权（Token无效或过期） |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 409 | 资源冲突（如重复创建） |
| 500 | 服务器内部错误 |

---

## 任务类型说明

| 任务类型 | 说明 |
|----------|------|
| signin | 每日签到 |
| wechat | 微信任务 |
| shake | 摇一摇 |
| todaycloud | 今日云朵 |
| aicloud | AI云朵 |
| blindbox | 盲盒 |
| redpacket | 红包 |
| store | 商店 |
| garden | 花园 |
| cloudphone | 云朵手机 |
| cloudbattle | 云朵大战 |
| invitefriends | 邀请好友 |
| messagepush | 消息推送 |
| backupgift | 备份礼包 |
| exchange | 兑换 |
| tasklist | 任务列表 |

---

## 更新日志

### v1.0.0 (2024-01-01)
- 初始版本发布
- 支持16种自动化任务
- 完整的用户和账号管理
- 数据统计和可视化
