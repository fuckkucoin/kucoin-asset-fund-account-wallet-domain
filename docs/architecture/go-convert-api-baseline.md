# Go 闪兑 Convert

对外 **Method + Path 以 `/api` 组为准**。

**Query/body 别名**：`bff_query.go` + `convert_bff.go`（`from`/`to`/`fromSize`/`toSize` 与同义 JSON 字段互认）。

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/v1/convert/currencies` | `listConvertCurrencies` |
| POST | `/api/v3/convert/quote` | `quoteConvert` |
| POST | `/api/v3/convert` | `placeConvert` |
| GET | `/api/v3/convert` | `listConvert` |
| GET | `/api/v3/convert/{id}` | `getConvert` |
| GET | `/api/v1/convert/limit/quote` | `limitQuote` |
| POST | `/api/v1/convert/limit/order` | `placeLimit` |
| GET | `/api/v1/convert/limit/order/detail` | `getLimit` |
| GET | `/api/v1/convert/limit/orders` | `listLimits` |
| DELETE | `/api/v1/convert/limit/order` | `cancelLimit` |

## 可选事件（Go→Java）

`POST /internal/v1/convert/events`（`convertflow`；`fundingbff` 未启用时不出站）。

## 响应字段（节选）

- **quote**：`quoteId`、`expireAt`（毫秒）、`accountType`
- **order**：`orderId`、`debitId`/`creditId`、`createdAt`（毫秒）
- **list**：`items`、`totalNum`

## 约束

- 可 mesh 转发；Mem/本进程走 `cvfund`。
- 事件失败只打日志，不阻断成交。
