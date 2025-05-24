# 入金归因运行时

观察线程在贷记之前可以多问一次归因结论。结论里的 `credit_intent` 为假时，不得创建入账。钩子默认关闭。关闭时，原有备注闸、错网、巡检差额、提前入账回退和 `dep:` 刷充熔断保持原样。

只发信封，不写审计仓，不改账本 Apply，不改出金，不改对账三方公式，不改归集证明链。速率键只用 `attrib:` 前缀。

## 能力

| 能力 | 落点 | 入账约束 |
| --- | --- | --- |
| 统一 Resolve | `internal/attribrt` | 输出结构化结论和置信度。`credit_intent` 为假则观察线程停在 `ledger.Create` 之前 |
| 噪声处置 | `internal/attribrt/noise` | `drop` / `hold` / `desk` / `delay`。这四类都不能入账 |
| UTXO 找零图 | `internal/attribrt/utxograph` | 找零、未绑定币、守恒失败、多用户合并都不入账 |
| 备注碰撞机 | `internal/attribrt/collide` | 两边都挡住，直到裁决只放行一方 |
| 链族策略 | `internal/attribrt/policy` | 未知代次、跨族代次、活路径上的过期代次失败关闭 |
| 台席与工人 | `internal/attribrt/desk`、`cmd/attrib-worker` | 只改结论和案件，不调用账本 |
| 信封 | `aa_attrib_envelope` | 供审计仓另行消费。本进程不写审计仓 |

打开钩子：本进程设置 `ATTRIB_REQUIRE=1`，或运营 `POST /v1/attrib/hook`。`attrib-worker` 进程自己打开钩子，不带动用户侧充值进程。

## 结论字段

`Resolve` 的顺序固定：策略代次、备注字符集、噪声、UTXO 图、绑定、碰撞、置信度、`attrib:` 速率。

| 字段 | 含义 |
| --- | --- |
| disposition | `pass` `drop` `hold` `desk` `delay` |
| credit_intent | 只有 `pass` 且没有拒绝原因时为真 |
| confidence | 从 10000 扣噪声罚分 |
| band | `credit` / `review` / `refuse`。低于链门槛为 `refuse` |
| policy_version | 实际裁定的代次。活路径空代次会钉到该族现行代次 |
| reasons | 原因码，见下表 |
| envelope_id | 本结论的信封编号 |

原因码：`unknown_chain`、`unknown_version`、`policy_family`、`policy_stale`、`policy_closed`、`memo_required`、`memo_charset`、`namespace_incomplete`、`unbound`、`user_mismatch`、`noise_dust`、`noise_self`、`noise_sweep`、`noise_poison`、`noise_airdrop`、`noise_decoy`、`noise_homoglyph`、`noise_spam`、`noise_script`、`noise_scale`、`utxo_change`、`utxo_unbound`、`utxo_conserve`、`utxo_merge`、`utxo_graph_required`、`collide_blocked`、`confidence_low`、`rate_limited`。

`drop` 只留信封，不进台席队列。`hold`、`desk`、`delay` 进队列。队列结案不是入账。

## 策略代次

每个族同时只有一个现行代次。行上的 `ChangeCredit` 必须为假，装载时发现为真就拒绝整张表。

| 代次 | 族 | 活路径 |
| --- | --- | --- |
| fam-memo-2020.09 | MEMO | 否 |
| fam-memo-2022.06 | MEMO | 否 |
| fam-memo-2024.11 | MEMO | 是 |
| fam-utxo-2021.07 | UTXO | 否 |
| fam-utxo-2024.11 | UTXO | 是 |
| fam-evm-2023.04 | EVM | 否 |
| fam-evm-2024.11 | EVM | 是 |
| fam-acct-2022.04 | ACCOUNT | 否 |
| fam-acct-2024.11 | ACCOUNT | 是 |

裁定：

1. 链不在目录：`unknown_chain`，失败关闭。
2. 代次号不在目录：`unknown_version`，回放也同样失败关闭。
3. 代次所属族和链的族不同：`policy_family`，回放也同样失败关闭。
4. 代次是同一族的历史代次，且不是回放：`policy_stale`，失败关闭。
5. 回放允许同一族历史代次，用来解释当时的置信度门槛，不能拿去给活的观察线程入账。观察钩子始终按活路径调用。

链适配器的广播族只抄到 `chainad_family`。对不上时不改写策略族，也不因此放行。

## 噪声

分类器按链策略里的启用列表运行。星号行是默认处置，链名行覆盖默认。最严的一条生效，顺序是 `desk` > `hold` > `delay` > `drop`。

| 分类器 | 何时命中 | 默认处置 |
| --- | --- | --- |
| dust | 金额低于链尘埃或资产下限 | drop |
| self | 来源和去向是同一地址 | drop |
| sweep | 来源在热钱包名单 | desk |
| script | `nulldata` / `op_return` | drop |
| scale | 小数位超过该链精度 | hold |
| poison | 与地址首尾相同且只有几位不同。名单为空不猜测 | desk |
| airdrop | 拒绝名单，或合约打来的非原生整额 | desk，名单命中则 drop |
| decoy | 零宽或拉丁与西里尔混写 | desk |
| homoglyph | 折叠骨架和原文不同（数字标签的前导零不算） | desk |
| spam | 同一来源一分钟内小额笔数达到门槛 | delay |

