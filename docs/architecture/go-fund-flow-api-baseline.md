# Go 资金流水查询与导出

**Method + Path 以下表为准**（`/api` 组）。

**可选事件（Go→Java）**：`POST /internal/v1/fund-flow/export-events`（`fundflow`）。

**Query/body 别名**：`bff_query.go` + `fund_flow_bff.go`（`uid`、`query=` 打包；导出 POST 可 query 回落）。

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/v1/accounts/ledgers` | `getLedgers` |
| GET | `/api/v1/hf/accounts/ledgers`、`/api/v1/hf/margin/account/ledgers` | 同上 |
| GET | `/api/v1/fund/orders` | `listFundOrders` |
| GET | `/api/v1/fund/orders/{orderId}` | `getFundOrder` |
| GET | `/api/v1/deposits` | `getDepositHistory` |
| GET | `/api/v3/accounts/transfer-list` | `listTransfers` |
| GET | `/api/v1/accounts/holds` | `frozenAssets` |
| POST | `/api/v1/accounts/ledgers/export` | `createLedgerExport` |
| GET | `/api/v1/accounts/ledgers/export/{exportId}` | `getLedgerExport` |
| GET | `/api/v1/accounts/ledgers/export/{exportId}/download` | `downloadLedgerExport` |
| GET | `/api/v1/accounts` | `getAccountList` |

## BFF 路径映射（节选）

| BFF | Go |
|-----|-----|
| `/bff/fund-flow/ledgers` | accounts/ledgers |
| `/bff/assets/orders` | fund/orders |
| 详情 | fund/orders/{orderId} |
| 创建导出 | ledgers/export POST |
| 轮询/下载 | export GET + download GET（`downloadPath` 在 status=done 时返回） |

## 字段

- **账单 items**：`id,currency,amount,fee,accountType,bizType,direction,context,createdAt`（`accountType` 映射 MAIN/TRADE）；`startAt/endAt` 为 **ledger seq**（非毫秒时间戳）。
- **fund/orders items**：`kind,orderId,currency,amount,status,createdAt,detail`；`kinds[]` 固定列表。
- **export**：`exportId,status,kind,format,startAt,endAt,createdAt`；失败含 `errorMessage`；完成含 `downloadPath`。

## 约束

- 导出窗口 `CheckExportWindow`（如 100 天）；`ErrDaily` 每日次数。
- `exportId` 归属 `requireUser`；他人 404。
- 仅 `status=done` 可下载。
