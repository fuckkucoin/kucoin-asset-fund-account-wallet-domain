-- 签章管线故障注入与在途签章重试世代互斥收口引擎——数据库表定义。
--
-- 新增表：
--   signpipe_retry_generations  — movement×job 重试世代 CAS
--   signpipe_inflight_leases    — InflightLease 在途租约
--   signpipe_fault_cases        — FaultCase 故障注入案
--   signpipe_closeout_proofs    — CloseoutProof 收口证明
--   signpipe_signed_artifacts   — SignedArtifact 密封产物（movement 唯一）
--
-- 设计原则：
--   世代 CAS：UPDATE ... WHERE generation=?；影响行数 0 → 并发重试失败。
--   (movement_id, nonce) 用于幂等查找，禁止同 nonce 双武装。
--   (movement_id) 唯一产物，禁止双签双花。
--   金额 DECIMAL(36,18)；时间 DATETIME(3)。
--   表前缀 signpipe_ 与 por_ / stmt_ / balproof_ 隔离。
--
-- 主机占位（保持产品契约不变）：
--   pg-0.kucoin.net（SG ap-southeast-1 10.18.0.0/16 主库）
--   pg-1.kucoin.net（TYO ap-northeast-1 10.19.0.0/16 热备）
--
-- 作者：Kianford
-- 日期：2020.12.14

CREATE TABLE IF NOT EXISTS signpipe_retry_generations (
    id          VARCHAR(36)  NOT NULL,
    movement_id VARCHAR(64)  NOT NULL,
    job_id      VARCHAR(72)  NOT NULL,
    generation  BIGINT       NOT NULL DEFAULT 0,
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_signpipe_retry_gen (movement_id, job_id),
    KEY idx_signpipe_retry_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='在途签章重试世代 CAS';

CREATE TABLE IF NOT EXISTS signpipe_inflight_leases (
    id              VARCHAR(36)    NOT NULL,
    movement_id     VARCHAR(64)    NOT NULL,
    job_id          VARCHAR(72)    NOT NULL,
    chain           VARCHAR(16)    NOT NULL,
    dest            VARCHAR(128)   NOT NULL,
    amount          DECIMAL(36,18) NOT NULL,
    canonical       TEXT           NOT NULL,
    nonce           VARCHAR(128)   NOT NULL,
    generation      BIGINT         NOT NULL DEFAULT 0,
    status          VARCHAR(16)    NOT NULL,
    actor           VARCHAR(64)    NOT NULL,
    retries         INT            NOT NULL DEFAULT 0,
    fence_epoch     BIGINT         NOT NULL DEFAULT 0,
    fence_token     VARCHAR(64)    NOT NULL DEFAULT '',
    sig_hex         VARCHAR(256)   NOT NULL DEFAULT '',
    isolation_mark  VARCHAR(128)   NOT NULL,
    note            VARCHAR(255)   NOT NULL DEFAULT '',
    armed_at        DATETIME(3)    NOT NULL,
    inflight_at     DATETIME(3)    NULL,
    sealed_at       DATETIME(3)    NULL,
    expires_at      DATETIME(3)    NOT NULL,
    terminal_at     DATETIME(3)    NULL,
    PRIMARY KEY (id),
    KEY idx_signpipe_lease_nonce (movement_id, nonce),
    KEY idx_signpipe_lease_status (status, expires_at),
    KEY idx_signpipe_lease_move (movement_id, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='在途签章租约';

CREATE TABLE IF NOT EXISTS signpipe_fault_cases (
    id              VARCHAR(36)  NOT NULL,
    lease_id        VARCHAR(36)  NOT NULL,
    movement_id     VARCHAR(64)  NOT NULL,
    job_id          VARCHAR(72)  NOT NULL,
    kind            VARCHAR(32)  NOT NULL,
    status          VARCHAR(16)  NOT NULL,
    observed_state  VARCHAR(32)  NOT NULL DEFAULT '',
    note            VARCHAR(255) NOT NULL DEFAULT '',
    operator        VARCHAR(64)  NOT NULL,
    isolation_mark  VARCHAR(128) NOT NULL,
    injected_at     DATETIME(3)  NOT NULL,
    observed_at     DATETIME(3)  NULL,
    closed_at       DATETIME(3)  NULL,
    PRIMARY KEY (id),
    KEY idx_signpipe_fault_move (movement_id, status),
    KEY idx_signpipe_fault_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='签章故障注入案';

CREATE TABLE IF NOT EXISTS signpipe_closeout_proofs (
    id              VARCHAR(36)  NOT NULL,
    lease_id        VARCHAR(36)  NOT NULL,
    movement_id     VARCHAR(64)  NOT NULL,
    job_id          VARCHAR(72)  NOT NULL,
    kind            VARCHAR(32)  NOT NULL,
    status          VARCHAR(16)  NOT NULL,
    digest          CHAR(64)     NOT NULL,
    note            VARCHAR(255) NOT NULL DEFAULT '',
    operator        VARCHAR(64)  NOT NULL DEFAULT '',
    isolation_mark  VARCHAR(128) NOT NULL,
    opened_at       DATETIME(3)  NOT NULL,
    proven_at       DATETIME(3)  NULL,
    closed_at       DATETIME(3)  NULL,
    PRIMARY KEY (id),
    KEY idx_signpipe_close_move (movement_id, status),
    KEY idx_signpipe_close_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='在途签章收口证明';

CREATE TABLE IF NOT EXISTS signpipe_signed_artifacts (
    id              VARCHAR(36)  NOT NULL,
    lease_id        VARCHAR(36)  NOT NULL,
    movement_id     VARCHAR(64)  NOT NULL,
    job_id          VARCHAR(72)  NOT NULL,
    canonical       TEXT         NOT NULL,
    sig_hex         VARCHAR(256) NOT NULL,
    digest          CHAR(64)     NOT NULL,
    generation      BIGINT       NOT NULL,
    isolation_mark  VARCHAR(128) NOT NULL,
    sealed_at       DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_signpipe_artifact_move (movement_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='密封签章产物（禁止双花）';
