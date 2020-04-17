# 错链/未认领充值回收与迟贷记竞态编排

## 概述

`internal/reclaim` 包**错链充值回收**（wrong-chain deposit reclaim）与**迟贷记竞态编排**（late-credit race orchestration）引擎。当用户在错误链上充值，或链上存在无法归因用户的充值输出时，运营人员通过本模块开立回收案件，锁定链上输出，并完成退款/归集操作。

本模块独立于 wrong-credit 冲销、 reorg 回滚、 预贷记归因，不与任何其他回收路径共享 FSM 或互斥锁。

---

## 模块结构

```
internal/reclaim/
├── doc.go 功能/边界/不变量声明
├── errors.go 领域错误定义
├── types.go FSM 状态/事件/SLA 常量和所有领域类型
├── fsm.go FSM 边表定义与 SLA 检查
├── occupancy.go ReclaimOccupancy：链上输出预占管理
├── mutex.go LateCreditMutex：充值路径与占用路径互斥
├── sweep.go SweepCatalog：回收后 Refund/Sweep leg 目录
├── service.go Service 门面：编排 FSM 推进与各子系统
├── persist.go Store：围绕 persist.KV 的持久化层
├── http.go HTTP API（Desk/ops 端点）
├── helpers.go 辅助函数（UUID、nonEmpty 等）
└── phase.go Prometheus 指标记录辅助

internal/metrics/
└── reclaim.go Prometheus 指标定义

migrations/
└── 0070_aa_reclaim.sql DDL（案件/占用/时间轴/sweep leg/竞态事件表）

testdata/reclaim/
├── wrong-chain-happy.json
├── unclaimed-cancel.json
├── sla-open-escalate.json
├── late-credit-race.json
├── auto-credit-terminates.json
└── double-claim-rejected.json

test/flow/
└── reclaim_wrongchain_test.go 表驱动（12 个场景）
```

---

## FSM 状态机

### 状态定义

| 状态 | 含义 | 是否终态 |
|------|------|----------|
| `open` | 案件已开立，等待运营预占输出 | 否 |
| `reserved` | 输出已预占，等待链上退款/归集 | 否 |
| `escalated` | 已升级至高级运营/核验部门 | 否 |
| `reclaimed` | 回收成功，资金已退回或归入核验账户 | **是** |
| `cancelled` | 回收取消（用户自行归因成功等） | **是** |
| `auto_credited` | 同链正常充值路径已完成贷记，案件自动关闭 | **是** |

### FSM 转移图

```
 ┌─────────────────────────────────────────────┐
 │ │
 open ──[reserve]──► reserved ──[reclaim]──► reclaimed │
 │ │ │
 │ ├──[auto_credit]──► auto_credited │
 │ │ │
 │ ├──[sla_reserve_breach]──► │
 │ │ │
 │ └──[cancel]──► cancelled │
 │ │
 ├──[cancel]──► cancelled │
 │ │
 └──[sla_open_breach / escalate]──► escalated │
 │ │
 ├──[reclaim]──► reclaimed │
 └──[cancel]──► cancelled │
 │
 (*) 任意非终态 ──[escalate]──► escalated ──────────────────────┘
```

### 事件定义

| 事件 | 触发方 | 说明 |
|------|--------|------|
| `reserve` | 运营 | 预占链上输出，阻止双重认领 |
| `reclaim` | 运营 | 确认回收操作完成 |
| `cancel` | 运营/系统 | 取消回收 |
| `escalate` | 运营 | 人工升级到高级处理 |
| `auto_credit` | 系统 | 同链正常充值路径已贷记成功 |
| `sla_open_breach` | 系统（SLA worker） | open 超过 72h 未预占 |
| `sla_reserve_breach` | 系统（SLA worker） | reserved 超过 168h 未回收 |

---

## SLA 约定

| 状态 | 超时时间 | 触发事件 | 结果 |
|------|----------|----------|------|
| `open` | 72 小时 | `sla_open_breach` | → `escalated` |
| `reserved` | 168 小时（7 天） | `sla_reserve_breach` | → `escalated` |

SLA 检查由 `Service.TickSLA` 驱动，建议每分钟调用一次。`escalated` 案件不再自动超时，由高级运营人工处理。

---

## ReclaimOccupancy：双重认领防护

`ReclaimOccupancy` 以 `TxHash:Vout`（`OutputKey`）为粒度管理链上输出的预占状态。

**核心保证：**
- 同一 `OutputKey` 至多被一个活跃 ReclaimCase 预占（`Occupy` 返回 `ErrOccupancyLocked`）。
- 相同 caseID 的重复 `Occupy` 调用幂等。
- 案件终态后（`reclaimed`/`cancelled`/`auto_credited`）自动调用 `Release`。

**进程重启恢复：**
- 占用记录持久化到 `reclaim_occupancy` 表。
- `Service.Hydrate` 在启动时从 Store 重建内存占用表。

---

