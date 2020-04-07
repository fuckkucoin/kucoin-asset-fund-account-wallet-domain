# 账户与资产持久化

进程通过 `app.Open` 装配。账本、充提单据、地址册、托管钱包、链上历史、围栏、出箱与审查案写入关系库。查询同一套 SQL 仓储。`app.OpenMem` 与 `--mem` 仅供单测或显式辅助；`AUTH_KIND=iam` 或 `APP_PROFILE=production|prod` 时 `--mem` 被拒绝。

## 装配

| 路径 | 账本 / 钱包 / KV | 链适配 |
| --- | --- | --- |
| `app.Open` | `ledger.SQLStore`、`wallet.SQLRepo`、`wallet.SQLHistory`、`persist.SQLKV` | `CHAIN_DRIVER=rpc`（默认）。节点 URL 来自配置或 `CHAIN_RPC_*`。`chain_driver=mock` 失败即关。 |

`Open` 打开库后执行 `persist.ApplySchema`、`persist.ApplyOnce`（`migrations/*.sql`）与 `ApplyKVSchema`。任一失败则进程不启动。`cmd/aa-migrate` 走同一套 DDL。`AUTH_KIND=iam` 或 `APP_PROFILE=prod|production` 时 `app.RefuseProduction` 失败即关：SQL DSN、`KMS_KIND=pkcs11` 或 `cloud-kms` / `cloudhsm`、IAM issuer + JWKS、KYT 厂商 URL、`auth.enabled`、`confirm_depth >= 1`；禁止不安全鉴权与助记词回显。

默认方言为 MySQL（`MYSQL_DSN` / `mysql-0.kucoin.net`，从库 `mysql-1.kucoin.net`）。仅当 `ACCOUNT_ASSETS_DIALECT=postgres` **且** `ACCOUNT_ASSETS_PG_DSN` 非空时改走 Postgres（`pg-0.kucoin.net` / `pg-1.kucoin.net`，SG `10.18` / TYO `10.19`）。`ACCOUNT_ASSETS_PG_DSN` 单独出现不会切换方言。

观察、确认、重组、广播在路径读配置中的 JSON-RPC。

## 表

`Open` 落地后的唯一表名共 **67**：

**migrations 0010–0013（aa_\\*，18）**

`aa_fences`、`aa_cases`、`aa_case_audit`、`aa_usage_hits`、`aa_addr_binds`、`aa_approvals`、`aa_deposits`、`aa_withdraws`、`aa_kv`、`aa_book_xfers`、`aa_audit_log`、`aa_policy_docs`、`aa_kyt_hits`、`aa_custody_inv`、`aa_nonce_slots`、`aa_sign_jobs`、`aa_policy_hits`、`aa_schema_version`

**migrations 0014（账本与托管，11）**

`ledger_accounts`、`transfers`、`ledger_journal`、`audit_log`、`address_allowlist`、`outbox_events`、`user_controls`、`custody_wallets`、`custody_addresses`、`chain_tx_history`、`submit_nonces`

**migrations 0015（2）**

`aa_outbox`、`aa_seq`

**migrations 0016（一等业务，14）**

`aa_dep_tickets`、`aa_wd_tickets`、`aa_recon_tickets`、`aa_ticket_events`、`aa_subacct_parents`、`aa_subacct_children`、`aa_subacct_grants`、`aa_subacct_hops`、`aa_sweep_schedules`、`aa_sweep_plans`、`aa_dualctl_requests`、`aa_dualctl_votes`、`aa_policy_meta`、`aa_kyt_evidence`

**migrations 0017（身份与地址，6）**

`aa_dest_allow`、`aa_dest_allow_gen`、`aa_addr_slots`、`aa_listing_assets`、`aa_listing_binds`、`aa_token_book`

**migrations 0018（运营配置与名单，11）**

`aa_sanctions`、`aa_vip_schedules`、`aa_vip_pins`、`aa_vip_fills`、`aa_fee_schedules`、`aa_risk_caps`、`aa_risk_items`、`aa_cfg_packs`、`aa_cfg_flags`、`aa_intel_tags`、`aa_intel_marks`

路径以 SQL 为权威：充提/对账工单、子户授权、归集计划、双控投票、出金白名单、充值地址绑定与库存槽、上架合约、代币簿、制裁名单、VIP/资金费率、风险敞口与升级单、运营配置包与开关、地址标签与分类。`aa_kv` 仅作回读/回填。钱包地址与账进程内址册走 `custody_addresses` / `address_allowlist`。

`persist.SchemaSQL` / `SchemaPostgres` 与 `aa_kv` 的 `ApplyKVSchema` 与上述 DDL 重叠，使用 `CREATE TABLE IF NOT EXISTS`。`aa_schema_version` 记录文件校验和，已落盘文件改写会被 `ApplyOnce` 拒绝。

