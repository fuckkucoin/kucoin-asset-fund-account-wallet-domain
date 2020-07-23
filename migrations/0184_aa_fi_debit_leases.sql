-- 站内转账借记互斥租约表（fi_debit_leases）。
--
-- 设计约束：
--   - idem_key 全局唯一；同 key 第二次申请须先检查已有 lease 状态。
--   - held 状态到 expires_at 后由 patrol worker 释放（state → released）。
--   - committed 后 ledger_id 必填，不得为空字符串。
--   - conflict 终态：同 key 发现冲突时置入，禁止再次申请。
--
-- 迁移编号：0184
-- 作者：Span / King
-- 日期：2022.06.10

CREATE TABLE IF NOT EXISTS fi_debit_leases (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY        COMMENT 'UUID v4',
    idem_key        VARCHAR(128) NOT NULL DEFAULT ''         COMMENT '等同 Send.IdempotencyKey',
    snap_id         VARCHAR(36)  NOT NULL DEFAULT ''         COMMENT '关联 fi_resolve_snaps.id',
    from_user_id    VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '扣款方用户 ID',
    to_user_id      VARCHAR(64)  NOT NULL DEFAULT ''         COMMENT '收款方（快照锁定）',
    asset           VARCHAR(16)  NOT NULL DEFAULT ''         COMMENT '币种',
    amount          VARCHAR(40)  NOT NULL DEFAULT '0'        COMMENT '金额（decimal string）',
    state           VARCHAR(16)  NOT NULL DEFAULT 'pending'  COMMENT 'pending|held|committed|released|conflict',
    ledger_id       VARCHAR(128) NOT NULL DEFAULT ''         COMMENT 'ledger.Transfer.ID（committed 后填）',
    acquired_at     DATETIME(3)  NULL                        COMMENT '进入 held 的时刻',
    committed_at    DATETIME(3)  NULL                        COMMENT '进入 committed 的时刻',
    expires_at      DATETIME(3)  NOT NULL                    COMMENT '租约 TTL（held 超过此时释放）',
    created_at      DATETIME(3)  NOT NULL,
    updated_at      DATETIME(3)  NOT NULL,

    UNIQUE KEY uk_idem_key   (idem_key),
    KEY idx_state_expires    (state, expires_at),
    KEY idx_snap_id          (snap_id),
    KEY idx_from_user        (from_user_id),
    KEY idx_updated_at       (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='站内转账借记互斥租约；同 idem_key 只有一条 held 租约，TTL 到期由 patrol 释放';