## LateCreditMutex：迟贷记竞态控制

当充值 `Observe/Credit` 路径与 `ReclaimOccupancy` 发生竞态时：

```
充值路径 → CheckCredit(outputKey) → 已占用 → ErrLateCreditBlocked
 → 生成 LateCreditRaceEvent
 → 运营消费 /reclaim/race-events
```

**竞态场景处理：**

| 场景 | 先到者 | 后到者 | 结果 |
|------|--------|--------|------|
| 占用先 | Reserve | Credit | Credit 被阻塞，生成竞态事件 |
| 贷记先 | Credit | Reserve | Reserve 成功，但后续 TryAutoCredit 检测到已贷记，触发 `auto_credited` |
| 同时到达 | mu 串行化 | — | 先到者获胜 |

**竞态事件队列：**
- 内存有界队列（上限 256 条）。
- 溢出时递增 `RaceDropCount` 计数器，不阻塞。
- 通过 `GET /reclaim/race-events` 端点消费（drain 方式）。

---

## Sweep Leg 目录

案件达到 `reclaimed` 终态后，运营通过 `POST /reclaim/sweep-legs` 创建 Sweep leg 目录，记录退款/归集操作的完整证明链。

**Leg 类型：**

| 类型 | 含义 |
|------|------|
| `occupancy_release` | 占用释放证明 |
| `refund_broadcast` | 退款交易已广播 |
| `refund_confirmed` | 退款交易已确认 |
| `sweep_to_hot` | 归集至热钱包（平台回收场景） |
| `proof_anchor` | SHA-256 证明链锚定 |

---

## HTTP API 端点

路由前缀：`/v1/reclaim/`

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/open` | `aml.open` | 开立回收案件 |
| POST | `/reserve` | `aml.open` | 预占链上输出 |
| POST | `/reclaim` | `aml.open` | 完成回收 |
| POST | `/cancel` | `aml.open` | 取消回收 |
| POST | `/escalate` | `aml.open` | 人工升级 |
| POST | `/auto-credit` | `ops.admin` | 同链自动贷记通知 |
| POST | `/sweep-legs` | `aml.open` | 创建 Sweep leg 目录 |
| POST | `/sla-tick` | `ops.admin` | 手动触发 SLA 检查 |
| GET | `/case` | `case.read` | 获取单个案件 |
| GET | `/cases` | `case.read` | 列举案件（带过滤） |
| GET | `/timeline` | `case.read` | 获取案件时间轴 |
| GET | `/sweep-legs` | `case.read` | 获取 Sweep leg 目录 |
| GET | `/occupancy` | `ops.admin` | 查看占用快照 |
| GET | `/race-events` | `ops.admin` | 消费竞态事件 |

---

## Prometheus 指标

| 指标名 | 类型 | 说明 | 告警建议 |
|--------|------|------|----------|
| `aa_reclaim_cases_opened_total` | Counter | 已开立案件数（kind/chain/asset） | — |
| `aa_reclaim_state_transitions_total` | Counter | 状态变更计数（from/to/chain/asset） | `to=escalated` 触发 P2 |
| `aa_reclaim_sla_breaches_total` | Counter | SLA 超时触发升级数 | `sla_reserve_breach` 触发 P1 |
| `aa_reclaim_occupancy_held` | Gauge | 当前活跃预占数 | > 1000 触发 P2 |
| `aa_reclaim_late_credit_blocked_total` | Counter | 迟贷记路径阻塞计数 | > 0 需人工核查 |
| `aa_reclaim_race_event_dropped_total` | Counter | 竞态事件队列溢出丢弃数 | > 0 需调查消费方 |
| `aa_reclaim_sweep_legs_created_total` | Counter | Sweep leg 目录创建数 | — |
| `aa_reclaim_active_cases` | Gauge | 当前活跃（非终态）案件数 | — |

---

## 数据库表

迁移文件：`migrations/0070_aa_reclaim.sql`

| 表名 | 用途 |
|------|------|
| `reclaim_case` | 案件主记录（FSM 状态/链上输出/SLA 等） |
| `reclaim_occupancy` | 链上输出预占记录（重启后恢复内存用） |
| `reclaim_timeline` | 案件时间轴（只追加） |
| `reclaim_sweep_leg` | 回收后退款/归集腿目录 |
| `reclaim_race_event` | 迟贷记竞态事件（运营消费） |

---

## 隔离边界

本模块严格隔离，不得与以下路径交叉：

- ` wrongcredit`：错贷记冲销路径，针对已 credited 的错误贷记，FSM 完全独立。
- ` reorg`：链上重组回滚批次，互斥通过各自独立锁（不共享 `CreditMutex`）。
- ` depclaim`：充值预贷记归因系统，reclaim 不 clone 也不共享其 FSM。
- ` conserve`：入金保守公式，不修改。
- `dust runtime`：尘额运行时，不干预。

---

## 作者信息

作者: Kinsley
日期: 2020.04.07
