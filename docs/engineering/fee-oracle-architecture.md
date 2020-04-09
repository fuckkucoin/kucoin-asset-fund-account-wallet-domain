# 多链手续费抽象与动态报价会计引擎——架构、FSM 与故障语义

> 代次：v1.0 
> 适用范围：`internal/feeoracle`、`internal/feereserve`、`internal/feeeng`（chain_quote 扩展）

---

## 1. 背景与动机

R1–R3 中，出金手续费由 `feeeng.Schedule`（BPS + flat + min/max）静态决定。 
这在以下场景中失效：

- **链费率剧烈波动**（ETH gas spike、BTC mempool 拥堵）：静态 BPS 产生估价严重偏离，导致用户少付或系统亏损。
- **跨链统一报价**：BTC/ETH/TRON/SOL 费率单位完全不同，需要规范化层才能核对和审计。
- **预检绑定-广播分离**：出金预检（freeze）和广播（submit）之间可能有数分钟延迟，期间费率已变。

R4 在不改变广播保守语义（conserve FSM、live-hash auto-unfreeze 路径）的前提下，分层叠加：

```
┌────────────────────────────────────────────────────────────┐
│ 出金预检 (precheck) │
│ QuoteWithdrawFee ──► QuoteVersion (immutable snapshot) │
│ BindPrecheck ──────► StateBound │
├────────────────────────────────────────────────────────────┤
│ 广播确认 (post-broadcast, read-only) │
│ AlignActual ───────► StateSettled (不触碰 conserve FSM) │
│ FeeReserve.Settle ─► delta entry (over/under/zero) │
├────────────────────────────────────────────────────────────┤
│ 后台 (background) │
│ fee-oracle-worker ─► PushTick ──► Oracle.GetAggregated │
│ ExpireOld cron ────► StateExpired │
└────────────────────────────────────────────────────────────┘
```

---

## 2. 模块结构

### 2.1 `internal/feeoracle`

| 文件 | 职责 |
|---|---|
| `errors.go` | 所有 sentinel error 定义 |
| `model.go` | `ChainFeeModel`：per-family 原始输入 → 原生资产数量 |
| `oracle.go` | `Oracle`：多源聚合、stale 过滤、偏差熔断 |
| `version.go` | `QuoteVersion` 值对象与状态枚举 |
| `store.go` | `QuoteVersionStore`：SQL / 内存双模持久化 |
| `service.go` | `Service`：协调 model + oracle + store；fallback 策略 |
| `http.go` | HTTP 路由（ops + 采集侧推送） |

### 2.2 `internal/feereserve`

| 文件 | 职责 |
|---|---|
| `errors.go` | sentinel errors |
| `reserve.go` | `FeeReserve` 领域模型、状态机方法、DeltaDir 枚举 |
| `journal.go` | 复式凭证生成（reserve / settle_zero / over / under / release） |
| `service.go` | `Service`：reserve → settle → release 编排；SQL 持久化 |
| `http.go` | HTTP 路由（delta desk、ops） |

### 2.3 `internal/feeeng`（扩展）

`chain_quote.go` 新增 `ChainFeeQuote` / `ChainFeeParams` / `Book.QuoteChain`：

- 以 oracle 报价费用为基准，再用 Schedule min/max 夹值；
- oracle 不可用时退化为纯静态档位（`ScheduleFallback=true`，必须审计）。

### 2.4 `cmd/fee-oracle-worker`

独立进程：轮询 HTTP 源 → 规范化 → `PushTick` → 转发到 fee-service。 
不写账本，不参与广播。

---

## 3. ChainFeeModel 规范化算法

各链族的规范化公式（原始输入 → 原生资产数量）：

| 链族 | 原始输入 | 规范化公式 |
|---|---|---|
| UTXO (BTC) | fee_rate (sat/vByte)、estimated_vbytes | `fee_rate × vbytes / 1e8` |
| EVM (ETH/BSC/…) | base_fee (gwei)、priority_fee (gwei)、gas_limit | `(base+priority) × gas_limit / 1e9` |
| TRON | energy (units)、energy_price (SUN)、bandwidth (bytes)、bw_price (SUN) | `(energy×e_price + bw×bw_price) / 1e6` |
| SOL | base_lamports、compute_units、micro_lamports/CU | `(base + CU×μL/CU/1e6) / 1e9` |
| Generic | native_fee (直传) | `native_fee` |

