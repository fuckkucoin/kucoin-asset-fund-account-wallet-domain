# 子账户 API Key

资金账户域能力：母账户为子账户签发、列举、修改、启停、应急全停/全开/全吊销、轮换、克隆、改口、过期/空闲/宽限/审计/吊销清扫、解锁、软吊销/恢复、撤销宽限、探针、统计、即将过期/锁中列表、指纹查找、用量重置与删除 API Key。落点 `internal/subkey`，公开 REST 由 `internal/kcfund` Classic / UTA 路径挂载。

## 能力

| 动作 | 行为要点 |
| --- | --- |
| 创建 | 母户鉴权；权限位规范化（含 InnerTransfer/Withdrawal/Earn；预设 readonly/trade/transfer/withdraw/earn）；IP 绑白（≤20，单 IP 或 CIDR）；出金/划转类权限强制 IP；单 subName ≤30、单母户 ≤200；口令 7–32 且须字母+数字；可选 `expireAt`/`notBefore`/`pathAllow`/`methodAllow`/`idleTTL`/`rateMax`/`dayCap`/`stickyIp`/`siteAllow`/`signVer`/`label`；管理面分钟窗默认 20 次 |
| 列表/详情/统计 | 按母户 / subName / apiKey / status / label 过滤；分页；`stats` 拆 enabled/disabled/revoked/locked/expired/expiring/idle；`expiring` 默认 72h；`locked`；`by-fp` 按 secretFp |
| 配额 | `quota`：单 sub limit=30；母户汇总 limit=200 |
| 修改 | 校验 passphrase；审计 detail 记变更字段名（perm/ip/site/...），不含 secret |
| 启停/全停全开 | `disable-all` / `enable-all`；revoked 不可直接 enabled |
| 轮换 | 冷却 5m；终身 ≤50；`rotateCount`/`rotatedIp`；宽限 10m |
| 克隆 | 复制策略签发新 Key；占用创建配额；secret 仅回显一次 |
| 改口 | 须字母+数字；禁止与旧口令相同 |
| 清扫 | sweep-expired/idle/grace/audit；`purge-revoked` 默认满 7 天物理删除 |
| 软吊销/恢复 | `revoke` / `revoke-all`；`restore` 拒绝仍过期或仍空闲的 Key |
| 错密锁 | 失败写 `lastFailAt`/`lastFailIp`/`denyCount`；`unlock` |
| 审计 | 含 actorIp、deny、since/until、action、limit；modify 记字段差 |
| 用量 | 含失败面与限流策略；`reset-usage` 只清 dayCount |
| 探针 | 含错密锁预检；不计数 |
| 删除 | 出金/划转类 enabled Key 须先 revoke |
| 调用面 | 签名 path 保留 query、去 fragment；重放槽上限 4096；限流窗口空键回收 |

## REST

- Classic：既有路径 + `GET .../stats|expiring|locked|by-fp`、`POST .../revoke-all|purge-revoked|clone|reset-usage`
- UTA：`/api/ua/v1|v2/user/sub-api-key/*` 与 Classic 对等

## 持久化

`0054` 增加 `last_fail_at`/`last_fail_ip`/`deny_count`。审计沿用 `0041` 的 `aa_sub_api_key_audit`（`created_at`/`client_ip`），`0048` 补 `actor_ip`。`since`/`until` 按 `created_at` 过滤。空母户不列出审计。
