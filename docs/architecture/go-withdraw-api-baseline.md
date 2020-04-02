# Go 链上提币接口

**Method + Path 以下表为准**（挂载在 `/api` 组，完整路径含 `/api` 前缀）。

**可选事件（Go→Java）**：`POST /internal/v1/withdraw-rails/status-events`（`withdrawrails` + `fundingbff`）。

**Query/body 别名（`bff_query.go`）**：显式 query、`uid`/`userId`，以及 BFF 打包的 `query=`。`POST` 白名单、新增地址、提交提币在无 JSON body 时可回落 query。

| Method | Path | 实现（kcfund） |
|--------|------|----------------|
| POST | `/api/v3/withdrawals`、`/api/v1/withdrawals` | `withdrawV3` |
| GET | `/api/v1/withdrawals` | `getWithdrawHistory` |
| GET | `/api/v1/withdrawals/lock` | `withdrawLock` |
| GET | `/api/ua/v2/withdrawals/quotas` | `getWithdrawQuotas` |
| GET | `/api/v1/withdrawals/preview` | `previewWithdraw` |
| GET | `/api/v1/withdrawals/fee` | `withdrawFeeQuote` |
| GET | `/api/v1/currencies/networks` | `currencyNetworks` |
| — | 地址簿/白名单 | `/api/v1/withdrawals/addresses*`、`whitelist-pref` |
| — | Travel | `/api/v1/travel/*`、`travel-rule` |
| GET | `/api/v1/withdrawals/{withdrawalId}` | `getWithdrawByID` |
| DELETE | `/api/v1/withdrawals/{withdrawalId}` | `cancelWithdraw` |
| POST | `/api/ua/v2/asset/withdrawal` | `withdrawV3`（UTA 分支） |

状态机、KYT、广播见 `internal/withdraw`（无对外 REST）。

## 响应字段

实现集中在 `withdraw_contract.go`：**产品字段名为准**，同名旧字段仍保留兼容。

| 接口 | 字段 | 说明 |
|------|------|------|
| lock | `locked`, `reason`, `unlockAt` | |
| quotas | `dailyLimit`, `usedAmount`, `remainAmount`, `singleLimit`, `accountFrozen`, `withdrawEnabled` | |
| preview | `arriveAmount`, `minWithdraw`, `withdrawEnabled`, `travelRuleRequired` | |
| networks[] | `minWithdraw`, `withdrawEnabled`, `maintenance` | |
| whitelist check | `valid`, `inWhitelist`, `needMemo`, `coolDownUntil` | |
| 列表/详情 | `withdrawalId`, `toAddress`, `txHash`, `failReason`, `updatedAt`, `confirmations` | 列表另含 `total` |
| whitelist GET | `enabled` | 与 `whitelist_only` 同值 |
| 提交 | `withdrawalId` | |
