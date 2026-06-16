# API 密钥轮换运维 Runbook

## 1. 概述

本文档描述供应商准入系统 API 密钥的生命周期管理、轮换操作、故障排查与监控告警。

### 1.1 密钥状态机

```
ACTIVE ──→ ROTATING ──→ EXPIRED ──→ (purge after grace period)
  │           │
  └── REVOKED ┘
```

| 状态 | 说明 | 可用于签名验证 |
|------|------|--------------|
| `ACTIVE` | 活跃密钥，主密钥 | ✅ |
| `ROTATING` | 轮换中，旧密钥的宽限期 | ✅（到期前） |
| `EXPIRED` | 已过期 | ❌ |
| `REVOKED` | 已吊销 | ❌ |

### 1.2 宽限期分级配置

按密钥用途配置不同的默认宽限期（`expire_old_hours`）：

| 用途 | 默认宽限期 | 说明 |
|------|-----------|------|
| `production` | 72 小时 | 生产环境核心调用，预留充足切换时间 |
| `staging` | 24 小时 | 预发布环境 |
| `development` | 8 小时 | 开发环境 |
| `internal` | 48 小时 | 内部系统集成 |
| `default` | 72 小时 | 未指定用途时的默认值 |

### 1.3 请求级返回码

| 场景 | HTTP 状态码 | Response Header | 说明 |
|------|------------|-----------------|------|
| 密钥有效（单密钥） | 200 | `X-API-Key-Status: valid` | - |
| 密钥有效（轮换中） | 200 | `X-API-Key-Status: rotating` | 提示调用方有新版本密钥可用 |
| 密钥已过期 | 401 | `X-API-Key-Status: expired`<br>`X-API-Key-Expired-At: <RFC3339>` | 旧 key 超过宽限期，验证失败 |
| 密钥无效/不存在 | 401 | - | AppKey 不存在 |
| 签名不匹配 | 401 | `X-API-Key-Status: valid/rotating` | 密钥本身有效但签名错误 |

---

## 2. 常规操作

### 2.1 创建新密钥

```bash
curl -X POST /api/v1/apikeys \
  -H "Content-Type: application/json" \
  -d '{
    "app_key": "vendor-portal-prod",
    "app_name": "供应商门户生产环境",
    "description": "供应商门户后端调用",
    "usage": "production"
  }'
```

**注意**：创建后立即保存 `app_secret`，服务端不存储明文。

### 2.2 轮换密钥（推荐方式）

```bash
curl -X POST /api/v1/apikeys/rotate \
  -H "Content-Type: application/json" \
  -d '{
    "app_key": "vendor-portal-prod",
    "expire_old_hours": 72,
    "remark": "季度例行轮换"
  }'
```

响应包含新密钥的 `app_secret`。

**轮换后操作清单**：
1. ✅ 保存新的 `app_secret` 到密码管理工具
2. ✅ 在配置中心/环境变量中更新密钥
3. ✅ 逐台滚动重启应用（或热加载配置）
4. ✅ 验证新密钥可正常调用
5. ✅ 监控旧密钥调用量（应逐步下降）
6. ⏳ 等待宽限期结束后旧密钥自动失效

### 2.3 立即吊销密钥（紧急情况）

```bash
curl -X POST /api/v1/apikeys/revoke \
  -H "Content-Type: application/json" \
  -d '{
    "app_key": "compromised-key",
    "reason": "密钥泄露"
  }'
```

**注意**：吊销立即生效，无宽限期。仅用于紧急情况。

### 2.4 查看密钥状态

```bash
curl /api/v1/apikeys?vendor_id=123
```

---

## 3. 自动化清理

### 3.1 清理策略

系统后台自动执行两阶段清理：

| 阶段 | 触发条件 | 动作 |
|------|---------|------|
| Phase 1 | 旧密钥超过 `expire_old_hours` | 状态从 `ROTATING` → `EXPIRED`，停止验证 |
| Phase 2 | 超过 90 天宽限保留期 | 物理删除 `EXPIRED` 和 `REVOKED` 记录 |

