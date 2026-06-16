# Storage 在线迁移灰度方案（Dual-Write）

## 1. 概述

本文档描述供应商准入系统存储后端从本地文件系统迁移到对象存储（S3/OSS）的在线灰度方案。整个迁移过程不中断业务，采用 **Dual-Write 双写 + 渐进流量切换 + 数据回补** 的策略。

### 1.1 迁移阶段总览

```
Phase 0: 准备期
    ↓
Phase 1: Shadow Write（只写不读，数据预热）
    ↓
Phase 2: Dual Write + Shadow Read（双写，主读 primary，异步校验 secondary）
    ↓
Phase 3: Dual Write + 灰度读（按比例/按租户切流量到 secondary）
    ↓
Phase 4: 100% Secondary Read + Dual Write（全量读 secondary，双写作为回滚缓冲）
    ↓
Phase 5: Secondary Only（下线 primary，迁移完成）
    ↓
Phase 6: 回滚（如有需要）
```

---

## 2. 核心组件

### 2.1 DualWriteStorage

`DualWriteStorage` 是迁移的核心组件，实现了 `Storage` 接口，内部包装两个 Storage 实例：

- **Primary**：当前主存储，所有读取默认走 primary
- **Secondary**：目标存储，迁移过程中逐步接管流量

### 2.2 工作模式

| 模式 | 写 Primary | 写 Secondary | 读来源 | 说明 |
|------|-----------|-------------|--------|------|
| `Primary` | ✅ | ❌ | Primary | 正常模式，等同于单存储 |
| `ShadowOnly` | ❌ | ✅ | Primary | 影子模式，只写 secondary，不影响主流程 |
| `Both` | ✅ | ✅ | Primary | 双写模式，数据同时写入两端 |

### 2.3 Shadow Check 影子校验

开启 `shadowCheck = true` 后，每次读操作会异步读取 secondary 进行一致性校验：
- 统计 Shadow 读取成功率
- 统计文件大小不一致的情况
- 指标上报到监控系统

---

## 3. 分阶段执行方案

### Phase 0: 准备期

**目标**：环境准备、代码部署、配置验证

**操作清单**：
1. ✅ 部署新代码（包含 `DualWriteStorage` 支持）
2. ✅ 配置 secondary 存储的 access key / secret
3. ✅ 创建目标 bucket / 目录
4. ✅ 验证 secondary 连通性（上传一个测试文件）
5. ✅ 配置监控大盘（写入成功率、读取成功率、数据一致性）
6. ✅ 准备回滚预案和回滚配置

**产出**：
- 双端存储配置就绪
- 监控大盘就绪
- 回滚配置文件准备好

---

### Phase 1: Shadow Write（影子写入）

**目标**：验证 secondary 写入链路，不影响业务

**配置**：
```go
storage := storage.NewDualWriteStorage(primary, secondary, storage.DualWriteShadowOnly)
storage.SetSecondaryFailHandler(func(key string, err error) {
    // 只记录日志/监控，不报错
    metrics.Increment("storage.secondary.write.fail")
})
```

**持续时间**：1-2 天

**验证指标**：
- Secondary 写入成功率 > 99.9%
- Secondary 写入延迟 P95 < 500ms
- 无业务报错（secondary 失败不影响主流程）

**监控告警**：
- Secondary 写入成功率 < 99% → Warning
- Secondary 写入成功率 < 95% → Critical（考虑暂停迁移）

**准入下阶段标准**：
- ✅ 连续 24 小时 secondary 写入成功率 > 99.9%
- ✅ 无因 secondary 写入导致的业务异常
- ✅ 文件大小、数量符合预期

---

### Phase 2: Dual Write + Shadow Read

**目标**：双写保证数据一致性，开启影子读校验

**配置**：
```go
storage := storage.NewDualWriteStorage(primary, secondary, storage.DualWriteBoth)
storage.SetShadowCheck(true)
storage.SetSecondaryFailHandler(func(key string, err error) {
    log.Warnf("secondary write failed: key=%s err=%v", key, err)
    // 失败告警但不阻塞
})
```

**双写语义保证**：
- Primary 写入失败 → 整体失败，不写 secondary
- Primary 成功，secondary 失败 → 整体成功，记录失败日志
- 双写都成功 → 整体成功

**持续时间**：3-5 天

**验证指标**：
- Primary 写入成功率 = 100%（无业务影响）
- Secondary 写入成功率 > 99.9%
- Shadow Read 一致性 > 99.9%（文件大小一致）
- 数据完整度：secondary 数据量与 primary 差值收敛

**影子校验逻辑**：
- 每次从 primary 读取后，异步读取 secondary 同一份数据
- 比较文件大小是否一致
- 统计 mismatch 数量和比例

**准入下阶段标准**：
- ✅ 双写运行稳定 3 天以上
- ✅ Shadow Read 一致性 > 99.9%
- ✅ 积压数据差异 < 0.1%

---

### Phase 3: 灰度读切换

**目标**：按比例将读流量切到 secondary，验证读取链路

**灰度策略（按百分比）**：

| 阶段 | 读流量比例 | 持续时间 | 观察重点 |
|------|-----------|---------|---------|
| 3.1 | 1% | 2 小时 | 错误率、延迟 |
| 3.2 | 5% | 4 小时 | 整体稳定性 |
| 3.3 | 10% | 半天 | 业务指标 |
| 3.4 | 25% | 1 天 | 全链路压测 |
| 3.5 | 50% | 1 天 | 系统负载 |
| 3.6 | 100% | 观察期 | 最终验证 |

**灰度实现方式**：
- 按 key 哈希取模：`hash(key) % 100 < threshold`
- 按供应商 ID 灰度：特定 vendor 走 secondary
- 按请求 ID 灰度：随机比例

