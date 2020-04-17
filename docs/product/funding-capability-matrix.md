# Funding 能力与代码落点

产品侧关心「用户能做什么」，工程侧关心「哪条 HTTP 最终落到哪个包」。下表是两者之间的映射，评审代码时可按能力域下钻。REST 字段与错误码以 `docs.assets.kucoin.local` 为契约源；路径级见 `architecture/aa-funding-api-baseline.md`。

| 能力域 | 典型入口 | 实现包 | 资金相关约束 |
| --- | --- | --- | --- |
| 充值地址 | `POST /api/v3/deposit-addresses` | `internal/address`, `internal/addrpool` | 一对一池 / FamMEMO 共享池；形态闸在 `wallet` |
| 链上入金 | observe → confirm | `internal/deposit`, `internal/ledger` | `(chain, tx_hash, output_index)` 只贷记一次；观测差额、已确认未贷记、重复输出必须落单；错备注/错网走回收工单；`dep:` 熔断只拦入金写 |
| 入金归因 | `POST /attrib/resolve`，钩子默认关 | `internal/attribrt` | 未绑定、噪声、UTXO 找零、备注碰撞、错误策略代次不得产生入账意图；速率键只用 `attrib:` |
| 链上出金 | freeze → submit → confirm | `internal/withdraw`, `internal/ledger` | 冻结先于 RPC；hash 持久化先于状态跃迁 |
| 地址簿 / 白名单 | 簿 CRUD + `WhitelistOnly`；读：`GET /v1/addresses/book`（分页）、`/book/item`、`/book/inspect`、`/allowlisted`；`GET /v1/destallow/entries`、`/item`、`/check`、`/whitelist-only`；`GET /api/v1/withdrawals/addresses`（及 `/item`、`/check`）、`/whitelist-pref`；写：`POST/PUT /v1/addresses/book`、`POST /v1/addresses/book/disable`、`POST /v1/addresses/book/enable`；`POST/DELETE /v1/destallow/entries`、`POST /v1/destallow/whitelist-only`；`POST/PUT /api/v1/withdrawals/addresses`、`/disable`、`/enable`；`POST/DELETE /api/v1/withdrawals/destallow`、`POST /api/v1/withdrawals/whitelist-pref` | `internal/address`, `internal/destallow` | 260325：地址大小写敏感全等；折叠命中不得冒充精确放行；停用条不得放行 |
| 充值地址读面 | `GET /v1/addresses/assigned`；`GET /v1/addrpool/assigned`、`/inspect`；`GET /api/v1|v3/deposit-addresses/assigned` | `internal/addrpool`（`InspectAssigned`） | 只读已分配 / FamMEMO 共享面；不 Allocate、不编造链上 RPC |
| 充值地址分配 / 释放 / 回收 | `POST /v1/addresses/assign`、`/release`、`/recycle`；`POST /v1/addrpool/assign`、`/release`、`/recycle`；`POST /api/v1|v3/deposit-addresses/assign`、`/release`、`/recycle` | `internal/addrpool`（`Assign` / `ReleaseUser` / `RecycleUser`） | 一对一池冷却后释放；FamMEMO 只绑 memo，不编造链上 RPC |
| 内部划转 | hop / flex | `internal/account`, `internal/subacct`, `internal/kcfund` | 同用户锁；无链上 RPC |
| 分片路由 / 跨片协议 | `led-shard`：`/v1/ledshard/intents`、`/routes`、`/vouchers`、`/desk` | `internal/ledshard` | 同用户同分片短路；系统科目钉扎；发布窗不改写已钉分片；凭证只贷一次 |
| 币种目录 / 账户偏好 | `GET /api/v1|v3/currencies`、`/api/v1/account-config` | `kcfund` + `fundpref`；链信息来自 `configs` |
| 网关编排 | query-gateway | `internal/fundmesh` | etcd Pick；传输失败时 kcfund 读本进程 ledger/withdraw |
| 账单导出 | `POST /api/v1/accounts/ledgers/export` | `kcfund/ledger_export`, `exjob` | 100 天窗口、日 5 次；可转发 statement-export |
| KYT / 旅行规则 | 广播前闸 | `internal/kyt`, `internal/travelvasp` | fail-closed；case 队列 |
| 托管签名 | PKCS#11 / CloudHSM | `internal/kms`, `internal/custody` | 拒绝 file keeper |
| 热钱包异常 | 全局 halt | `internal/custodyhalt`, `internal/fundsafety` | Engage 后清扫不得 Clear |
| 证据仓 / 法医重放 | `audit-wh`：`/v1/audit-wh/timeline`、`/replay`、`/exports` | `internal/auditchain` | 只追加；断链冻结并拒绝信任导出；重放不一致只告警，不入账 |
| 热预留下界 | `/v1/hotreserve/forecast`、`/reservations`、`/align` | `internal/hotreserve` | 保守下界减预留减新意图为负则拒签；不对 sweeprun 写路径 |
| 冷签批次绑定 | `/v1/cerbind/sessions`、`/gate/check` | `internal/cerbind` | 未到 sign_ready 的冷签批次不得签播；不改广播相位 |
| 资金滥用熔断 | `GET/POST /v1/ops/fundcircuit/*` | `internal/fundcircuit` | 键空间 `fc:`，与 `dep:`/`wd:` 并存只读桥接；跨入金/出金/划转聚合跳闸只挡新写，不解冻筛查中或已广播的在途单；解除须双人（详见 `product/fund-circuit-bus.md`） |
| 审查结案围栏 | `cmd/case-fence-gate` | `internal/casefencegate` | 双结互斥；裸 Decide 失败即关；策略漂移拒提交 |
| 闪兑报价租约 | `cmd/cv-quote-lease` | `internal/cvquotelease` | 过期/牌价指纹漂移拒成交；不得静默按旧牌价兑 |
| 占用窗口租约 | `cmd/usage-lease-gate` | `internal/usageleasegate` | 窗口指纹漂移拒写入 |
| 法币牌价租约 | `cmd/fx-mark-lease-gate` | `internal/fxmarkleasegate` | 牌价/来源过期或漂移拒落账 |
| Memo 效果互斥 | `cmd/memo-effect-gate` | `internal/memoeffectgate` | 审批贷记与自动认领互斥；半切可发现，禁止双贷 |
| 出金批次残差围栏 | `cmd/wdbatch-residue-fence` | `internal/batchresiduefence` | Retry 残差与 Submit 双冻失败即关 |
| 密钥来源 | 进程启动 / `/ops` | `internal/keysrc`, `internal/opsfc` | 只认 env/KMS；`*-local-hmac` 与仓内钥失败即关 |
| 偿付证明 | `GET /v1/solvency/attestations`、`POST /v1/solvency/attestations/:id/proof`、`POST /v1/solvency/ops/publish`、`GET /v1/solvency/drifts` | `internal/solvency`；旧 `por` / `balproof` 只保留废弃工程快照 | 封存前不得发布；观察短缺开漂移工单；不改账本 Apply 与充提状态机 |
| 用户资金物化视图 | `GET /v1/funding/views/strong`、`/eventual`、`/lag`；`POST /v1/funding/views/rebuild`；`GET /v1/assets/frozen?source=projection` | `internal/fundmat`、`internal/fundread` | 强一致读要求投影水位覆盖围栏；未追平拒绝并返回水位。可用/冻结账本，容差 0。不发布偿付证明，不写审计分区 |


事故与变更记录：`ops.kucoin.local/postmortem/`。架构总览：`account-assets-architecture.md`。