金额列 `DECIMAL(36,18)`。入金自然键 `(chain, tx_hash, output_index)`（`0019` 起）。围栏列 `fence_epoch` / `fence_token`。

出金闸门用 `aa_dest_allow`（档新地址 24h 冷却）。账进程内址册用 `address_allowlist`。托管充值地址用 `custody_addresses`（含 `derivation_path` / `hd_index`）。

`0019` 只做 ALTER，不新增表。`0020` 增加 `aa_isolated_pairs`、`aa_sub_api_keys`、`aa_uta_mode`、`aa_uta_leverage`、`aa_uta_interest`（5）。`0036` 增加幂等键与账本漂移闸。`0037` 增加账单导出任务。`0038` 增加地址绑定用户索引。`0039` 给 `address_allowlist` 加写面列（asset/disabled/审计时间）。`0040` 增加 UTA 借还读面六表：`aa_uta_rate_ladder`、`aa_uta_liability`、`aa_uta_collateral`、`aa_uta_repay_history`、`aa_uta_risk_snapshot`、`aa_uta_cross_margin`。`0041` 增加子户 Key 审计与配额：`aa_sub_api_key_audit`、`aa_sub_key_quota`（及主表 `state`/`rotated_at`/`revoked_at`）。`0042` 增加旅行规则对手目录与出站案件：`aa_travel_vasps`、`aa_travel_cases`。`0043` 增加入金观测差分表 `aa_recon_deposit_diff`（按对账工单号覆盖，不改工单状态列）。`0044` 对 `aa_sub_api_keys` 增加 `status`/`updated_at`。`0045` 增加 `expire_at`/`last_used_at`。`0046` 增加 `fail_count`/`locked_until` 并拓宽 `permission`/`ip_whitelist`。`0047` 增加资金偏好表 `aa_fund_pref`、`aa_fund_pref_audit`。`0048` 增加 `last_used_ip`，并给既有 `aa_sub_api_key_audit` 加 `actor_ip`（不重建审计表）。`0049` 增加 `pass_enc`/`call_count`。`0050` 增加日配额、限流、指纹与轮换宽限列。`0051` 增加 `created_ip`/`path_allow`/`idle_ttl`。`0052` 增加 `method_allow`/`not_before`/`sticky_ip`。`0053` 增加 `site_allow`/`sign_ver`/`label`/`rotate_count`/`rotated_ip`。`0054` 增加 `last_fail_at`/`last_fail_ip`/`deny_count`。

`0055` 增加平台热温冷归集证明链五表：`aa_sweep_proof_plans`、`aa_sweep_proof_legs`、`aa_sweep_proof_links`、`aa_sweep_proof_comps`、`aa_sweep_proof_refs`。这五张表只记影子库存，不写出金冻结，也不改账本围栏。`0047` 仍是资金偏好表。

`0056` 增加资金域证据仓七表：`aa_audit_segment`、`aa_audit_entry`、`aa_audit_checkpoint`、`aa_audit_`、`aa_audit_replay_job`、`aa_audit_alert`、`aa_audit_export`。条目只插入。SQL 仓储不提供改写正文或段根的语句。`0043` 仍是入金观测差分。

`0057` 增加资金因果图五表：`fund_graph_nodes`、`fund_graph_edges`、`fund_graph_s`、`fund_graph_incidents`、`fund_graph_audit`。只记图，不改充值、出金或账本状态机。`0044` 仍是子户 Key 的 `status`/`updated_at`。

`0058` 增加账本分片投影十一表：`led_shard_shards`、`led_shard_routes`、`led_shard_intents`、`led_shard_steps`、`led_shard_vouchers`、`led_shard_balances`、`led_shard_postings`、`led_shard_idem`、`led_shard_alerts`、`led_shard_desk`、`led_shard_hints`。这些表不改账本科目行、围栏令牌或补账分录。`0056` 仍是资金域证据仓。

`0084` 增加账本批次孤儿分录发现与封堵引擎六表：`lo_batch_gens`（批次世代 CAS 锁）、`lo_orphan_reports`（孤儿扫描报告）、`lo_orphan_items`（孤儿分录明细）、`lo_seal_actions`（封堵/冲正/忽略动作证明）、`lo_integrity_proofs`（批次终态完整性证明）、`lo_idem_blocks`（封堵后幂等拒写记录）。表前缀 `lo_` 与既有 `aa_/fx_` 隔离，不改写任何既有表。

## 编排

`deploy/docker-compose.yml` 的应用环境使用 MySQL DSN 与 `CHAIN_DRIVER=rpc`，进程走 `Open`，不传 `--mem`。`deploy/compose-sql.yml` 只起 Postgres / Redis / Kafka；要对 `Open` 使用该库须同时设置 `ACCOUNT_ASSETS_DIALECT=postgres`。

