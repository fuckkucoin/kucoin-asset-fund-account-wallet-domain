-- 法币轨道授权捕获结清与退款租约收口引擎迁移。
--
-- 表：
--   fiat_settle_leases          — Authorize 签发、Settle 原子消费
--   fiat_limits_reservations    — Check 后占用，Record 仅 Settle 成功
--   fiat_settle_proofs          — 指纹漂移 / 互斥 / 捕获孤儿证明仓
--   fiat_settle_audit           — 结清审计
--   fiat_refund_leases          — claw/refund_lease
--   fiat_settle_idem            — 幂等键
--   fiat_settle_booking_locks   — Settle↔Refund 互斥令牌
--
-- 原则：
--   1. booking_id 上未终态租约由应用 CAS + 互斥锁保证单写。
--   2. Capture 成功而入账失败写入 compensate，禁止二次 Capture。
--   3. 不引用 fiatadd / fiatjnl / fiatbreak 外键，隔离在应用层。
--
-- 迁移标识：0132（与 main 既有 0071_stablebr_mismatch 冲突后重编号）
-- 作者：Span
-- 日期：2021.12.18

-- +migrate Up

CREATE TABLE IF NOT EXISTS fiat_settle_leases (
    lease_id            VARCHAR(64)   NOT NULL COMMENT 'fiat_settle_lease_id',
    booking_id          VARCHAR(64)   NOT NULL COMMENT '法币轨道预约',
    user_id             VARCHAR(64)   NOT NULL,
    kind                VARCHAR(16)   NOT NULL COMMENT 'onramp|offramp',
    asset               VARCHAR(16)   NOT NULL,
    amount              VARCHAR(64)   NOT NULL,
    currency            VARCHAR(16)   NOT NULL,
    psp_intent          VARCHAR(64)   NOT NULL COMMENT '授权时钉扎的 PSPIntent',
    kyc_tier            VARCHAR(16)   NOT NULL DEFAULT '',
    kyc_fingerprint     VARCHAR(64)   NOT NULL COMMENT 'KYC 档位/材料指纹',
    limits_fingerprint  VARCHAR(64)   NOT NULL COMMENT '限额策略指纹',
    reservation_id      VARCHAR(64)   NULL,
    capture_ref         VARCHAR(64)   NULL,
    capture_sealed      TINYINT(1)    NOT NULL DEFAULT 0 COMMENT 'Capture 已成功，禁二次',
    ledger_id           VARCHAR(64)   NULL,
    state               VARCHAR(16)   NOT NULL DEFAULT 'authorized'
                        COMMENT 'authorized|settle_bound|settled|refunding|refunded|compensate|revoked|expired',
    generation          BIGINT        NOT NULL DEFAULT 1,
    ttl_seconds         BIGINT        NOT NULL DEFAULT 900,
    issued_at           DATETIME(3)   NOT NULL,
    expires_at          DATETIME(3)   NOT NULL,
    bound_at            DATETIME(3)   NULL,
    settled_at          DATETIME(3)   NULL,
    revoked_at          DATETIME(3)   NULL,
    actor               VARCHAR(64)   NOT NULL DEFAULT '',
    note                VARCHAR(255)  NOT NULL DEFAULT '',
    compensate_reason   VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (lease_id),
    KEY idx_fsl_booking (booking_id, state),
    KEY idx_fsl_state (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币结清租约（Authorize→Settle 钉扎）';

CREATE TABLE IF NOT EXISTS fiat_limits_reservations (
    reservation_id      VARCHAR(64)   NOT NULL,
    booking_id          VARCHAR(64)   NOT NULL,
    user_id             VARCHAR(64)   NOT NULL,
    kind                VARCHAR(16)   NOT NULL,
    amount              VARCHAR(64)   NOT NULL,
    limits_fingerprint  VARCHAR(64)   NOT NULL,
    state               VARCHAR(16)   NOT NULL DEFAULT 'reserved'
                        COMMENT 'reserved|recorded|released|expired',
    lease_id            VARCHAR(64)   NULL,
    created_at          DATETIME(3)   NOT NULL,
    updated_at          DATETIME(3)   NOT NULL,
    recorded_at         DATETIME(3)   NULL,
    released_at         DATETIME(3)   NULL,
    release_reason      VARCHAR(128)  NOT NULL DEFAULT '',
    PRIMARY KEY (reservation_id),
    KEY idx_flr_booking (booking_id),
    KEY idx_flr_user (user_id, kind, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币限额占用（Check 后占用，Settle 成功才 Record）';

CREATE TABLE IF NOT EXISTS fiat_settle_proofs (
    proof_id        VARCHAR(64)   NOT NULL,
    lease_id        VARCHAR(64)   NOT NULL,
    booking_id      VARCHAR(64)   NOT NULL,
    kind            VARCHAR(64)   NOT NULL,
    fail_closed     TINYINT(1)    NOT NULL DEFAULT 1,
    winner          VARCHAR(128)  NOT NULL DEFAULT '',
    loser           VARCHAR(128)  NOT NULL DEFAULT '',
    summary         TEXT          NOT NULL,
    capture_sealed  TINYINT(1)    NOT NULL DEFAULT 0,
    settled_held    TINYINT(1)    NOT NULL DEFAULT 0,
    opened_at       DATETIME(3)   NOT NULL,
    exported_at     DATETIME(3)   NULL,
    detail          TEXT          NULL,
    PRIMARY KEY (proof_id),
    KEY idx_fsp_lease (lease_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币结清互斥与漂移证明仓';

CREATE TABLE IF NOT EXISTS fiat_settle_audit (
    audit_id    VARCHAR(64)   NOT NULL,
    lease_id    VARCHAR(64)   NULL,
    booking_id  VARCHAR(64)   NOT NULL,
    action      VARCHAR(32)   NOT NULL,
    actor       VARCHAR(64)   NOT NULL,
    from_state  VARCHAR(16)   NOT NULL DEFAULT '',
    to_state    VARCHAR(16)   NOT NULL DEFAULT '',
    detail      VARCHAR(255)  NOT NULL DEFAULT '',
    created_at  DATETIME(3)   NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_fsa_lease (lease_id, created_at),
    KEY idx_fsa_booking (booking_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='法币结清租约审计';

CREATE TABLE IF NOT EXISTS fiat_refund_leases (
    refund_lease_id  VARCHAR(64)   NOT NULL,
    settle_lease_id  VARCHAR(64)   NOT NULL,
    booking_id       VARCHAR(64)   NOT NULL,
    psp_intent       VARCHAR(64)   NOT NULL,
    reverse_ledger   VARCHAR(64)   NULL,
    psp_refund_ref   VARCHAR(64)   NULL,
    state            VARCHAR(16)   NOT NULL DEFAULT 'issued'
                     COMMENT 'issued|bound|completed|failed|blocked',
    actor            VARCHAR(64)   NOT NULL,
    reason           VARCHAR(255)  NOT NULL,
    issued_at        DATETIME(3)   NOT NULL,
    completed_at     DATETIME(3)   NULL,
    fail_reason      VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (refund_lease_id),
    KEY idx_frl_booking (booking_id, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='已结清退款 claw/refund_lease';

CREATE TABLE IF NOT EXISTS fiat_settle_idem (
    idem_key   VARCHAR(128) NOT NULL,
    result_id  VARCHAR(64)  NOT NULL,
    PRIMARY KEY (idem_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='结清闸幂等';

CREATE TABLE IF NOT EXISTS fiat_settle_booking_locks (
    booking_id  VARCHAR(64)  NOT NULL,
    holder      VARCHAR(128) NOT NULL,
    purpose     VARCHAR(16)  NOT NULL COMMENT 'settle|refund',
    expires_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (booking_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Settle 与 Refund 互斥令牌';

-- +migrate Down

DROP TABLE IF EXISTS fiat_settle_booking_locks;
DROP TABLE IF EXISTS fiat_settle_idem;
DROP TABLE IF EXISTS fiat_refund_leases;
DROP TABLE IF EXISTS fiat_settle_audit;
DROP TABLE IF EXISTS fiat_settle_proofs;
DROP TABLE IF EXISTS fiat_limits_reservations;
DROP TABLE IF EXISTS fiat_settle_leases;
