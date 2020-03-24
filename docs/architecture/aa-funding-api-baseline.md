# Account & Funding API 基线

本文件列出 Account & Funding REST（Classic + UTA）路径与本仓库后端落点。契约原文见 API 目录；内部主机与区域地址不变。

## REST 路径基线

来源：

- https://docs.assets.kucoin.local
- https://docs.assets.kucoin.local/rest/account-info/account-funding/get-account-list-spot
- https://docs.assets.kucoin.local/rest/account-info/deposit/add-deposit-address-v3
- https://docs.assets.kucoin.local/rest/account-info/withdrawals/withdraw-v3
- https://docs.assets.kucoin.local/rest/account-info/withdrawals/get-withdrawal-quotas
- https://docs.assets.kucoin.local/v2/rest/ua/get-account-currency-assets-uta
- https://docs.assets.kucoin.local/v2/rest/ua/classic-account-balance
- https://docs.assets.kucoin.local/rest/account-info/account-funding/get-account-summary-info
- https://docs.assets.kucoin.local/rest/account-info/account-funding/get-apikey-info
- https://docs.assets.kucoin.local/rest/account-info/account-funding/get-account-type-spot
- https://docs.assets.kucoin.local/rest/account-info/account-funding/get-account-ledgers-futures
- https://docs.assets.kucoin.local/rest/account/transfer/flextransfer
- https://docs.assets.kucoin.local/rest/ua/get-leverage
- https://docs.assets.kucoin.local/rest/ua/modify-leverage-uta
- https://docs.assets.kucoin.local/rest/ua/modify-sub-account-api
- https://docs.assets.kucoin.local/introduction-uta

