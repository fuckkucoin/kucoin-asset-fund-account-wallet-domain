-- 风险评分放行与出金提交绑定租约漂移封堵引擎——数据库表定义。
--
-- 新增表：
--   risk_score_generations   — Halt/Pack/Mode/Window 评分世代 CAS
--   risk_score_leases        — ScoreLease 放行租约
--   risk_score_drift_proofs  — ScoreDriftProof 漂移证明
--   risk_score_lease_audit   — 签发/提交/翻转审计
--
-- 设计原则：
--   世代 CAS：UPDATE ... WHERE halt_gen=? / pack_gen=? / mode_gen=? / window_gen=?
--   影响行数 0 → 并发翻转失败关闭。
--   (user_id, nonce) 用于幂等查找，禁止同 nonce 双签发。
--   金额 DECIMAL(36,18)；时间 DATETIME(3)。
--   表前缀 risk_score_ 与 risk_screen_ / destgraph_ / limit_commit_ 隔离。
--
-- 主机占位（保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：Kianford
-- 日期：2020.12.10

CREATE TABLE IF NOT EXISTS risk_score_generations (
    id             VARCHAR(36)  NOT NULL,
    halt_gen       BIGINT       NOT NULL DEFAULT 1,
    halt_on        TINYINT      NOT NULL DEFAULT 0,
    pack_gen       BIGINT       NOT NULL DEFAULT 1,
    pack           VARCHAR(32)  NOT NULL DEFAULT 'kyc0',
    pack_fp        VARCHAR(64)  NOT NULL DEFAULT '',
    mode_gen       BIGINT       NOT NULL DEFAULT 1,
    mode           VARCHAR(16)  NOT NULL DEFAULT 'enforce',
    window_gen     BIGINT       NOT NULL DEFAULT 1,
    window_digest  VARCHAR(64)  NOT NULL DEFAULT '',
    updated_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评分 Halt/Pack/Mode/Window 世代 CAS';

INSERT IGNORE INTO risk_score_generations (id, halt_gen, halt_on, pack_gen, pack, mode, mode_gen, window_gen)
VALUES ('singleton', 1, 0, 1, 'kyc0', 'enforce', 1, 1);

CREATE TABLE IF NOT EXISTS risk_score_leases (
    id              VARCHAR(48)    NOT NULL,
    user_id         VARCHAR(64)    NOT NULL,
    chain           VARCHAR(16)    NOT NULL,
    dest            VARCHAR(128)   NOT NULL,
    asset           VARCHAR(16)    NOT NULL,
    amount          DECIMAL(36,18) NOT NULL,
    kind            VARCHAR(16)    NOT NULL,
    pack            VARCHAR(32)    NOT NULL,
    mode            VARCHAR(16)    NOT NULL,
    pack_fp         VARCHAR(64)    NOT NULL,
    halt_gen        BIGINT         NOT NULL,
    pack_gen        BIGINT         NOT NULL,
    mode_gen        BIGINT         NOT NULL,
    window_gen      BIGINT         NOT NULL,
    window_digest   VARCHAR(64)    NOT NULL,
    velocity_1h     INT            NOT NULL DEFAULT 0,
    velocity_24h    INT            NOT NULL DEFAULT 0,
    dest_count      INT            NOT NULL DEFAULT 0,
    dest_count_24h  INT            NOT NULL DEFAULT 0,
    cluster_size    INT            NOT NULL DEFAULT 0,
    correlated      INT            NOT NULL DEFAULT 0,
    new_dests       INT            NOT NULL DEFAULT 0,
    fingerprint     VARCHAR(64)    NOT NULL,
    nonce           VARCHAR(128)   NOT NULL,
    status          VARCHAR(16)    NOT NULL,
    isolation_mark  VARCHAR(160)   NOT NULL,
    note            VARCHAR(255)   NOT NULL DEFAULT '',
    issued_at       DATETIME(3)    NOT NULL,
    bound_at        DATETIME(3)    NULL,
    consumed_at     DATETIME(3)    NULL,
    expires_at      DATETIME(3)    NOT NULL,
    terminal_at     DATETIME(3)    NULL,
    bind_ticket     VARCHAR(96)    NOT NULL DEFAULT '',
    eval_allow      TINYINT        NOT NULL DEFAULT 1,
    eval_reason     VARCHAR(128)   NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    UNIQUE KEY uq_risk_score_lease_nonce (user_id, nonce),
    KEY idx_risk_score_lease_status (status, expires_at),
    KEY idx_risk_score_lease_user (user_id, status),
    KEY idx_risk_score_lease_fp (fingerprint)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评分放行租约 score_lease';

CREATE TABLE IF NOT EXISTS risk_score_drift_proofs (
    id               VARCHAR(48)  NOT NULL,
    lease_id         VARCHAR(48)  NOT NULL DEFAULT '',
    user_id          VARCHAR(64)  NOT NULL,
    kind             VARCHAR(32)  NOT NULL,
    status           VARCHAR(16)  NOT NULL,
    expected_halt    BIGINT       NOT NULL DEFAULT 0,
    observed_halt    BIGINT       NOT NULL DEFAULT 0,
    expected_pack    VARCHAR(32)  NOT NULL DEFAULT '',
    observed_pack    VARCHAR(32)  NOT NULL DEFAULT '',
    expected_mode    VARCHAR(16)  NOT NULL DEFAULT '',
    observed_mode    VARCHAR(16)  NOT NULL DEFAULT '',
    expected_fp      VARCHAR(64)  NOT NULL DEFAULT '',
    observed_fp      VARCHAR(64)  NOT NULL DEFAULT '',
    expected_win     VARCHAR(64)  NOT NULL DEFAULT '',
    observed_win     VARCHAR(64)  NOT NULL DEFAULT '',
    expected_win_gen BIGINT       NOT NULL DEFAULT 0,
    observed_win_gen BIGINT       NOT NULL DEFAULT 0,
    digest           VARCHAR(64)  NOT NULL,
    isolation_mark   VARCHAR(160) NOT NULL,
    note             VARCHAR(255) NOT NULL DEFAULT '',
    closer_a         VARCHAR(64)  NOT NULL DEFAULT '',
    closer_b         VARCHAR(64)  NOT NULL DEFAULT '',
    close_reason     VARCHAR(255) NOT NULL DEFAULT '',
    opened_at        DATETIME(3)  NOT NULL,
    proven_at        DATETIME(3)  NULL,
    closed_at        DATETIME(3)  NULL,
    PRIMARY KEY (id),
    KEY idx_risk_score_proof_lease (lease_id, kind),
    KEY idx_risk_score_proof_user (user_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评分漂移证明';

CREATE TABLE IF NOT EXISTS risk_score_lease_audit (
    id        VARCHAR(48)  NOT NULL,
    kind      VARCHAR(32)  NOT NULL,
    lease_id  VARCHAR(48)  NOT NULL DEFAULT '',
    proof_id  VARCHAR(48)  NOT NULL DEFAULT '',
    user_id   VARCHAR(64)  NOT NULL DEFAULT '',
    actor     VARCHAR(64)  NOT NULL DEFAULT '',
    note      VARCHAR(255) NOT NULL DEFAULT '',
    at_ts     DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    KEY idx_risk_score_audit_lease (lease_id, at_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评分租约审计';
