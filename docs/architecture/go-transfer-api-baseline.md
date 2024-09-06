# Go 账户间划转接口

**Method + Path 以下表为准**（挂载在 `/api` 组）。

**可选事件（Go→Java）**：`POST /internal/v1/account-transfers/events`（`accounttransfers` + `fundingbff`）。

**Query/body 别名**：`bff_query.go` + `transfer_bff.go` — 支持 `uid` query、`query=` 打包；写接口在无 JSON body 时可回落 query。

| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/v3/accounts/universal-transfer` | `flexTransfer` |
| POST | `/api/v2/accounts/inner-transfer`、`/api/v1/accounts/inner-transfer` | 同上 |
| POST | `/api/ua/v1/account/transfer`、`/api/ua/v2/account/transfer` | 同上 |
| GET | `/api/v1/accounts/transferable` | `getTransferQuotas` |
| GET | `/api/v3/accounts/transfer-quotas?type=` | 同上 |
| GET | `/api/ua/v2/account/transfer-quota` | `getTransferQuotas` |
| GET | `/api/ua/v2/account/transfer-quotas`、`/api/ua/v1/account/transfer-quotas` | 同上 |
| GET | `/api/v3/accounts/transfer-list` | `listTransfers` |
| GET | `/api/v1/accounts/transfer-list`、`/api/v1/accounts/transfers` | 同上 |
| POST | `/api/v2/accounts/sub-transfer` | `subTransfer` |
| POST | `/api/v1/sub/transfer-permission` | `setSubTransferPerm` |
| GET | `/api/v1/accounts` | `getAccountList` |

## BFF 路径映射（节选）

| BFF | Go |
|-----|-----|
| `GET /bff/accounts/transferable` | 可转余额，`type` + 可选 `currency` |
| `GET /bff/ua/transfer-quotas` | UA quota，`accountType` / `symbol` |
| `POST /bff/accounts/transfers` | universal-transfer，JSON 或 query |
| `POST /bff/accounts/sub-transfers` | universal-transfer（母子）或 sub-transfer |
| `GET /bff/accounts/transfer-list` | transfer-list |

## 响应字段

- 提交成功：`orderId`、`clientOid`、`status`（同步 `SUCCESS`）— `transfer_contract.go`
- 可转余额：`currency`、`balance`、`available`、`holds`/`hold`、`transferable`；UA 路径另含 `accountType`、可选 `symbol`
- 列表项：`orderId`、`fromAccountType`、`toAccountType`（MAIN/TRADE 等）、`createdAt` 毫秒

## 业务约束（Go 侧）

- `clientOid` 必填（或 `Idempotency-Key` header）
- INTERNAL：`fromAccountType` 与 `toAccountType` 映射到同一 book 时拒绝（含 MAIN/FUNDING）
- ISOLATED quota：`symbol` 必填
- 母子：`FundSurf.SubXferPerm` + `HopFunds` saga
