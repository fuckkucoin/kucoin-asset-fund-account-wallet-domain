# 审计仓

资金域证据面。分段只追加，检查点默克尔只在仓内核对，法医重放只读重建余额。产品说明见 [`../product/audit-warehouse.md`](../product/audit-warehouse.md)。

## 目录

| 路径 | 职责 |
| --- | --- |
| `internal/auditchain` | 段、哈希、默克尔、内存仓、SQL 仓、巡检、作业、导出、HTTP、周期任务 |
| `internal/auditchain/ingest` | 信封与挂钩。充提只调 `Try` |
| `internal/auditchain/replay` | 重放机。不引用账本与充提 |
| `internal/auditchain/fixture` | 五段验收演练 |
| `internal/auditlog/whpage.go` | 轨迹分页与 CSV |
| `internal/dualctl/replayfence.go` | 作业围栏必须前进 |
| `migrations/0056_aa_audit_warehouse.sql` | `aa_audit_*` |
| `cmd/audit-wh` | 仓进程 |
| `cmd/forensic-replay` | 重放入口 |
| `configs/auditwh/sequence.json` | 贷记到部分解冻的文件夹具 |

既有内存哈希链 `auditchain.Service` 仍在，给原来的追加接口。证据仓是另一套仓储，不把那条内存链改成可回写日志。

## 挂钩

默认 `AA_AUDIT_INGEST` 关闭。打开后，归因、贷记、冻结跃迁、用户控制位、补账清扫、归集成交、hop 完成、对账开单会投递信封。推送失败不改变这些函数的返回值。严格模式只关闭信任导出。

重放资金科目时只读 `Accounts`。巡检和作业执行路径没有账本写方法。

## 边界

证据仓、摄取、重放、演练、查询投影，以及 `cmd/audit-wh` 与 `cmd/forensic-replay`，构成这条能力。表结构见迁移 `0056`，行为说明见 `docs/product/audit-warehouse.md`。
