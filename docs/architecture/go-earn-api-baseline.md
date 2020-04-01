# Go 理财 Earn

**用户读/快捷写**：`/api/v1/earn/*`。**账本写/持仓读**：`/v1/earn/*`（`internal/earn/http.go`）。

**Query/body 别名**：`bff_query.go` + `earn_bff.go`（`clientOid`/`idempotencyKey`、`orderId`/`positionId` 等）。

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/v1/earn/products` | `earnProducts` |
| GET | `/api/v1/earn/hold-assets` | `earnHold` |
| POST | `/api/v1/earn/orders` | `earnPurchase` |
| DELETE | `/api/v1/earn/orders` | `earnRedeem` |
| PUT | `/api/v1/earn/orders/auto-subscribe` | `earnAuto` |
| POST | `/api/v1/earn/orders/redeem-early` | `earnRedeemEarly`（写账与 `/v1/earn/redeem-early` 相同逻辑） |

### account-book（`/v1/earn`，非 `/api` 组）

| Method | Path |
|--------|------|
| GET | `/v1/earn/products` |
| GET | `/v1/earn/positions` |
| POST | `/v1/earn/subscribe` |
| POST | `/v1/earn/redeem` |
| POST | `/v1/earn/redeem-early` |
| POST | `/v1/earn/tick`、`/v1/earn/accrue`、`/v1/earn/claim` |

## 可选事件（Go→Java）

`POST /internal/v1/earn/events`（`earnflow` + `fundingbff`；未启用时不出站）。

## 响应字段（`/api`）

- **products[]**：`id, asset, category, dailyBps, aprDisplay, lockDays, minSubscribe, maxPosition, earlyBps, active, autoSubscribe`
- **hold-assets items[]**：`currency, holdAmount`
- **position**：`orderId`/`id`, `productId`, `principal`, `accrued`, `claimed`, `status`, `openedAt`, `unlockAt`, `autoSubscribe`

## 约束

- 用户：`uid` query 或 `X-User-Id`；壳 `{"code":"200000","data":...}`。
- 写账推荐 account-book subscribe/redeem；BFF 仅调 `/api` 时仍走同一账本逻辑。
