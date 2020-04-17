# Go 资产总览与多账户余额

Go 提供资产读面与开户写能力（`/v1/assets/*` 与 `/api/v1/*`）。

- **资产读面**：`GET /v1/assets/*`（`internal/gateway/assets_surface.go`）
- **Classic 科目**：`GET /api/v1/accounts`（`kcfund/accounts.go`）
- **开户**：`POST /v1/accounts/open-all`（account-book）与 `POST /api/v1/accounts/open-all`（用户轨别名）
- **UA**：`GET /api/ua/v2/account/balance` 等（`kcfund` `uta`）

**Go→Java 事件回调**：本模块无必须回调。

## 资产读面（overview / valuation / settings）

| Method | Path | 说明 |
|--------|------|------|
| GET | `/v1/assets/overview` | `portfolio_total`, `valued`, `accounts`, `balances`, `quote`, `hide_small`, `valuation_incomplete` |
| GET | `/v1/assets/valuation` | 扁平字段 + 嵌套 `valuation`（兼容） |
| GET | `/v1/assets/accounts` | 分 book 分组 |
| GET | `/v1/assets/settings` | quote / hide_small / preference |
| GET | `/v1/assets/lite` | 低带宽 lite |

**Query**：`uid` / `user_id` / `X-User-Id`；`quote` / `quoteCurrency`；`hide_small` / `hideSmall`。

## Classic / 开户 / 偏好

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/v1/accounts` | `getAccountList`（`mergeKCAccounts`） |
| POST | `/v1/accounts/open-all` | `account` 包 |
| POST | `/api/v1/accounts/open-all` | `openAllAccounts` |
| GET | `/api/ua/v2/account/balance` | `classicBalance` |
| GET | `/api/ua/v1/account` | `utaOverview` |
| PUT | `/api/v1/account-preference` | `putAccountPreference` + query 回落 |

## 字段口径

- **available / holds**：与提现、划转可用一致；Earn 占用进 holds。
- **valuation_incomplete=true**：部分币种无 FX，UI 勿当精确总价。
- 上游可将 snake_case 转 camelCase（`portfolioTotal` 等）。
