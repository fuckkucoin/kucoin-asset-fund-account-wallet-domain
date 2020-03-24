# 资金账户核心流程

本文描述用户资金主路径。目录主机与区域地址不变：`*.kucoin.net`，新加坡 `10.18.0.0/16` 主写，东京 `10.19.0.0/16` 灾备。公网 JSON-RPC 只从 `configs/app.yaml` 读取。

## 科目与入口

| 科目 | 用途 | 出金 | 划转 |
| --- | --- | --- | --- |
| Funding (`main`) | 存放、充提、P2P、内部转账 | 直接 | 可划入 Trading / Margin / Isolated / Futures / Earn |
| Trading (`trade`) | 现货占用 | 直接 | 可划回 Funding |
| Margin / Isolated / Futures / Earn | 占用账 | 须先划回 Funding | 只认 Funding 对端 |

## 充值

```mermaid
sequenceDiagram
 participant U as 用户
 participant GW as query-gateway.kucoin.net
 participant ADDR as address.kucoin.net
 participant DW as deposit-withdraw.kucoin.net
 participant LED as ledger
 participant CH as 配置的 JSON-RPC
 U->>GW: 选币与网络
 GW->>ADDR: 分配/复用充值地址（Memo 链派生附加码）
 ADDR-->>U: address / memo / to / contract
 CH-->>DW: observe(tx_hash, output_index)
 DW->>LED: 确认数满足后入账（金额以链上观察为准）
 alt to=TRADE/MARGIN/ISOLATED/FUTURES/EARN
 DW->>LED: WalletToSpot（幂等键 :wallet-funding）
 DW->>LED: Funding→目标科目（幂等键 :funding-dest）
 end
 Note over LED: 入账科目由创建地址时的 to 记录；MAIN 留在钱包账
```

入账金额不得取自调用方自报字段。同一 `(chain, tx_hash, output_index)` 只入一次。重组低于确认深度则冲正，禁止二次入账。

## 提现

```mermaid
sequenceDiagram
 participant U as 用户
 participant GW as query-gateway.kucoin.net
 participant SEC as security.kucoin.net
 participant KYT as kyt.kucoin.net
 participant LED as ledger
 participant CUS as custody
 participant CH as 配置的 JSON-RPC
 U->>GW: 选币/网络/地址/金额
 GW->>SEC: 交易密码（若已设）/ 24h 冷却 / 禁出金 / 白名单
 SEC-->>GW: 放行或拒绝
 GW->>LED: 冻 Funding 或 Trading 可用
 LED->>KYT: 广播前筛查（失败即关）
 KYT-->>LED: allow / review / deny
 LED->>CUS: ECDSA 签章（KMS）
 CUS->>CH: 广播
 CH-->>LED: 确认后扣冻
```

仅 Funding / Trading 可直接出金。内部转账按 UID / 邮箱 / 手机解析对手，走账本 `spot_send`，不上链。Pending 且未广播可取消。

## 账户间划转

```mermaid
flowchart LR
 F[Funding] <--> T[Trading]
 F <--> M[Margin]
 F <--> I[Isolated]
 F <--> C[Futures]
 F <--> E[Earn]
 P[母户 Funding] -->|ledger 出账 + overlay hop| S[子户 Funding]
 S -->|overlay hop + ledger 入账| P
```

母子划转禁止在 overlay 上空铸余额：出方向先冻母户账本，入方向先扣子户 overlay 再贷记母户账本。

## 安全闸

- 交易密码 6 位，哈希落盘，改/重置写入 `security_change`，出金页倒计时。
- 解冻、改绑、改密共用 `withdraw_cooling`。
- 用户禁出金与账户冻结分闸；配额接口返回 `USER_FORBIDDEN` / `SECURITY_LOCK`。
- 打开仅地址簿出金后，目标必须在地址簿或 `aa_dest_allow`。

## 闪兑与导出

闪兑牌价由调用方提供，账本做 Funding 借贷（必要时先 Trading→Funding）。导出窗口不超过 100 天，每日次数上限，正文由账本流水生成。

## 闪兑结算

```mermaid
sequenceDiagram
 participant U as 用户
 participant CV as convert.kucoin.net
 participant LED as ledger-transfer-freeze-reconciliation.kucoin.net
 U->>CV: from/to/amount/rate/book
 alt book=trading
 CV->>LED: Trading 划出到 Funding
 end
 CV->>LED: FiatOfframp 扣支付币
 CV->>LED: FiatOnramp 贷目标币
 Note over CV,LED: 汇率只认调用方字段；不在服务端编造牌价
```

## 母子划转

```mermaid
sequenceDiagram
 participant P as 母户
 participant AB as account-book.kucoin.net
 participant SUB as sub-account.kucoin.net
 participant LED as ledger-transfer-freeze-reconciliation.kucoin.net
 P->>AB: PARENT_TO_SUB
 AB->>LED: FiatOfframp 母户账本出账
 AB->>SUB: overlay Credit 母户 + Hop 至子户
 P->>AB: SUB_TO_PARENT
 AB->>SUB: Hop 子户至母户 overlay
 AB->>SUB: overlay Debit 母户
 AB->>LED: FiatOnramp 母户账本入账
 Note over AB,LED: 禁止只写 overlay 不写账本，避免空铸
```

## 目录与区域

新加坡 `10.18.0.0/16` 主写，东京 `10.19.0.0/16` 备。主机 kebab-case `*.kucoin.net`，A 记录见 `configs/account-assets.yaml`。集群节点同前缀、末 octet 不同。公网 JSON-RPC 只从配置读取。