`0120` 增加 UTA 全仓/经典模式切换与在途负债抵押竞态封堵引擎五表：`uta_mode_gates`（世代 CAS 锁与模式审计）、`uta_mode_transitions`（切换事件记录）、`uta_liab_locks`（在途负债抵押租约）、`uta_drift_proofs`（漂移证明与摘要）、`uta_mode_idem_blocks`（幂等拒写记录）。表前缀 `uta_` 与既有 `aa_/fx_/lo_` 隔离，不改写任何既有表。

`0131` 增加迟贷记持久世代三表：`late_credit_gens`、`late_credit_race_events`、`late_credit_proofs`（自 0070 重编号）。`0132` 增加法币结清租约七表：`fiat_settle_leases`、`fiat_limits_reservations`、`fiat_settle_proofs`、`fiat_settle_audit`、`fiat_refund_leases`、`fiat_settle_idem`、`fiat_settle_booking_locks`（自 0071 重编号）。两文件均不改写既有编排表。

`0133` 增加签章管线故障注入与在途重试世代互斥五表：`signpipe_retry_generations`、`signpipe_inflight_leases`、`signpipe_fault_cases`、`signpipe_closeout_proofs`、`signpipe_signed_artifacts`（自 0053 重编号，避开既有 `0053_aa_sub_api_key_site_sign_label`）。不改写 signpipe 内核表、ledger Apply / fence、por / stmt / balproof。

`0134` 增加稳定币证明铸赎租约五表：`stable_mint_leases`、`stable_half_proofs`、`stable_mint_audit`、`stable_mint_gens`、`stable_mint_denies`（自 0065 重编号，避开既有 `0065_wrong_credit`）。不改写桥 mint/burn 双边匹配、brmismatch / suspgate、clearing 释放贷记或 Conserve 金额公式。

`0135` 增加冷签 Decide→Exec `decide_lease` 五表：`cer_decide_leases`、`cer_drift_proofs`、`cer_decide_audit`、`cer_decide_flip_events`、`cer_decide_generations`（自 0060 重编号，避开既有 `0060_hot_reserve_forecast`）。不改写仪式批次绑定 FSM、HSM 槽轮换或 signpipe 故障注入内核。

`0136` 增加出金风控筛查放行租约三表：`risk_screen_generations`、`risk_screen_leases`、`risk_screen_proofs`（自 0054 重编号，避开既有 `0054_aa_sub_api_key_fail_deny`）。不改写 wdguard 三联钉、wdverify 挑战或 dualctl 双控票证。

`0139` 增加风险分值放行与出金提交绑定租约四表：`risk_score_generations`、`risk_score_leases`、`risk_score_drift_proofs`、`risk_score_lease_audit`（自 0056 重编号，避开既有 `0056_aa_audit_warehouse`）。不改写 screenleasegate / destgraph / limitgate / policypack 内核。

`0140` 增加托管 FeeBump 世代占用与 RBF 孤儿收口五表：`custody_feebump_gens`、`custody_broadcast_occs`、`custody_rbf_orphan_proofs`、`custody_feebump_audits`、`custody_feebump_epochs`（自 destin3 `0061_aa_feebumpgate` 重编号，避开既有 `0061_aa_cer_bind`）。不改写冷签仪式批次、Decide 租约或 HSM 槽。

`0141` 增加充值占用三桶围栏与击穿证明表族（自 destin3 `0062_aa_occupbucketfence` 重编号，避开既有 `0062_aa_dust_guard`）。三桶账本落在 `internal/occupbook`，与既有 `internal/depoccup` 占用策略册隔离，不改写 r4 延迟比例或 limitgate 租约。

`0142` 增加地址池回收复用迟观察错户封堵表族：`addrpool_recycle_epochs`、`addrpool_observe_binds`、`addrpool_miscredit_proofs`、`addrpool_observe_records`、`addrpool_recycle_epoch_audits`（自 destin4 `0073_addrpool_recycle_epoch` 重编号，避开既有 `0073_price_poison`）。不改写 dustleak / attribrt / claimmutex FSM。

`0143` 增加托管熔断 Clear↔Engage 滑窗世代表族：`custody_halt_epochs`、`custody_slip_proofs`、`custody_clear_window_cases`、`custody_halt_intents`、`custody_halt_quorum`、`custody_halt_epoch_audits`（自 destin4 `0074_custody_halt_epoch` 重编号，避开既有 `0074_aa_wd_shard_plan`）。不改写 r10 误清四眼、r13 禁提或 haltclear 内核。

`0154`–`0173` 为 destin6/dev6 闸表重编号（避开主干已占用的 `0044`–`0063`）：偿付证明、资金物化视图、闪兑路径占用/回滚、配置灰度双写、灾备回灌闸、强平计划、热库击穿、逐仓借贷比、偏好路由世代、禁提闸、hop 补偿、出箱一次性停机、入金贷记出箱、手续费报价租约、准备金快照、站内转账闸、白名单条目、跨区腿移动围栏、分片冻结世代。