**回滚触发条件**：
- 读取错误率 > 0.1%
- 读取延迟 P95 上涨 > 50%
- 业务成功率下降 > 0.5%
- 任何数据一致性问题

**准入下阶段标准**：
- ✅ 100% 流量在 secondary 上稳定运行 24 小时
- ✅ 所有业务指标无衰退
- ✅ 错误率与 primary 持平或更低

---

### Phase 4: 100% Secondary + Dual Write 保留期

**目标**：全量使用 secondary，保留双写作为回滚缓冲区

**配置**：
- 读：100% secondary
- 写：仍然双写（primary + secondary）

**持续时间**：7 天（一周观察期）

**目的**：
- 全量验证 secondary 的稳定性和性能
- 保留双写作为快速回滚的安全垫
- 如果出现问题，可以立刻切回 primary 读

**准入下阶段标准**：
- ✅ 连续 7 天稳定运行
- ✅ 无数据丢失或一致性问题
- ✅ 性能、成本符合预期

---

### Phase 5: Secondary Only（迁移完成）

**目标**：下线 primary，迁移完成

**配置**：
```go
storage := secondary // 直接使用 secondary，不再用 DualWrite
```

**操作步骤**：
1. 确认 primary 上已没有读写流量
2. 备份 primary 数据（归档到冷存储）
3. 从配置中移除 primary 相关配置
4. 关闭 DualWrite 相关监控告警

**后续观察**：
- 再观察 7 天
- 确认无误后可考虑删除 primary 历史数据

---

## 4. 数据回补与迁移

### 4.1 存量数据迁移

在 Phase 2 双写开启后，使用 `storage.Migrate` 函数批量迁移历史数据：

```go
result, err := storage.Migrate(ctx, primary, secondary, "qualifications/", 100)
// result.Migrated: 已迁移数量
// result.Failed: 失败数量
// result.Elapsed: 耗时
```

### 4.2 迁移速度控制

- 低峰期：加快迁移速度（并发数调大）
- 高峰期：降低迁移速度（减小并发数）
- 支持暂停/恢复

### 4.3 最终一致性校验

迁移完成后，执行全量数据校验：
- 两端文件数量对比
- 抽样文件内容 MD5 校验
- 大文件完整性校验

---

## 5. 监控与可观测性

### 5.1 核心指标

| 指标 | 说明 | 告警阈值 |
|------|------|---------|
| `storage.primary.write.success_rate` | Primary 写入成功率 | < 99.99% |
| `storage.secondary.write.success_rate` | Secondary 写入成功率 | < 99% |
| `storage.secondary.write.fail_total` | Secondary 写入失败数 | > 10/min |
| `storage.read.from_secondary_ratio` | 读 secondary 比例 | - |
| `storage.shadow.mismatch_ratio` | 影子校验不一致率 | > 0.1% |
| `storage.write.latency.p95` | 写入延迟 P95 | > 1s |
| `storage.read.latency.p95` | 读取延迟 P95 | > 500ms |
| `storage.migrate.progress` | 迁移进度百分比 | - |

### 5.2 日志要点

- 所有 secondary 写入失败都要记录 key 和错误原因
- 影子校验不一致记录 key 和两边大小
- 灰度切换时记录切换比例和命中情况

---

## 6. 回滚预案

### 6.1 快速回滚（分钟级）

如果迁移中出现问题，立刻回滚读流量：

1. 修改配置，将读流量 100% 切回 primary
2. 配置热加载或重启服务
3. 双写仍然保留，数据不丢失

### 6.2 数据回滚（如 secondary 数据损坏）

1. 停止 secondary 写入
2. 确认 primary 数据完整
3. 切回 primary-only 模式
4. 分析 secondary 数据损坏原因
5. 修复后重新开始迁移

### 6.3 回滚验证

回滚后验证：
- 业务成功率恢复正常
- 读写延迟恢复正常
- 无新增数据一致性问题

---

## 7. 风险与应对

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| Secondary 写入性能不足 | 中 | 高 | 提前压测；降级为异步写入 |
| 数据不一致 | 低 | 极高 | 影子校验提前发现；双写保证新数据一致 |
| 迁移期间成本上升 | 高 | 低 | 控制迁移速度；用满额度再迁移 |
| 回滚时数据丢失 | 低 | 极高 | 始终保留双写直到稳定运行 7 天以上 |
| 大文件迁移超时 | 中 | 中 | 分片上传；断点续传；超时重试 |

---

## 8. 操作 checklist

### 迁移前
- [ ] 代码部署到位，DualWrite 功能可用
- [ ] Secondary 存储账号配置完毕
- [ ] 监控大盘搭建完成
- [ ] 告警规则配置完成
- [ ] 回滚配置文件准备好
- [ ] 业务方通知到位
- [ ] 迁移时间窗口确认（避开业务高峰）

### 迁移中
- [ ] Phase 1 影子写入验证
- [ ] Phase 2 开启双写 + 影子读
- [ ] 存量数据迁移执行
- [ ] 数据一致性校验
- [ ] Phase 3 灰度读逐步放大
- [ ] 各阶段观察期满足
- [ ] 业务方确认无异常

### 迁移后
- [ ] Phase 4 全量读 + 双写保留 7 天
- [ ] Phase 5 切换为 secondary-only
- [ ] 旧数据归档备份
- [ ] 迁移总结报告
- [ ] 相关文档更新

---

## 9. 相关代码入口

- 核心实现：`internal/storage/storage.go` 中的 `DualWriteStorage`
- 批量迁移：`storage.Migrate()` 函数
- 配置初始化：`cmd/server` 中的存储初始化逻辑
- 配置参数：通过环境变量或配置文件切换模式
