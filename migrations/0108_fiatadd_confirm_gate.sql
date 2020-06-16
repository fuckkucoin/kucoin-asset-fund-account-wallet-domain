-- 法币上分确认世代闸迁移。
--
-- 包含两张表：
--   fiatadd_confirm_leases  — 确认世代租约（ConfirmLease）
--   fiatadd_fork_cases      — 分叉案例（FiatAddForkCase）
--
-- 设计原则：
--   1. fiatadd_confirm_leases.state 由应用层 CAS 控制状态机（issued→credited/forked/expired）；
--      数据库层不添加触发器。
--   2. (session_id, confirm_gen) 唯一索引确保同一会话同一世代仅一条租约（InnoDB gap lock）。
--   3. fiatadd_fork_cases 一旦 open，同一 session_id 阻断后续 Confirm，直到 closed。
--   4. credit_effects 嵌入租约表（credit_id 字段）；不单独建表，减少跨表事务。
--   5. 两表均不引用 fiatadd KV 行外键；隔离边界在应用层保证。
--   6. idem_key 唯一索引确保贷记效果幂等写入（InnoDB gap lock）。
--
-- 迁移标识：0108
-- 作者：Span
-- 日期：2021.12.08

-- +migrate Up

CREATE TABLE IF NOT EXISTS fiatadd_confirm_leases (
    lease_id        VARCHAR(64)  NOT NULL,
    session_id      VARCHAR(64)  NOT NULL COMMENT '关联 fiatadd Session ID',
    user_id         VARCHAR(64)  NOT NULL,
    asset           VARCHAR(32)  NOT NULL,
    amount          VARCHAR(64)  NOT NULL COMMENT '贷记金额',

    -- 世代与签名
    confirm_gen     BIGINT       NOT NULL DEFAULT 0 COMMENT '确认世代（单调递增）',
    sig_digest      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '签名 SHA-256 摘要',

    -- 状态
    state           VARCHAR(16)  NOT NULL DEFAULT 'issued'
                        COMMENT 'issued|credited|forked|expired',

    -- 贷记结果
    credit_id       VARCHAR(64)  NULL     COMMENT 'ledger Transfer ID',

    -- 时间戳
    issued_at       DATETIME(3)  NOT NULL COMMENT '签发时刻（UTC）',
    expires_at      DATETIME(3)  NOT NULL COMMENT '过期时刻（UTC）',
    credited_at     DATETIME(3)  NULL     COMMENT '贷记完成时刻',
    forked_at       DATETIME(3)  NULL     COMMENT '分叉发现时刻',
    expired_at      DATETIME(3)  NULL     COMMENT '过期回收时刻',

    -- 操作元数据
    actor           VARCHAR(128) NOT NULL DEFAULT '' COMMENT '操作者标识',
    note            TEXT         NULL     COMMENT '状态变更备注',

    PRIMARY KEY (lease_id),
    UNIQUE KEY uk_fiatadd_lease_session_gen (session_id, confirm_gen),
    KEY idx_fiatadd_lease_session   (session_id, state),
    KEY idx_fiatadd_lease_expires   (expires_at, state),
    KEY idx_fiatadd_lease_state     (state, issued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币上分确认世代租约';

CREATE TABLE IF NOT EXISTS fiatadd_fork_cases (
    case_id             VARCHAR(64)  NOT NULL,
    session_id          VARCHAR(64)  NOT NULL COMMENT '关联 fiatadd Session ID',
    confirm_gen         BIGINT       NOT NULL DEFAULT 0,
    lease_id            VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '关联租约 ID',
    ledger_credit_id    VARCHAR(64)  NULL     COMMENT '已落账的 ledger Transfer ID（若有）',
    session_status      VARCHAR(16)  NOT NULL DEFAULT '' COMMENT '发现分叉时的 Session.Status',

    -- 分叉类型
    kind                VARCHAR(64)  NOT NULL
                            COMMENT 'ledger_credited_session_pending|session_credited_ledger_missing',

    -- 案例状态
    state               VARCHAR(16)  NOT NULL DEFAULT 'open'
                            COMMENT 'open|closed',

    -- 时间戳
    opened_at           DATETIME(3)  NOT NULL COMMENT '案例打开时刻（UTC）',
    closed_at           DATETIME(3)  NULL     COMMENT '案例关闭时刻',

    -- 收口元数据
    closed_by           VARCHAR(128) NULL     COMMENT '四眼收口操作者',
    resolution          TEXT         NULL     COMMENT '收口决议（补录/冲正/确认幂等）',
    note                TEXT         NULL     COMMENT '分叉详情',

    PRIMARY KEY (case_id),
    KEY idx_fiatadd_fork_session   (session_id, state),
    KEY idx_fiatadd_fork_state     (state, opened_at),
    KEY idx_fiatadd_fork_lease     (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币上分分叉案例（ledger 与 Session 状态不一致）';

CREATE TABLE IF NOT EXISTS fiatadd_credit_effects (
    effect_id       VARCHAR(64)  NOT NULL,
    session_id      VARCHAR(64)  NOT NULL COMMENT '关联 fiatadd Session ID',
    confirm_gen     BIGINT       NOT NULL DEFAULT 0,
    lease_id        VARCHAR(64)  NOT NULL DEFAULT '',
    credit_id       VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'ledger Transfer ID',
    sig_digest      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '签名 SHA-256 摘要',
    amount          VARCHAR(64)  NOT NULL,
    asset           VARCHAR(32)  NOT NULL,
    user_id         VARCHAR(64)  NOT NULL,
    created_at      DATETIME(3)  NOT NULL COMMENT '证明创建时刻（UTC）',

    PRIMARY KEY (effect_id),
    UNIQUE KEY uk_fiatadd_effect_session_gen (session_id, confirm_gen),
    KEY idx_fiatadd_effect_session  (session_id),
    KEY idx_fiatadd_effect_created  (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币上分贷记效果不可变证明';

-- +migrate Down

DROP TABLE IF EXISTS fiatadd_credit_effects;
DROP TABLE IF EXISTS fiatadd_fork_cases;
DROP TABLE IF EXISTS fiatadd_confirm_leases;
