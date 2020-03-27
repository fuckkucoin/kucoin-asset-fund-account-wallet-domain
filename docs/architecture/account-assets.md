# 账户与资产

本文给出账户与资产模块划分、科目规则与资金主路径。实现位于 `internal/account`、`internal/ledger`、`internal/deposit`、`internal/withdraw` 及同级产品包。单据状态见 [资金单据状态机](fund-state.md)，科目与账本落点见 [科目与账本落点](book-subject.md)。落库与 `Open` / `OpenMem` 边界见 [持久化](persistence.md)。

## 模块图

查询网关聚合只读面。写路径按科目、账本、充提、地址、托管、筛查拆分。子账户、手续费入账、法币凭证、尘兑换、内部转账闸挂在账户或充提进程。

**账户与资产模块**

```mermaid
flowchart TB
 Q[query-gateway.kucoin.net]

 subgraph books [科目]
 ACC[account 科目生命周期]
 SUB[subacct 母户/子户]
 HOP[合法 hop]
 end

 subgraph led [账本]
 L[ledger 分录与状态机]
 X[xferstore 科目划转意图]
 R[recon 对账]
 F[fence / outbox]
 end

 subgraph rails [进出金]
 DEP[deposit 观察入账]
 WD[withdraw 冻账出金]
 ATTR[入金归因]
 POOL[addrpool 地址池]
 ADDR[addresssvc]
 CLAIM[depclaim 认领]
 WDP[wdpath 出金路径]
 end

 subgraph risk [筛查与限额]
 KYT[kyt / kytsvc]
 WG[wdguard 预检]
 AL[destallow 白名单]
 TR[travelvasp 旅行规则信封]
 LIM[limitgate / usage]
 end

 subgraph custody [托管边界]
 CUS[custody 双人审批]
 HOT[hotinv / hotbound]
 TRE[treasury 金库带宽]
 end

 subgraph product [账户资金产品]
 FEE[feecredit 手续费入资金账]
 FIAT[fiatjnl 法币凭证]
 DUST[dustpath 尘兑换]
 P2P[p2pgate 内部转账闸]
 BILL[bookbill 多科目账单]
 FP[fundpath 路径编排]
 end

 Q --> ACC
 Q --> L
 Q --> BILL
 ACC --> HOP --> L
 SUB --> L
 DEP --> ATTR
 DEP --> L
 WD --> WG --> KYT
 WD --> L
 WD --> CUS
 ADDR --> POOL
 CLAIM --> ATTR
 WDP --> AL
 WDP --> TR
 WDP --> WG
 FP --> POOL
 FP --> DEP
 FP --> HOP
 FP --> WDP
 FP --> HOT
 CUS --> HOT
 HOT --> TRE
 FEE --> L
 FIAT --> L
 P2P --> L
 L --> R
 L --> F
 LIM --> L
```

## 科目

| 科目 | 账本落点 | 说明 |
| --- | --- | --- |
| funding | spot | 默认已开；充提与内部 hop 的枢纽 |
| trading | trading | 只记账，不跑撮合 |
| margin | margin | 只记账，不跑强平 |
| futures | futures | 只记账，不跑合约引擎 |
| earn | earn | 申购/赎回 hop |

合法 hop 仅资金账与其余科目互转，科目之间不得直跳：

| 方向 | 账本 Direction |
| --- | --- |
| funding → trading | `funding_to_trading` |
| trading → funding | `trading_to_funding` |
| funding → margin | `funding_to_margin` |
| margin → funding | `margin_to_funding` |
| funding → futures | `funding_to_futures` |
| futures → funding | `futures_to_funding` |
| funding → earn | `earn_subscribe` |
| earn → funding | `earn_redeem` |

科目状态：`active` / `open` 可借贷；`frozen` 停进出；`closed` 要求可用与冻结均为零。funding 不得关闭。

子账户：母户冻则子户级联冻；解母户不解子户。子户之间 hop 拒绝，须经母户。子户出金/入金/划转受权限表约束。

## 账本方向（账户资金）

| Direction | 冻结 | 链 | 入账 |
| --- | --- | --- | --- |
| `wallet_deposit` | 无 | 观察并确认 | 用户 funding/wallet，金额来自观察 |
| `wallet_withdraw` | 源账可用 → 冻结 | 签名后广播 | 消耗冻结；现货科目不变 |
| `spot_to_wallet` / `wallet_to_spot` | 源账 | 无 | 同事务入对方账 |
| `spot_send` | 发送方 funding | 无 | 对手 funding |
| `funding_to_*` / `*_to_funding` | 源科目 | 无 | 目标科目，经桥 |
| `fiat_onramp` / `fiat_offramp` | 出金冻资金账 | 无 | 桥与资金账互转 |
| `hot_cold_sweep` | 无 | 无 | `system.bridge` → `system.cold` |