### 3.2 手动触发清理

```bash
curl -X POST /api/v1/apikeys/cleanup \
  -H "Content-Type: application/json" \
  -d '{"grace_period_days": 90}'
```

响应：
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "rotated_expired": 5,
    "revoked_purged": 12
  }
}
```

### 3.3 自动清理任务配置

在服务启动时启用：

```go
apikeySvc := services.NewAPIKeyService()
go apikeySvc.StartAutoCleanup(ctx, 24*time.Hour, 90)
```

---

## 4. 监控与告警

### 4.1 关键指标

| 指标 | 告警阈值 | 说明 |
|------|---------|------|
| 轮换中密钥数量 | > 10 | 可能存在未完成的轮换 |
| 过期密钥数量 | > 50 | 清理任务可能异常 |
| 宽限期内旧密钥调用占比 | > 30% 持续 24h | 调用方可能未升级 |
| 自动清理失败次数 | > 0 | 清理任务异常 |

### 4.2 轮换监控

通过 `X-API-Key-Status: rotating` 响应头识别仍在使用旧密钥的调用方：

```bash
# Nginx 日志统计旧密钥调用量
grep 'X-API-Key-Status: rotating' access.log | wc -l
```

---

## 5. 故障排查

### 5.1 调用方报 401 "API Key 已过期"

**可能原因**：旧密钥超过了宽限期，未及时切换到新密钥。

**排查步骤**：
1. 查看响应头 `X-API-Key-Expired-At` 确认过期时间
2. 确认调用方是否已配置新密钥
3. 确认配置是否已生效（是否重启/热加载）
4. 如确实来不及切换，可临时重新创建一个宽限期更长的轮换

**紧急恢复**：
- 重新调用 Rotate 接口不会恢复已过期的旧版本
- 需要创建新密钥并更新所有调用方

### 5.2 轮换后调用失败

**可能原因**：
- 新密钥未正确保存
- 签名算法不一致
- 时间戳偏差过大

**排查步骤**：
1. 确认 `app_key` 和 `app_secret` 正确
2. 检查签名算法（HMAC-SHA256）
3. 检查时间戳（与服务器偏差不超过 5 分钟）
4. 检查 nonce 防重放

### 5.3 自动清理任务不工作

**排查步骤**：
1. 确认服务启动时调用了 `StartAutoCleanup`
2. 检查 context 是否已被取消
3. 查看应用日志中是否有 `[apikey-cleanup]` 相关错误
4. 手动调用 cleanup 接口验证

---

## 6. 最佳实践

### 6.1 轮换策略

| 密钥级别 | 轮换周期 | 宽限期 |
|---------|---------|-------|
| 生产环境核心 | 90 天 | 72 小时 |
| 生产环境非核心 | 180 天 | 72 小时 |
| 预发布/测试 | 季度 | 24 小时 |
| 开发环境 | 半年 | 8 小时 |

### 6.2 多实例部署

- 密钥数据存在数据库中，多实例共享
- 自动清理任务每个实例都会运行，但数据库操作是幂等的，无冲突
- 建议只在一个实例上启用自动清理（通过配置开关控制）

### 6.3 安全建议

1. **密钥分发**：使用专用密码管理工具（如 Vault、KMS）
2. **禁止硬编码**：密钥不允许出现在代码、配置文件中
3. **最小权限**：每个接入方单独分配密钥，不共用
4. **定期审计**：每月审查密钥列表，吊销不再使用的密钥
5. **操作留痕**：所有密钥变更操作记录审计日志

---

## 7. 附录：API 速查

| 操作 | 方法 | 路径 |
|------|------|------|
| 创建密钥 | POST | `/api/v1/apikeys` |
| 查询密钥列表 | GET | `/api/v1/apikeys` |
| 轮换密钥 | POST | `/api/v1/apikeys/rotate` |
| 吊销密钥 | POST | `/api/v1/apikeys/revoke` |
| 触发清理 | POST | `/api/v1/apikeys/cleanup` |
| 查询审批流（测试用） | GET | `/api/v1/vendors/{id}/approval/flow` |