规范化后得到 `NormalizedFee.NativeAmount`（链原生资产单位）。 
若用户计价资产与原生资产不同（如 USDT/ETH），通过 `oracle.Feed.Convert` 换算。

---

## 4. Oracle 聚合与熔断

### 4.1 数据流

```
Source A (HTTP) ──► PushTick ──┐
Source B (Kafka) ──► PushTick ──┼──► Oracle.ticks (per source)
Source C (static)──► PushTick ──┘
 │
 ▼
 GetAggregated(chain)
 │
 ┌────────┴────────────┐
 │ 1. 过滤 stale 源 │
 │ 2. 偏差检查 │
 │ 3. 权重加权平均 │
 └─────────────────────┘
 │
 ChainFeeTick
```

### 4.2 Stale 检测

每个源有独立的 `StaleDur`（默认 120 秒）。若 `now - tick.RecordedAt > StaleDur`，该源被排除。 
**全部源 stale → `ErrStale`，报价被拒，不允许静默降级到过期费率。**

### 4.3 偏差熔断（Deviation Circuit Break）

对所有存活源的 `NativeAmount`：

```
deviation_BPS = (max - min) / min × 10000
```

若 `deviation_BPS > Oracle.DeviationBPS`（默认 500，即 5%）→ `ErrDeviation`，拒绝报价。

偏差熔断与 `fx.Circuit`（半开探针）无关，是纯同步检查； 
未来可扩展为带回冷时间的熔断器，但需显式审批配置变更。

### 4.4 权重聚合

```
aggregated = Σ(source_i.NativeAmount × source_i.Weight) / Σ(source_i.Weight)
```

权重默认 1；关键源（如官方节点）可设更高权重抑制噪声源。

---

## 5. QuoteVersion FSM

```
 ┌─────────────┐
 创建 │ active │──── CanBind ────► StateBound
 ────────► │ (可绑定) │
 └─────────────┘
 │
 │ IsExpired=true
 ▼
 ┌─────────────┐
 │ expired │ ── 不可绑定；不可结算
 └─────────────┘

 StateBound ──── AlignActual ──► settled （只读，不改 conserve）
```

**状态转换规则：**

| 当前状态 | 操作 | 结果 | 错误 |
|---|---|---|---|
| active, 未过期 | Bind(wid) | bound | — |
| active, 已过期 | Bind(wid) | — | ErrVersionExpired |
| bound, 同 wid | Bind(wid) | bound（幂等） | — |
| bound, 不同 wid | Bind(wid) | — | ErrVersionConflict |
| settled | Bind(wid) | — | ErrVersionSettled |
| active/bound | AlignActual(fee) | settled | — |
| settled | AlignActual(fee) | settled（幂等） | — |

**过期（expire）由后台 cron `ExpireOld` 推进**，不阻塞热路径； 
热路径查询时 `CanBind` 实时检查 `IsExpired(now)`，不依赖 DB 状态列。

---

## 6. FeeReserve 会计 FSM

```
 ┌──────────────┐
 Reserve │ reserved │──── Settle(actual) ──► settled
 ─────────►│ (冻结预估) │
 └──────────────┘
 │
 │ Release（取消出金）
 ▼
 ┌──────────────┐
 │ released │ ── 退还冻结，不进 system.fee
 └──────────────┘
```

**Delta 方向与账务凭证：**

```
over (actual < reserved)：
 Dr system:fee_reserve Reserved
 Cr system:fee Actual ← 实际手续费
 Cr user:fee_refund Delta ← 退还用户多冻部分

under (actual > reserved)：
 Dr system:fee_reserve Reserved
 Dr user:fee_topup Delta ← 追收用户不足部分
 Cr system:fee Actual

zero (actual == reserved)：
 Dr system:fee_reserve Reserved
 Cr system:fee Reserved

release：
 Dr system:fee_reserve Reserved
 Cr user:fee_hold Reserved ← 逆向退还
```

