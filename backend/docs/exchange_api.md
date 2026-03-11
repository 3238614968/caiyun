# 兑换中心 API 文档

## 概述

兑换中心是移动云盘系统的核心功能模块，允许用户使用云朵进行商品兑换。支持抢兑任务管理、自动更新商品库、并发控制等功能。

## 数据模型

### Product 商品
```json
{
  "id": 1,
  "prize_id": "string",
  "prize_name": "string",
  "p_order": 0,
  "category": "string",
  "daily_remainder_count": 0,
  "memo": "string",
  "is_active": true,
  "updated_at": "2026-02-20T10:00:00Z",
  "created_at": "2026-02-20T10:00:00Z"
}
```

### ExchangeAccount 兑换账号
```json
{
  "id": 1,
  "user_id": 1,
  "account_id": 1,
  "phone": "138****1234",
  "auth": "Basic xxxxx",
  "token": "string",
  "jwt_token": "string",
  "remark": "主账号",
  "exchange_time_1": "10:00:00",
  "exchange_time_2": "16:00:00",
  "is_active": true,
  "last_exchange_at": "2026-02-20T10:00:00Z",
  "updated_at": "2026-02-20T10:00:00Z",
  "created_at": "2026-02-20T10:00:00Z"
}
```

### ExchangeTask 抢兑任务
```json
{
  "id": 1,
  "user_id": 1,
  "exchange_account_id": 1,
  "product_id": 1,
  "prize_id": "string",
  "prize_name": "爱奇艺会员月卡",
  "task_type": "fixed",
  "max_attempts": 5,
  "attempted_count": 2,
  "status": "pending",
  "last_attempt_at": "2026-02-20T10:00:00Z",
  "last_result": "兑换成功",
  "success_count": 1,
  "fail_count": 1,
  "updated_at": "2026-02-20T10:00:00Z",
  "created_at": "2026-02-20T10:00:00Z"
}
```

## API 接口

### 1. 商品管理

#### 1.1 搜索商品
```http
GET /api/exchange/products/search
```

**请求参数:**
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| keyword | string | 否 | 搜索关键词，模糊匹配商品名称 |
| limit | int | 否 | 返回数量限制，默认 20 |

**响应示例:**
```json
{
  "products": [
    {
      "id": 1,
      "prize_id": "M20260220001",
      "prize_name": "爱奇艺黄金会员月卡",
      "p_order": 99,
      "category": "视频类会员",
      "daily_remainder_count": 100,
      "is_active": true
    }
  ],
  "total": 1
}
```

#### 1.2 获取商品分类
```http
GET /api/exchange/products/categories
```

**响应示例:**
```json
{
  "categories": [
    "其他权益奖品",
    "视频类会员",
    "音乐类会员",
    "外卖美食权益"
  ]
}
```

### 2. 兑换账号管理

#### 2.1 添加兑换账号
```http
POST /api/exchange/accounts
```

**请求体:**
```json
{
  "account_id": 1,
  "remark": "主账号",
  "exchange_time_1": "10:00:00",
  "exchange_time_2": "16:00:00"
}
```

**响应示例:**
```json
{
  "account": {
    "id": 1,
    "user_id": 1,
    "account_id": 1,
    "phone": "138****1234",
    "remark": "主账号",
    "exchange_time_1": "10:00:00",
    "exchange_time_2": "16:00:00",
    "is_active": true
  }
}
```

#### 2.2 获取兑换账号列表
```http
GET /api/exchange/accounts
```

**响应示例:**
```json
{
  "accounts": [
    {
      "id": 1,
      "remark": "主账号",
      "phone": "138****1234",
      "exchange_time_1": "10:00:00",
      "exchange_time_2": "16:00:00",
      "is_active": true,
      "tasks": []
    }
  ],
  "total": 1
}
```

#### 2.3 更新兑换账号配置
```http
PUT /api/exchange/accounts/:id
```

**请求体:**
```json
{
  "remark": "新备注",
  "exchange_time_1": "09:59:00",
  "exchange_time_2": "15:59:00",
  "is_active": true
}
```

