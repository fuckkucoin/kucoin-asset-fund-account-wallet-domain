-- 热库存预检租约引擎迁移。
--
-- 包含两张表：
--   hotinv_precheck_leases — 热库存预检租约（HotPrecheckLease）
--   hotinv_drift_proofs    — 活库存漂移证明（InvDriftProof）
--
-- 设计原则：
--   1. hotinv_precheck_leases.state 由应用层 CAS 控制状态机；数据库层不添加触发器。
--   2. hotinv_drift_proofs 不可变：无 UPDATE，只有 INSERT。
--   3. 两表均不引用 hotbound / hotreserve 外键；隔离边界在应用层保证。
--   4. idem_key 唯一索引确保幂等复用的并发安全（InnoDB gap lock）。
--
-- 迁移标识：0107
-- 作者：Axel
-- 日期：2021.07.22

-- +migrate Up

CREATE TABLE IF NOT EXISTS hotinv_precheck_leases (
    lease_id        VARCHAR(64)  NOT NULL,
    asset           VARCHAR(32)  NOT NULL,
    window          VARCHAR(12)  NOT NULL COMMENT '时间窗口 yyyyMMddHHmm',
    idem_key        VARCHAR(64)  NOT NULL COMMENT 'asset:window 幂等键',
    state           VARCHAR(16)  NOT NULL DEFAULT 'issued'
                        COMMENT 'issued|bound|consumed|expired|drifted',

    -- 快照摘要（签发时刻）
    digest_hex      VARCHAR(64)  NOT NULL COMMENT 'SHA-256 快照摘要',
    hot_avail       VARCHAR(64)  NOT NULL COMMENT '热账可用余额（签发时刻）',
    warm_avail      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '温账可用余额',
    cold_avail      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '冷账可用余额',
    hot_max         VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'Cap 热库上限',

    -- Cap/Band 快照
    cap_snapshot    VARCHAR(64)  NOT NULL DEFAULT '',
    band_hot_low    VARCHAR(64)  NOT NULL DEFAULT '',
    band_hot_high   VARCHAR(64)  NOT NULL DEFAULT '',
    band_warm_low   VARCHAR(64)  NOT NULL DEFAULT '',
    band_warm_high  VARCHAR(64)  NOT NULL DEFAULT '',

    -- 时间戳
    issued_at       DATETIME(3)  NOT NULL COMMENT '签发时刻（UTC）',
    expires_at      DATETIME(3)  NOT NULL COMMENT '过期时刻（UTC）',
    bound_at        DATETIME(3)      NULL COMMENT '绑定时刻',
    consumed_at     DATETIME(3)      NULL COMMENT '消费/漂移时刻',

    -- 关联
    drift_proof_id  VARCHAR(64)      NULL COMMENT '关联漂移证明 ID',
    actor           VARCHAR(128) NOT NULL DEFAULT '' COMMENT '签发方标识',
    note            TEXT             NULL COMMENT '状态变更备注',

    PRIMARY KEY (lease_id),
    UNIQUE KEY uk_hotinv_precheck_idem (idem_key),
    KEY idx_hotinv_precheck_asset_state (asset, state),
    KEY idx_hotinv_precheck_expires (expires_at, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='热库存预检租约';

CREATE TABLE IF NOT EXISTS hotinv_drift_proofs (
    proof_id                    VARCHAR(64)  NOT NULL,
    lease_id                    VARCHAR(64)  NOT NULL COMMENT '关联预检租约',
    asset                       VARCHAR(32)  NOT NULL,
    window                      VARCHAR(12)  NOT NULL,

    -- 签发时刻快照字段
    issue_digest_hex            VARCHAR(64)  NOT NULL,
    issue_hot_avail             VARCHAR(64)  NOT NULL,
    issue_warm_avail            VARCHAR(64)  NOT NULL DEFAULT '',
    issue_cold_avail            VARCHAR(64)  NOT NULL DEFAULT '',
    issue_hot_max               VARCHAR(64)  NOT NULL DEFAULT '',
    issue_captured_at           DATETIME(3)  NOT NULL,

    -- 复核时刻快照字段
    recheck_digest_hex          VARCHAR(64)  NOT NULL,
    recheck_hot_avail           VARCHAR(64)  NOT NULL,
    recheck_warm_avail          VARCHAR(64)  NOT NULL DEFAULT '',
    recheck_cold_avail          VARCHAR(64)  NOT NULL DEFAULT '',
    recheck_hot_max             VARCHAR(64)  NOT NULL DEFAULT '',
    recheck_captured_at         DATETIME(3)  NOT NULL,

    -- 漂移量
    hot_avail_delta             VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'recheck - issue',
    warm_avail_delta            VARCHAR(64)  NOT NULL DEFAULT '',
    digest_mismatch             TINYINT(1)   NOT NULL DEFAULT 1,
    severity                    VARCHAR(16)  NOT NULL COMMENT 'minor|major|critical',
    reason                      TEXT         NOT NULL,

    created_at                  DATETIME(3)  NOT NULL COMMENT '证明创建时刻（UTC）',

    PRIMARY KEY (proof_id),
    KEY idx_hotinv_drift_lease (lease_id),
    KEY idx_hotinv_drift_asset_sev (asset, severity),
    KEY idx_hotinv_drift_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='热库存活库存漂移证明（不可变）';

-- +migrate Down

DROP TABLE IF EXISTS hotinv_drift_proofs;
DROP TABLE IF EXISTS hotinv_precheck_leases;