| 能力 | REST 路径 | AA 落点 |
| --- | --- | --- |
| 现货账户列表 | `GET /api/v1/accounts` | `internal/kcfund` → `ledger.Accounts` |
| 现货账户详情 | `GET /api/v1/accounts/{accountId}` | 同上 |
| 现货账户类型（HF 开户标记） | `GET /api/v1/hf/accounts/opened` | 本仓库无独立 HF 账本，返回 `false`（走 `trade`） |
| 现货 HF 账户类型 | `GET /api/v1/hf/accounts/type` | 固定 `trade` + `hfOpened:false` |
| 账户摘要 | `GET /api/v1/account-summary` | 科目聚合 + `fundpref` 计价币 |
| 币种目录 | `GET /api/v1|v2|v3/currencies` | `chains` + `fundsurf.Networks` |
| 出金手续费读 | `GET /api/v1/withdrawals/fee` | `Risk` / `Transfer` 配置 |
| 账户配置/偏好 | `GET /api/v1/account-config`、`GET|POST /api/v1/account/preference` | `fundpref` |
| V2 账户信息 | `GET /api/v2/account/info` | 子户数量 |
| 流水导出 | `POST /api/v1/accounts/ledgers/export` | `ledger_export` → statement-export / `exjob` |
| 网关 mesh 读科目 | `GET /api/v1/accounts` | `fundmesh.LedgerAccounts` → 本进程 ledger |
| 网关 mesh 充提写 | 链上出金/取消 | 风控闸后 `fundmesh.DepositWithdraw` → `withdraw` |
| API Key 信息 | `GET /api/v1/user/api-key`、`GET /api/ua/v1/user/api-key` | `subkey.List` 首条；不回显 secret |
| 流水 Spot/Margin | `GET /api/v1/accounts/ledgers` | `ledger.ListBills` |
| 流水 Trade HF | `GET /api/v1/hf/accounts/ledgers` | 同上，按科目过滤 |
| 流水 Margin HF | `GET /api/v1/hf/margin/account/ledgers`、`GET /api/v3/hf/margin/account/ledgers` | 同上；v3 为 lastId 游标 |
| 流水 Futures | `GET /api/v1/transaction-history` | 同上，仅 contract 科目；`dataList` |
| 合约账户 | `GET /api/v1/account-overview` | futures 科目 |
| 全仓杠杆 | `GET /api/v1/margin/account`、`GET /api/v3/margin/accounts`、`GET /api/v1\|v3/hf/margin/account` | margin 科目；v1 用 totalBalance 字段；v3 按 quoteCurrency 汇总；liability 默认 0 |
| 全仓流水 | `GET /api/v1/margin/account/ledgers` | mesh `LedgerBills` 或本进程 `ListBills`；`currency/bizType/direction/startAt/endAt` + 分页 |
| 逐仓杠杆 | `GET /api/v1/isolated/accounts`、`GET /api/v1\|v3/isolated/account/{symbol}`、`GET /api/v3/isolated/accounts` | isobook 已开仓先，mesh isolated 总账补缺失对；详情与列表同源；GET 不落库 |
| 逐仓流水 | `GET /api/v1/isolated/accounts/ledgers` | 与全仓流水同过滤，科目 isolated |
| Classic 杠杆目录 | `GET /api/v1/margin/config`、`GET /api/v3/margin/currencies`、`GET /api/v3/margin/symbols`、`GET /api/v1\|v3/isolated/symbols` | 公开杠杆参数与交易对目录；利率/可借未配置为 0，`borrowEnabled`/`repayEnabled` 关闭 |
| UTA 总览 | `GET /api/ua/v1/account` | 全科目聚合 |
| UTA 币种资产 | `GET /api/ua/v1/unified/account/balance`、`GET /api/ua/v2/unified/account/balance` | 同上 |
| Classic 币种资产（UA） | `GET /api/ua/v2/account/balance` | 按 `accountType` 过滤科目 |
| UTA 账户模式 | `GET/POST /api/ua/v1/account/mode` | `utabook` UNIFIED / CLASSIC |
| UTA 杠杆 | `GET /api/ua/v1/unified/account/leverage`；`POST .../modify-leverage`、`modify-cross-leverage` | 只存公开字段 |
| UTA 利率 / 可借 | `GET /api/ua/v1/account/interest-limits`、`interest-history`、`interest-history-paged`、`market/borrowable-currency` | `utabook` 利率阶梯（SQL/KV）；未配置限额为 0 |
| UTA 借还读 | `GET /api/ua/v1/margin/borrow/info`、`borrow/liability`、`repay/history` | prod 先 `fundmesh.UTAAccountBook`；回落 `utabook` |
| UTA 风险读 | `GET /api/ua/v1/account/risk-rate`、`/api/ua/v1/unified/account/risk` | 风险快照与 `riskUnits`；缺数据为零值 |
| UTA 全仓/逐仓读 | `GET /api/ua/v1/margin/cross/account`、`isolated/account`、`margin/account` | 全仓 `utabook`；逐仓 overlay `isobook` |
| UTA 抵押目录 | `GET /api/ua/v1/account/collateral-catalog` | 折扣与可抵押开关；未配置 fail-closed |
| 资金页 UTA 读 | `GET /v1/assets/uta/*`（borrow-info、liability、repay-history、risk、cross/isolated-margin、collateral、rate-ladder） | gateway 聚合 `rt.UTA` |
| Classic 币种资产 | `GET /api/v1/accounts?currency=` | 同上 |
| 申请充值地址 V3 / V1 | `POST /api/v3/deposit-address/create`、`POST /api/v1/deposit-addresses` | `addresssvc.DepositAddress` |
| 查询充值地址 V3 / V2 / V1 | `GET /api/v3/deposit-addresses`、`GET /api/v2/deposit-addresses`、`GET /api/v1/deposit-addresses` | 同上 |
| 充值历史 | `GET /api/v1/deposits`、`GET /api/v1/hist-deposits` | `ledger.List` WalletDeposit + `ListIncomingInner`；`currency/status/isInner/format=csv` |
| 出金额度 | `GET /api/v1/withdrawals/quotas` | `config.Risk` 日限额 |
| 出金 V3 / V1 | `POST /api/v3/withdrawals`、`POST /api/v1/withdrawals` | `withdraw.Request` / `fundint.Send`（isInner） |
| 取消出金 | `DELETE /api/v1/withdrawals/{id}` | `withdraw.Cancel` |
| 出金历史 / 按 ID | `GET /api/v1/withdrawals` / `/{id}`、`GET /api/v1/hist-withdrawals` | `ledger.List` / `Get`；`currency/status/isInner/format=csv` |
| 划转额度 | `GET /api/v1/accounts/transferable`、`GET /api/v3/accounts/transfer-quotas` | 科目 available |
| Flex / 内部划转 | `POST /api/v3/accounts/universal-transfer`、`POST /api/v2/accounts/inner-transfer`、`POST /api/v1/accounts/inner-transfer` | `account.Move` / `subacct.Hop` |
| 主子划转 V2 | `POST /api/v2/accounts/sub-transfer` | `subacct.Hop`（direction OUT/IN） |
| 账户摘要 | `GET /api/v2/user-info`、`GET /api/v1/user-info` | Java 门户；本仓不再挂该读面 |
| 子账户列表 V1/V2 | `GET /api/v1/sub-accounts`、`GET /api/v2/sub/user` | `subacct` |
| 子账户余额/现货/合约 V2 | `GET /api/v1/sub-accounts/{id}/balance`、`/api/v2/sub-accounts/spot`、`/futures` | 子户 overlay `ListOwnerBooks` |
| 子账户转账权限 | `POST /api/v1/sub/transfer-permission` | 权限标记 |
| 子账户 API Key | Classic `GET/POST/DELETE /api/v1/sub/api-key`、`POST .../update|status|disable-all|enable-all|rotate|passphrase|sweep-*|unlock|revoke|revoke-all|restore|purge-revoked|clone|reset-usage|revoke-grace`、`GET .../detail|quota|audit|usage|probe|stats|expiring|locked|by-fp|permission-catalog`；UTA `/api/ua/v1|v2/user/*sub-api-key*` 对等 | prod 先 `fundmesh.SubKeysAccountBook`→account-book `/v1/sub-keys/*`，mesh 失败回落 `subkey`：口令字母+数字且禁复用；母户配额 200、单 sub 30，运维显式行更紧；管理面 20/分；轮换终身 50 且进入 rotating；恢复拒过期/空闲；出金类删除须先吊销；失败面 lastFail*/denyCount；吊销态（status 或 state）鉴权拒绝；审计 since/until 与 modify 字段差；签名 path 保留 query；重放槽封顶 4096；资金页 `GET /v1/assets/sub-keys/*` |
| 基础费率 / 实际费率 | `GET /api/v1/base-fee`、`GET /api/v1/trade-fees` | `vipfee.Resolve` |
| KCS 抵扣 | `GET /api/v1/base-fee?currencyType=1` | 费率查询开关 |
| 闪兑 | `POST/GET /api/v3/convert`、`POST /api/v3/convert/quote` | prod 先 `fundmesh`→convert；mesh 失败时 `cvfund`（`quoteId` 幂等、`accountType` Funding/Trading/BOTH） |
| 限价闪兑 | `/api/v1/convert/limit/*` | 牌价/下单/撤单经 mesh；`POST .../limit/order/fill` 与 `freeze-integrity` 本进程撮合回调 |
| Earn 申购/赎回/持仓 | `POST/DELETE /api/v1/earn/orders`、`GET /api/v1/earn/hold-assets`、`GET /api/v1/earn/products` | prod 先 `fundmesh`→account-book；回落 `earn`（`holdcat` 冻结、`clientOid` 幂等） |
| 资金页理财读 | `GET /v1/assets/earn/holds`、`GET /v1/assets/earn/summary` | 本进程 `earn` 持仓聚合 |
| P2P 占用读 | `GET /v1/assets/p2p/holds`、`/summary`、`/details`、`/flow`、`/merchant`、`/merchant/summary`、`/holds/:id` | gateway 聚合 `p2pgate.Funds`；`holds`/`summary` 字段兼容既有客户端 |
| P2P 占用公开 REST | `GET /api/v1/p2p/holds`、`/details`、`/flow`、`/merchant`、`/summary` | prod 先 `fundmesh.P2PFunding`→p2p-funding `/v1/p2p/*`；回落本进程 `p2pgate.Funds` |
| P2P 占用原生读 | `GET /v1/p2p/details`、`/flow`、`/merchant`、`/summary` | `p2pgate.RegisterOccupancyRead`；写路径仍为 `/v1/p2p/buy` 等 |
| 资金页杠杆读 | `GET /v1/assets/margin/cross`、`/isolated`、`/isolated/{symbol}`、`/ledgers`、`/summary` | ledger 全仓 + `isobook` 已开仓逐仓；不补默认交易对；科目读失败关闭，不回空成功 |
| 冻结明细 | `GET /api/v1/accounts/holds`、`GET /v1/assets/frozen` | prod 先 `fundmesh`→fund-surface；回落 `fundsurf.Frozen`；`format=csv` |
| 冻结汇总 | `GET /v1/assets/frozen/summary` | 按币种汇总冻结行（gateway） |
| 资金物化视图 | `GET /v1/funding/views/strong`、`GET /v1/funding/views/eventual`、`GET /v1/funding/views/lag`、`POST /v1/funding/views/rebuild`；冻结明细 `GET /v1/assets/frozen?source=projection` | `fundread` 读围栏 + `fundmat` 投影。强一致要求水位追平；最终一致可带旧值与 watermark。不发布偿付证明 |
| 出金锁倒计时 | `GET /api/v1/withdrawals/lock`、`GET /v1/security/withdraw-lock` | `ledger.WithdrawLockUntil` |
| 网络目录 | `GET /api/v1/currencies/networks`、`GET /v1/assets/networks` | mesh 或 `fundsurf.Networks`（`chains` 配置） |
| 选网提示 | `GET /api/v1/currencies/address-hints` | 形态候选，不自动选定 chain |
| 地址簿 | `GET/POST/DELETE /api/v1/withdrawals/addresses` | `addresssvc` + `destallow` |
| Memo 自助工单 | `POST /api/v1/deposits/memo-recover` | `fundsurf` |
| 旅行规则门槛 | `GET /api/v1/travel/requirements` | `travelvasp` 走廊+USD 名义；失败关闭 |
| 旅行 outbound | `POST /api/v1/travel/outbound`、`POST .../:id/submit` | 出站报文状态机；达门槛时出金须 `travelMessageId` |
| 出站旅行规则台 | `GET /api/v1/withdrawals/travel-rule*` | 案件/对手/健康/回执；未指名 VASP 按 UNHOSTED 走廊，指名未解析失败关闭 |
| 资金页旅行读 | `GET /v1/assets/travel/policy`、`/requirements` | gateway 聚合门槛表 |
| 交易密码 | `POST /v1/security/trading-password` | `tradepw` |
| 热油箱出金瞬时门控 | `POST /v1/float/egress/check` | `floatdesk.CheckEgress`；不签发租约 |
| 出金门控租约 | `POST /api/v1/egressleasegate/lease/check`、`POST .../broadcast/assert` | `egressleasegate`；缺租约 fail-closed |
| 归集同意租约 | `POST /api/v1/agreeleasegate/lease/agree`、`POST .../plan/assert`、`POST .../exec/assert` | `agreeleasegate`；缺租约 fail-closed |

