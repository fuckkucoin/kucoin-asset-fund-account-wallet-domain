-- 准备金快照世代与公证活账漂移封堵引擎：数据库表定义。
--
-- 设计原则：
--   1. por_snap_gens：每次 BuildWithGen 封印一行，绑定 Merkle 根、totals 指纹、
--      as_of、store 水位；state FSM：pending→sealed→attested→expired|revoked。
--   2. por_attestations：每次 Attest 创建一行，绑定 snap_gen_id；
--      缺 snap_gen_id 的插入在应用层被拒绝；state FSM：pending→published→revoked。
--   3. por_snap_proofs：漂移证明，只追加，不修改；kind 区分 replay/compare。
--
-- 与前轮隔离：
--   - 不共享 balproof / investigate / solvency 表（三类已占核心逻辑）；
--   - 不修改 aa_por_snapshots 表（por 内核只读消费）；
--   - snap_gen_id 仅在本三表中写入，供他包只读引用。
--
-- 作者：SnapGateEngine
-- 日期：2020.12.17

-- ──────────────────────────────────────────────────────────────────────────────
-- 1. por_snap_gens — 快照世代租约
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS por_snap_gens (
    snap_gen_id        VARCHAR(36)   NOT NULL,
    snapshot_id        VARCHAR(36)   NOT NULL,
    merkle_root        VARCHAR(64)   NOT NULL   COMMENT 'hex SHA-256 Merkle 根',
    leaves_digest      VARCHAR(64)   NOT NULL   COMMENT 'leaves 集合摘要根（hex）',
    totals_fingerprint VARCHAR(64)   NOT NULL   COMMENT 'totals 指纹（hex）',
    as_of              DATETIME(3)   NOT NULL   COMMENT '快照取样时刻（UTC）',
    store_mark         BIGINT        NOT NULL   DEFAULT 0 COMMENT 'store 水位（ledger 自增序列近似）',
    user_count         INT           NOT NULL   DEFAULT 0,
    leaf_count         INT           NOT NULL   DEFAULT 0,
    state              VARCHAR(16)   NOT NULL   DEFAULT 'sealed'
                         COMMENT 'sealed|attested|expired|revoked',
    ttl_seconds        BIGINT        NOT NULL   DEFAULT 86400 COMMENT '世代有效期（秒）',
    sealed_at          DATETIME(3)   NOT NULL,
    attested_at        DATETIME(3)   NULL,
    expired_at         DATETIME(3)   NULL,
    revoked_at         DATETIME(3)   NULL,
    created_at         DATETIME(3)   NOT NULL,
    updated_at         DATETIME(3)   NOT NULL,
    PRIMARY KEY (snap_gen_id),
    INDEX idx_psg_snapshot_id  (snapshot_id),
    INDEX idx_psg_state        (state),
    INDEX idx_psg_sealed_at    (sealed_at),
    INDEX idx_psg_expire_check (state, sealed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='por 快照世代租约；BuildWithGen 封印，Prove/Attest 必须绑定。';

-- ──────────────────────────────────────────────────────────────────────────────
-- 2. por_attestations — 公证包
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS por_attestations (
    attest_id          VARCHAR(36)   NOT NULL,
    snap_gen_id        VARCHAR(36)   NOT NULL   COMMENT '外键 por_snap_gens.snap_gen_id',
    snapshot_id        VARCHAR(36)   NOT NULL,
    merkle_root        VARCHAR(64)   NOT NULL,
    totals_fingerprint VARCHAR(64)   NOT NULL,
    export_payload     MEDIUMBLOB    NOT NULL   COMMENT '公证导出内容（JSON）',
    export_hash        VARCHAR(64)   NOT NULL   COMMENT 'export_payload SHA-256（hex）',
    state              VARCHAR(16)   NOT NULL   DEFAULT 'pending'
                         COMMENT 'pending|published|revoked',
    requester          VARCHAR(64)   NOT NULL   DEFAULT '' COMMENT '发起操作人 ID',
    published_at       DATETIME(3)   NULL,
    revoked_at         DATETIME(3)   NULL,
    revoke_reason      VARCHAR(256)  NOT NULL   DEFAULT '',
    created_at         DATETIME(3)   NOT NULL,
    updated_at         DATETIME(3)   NOT NULL,
    PRIMARY KEY (attest_id),
    INDEX idx_pa_snap_gen_id (snap_gen_id),
    INDEX idx_pa_state       (state),
    INDEX idx_pa_snapshot_id (snapshot_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='por 公证包；缺 snap_gen_id 的插入在应用层拒绝（Fail-Closed）。';

-- ──────────────────────────────────────────────────────────────────────────────
-- 3. por_snap_proofs — 活账漂移证明（只追加）
-- ──────────────────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS por_snap_proofs (
    proof_id           VARCHAR(36)   NOT NULL,
    snap_gen_id        VARCHAR(36)   NOT NULL   COMMENT '外键 por_snap_gens.snap_gen_id',
    snapshot_id        VARCHAR(36)   NOT NULL,
    kind               VARCHAR(16)   NOT NULL   COMMENT 'replay|compare',
    sealed_root        VARCHAR(64)   NOT NULL   COMMENT '封印 Merkle 根',
    live_root          VARCHAR(64)   NOT NULL   COMMENT '实时 Merkle 根',
    sealed_totals      MEDIUMBLOB    NOT NULL   COMMENT '封印 totals（JSON）',
    live_totals        MEDIUMBLOB    NOT NULL   COMMENT '实时 totals（JSON）',
    drift_assets       MEDIUMBLOB    NOT NULL   COMMENT '漂移资产列表（JSON）',
    sealed_store_mark  BIGINT        NOT NULL   DEFAULT 0,
    live_store_mark    BIGINT        NOT NULL   DEFAULT 0,
    detected_at        DATETIME(3)   NOT NULL,
    created_at         DATETIME(3)   NOT NULL,
    PRIMARY KEY (proof_id),
    INDEX idx_psp_snap_gen_id  (snap_gen_id),
    INDEX idx_psp_snapshot_id  (snapshot_id),
    INDEX idx_psp_detected_at  (detected_at),
    INDEX idx_psp_kind         (kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin
  COMMENT='por 活账漂移证明；只追加，不修改，不删除。';