热钱包、拒绝名单、地址、合约标记由 `POST /v1/attrib/book` 登记。空名单不把正常地址打成噪声。

## UTXO 图

UTXO 族必须带输入输出，或事先由台席/工人解析过同一笔交易并留下图。没有图时 `utxo_graph_required`，不入账。

打标顺序：

1. 空数据脚本：`nulldata`，不入账。
2. 未知脚本：工单 `unknown_script`，不入账。
3. 低于脚本尘埃：`dust`，不入账。
4. 输出地址等于任一输入地址，或落在热钱包找零名单：`change`，不入账。
5. 地址或 `txid:vout` 绑到用户：`payment`。多用户输入时仍不入账。
6. 只剩一个未绑定输出且已经有付款：该输出改标找零。多个未绑定输出各自开工单，不猜。

守恒：输入合计减输出合计等于手续费。手续费低于 `FeeMin` 或高于本金的 `FeeMaxBps`，整笔 `utxo_conserve`，所有输出的入账标记清掉。声明手续费和算出来的不一致同样失败。

多输入若落到两个以上已绑定用户，整笔 `utxo_merge`，不把币分给其中任何一个。没有绑定的外部付款人不算合并。

## 碰撞状态机

折叠键是 `链|命名空间|骨架`。`chain_addr` 命名空间还要收款地址，缺地址则 `namespace_incomplete`，不猜共享地址以外的值。共享池里已经登记的地址可以补上。

数字标签去掉前导零。`00123` 和 `123` 是同一骨架。形近字收成 ASCII 骨架后再。入账查找仍用原始串，折叠只用于发现撞车。

没有第二个人时不建案。先占住骨架的用户可以按其余规则入账。第二个人出现时：

```
clear --conflict/block_both--> blocked
clear --conflict/review------> suspect
suspect --timeout------------> blocked
suspect --assign-------------> desk
blocked --assign-------------> desk
desk --prove-----------------> evidence
desk --pick------------------> released
desk --pick_none-------------> rejected
desk --timeout---------------> expired
evidence --accept------------> arbitrate
evidence --reject_proof------> blocked
evidence --timeout-----------> expired
arbitrate --pick-------------> released
arbitrate --pick_none--------> rejected
arbitrate --timeout----------> expired
released --conflict(新用户)--> blocked
rejected --conflict(新用户)--> blocked
expired --conflict(新用户)---> blocked
```

`suspect`、`blocked`、`desk`、`evidence`、`arbitrate`、`rejected`、`expired` 都不允许任何主张人入账。`released` 只允许胜诉用户。已在案里的败诉人再次出现，不推翻已经作出的裁决。

派单和裁决的操作者不能是 `system`。裁决理由至少 8 个字符，胜诉人必须是主张人之一。超时只接受 `attrib-worker`。

`suspect` 30 分钟、`desk` 和 `evidence` 24 小时、`arbitrate` 8 小时。到期后 `suspect` 进入 `blocked`，台席上的未决进入 `expired`。两条都是失败关闭。

重放从 `clear` 按日志再走一遍。终态或胜诉人不一致则重放失败。

## 速率

键形如 `attrib:{scope}|{a}|{b}|{c}`。`scope` 不能是 `dep` 或 `wd`。拼不出 `attrib:` 前缀就拒绝计数。超限后记 `rate_limited`，处置至少是 `delay`，并且不再把拒绝本身算进窗口。

## 工人

`cmd/attrib-worker` 每 15 秒跑一拍：到期碰撞案，以及到点的 `delay` 结论重跑。重跑只写新结论和信封。真正贷记仍要观察线程再次 Resolve，并且钩子认可得的入账意图。

## 回放场景

台席 `GET /v1/attrib/fixtures` 看清单，`POST /v1/attrib/replay/:name` 用新引擎跑场景。场景对不上清单时返回冲突，不会改结论。

| 场景 | 入账意图 | 处置 | 原因 |
| --- | --- | --- | --- |
| unbound_memo | 否 | desk | unbound |
| dust_drop | 否 | drop | noise_dust |
| self_send | 否 | drop | noise_self |
| utxo_change | 否 | drop | utxo_change |
| utxo_payment | 是 | pass | |
| utxo_conserve | 否 | desk | utxo_conserve |
| utxo_unbound | 否 | desk | utxo_unbound |
| memo_collision | 否 | desk | collide_blocked |
| memo_released | 是 | pass | 胜诉用户 |
| memo_loser | 否 | desk | collide_blocked |
| unknown_version | 否 | desk | unknown_version |
| cross_family | 否 | desk | policy_family |
| stale_policy | 否 | desk | policy_stale |
| stale_replay | 是 | pass | 仅回放 |
| homoglyph_memo | 否 | desk | noise_homoglyph |
| leading_zero_tag | 否 | desk | collide_blocked |
| utxo_merge | 否 | desk | utxo_merge |

## 表

`migrations/0059_aa_attrib_runtime.sql`：`aa_attrib_decision`、`aa_attrib_envelope`、`aa_attrib_collision`、`aa_attrib_coin_ticket`。碰撞唯一键含链、命名空间和折叠键。进程启动时回填碰撞案，避免重启后把已挡住的备注重新放行。库没挂上或表还没建时，结论留在内存，不因此放行一笔不该入的账，也不伪造贷记。台席读要求 `case.read`，写要求 `ops.admin`。`preview` 只计算结论，不写碰撞索引、速率窗和图缓存。