既有 `/v1/*` 进程路由保留。兼容面统一 `{code,data}`。

### 子账户 API Key 边界

- **本域（`internal/subkey`）**：创建 / 列表 / 详情 / 统计 / 即将过期 / 锁中 / 指纹查找 / 修改 / 删除（出金类须先吊销） / 克隆 / 用量重置 / 权限位 / IP 绑白 / 启停 / 配额（sub 30 / 母户 200） / 应急全停全开全吊销 / 轮换（冷却+终身 50） / 改口（强度+禁复用） / 清扫含 purge-revoked / 软吊销与恢复（拒过期/空闲） / 管理面限流 / 失败面 / 审计时间窗与字段差 / 调用面签名 path 与重放封顶；SQL `aa_sub_api_keys` + `aa_sub_api_key_audit`。
- **非本域**：子户开户与划转额度、hop、velocity 主逻辑在 `internal/subacct`，子 Key 面不得扩写额度闸门。
- **挂载**：仅 `internal/kcfund` Classic `/api/v1/sub/api-key*` 与 UTA `/api/ua/v*/user/*sub-api-key*`。


## 工程与中间件

- https://docs.platform.kucoin.local/vendor/go-zero/guides/microservice/service-discovery/
- https://docs.assets.kucoin.local/ext/zeromicro/go-zero
- https://docs.platform.kucoin.local/vendor/etcd/v3.5/learning/api/
- https://docs.platform.kucoin.local/vendor/go-zero/components/queue/kafka/
- https://docs.assets.kucoin.local/ext/wmh/custodial-wallet
- https://docs.assets.kucoin.local/architecture/account-assets/ledger-cookbook
- https://docs.platform.kucoin.local/vendor/kafka/kraft-operations
- https://docs.platform.kucoin.local/vendor/redis/sentinel-ops
- https://docs.platform.kucoin.local/vendor/postgresql/16/warm-standby

