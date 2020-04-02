# 账本与托管工程基线

充提与划转最容易出资损的是「状态机 + 幂等 + 冻结」三件事。下面列出本仓库已经编码的不变量，评审时可直接代码与名，而不是重新发明规则。

## 账本

- 双分录：每笔资金移动在同一 `transfer_id` 下序号递增；MySQL 侧 `(transfer_id, entry_seq)` 唯一。
- 冻结先于链上动作：出金/闪兑在 `wallet` 或 `spot` 科目冻结后，RPC 在 DB 事务外执行。
- 幂等：HTTP 层强制 `Idempotency-Key`；域层 `sameIntent` 金额、资产、地址、入金 hash。
- 用户锁：`lock:user:{id}` 覆盖 spot/wallet/swap 交叉路径，避免按资产锁死锁。

## 链上边界

- `persistTxHash` 在 `markSubmitted` 之前写入，避免 Recover 误解冻已广播单。
- 重复 `(chain, tx_hash)` 拒绝第二笔贷记/占用冻结（`ErrDuplicateHash` → 409）。
- EVM 族地址簿互认；FamMEMO 归因与 `BindUnique` 绑定。

## 剖面

`app.RefuseProduction`：IAM JWKS、SQL DSN、KYT vendor、etcd、确认深度、禁 mock chain、禁 seed 用户派生地址等。详见 `aa-prod-gap-close.md`。

## 参考

- https://docs.assets.kucoin.local/architecture/account-assets/ledger-cookbook
- https://docs.platform.kucoin.local/vendor/go-zero/guides/microservice/service-discovery/
- https://kb.wallet.binance-internal.local/architecture/double-entry-invariants
