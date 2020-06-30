-- 分层冻结运维加冻释放缺口结案世代互斥收口引擎——数据库表定义。
--
-- 新增表：
--   hold_ops_generations   — hold_ops_gen CAS
--   hold_ops_leases        — HoldOpsLease 运维租约
--   hold_ops_proofs        — HoldOpsRaceProof 竞态证明
--   hold_ops_audit         — 不可重放审计行
--   hold_ops_shadow_tokens — 影子 token 投影（≠ holdcat 分类账）
--   hold_ops_shadow_gaps   — 影子缺口投影（≠ holdrel 协议公式）
--
-- 设计原则：
--   世代 CAS：UPDATE ... WHERE ops_gen=?；影响行数 0 → 并发抬世代失败。
--   (actor, nonce) 用于幂等查找，禁止同 nonce 双签发。
--   金额 DECIMAL(36,18)；时间 DATETIME(3)。
--   表前缀 hold_ops_ 与 hold_cat / hold_patrol / aml_hold 隔离。
--
-- 主机占位（保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：Kianford
-- 日期：2020.12.24

CREATE TABLE IF NOT EXISTS hold_ops_generations (
    id         VARCHAR(36)  NOT NULL,
    ops_gen    BIGINT       NOT NULL DEFAULT 1,
    fence_on   TINYINT      NOT NULL DEFAULT 0,
    note       VARCHAR(255) NOT NULL DEFAULT '',
    updated_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分层冻结运维世代 CAS';

INSERT IGNORE INTO hold_ops_generations (id, ops_gen, fence_on)
VALUES ('singleton', 1, 0);

CREATE TABLE IF NOT EXISTS hold_ops_leases (
    id              VARCHAR(48)    NOT NULL,
    ops_id          VARCHAR(48)    NOT NULL,
    kind            VARCHAR(16)    NOT NULL,
    user_id         VARCHAR(64)    NOT NULL,
    asset           VARCHAR(16)    NOT NULL,
    amount          DECIMAL(36,18) NOT NULL,
    token_id        VARCHAR(64)    NOT NULL DEFAULT '',
    gap_id          VARCHAR(64)    NOT NULL DEFAULT '',
    ref_id          VARCHAR(96)    NOT NULL DEFAULT '',
    reason          VARCHAR(64)    NOT NULL DEFAULT '',
    actor           VARCHAR(64)    NOT NULL,
    nonce           VARCHAR(128)   NOT NULL,
    ops_gen         BIGINT         NOT NULL,
    status          VARCHAR(16)    NOT NULL,
    isolation_mark  VARCHAR(160)   NOT NULL,
    note            VARCHAR(255)   NOT NULL DEFAULT '',
    issued_at       DATETIME(3)    NOT NULL,
    bound_at        DATETIME(3)    NULL,
    consumed_at     DATETIME(3)    NULL,
    expires_at      DATETIME(3)    NOT NULL,
    terminal_at     DATETIME(3)    NULL,
    bind_ticket     VARCHAR(96)    NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    UNIQUE KEY uq_hold_ops_lease_nonce (actor, nonce),
    KEY idx_hold_ops_lease_status (status, expires_at),
    KEY idx_hold_ops_lease_token (token_id, status),
    KEY idx_hold_ops_lease_ops (ops_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分层冻结运维租约';

CREATE TABLE IF NOT EXISTS hold_ops_proofs (
    id               VARCHAR(48)  NOT NULL,
    lease_id         VARCHAR(48)  NOT NULL DEFAULT '',
    ops_id           VARCHAR(48)  NOT NULL DEFAULT '',
    token_id         VARCHAR(64)  NOT NULL DEFAULT '',
    gap_id           VARCHAR(64)  NOT NULL DEFAULT '',
    user_id          VARCHAR(64)  NOT NULL,
    kind             VARCHAR(32)  NOT NULL,
    status           VARCHAR(16)  NOT NULL,
    expected_gen     BIGINT       NOT NULL DEFAULT 0,
    observed_gen     BIGINT       NOT NULL DEFAULT 0,
    winner_lease     VARCHAR(48)  NOT NULL DEFAULT '',
    loser_lease      VARCHAR(48)  NOT NULL DEFAULT '',
    open_token_count INT          NOT NULL DEFAULT 0,
    note             VARCHAR(255) NOT NULL DEFAULT '',
    isolation_mark   VARCHAR(160) NOT NULL,
    opened_at        DATETIME(3)  NOT NULL,
    proven_at        DATETIME(3)  NULL,
    closed_at        DATETIME(3)  NULL,
    eye_a            VARCHAR(64)  NOT NULL DEFAULT '',
    eye_b            VARCHAR(64)  NOT NULL DEFAULT '',
    clear_reason     VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    KEY idx_hold_ops_proof_user (user_id, status),
    KEY idx_hold_ops_proof_token (token_id, kind, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分层冻结运维竞态证明';

CREATE TABLE IF NOT EXISTS hold_ops_audit (
    id          VARCHAR(48)  NOT NULL,
    ops_id      VARCHAR(48)  NOT NULL,
    lease_id    VARCHAR(48)  NOT NULL,
    token_id    VARCHAR(64)  NOT NULL DEFAULT '',
    gap_id      VARCHAR(64)  NOT NULL DEFAULT '',
    ops_gen     BIGINT       NOT NULL,
    actor       VARCHAR(64)  NOT NULL,
    kind        VARCHAR(16)  NOT NULL,
    outcome     VARCHAR(32)  NOT NULL,
    proof_id    VARCHAR(48)  NOT NULL DEFAULT '',
    note        VARCHAR(255) NOT NULL DEFAULT '',
    at_ts       DATETIME(3)  NOT NULL,
    replay_key  VARCHAR(64)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_hold_ops_audit_replay (replay_key),
    KEY idx_hold_ops_audit_ops (ops_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='分层冻结运维不可重放审计';

CREATE TABLE IF NOT EXISTS hold_ops_shadow_tokens (
    id       VARCHAR(64)    NOT NULL,
    user_id  VARCHAR(64)    NOT NULL,
    asset    VARCHAR(16)    NOT NULL,
    amount   DECIMAL(36,18) NOT NULL,
    reason   VARCHAR(64)    NOT NULL DEFAULT '',
    ref_id   VARCHAR(96)    NOT NULL DEFAULT '',
    gap_id   VARCHAR(64)    NOT NULL DEFAULT '',
    is_open  TINYINT        NOT NULL DEFAULT 1,
    PRIMARY KEY (id),
    KEY idx_hold_ops_tok_gap (gap_id, is_open),
    KEY idx_hold_ops_tok_user (user_id, is_open)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='运维影子 token 投影';

CREATE TABLE IF NOT EXISTS hold_ops_shadow_gaps (
    id               VARCHAR(64)    NOT NULL,
    user_id          VARCHAR(64)    NOT NULL,
    asset            VARCHAR(16)    NOT NULL,
    amount           DECIMAL(36,18) NOT NULL,
    acked            TINYINT        NOT NULL DEFAULT 0,
    closed           TINYINT        NOT NULL DEFAULT 0,
    open_token_count INT            NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='运维影子缺口投影';