系统科目：`system.bridge`（热/桥）、`system.cold`（冷）、`system.fee`（手续费）。用户资金账不计入托管库存观察。

## 资金主路径

### 充值入账主路径

链上 `Inspect` 在事务外执行。入账额为观察量。`(chain, tx_hash)` 唯一；重复哈希冲突。确认深度按链策略。重组对已入账分录做反向流水，状态 `reorged`，不得二次 credit。尘阈值以下不得单独开入账。

**充值入账主路径**

```mermaid
sequenceDiagram
 participant C as 调用方
 participant DW as deposit-withdraw
 participant CH as 链观察
 participant L as ledger
 participant B as 用户资金账

 C->>DW: 观察请求（用户、链、哈希、资产、幂等键）
 DW->>CH: Inspect（事务外）
 CH-->>DW: 观察金额与确认数
 DW->>DW: 归因 / 尘闸 / 确认深度
 DW->>L: Create wallet_deposit（观察额）
 L->>B: credit 可用
 L-->>DW: 已入账
 Note over L: 重组则反向分录，禁止二次 credit
```

### 提现广播主路径

先冻后筛。白名单、策略包、风险窗、KYT 任一步失败即关，不得 `Submit`。复审开案并保持冻结；过期复审不等于放行。签名按次解包。RPC 在事务外；先落哈希再标 `chain_submitted`。确认只消耗冻结。

**提现冻账至广播**

```mermaid
sequenceDiagram
 participant C as 调用方
 participant DW as deposit-withdraw
 participant L as ledger
 participant G as wdguard / KYT
 participant K as kms-vault
 participant CH as 链广播

 C->>DW: 出金（用户、链、目标、资产、金额、幂等键）
 DW->>L: 冻源账可用
 DW->>G: 白名单 → 策略 → 分值 → KYT
 alt 拒绝或筛子出错
 G-->>DW: 失败即关
 Note over L: 冻结保留或回滚，不得 Submit
 else 放行
 DW->>K: 按次解包签名
 DW->>CH: Submit（事务外）
 DW->>L: 落哈希 / chain_submitted
 CH-->>DW: 确认
 L->>L: consume_frozen
 end
```

### 科目内部 hop

同一用户锁。冻源科目可用，经桥贷目标科目。`xferstore` 对同一幂等键锁定科目对与金额。源或目标冻结则拒绝。

**科目内部 hop**

```mermaid
flowchart LR
 A[源科目可用] -->|freeze| F[源科目冻结]
 F -->|bridge hop| BR[system.bridge]
 BR -->|credit| B[目标科目可用]
 A -.->|Conserve| B
```

### 冻结与解冻

科目级冻结停该科目借贷。用户级风控冻结阻断新开冻。可用→冻结、冻结→可用均为成对分录。关闭科目前余额必须为零。

**科目冻结**

```mermaid
stateDiagram-v2
 [*] --> closed
 closed --> active: open
 active --> frozen: freeze
 frozen --> active: unfreeze
 active --> closed: 余额为零后 close
 frozen --> frozen: 借贷拒绝
```

## 路径编排

`internal/fundpath` 顺序串联：地址分配 → 归因绑定 → 观察入账 → 科目 hop → 出金白名单 / 旅行规则信封 / KYT / 预检 / 热账库存。任一步失败即关。已完成的入金不因后续出金闸失败而回滚。充提包不得反向依赖该编排包，以避免导入环。

## 资金安全闸

| 闸 | 行为 |
| --- | --- |
| `money.Parse` | 空串、科学计数、非正、超过 18 位小数拒绝 |
| `money.Balanced` | 按资产借贷合计相等 |
| `money.Conserve` | hop / 切费 / 入账两侧合计相等 |
| `persist.Fence` | epoch 只增；token 不匹配则 `ErrStaleFence` |
| 对账漂移 | 脏报告阻断新开划转 |
| 热账 cap | 超限或库存不可读则出金失败即关 |
| 停写 | halt / 无主 / 脑裂拒绝新写 |

手续费报价满足 fee + net = gross。返佣贷记资金账，受日限额约束。法币凭证在 KYC 未通过时拒绝。尘兑换按牌价组源资产借与 USDT 贷两套轧平分录。

旅行规则信封为工程校验（规范串与校验和），不构成持牌申报。KYT 为广播前筛查口，不构成持牌核验认证。
