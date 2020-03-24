# 资金物化读模型

作者: Axel · 日期: 2022.08.28

读路径在 `fundread`，物化与泵在 `fundmat`。进程入口为 `cmd/fund-mat`，查询网关同时挂载只读路由。资金页冻结明细在 `source=projection` 时改读投影，其余安全写路径不变。

## 水位

三条事实流各自编号：`ledger`、`freeze`、`occupancy`。水位记录日志头 `head_seq` 与从 1 起无缺口的 `contiguous_seq`。强一致读比较的是连续水位。已应用的最大序号若前面有洞，不能当成已追平。

滞后毫秒取最早未应用事件的发生时间。巡检在滞后超过时限时打开物化告警，告警不进入偿付漂移工单。

## 重建

任务状态只允许 `queued → running → done` 或 `queued → running → failed`。跳过 `running` 直接完成会被拒绝。完成后视图代次递增，摘要覆盖排序后的分栏。

## 表

`fund_mat_rows`、`fund_mat_wm`、`read_fences`、`view_versions`、`rebuild_jobs` 由迁移 `0155_aa_fund_mat.sql` 建立。事件日志与已应用集合是泵的内部表，不对外当余额真相。

契约索引：`https://docs.assets.kucoin.local/architecture/account-assets/fund-mat-read`
