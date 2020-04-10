# 资金页网关读面

作者：Kinsley
日期：2020.03.25

query-gateway 上 `/v1/assets/*` 由 `internal/gateway` 提供，与 `/api/*`（`kcfund`）并列。读路径编排见 `aa-funding-page-flows.md`；跨服务发现见 `fund-public-rest-mesh.md`。

| 路径 | 行为 |
| --- | --- |
| `GET /v1/assets/overview` | Unified 余额 + fundmesh/ledger 科目 + fundpref 计价与隐藏小额 + 牌价估值 |
| `GET /v1/assets/valuation` | 仅估值块；缺牌价行计入 `skipped_no_mark` |
| `GET /v1/assets/settings` | 用户偏好与默认配置合并视图 |
| `GET /v1/assets/accounts` | 科目按 book 分组 |
| `GET /v1/assets/lite` | 24h 充提与 Funding 合计（`page.go`） |
| `GET/POST /v1/assets/fiat/sessions*` | `fiatadd`；Confirm 须 HMAC 签名后 `FiatOnramp` 贷记 |
| `GET /v1/assets/p2p/holds`、`/summary` | `p2pgate` 开放占用与摘要（`open_holds`/`frozen_total`/`by_asset` 兼容） |
| `GET /v1/assets/p2p/details`、`/holds/:id` | 占用明细（含已释放）与单笔时间线 |
| `GET /v1/assets/p2p/flow` | P2P 资金流水（买入/卖出占用/保证金/释放）；`adjacent=1` 含红包邻接 |
| `GET /v1/assets/p2p/merchant`、`/merchant/summary` | 商家保证金占用与按订单汇总 |
| `GET /v1/assets/p2p/orders`、`/integrity`、`/snapshot` | 订单拼版、标签-日记对账、资金页快照 |
| `GET /v1/assets/marks` | `fxmark` 按 quote 折算；缺行 `incomplete=true` |
| `GET /v1/assets/feed` | 资金账户相关单据（充提/法币/P2P） |
| `GET /v1/assets/earn/holds`、`/summary` | 理财持仓与按币种本金汇总（open/locked） |
| `GET /v1/assets/margin/cross`、`/cross/ledgers` | Classic 全仓科目与流水；`queryType`/`currency`/`hide_small`；流水字段一致 kcfund |
| `GET /v1/assets/margin/isolated`、`/isolated/{symbol}`、`/isolated/ledgers` | 已开仓或 isolated 总账叠对；详情内存预览不落库 |
| `GET /v1/assets/margin/ledgers`、`/summary` | mesh 先账单；分页与 `bizType/direction/startAt` 过滤 |
| `GET /v1/assets/margin/config`、`/symbols`、`/currencies` | 与 Classic 目录同口径；可借未配置时 `borrowEnabled` 关闭 |
| `GET /v1/assets/frozen`、`/frozen/summary`、`/networks` | `fundsurf` 冻结与链网络目录（与 `/api` 读同源） |

牌价：`fxmark`；`valuation_incomplete=true` 表示存在无法折算的资产，禁止回填 `portfolio_total`。
