# Go 主子账户管理接口

**`/api/*`** 对用户/query 面；**`/v1/subacct/*`** 在 account-book 进程（`subacct.Register`）。

**可选事件（Go→Java）**：`POST /internal/v1/sub-accounts/events`（`subaccounts` + `fundingbff`）。

**Query/body 别名**：`bff_query.go` + `sub_bff.go`（`uid`、打包 `query=`、写 body 空时 query 回落）。

| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/v2/sub/user` | `addSubAccount` |
| GET | `/api/v1/sub-accounts` | `listSubAccounts`（`items`+`totalNum`） |
| GET | `/api/v2/sub/user` | 同上（分页字段） |
| GET | `/api/v1/sub-accounts/{subUserId}` | `getSubAccount` |
| POST | `/v1/subacct/freeze` | `subacct` 包（account-book） |
| POST | `/api/v1/sub-accounts/{subUserId}/freeze` | `freezeSubAccount` |
| POST | `/v1/subacct/children/close` | account-book |
| POST | `/api/v1/sub-accounts/{subUserId}/close` | `closeSubAccount` |
| POST | `/v1/subacct/label` | account-book |
| POST/PUT | `/api/v1/sub-accounts/{subUserId}/label` | `updateSubAccountLabel` |
| POST | `/v1/subacct/privileges` | account-book |
| POST/PUT | `/api/v1/sub-accounts/{subUserId}/privileges` | `setSubAccountPrivileges` |
| POST | `/api/v1/sub/transfer-permission` | `setSubTransferPerm` |
| PUT | `/api/v1/sub-accounts/{subUserId}/transfer-permission` | `setSubTransferPermPath` |
| GET | `/api/v1/sub-accounts/{subUserId}/balance` | `getSubBalance` |
| GET | `/v1/subacct/snapshot` | account-book |
| GET | `/api/v1/sub-accounts/snapshot` | `getSubSnapshot` |
| GET | `/v1/subacct/freeze-audit` | account-book |

## BFF 映射（节选）

| BFF 能力 | Go |
|----------|-----|
| 列表 | sub-accounts 或 v2/sub/user |
| 创建 | POST v2/sub/user |
| 划转开关 | transfer-permission（KV key：**parent.id + child.id**，与 HopFunds 一致） |
| 冻结 | subacct/freeze 或 API freeze |
| 余额 | sub-accounts/{id}/balance |

## 响应字段（`sub_contract.go`）

列表/详情：`subUserId`, `subName`, `remarks`, `status`, `privileges[]`, `transferable`, `userId`, 可选 `freezeReason`, `createdAt`。 
创建：另含 `access`。余额：`items[]` 含 `currency`, `available`, `holds`（保留 `accounts` 兼容）。