#### 2.4 删除兑换账号
```http
DELETE /api/exchange/accounts/:id
```

### 3. 抢兑任务管理

#### 3.1 创建抢兑任务
```http
POST /api/exchange/tasks
```

**请求体:**
```json
{
  "exchange_account_id": 1,
  "product_id": 1,
  "task_type": "fixed",
  "max_attempts": 5
}
```

**参数说明:**
- `task_type`: `fixed` - 固定次数，`long_term` - 长期抢兑
- `max_attempts`: 最大抢兑次数 (仅 fixed 模式有效)

**响应示例:**
```json
{
  "task": {
    "id": 1,
    "user_id": 1,
    "exchange_account_id": 1,
    "product_id": 1,
    "prize_id": "M20260220001",
    "prize_name": "爱奇艺黄金会员月卡",
    "task_type": "fixed",
    "max_attempts": 5,
    "status": "pending"
  }
}
```

#### 3.2 获取抢兑任务列表
```http
GET /api/exchange/tasks
```

**响应示例:**
```json
{
  "tasks": [
    {
      "id": 1,
      "prize_name": "爱奇艺黄金会员月卡",
      "exchange_account": {
        "id": 1,
        "remark": "主账号"
      },
      "task_type": "fixed",
      "max_attempts": 5,
      "attempted_count": 2,
      "status": "pending",
      "success_count": 1,
      "fail_count": 1
    }
  ],
  "total": 1
}
```

#### 3.3 更新抢兑任务
```http
PUT /api/exchange/tasks/:id
```

**请求体:**
```json
{
  "max_attempts": 10
}
```

#### 3.4 删除抢兑任务
```http
DELETE /api/exchange/tasks/:id
```

#### 3.5 立即执行抢兑任务
```http
POST /api/exchange/tasks/:id/execute
```

**说明:** 异步执行抢兑任务，不等待结果返回。执行结果通过 WebSocket 推送。

## WebSocket 通知

### 抢兑完成通知
```json
{
  "type": "exchange_complete",
  "data": {
    "task_id": 1,
    "account_name": "主账号",
    "product_name": "爱奇艺黄金会员月卡",
    "success": true,
    "message": "兑换成功"
  }
}
```

## 系统配置

### 管理员可配置项

1. **抢兑并发数** (`exchange_concurrency`)
   - 默认值：10
   - 说明：同时执行的抢兑任务数量

2. **第一次抢兑时间** (`exchange_schedule_time_1`)
   - 默认值：10:00
   - 说明：每天第一次自动抢兑时间

3. **第二次抢兑时间** (`exchange_schedule_time_2`)
   - 默认值：16:00
   - 说明：每天第二次自动抢兑时间

4. **商品库自动更新** (`exchange_auto_update_products`)
   - 默认值：true
   - 说明：是否每天自动更新商品库

5. **商品库更新时间** (`exchange_update_time`)
   - 默认值：08:00
   - 说明：每天自动更新商品库的时间

## 抢兑流程

1. **用户添加兑换账号**
   - 从云盘账号中选择一个添加到兑换账号
   - 设置抢兑时间 (默认 10:00 和 16:00)

2. **创建抢兑任务**
   - 搜索商品并选择
   - 选择用于抢兑的账号
   - 设置任务类型 (固定次数或长期抢兑)

3. **自动抢兑**
   - 到达抢兑时间前 3 秒初始化抢兑列表
   - 时间到达后并发执行抢兑 (默认 10 线程)
   - 抢兑成功后自动切换其他账号继续请求
   - 出现"奖品单日已耗尽"时自动结束该任务

4. **结果通知**
   - 抢兑结果通过 WebSocket 实时推送
   - 记录抢兑历史到数据库

## 错误码

| 错误码 | 说明 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未授权 |
| 403 | 无权操作 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 注意事项

1. 每个云盘账号只能添加一次为兑换账号
2. 同一账号同一商品不能创建重复的抢兑任务
3. 抢兑任务执行期间可以查看、修改和删除
4. 长期抢兑任务会持续执行直到手动删除或奖品耗尽
5. 建议提前 3-5 分钟设置抢兑时间，避开高峰期
