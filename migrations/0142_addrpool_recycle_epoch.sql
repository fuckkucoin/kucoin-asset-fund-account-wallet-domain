-- 地址池回收复用后迟观察贷记错户封堵引擎迁移。
--
-- 表：
--   addrpool_recycle_epochs        — 按 (chain, address) 持久化 recycle_epoch
--   addrpool_observe_binds         — 当前观察贷记绑定（再分配立即吊销旧绑定）
--   addrpool_miscredit_proofs      — MiscreditProof 错户证明仓
--   addrpool_observe_records       — 观察贷记复验记录
--   addrpool_recycle_epoch_audits  — 审计轨迹
--
-- 原则：
--   1. generation 单调递增，禁止回拨。
--   2. 无 epoch / 陈旧 epoch / 冷却未满 Ready 不得进入成功复用或贷记。
--   3. 不引用 dustleak / attribrt / claimmutex 外键；隔离在应用层。
--
-- 迁移标识：0142（自 destin4 0073 重编号，避开既有 0073_price_poison）
-- 作者：Kinsley
-- 日期：2021.08.14

-- +migrate Up

CREATE TABLE IF NOT EXISTS addrpool_recycle_epochs (
    epoch_id        VARCHAR(64)   NOT NULL COMMENT '回收世代记录 ID',
    chain           VARCHAR(32)   NOT NULL COMMENT '链标识',
    address         VARCHAR(128)  NOT NULL COMMENT '充值地址',
    generation      BIGINT        NOT NULL COMMENT '单调世代号',
    state           VARCHAR(16)   NOT NULL DEFAULT 'cooling'
                        COMMENT 'cooling|ready|assigned|retired',
    prev_user       VARCHAR(64)   NOT NULL DEFAULT '' COMMENT '回收前属主',
    next_user       VARCHAR(64)   NULL COMMENT '再分配属主',
    cool_until      DATETIME(3)   NOT NULL COMMENT '冷却截止',
    marked_at       DATETIME(3)   NOT NULL,
    ready_at        DATETIME(3)   NULL,
    assigned_at     DATETIME(3)   NULL,
    actor_id        VARCHAR(128)  NOT NULL DEFAULT '',
    idem_key        VARCHAR(128)  NULL,
    recycle_ref     VARCHAR(128)  NULL COMMENT '只读挂钩 RecycleRec',
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (epoch_id),
    UNIQUE KEY uk_are_slot (chain, address),
    KEY idx_are_state (state, cool_until)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='地址池回收世代（Mark/Ready/Assign）';

CREATE TABLE IF NOT EXISTS addrpool_observe_binds (
    bind_id         VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL,
    generation      BIGINT        NOT NULL,
    user_id         VARCHAR(64)   NOT NULL,
    state           VARCHAR(16)   NOT NULL DEFAULT 'active' COMMENT 'active|revoked',
    bound_at        DATETIME(3)   NOT NULL,
    revoked_at      DATETIME(3)   NULL,
    revoke_reason   VARCHAR(64)   NULL,
    PRIMARY KEY (bind_id),
    UNIQUE KEY uk_aob_slot (chain, address),
    KEY idx_aob_user (user_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='地址观察贷记绑定（再分配吊销）';

CREATE TABLE IF NOT EXISTS addrpool_miscredit_proofs (
    proof_id        VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL DEFAULT '',
    generation      BIGINT        NOT NULL,
    carried_gen     BIGINT        NOT NULL DEFAULT 0,
    kind            VARCHAR(64)   NOT NULL COMMENT 'late_observe_old_user 等',
    state           VARCHAR(16)   NOT NULL DEFAULT 'open' COMMENT 'open|acked',
    prev_user       VARCHAR(64)   NOT NULL DEFAULT '',
    attempt_user    VARCHAR(64)   NOT NULL DEFAULT '',
    current_user    VARCHAR(64)   NOT NULL DEFAULT '',
    tx_hash         VARCHAR(128)  NULL,
    asset           VARCHAR(32)   NULL,
    amount          VARCHAR(64)   NULL,
    disposition     VARCHAR(16)   NOT NULL DEFAULT 'reject',
    fail_closed     TINYINT(1)    NOT NULL DEFAULT 1,
    summary         TEXT          NOT NULL,
    opened_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    acked_at        DATETIME(3)   NULL,
    exported_at     DATETIME(3)   NULL,
    observe_id      VARCHAR(64)   NULL,
    PRIMARY KEY (proof_id),
    KEY idx_amp_slot (chain, address, state),
    KEY idx_amp_kind (kind, opened_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='回收复用错户贷记证明仓';

CREATE TABLE IF NOT EXISTS addrpool_observe_records (
    observe_id      VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL DEFAULT '',
    generation      BIGINT        NOT NULL DEFAULT 0,
    carried_gen     BIGINT        NOT NULL DEFAULT 0,
    user_id         VARCHAR(64)   NOT NULL,
    tx_hash         VARCHAR(128)  NOT NULL,
    asset           VARCHAR(32)   NOT NULL,
    amount          VARCHAR(64)   NOT NULL,
    outcome         VARCHAR(16)   NOT NULL COMMENT 'credited|rejected|quarantined|idempotent',
    proof_id        VARCHAR(64)   NULL,
    idem_key        VARCHAR(128)  NOT NULL,
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (observe_id),
    UNIQUE KEY uk_aor_idem (idem_key),
    KEY idx_aor_slot (chain, address, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='观察贷记复验记录';

CREATE TABLE IF NOT EXISTS addrpool_recycle_epoch_audits (
    event_id        VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    address         VARCHAR(128)  NOT NULL,
    kind            VARCHAR(32)   NOT NULL,
    actor           VARCHAR(128)  NOT NULL DEFAULT '',
    detail          TEXT          NULL,
    occurred_at     DATETIME(3)   NOT NULL,
    PRIMARY KEY (event_id),
    KEY idx_area_slot (chain, address, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='回收世代审计轨迹';

-- +migrate Down

DROP TABLE IF EXISTS addrpool_recycle_epoch_audits;
DROP TABLE IF EXISTS addrpool_observe_records;
DROP TABLE IF EXISTS addrpool_miscredit_proofs;
DROP TABLE IF EXISTS addrpool_observe_binds;
DROP TABLE IF EXISTS addrpool_recycle_epochs;
