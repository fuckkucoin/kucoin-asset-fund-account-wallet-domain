-- 站内转账误路由（串户）案例表（fi_misroute_cases）。
--
-- 设计约束：
--   - snap_id + ledger_id 唯一确定一次偏差事件，不得重复开案。
--   - state FSM：open → proven → closed；open → closed（Dismiss 路径）。
--   - proof_id 在 proven 后填写；closed 无需 proof_id（Dismiss 路径）。
--   - confirmed_by 与 confirmed_at 须同时填写或同时为空。
--
-- 迁移编号：0185
-- 作者：Span / Likeo
-- 日期：2022.06.12

CREATE TABLE IF NOT EXISTS fi_misroute_cases (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY        COMMENT 'UUID v4',
    snap_id         VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT 'fi_resolve_snaps.id',
    ledger_id       VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'ledger.Transfer.ID',
    from_user_id    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '发起方用户 ID',
    snap_to_user_id VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '快照锁定收款方',
    actual_to_uid   VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '实际贷记用户（目录改绑后可能不同）',
    asset           VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '币种',
    amount          VARCHAR(40)  NOT NULL DEFAULT '0'        COMMENT '金额（decimal string）',
    idem_key        VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'Send 幂等键',
    state           VARCHAR(16)  NOT NULL DEFAULT 'open'     COMMENT 'open|proven|closed',
    proof_id        VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT '关联效果证明（proven 后填）',
    scan_note       VARCHAR(512) NOT NULL DEFAULT ''         COMMENT '扫描器注释',
    confirmed_by    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '管理员 ID',
    confirmed_at    DATETIME(3)  NULL                        COMMENT '确认时刻',
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,

    -- 同事件不重复开案
    UNIQUE KEY uk_snap_ledger (snap_id, ledger_id),
    KEY idx_state          (state),
    KEY idx_from_user      (from_user_id),
    KEY idx_idem_key       (idem_key),
    KEY idx_updated_at     (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='站内转账串户（误路由）案例仓；open→proven→closed；缺失不得放行资金';