双分录、入金 observe→credit、出金 freeze→broadcast→confirm、outbox、幂等、服务发现、动态配置、Kafka 通知、Redis 锁、SQL 主从：见 `aa-middleware-topology.md`、`deposit-withdraw-pipeline.md`。

## 偿付证明（内控）

契约说明见 `docs.assets.kucoin.local` 资金证明一节。下列路径由 `internal/solvency` 提供。`por` / `balproof` 仍在，响应标明 `deprecated=true` 且 `solvency=false`，不能当作偿付根。

| 能力 | REST 路径 | AA 落点 |
| --- | --- | --- |
| 证明单查询 | `GET /v1/solvency/attestations`、`GET /v1/solvency/attestations/{id}` | `solvency` 只读 |
| 用户包含路径 | `POST /v1/solvency/attestations/{id}/proof` | 已封存或已发布根；不回读调用之后的账面 |
| 路径复验 | `POST /v1/solvency/verify` | 只读，不改证明单 |
| 免责声明归类 | `GET /v1/solvency/legacy` | 工程免责声明返回 `solvency=false` |
| 调度与封存 | `POST /v1/solvency/ops/schedules`、`POST /v1/solvency/ops/run`、`POST /v1/solvency/ops/tick` | 窗口内封存；过窗废弃 building |
| 发布与作废 | `POST /v1/solvency/ops/publish`、`POST /v1/solvency/ops/revoke` | 仅 sealed 可发布；内控档须确认 |
| 漂移工单 | `GET /v1/solvency/drifts`、`POST /v1/solvency/drifts/{id}/ack`、`POST /v1/solvency/drifts/{id}/close` | open → acked → closed |

## 专项（REST 未覆盖）

逐仓/全仓强平、子账户 Key 分发、UTA 借还清算写路径、Classic `POST /api/v3/margin/borrow` 等能力在杠杆/账户邻域服务实现。本仓库暴露资金科目与 REST 读视图：负债/还本/风险来自 `utabook`+SQL；未配置利率阶梯时限额字段为 0（不编造）。

## 热预留（内部）

`GET/POST /v1/hotreserve/forecast`、`/operational`、`/reservations`、`/journal`、`/align`、`/hints/suggest`、`/plans` 由 `internal/hotreserve` 提供。签名与提交前读取同一保守下界。归集只返回计划编号。契约说明见 `docs/product/hot-reserve-forecast.md`。