所有凭证集合经 `money.Balanced`（总借 == 总贷）校验后才能落账。

---

## 7. 故障语义

### 7.1 Oracle 不可用

| 场景 | 行为 |
|---|---|
| 全部源 stale | `ErrStale`，报价 API 返回 503；**不允许静默使用过期费率** |
| 偏差超阈值 | `ErrDeviation`，报价 API 返回 503；ops 需介入确认源数据 |
| 无源配置 | `ErrNoSources`，报价 API 返回 503 |
| 允许 fallback（显式配置）| 使用 feeeng.Schedule 静态档位；`ScheduleFallback=true` 写入代次记录 |

fallback 路径**必须**通过 `Service.FallbackAudited=true` 显式开启，且每次生成的 `QuoteVersion.Tick.Normalized.RawDetail` 中包含 oracle 报错原因，供审计。

### 7.2 代次过期

- 过期代次不可绑定，出金管道必须重新调用 `QuoteWithdrawFee` 获取新代次。
- 过期不自动解冻 `FeeReserve`（reserve 可能已写入但未广播）；取消广播由出金 FSM 调用 `Release`。

### 7.3 广播后 AlignActual

- `AlignActual` 仅写 `StateSettled`，不修改账本冻结记录，不触发 auto-unfreeze。
- conserve FSM（live-hash conserve）由 `withdraw` 包独立管理，R4 不介入。
- 实际费用写入是**只读一致**，过大的 delta（under > 10%）可配置告警阈值，由 ops 介入。

### 7.4 幂等保证

| 操作 | 幂等键 | 幂等行为 |
|---|---|---|
| `Reserve(wid)` | withdraw_id | 重复调用返回已有 FeeReserve |
| `Settle(wid)` | withdraw_id | 已 settled 返回 ErrState（不允许用不同 actual 覆盖） |
| `Release(wid)` | withdraw_id | 已 released 幂等返回 |
| `Store.Save(ver)` | version_id | 已存在返回原代次，不覆盖 |
| `Store.Bind(ver, wid)` | (version_id, withdraw_id) | 相同组合幂等返回 |

---

## 8. SQL 表概览

| 表 | 用途 |
|---|---|
| `fee_oracle_sources` | oracle 源配置（kind/chain/family/weight/stale_sec） |
| `fee_oracle_ticks` | 单源 tick 历史（仅记录，不用于实时聚合） |
| `fee_quote_versions` | 不可变报价代次快照（含 tick_json） |
| `fee_reserves` | 出金手续费储备记录（UNIQUE withdraw_id） |
| `fee_reserve_audit` | 储备状态变更审计流水 |

所有金额列为 `DECIMAL(36,18)`，时间列为 `DATETIME(3)`（MySQL）。 
不修改 `aa_fee_schedules`、`transfer`、`swap` 任何已有列。

---

## 9. 运维接口

| 端点 | 描述 |
|---|---|
| `GET /feeoracle/health` | oracle 整体健康（各链源 stale 状态） |
| `GET /feeoracle/ticks` | 所有已知 tick 列表 |
| `GET /feeoracle/versions/:id` | 按代次 ID 回放报价快照 |
| `POST /feeoracle/sources` | 注册 oracle 源 |
| `POST /feeoracle/quote` | 手动触发报价（ops 验证） |
| `GET /feereserve/delta` | 列出待处理 delta（over/under 清单） |
| `GET /feereserve/health` | 储备服务状态统计 |
| `POST /feereserve/settle` | 手动结算（广播确认 worker 调用） |

---

## 10. 扩展点

- **多链原生资产→USD 换算**：已通过 `oracle.Feed.Convert` 接口预留，接入 `fxmark.Book` 即可。
- **Kafka 源适配**：`SourceKind = "kafka"`，`fee-oracle-worker` 可新增 Kafka consumer goroutine，复用 `PushTick`。
- **动态偏差阈值**：`Oracle.DeviationBPS` 可通过 `cfgwatch.Store` 热更新，无需重启。
- **储备 delta 自动平账**：当 `DeltaDir == over` 时，可由 worker 调用 ledger.Create 完成退款，目前保留为 ops 手动审核。
