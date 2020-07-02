-- 托管 FeeBump 世代、广播占用与 RBF 孤儿证明表族。
-- 含：custody_feebump_gens、custody_broadcast_occs、custody_rbf_orphan_proofs、
--     custody_feebump_audits、custody_feebump_epochs。
-- 不含：冷签仪式批次表、Decide 租约表、HSM 槽表、链上 RBF 编解码表。
--
-- 作者：Eco
-- 日期：2025.08.22

-- ── custody_feebump_gens ─────────────────────────────────────────────────────
-- 状态：signed → broadcast → superseded | final；signed|broadcast → blocked。
-- 同 gen_id 主键；idempotency_key 唯一。无 DELETE。
CREATE TABLE IF NOT EXISTS custody_feebump_gens (
    gen_id              VARCHAR(36)     NOT NULL,
    transfer_id         VARCHAR(64)     NOT NULL,
    generation          BIGINT          NOT NULL DEFAULT 1,
    prev_broadcast_hash VARCHAR(64)     NOT NULL DEFAULT '',
    new_broadcast_hash  VARCHAR(64)     NOT NULL DEFAULT '',
    new_fee             VARCHAR(64)     NOT NULL,
    prev_fee            VARCHAR(64)     NOT NULL DEFAULT '',
    destination_tag     VARCHAR(128)    NOT NULL DEFAULT '',
    occupancy_fp        VARCHAR(64)     NOT NULL DEFAULT '',
    chain_code          VARCHAR(16)     NOT NULL DEFAULT '',
    dest_to             VARCHAR(191)    NOT NULL DEFAULT '',
    amount              VARCHAR(64)     NOT NULL DEFAULT '',
    asset               VARCHAR(16)     NOT NULL DEFAULT '',
    canon_digest        VARCHAR(64)     NOT NULL DEFAULT '',
    sig_digest          VARCHAR(64)     NOT NULL DEFAULT '',
    state               VARCHAR(16)     NOT NULL DEFAULT 'signed',
    occ_id              VARCHAR(36)     NOT NULL DEFAULT '',
    idempotency_key     VARCHAR(128)    NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    signed_at           DATETIME(3)     NOT NULL,
    broadcast_at        DATETIME(3)     NULL,
    superseded_at       DATETIME(3)     NULL,
    final_at            DATETIME(3)     NULL,
    blocked_at          DATETIME(3)     NULL,
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (gen_id),
    UNIQUE KEY uq_fbg_idem (idempotency_key),
    KEY idx_fbg_xfer_gen (transfer_id, generation),
    KEY idx_fbg_state (state, signed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='托管抬费世代 feebump_gen';

-- ── custody_broadcast_occs ───────────────────────────────────────────────────
-- 状态：held → retired | orphaned。无 DELETE。
CREATE TABLE IF NOT EXISTS custody_broadcast_occs (
    occ_id              VARCHAR(36)     NOT NULL,
    gen_id              VARCHAR(36)     NOT NULL,
    transfer_id         VARCHAR(64)     NOT NULL,
    generation          BIGINT          NOT NULL DEFAULT 1,
    broadcast_hash      VARCHAR(64)     NOT NULL DEFAULT '',
    occupancy_fp        VARCHAR(64)     NOT NULL DEFAULT '',
    asset               VARCHAR(16)     NOT NULL DEFAULT '',
    amount              VARCHAR(64)     NOT NULL DEFAULT '',
    fee                 VARCHAR(64)     NOT NULL DEFAULT '',
    state               VARCHAR(16)     NOT NULL DEFAULT 'held',
    held_at             DATETIME(3)     NOT NULL,
    retired_at          DATETIME(3)     NULL,
    orphaned_at         DATETIME(3)     NULL,
    expire_at           DATETIME(3)     NOT NULL,
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (occ_id),
    UNIQUE KEY uq_fbo_gen (gen_id),
    KEY idx_fbo_xfer_state (transfer_id, state),
    KEY idx_fbo_expire (state, expire_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='托管广播热占 occupancy';

-- ── custody_rbf_orphan_proofs ────────────────────────────────────────────────
-- INSERT IGNORE；无 UPDATE 除 CAS 关单；无 DELETE。
CREATE TABLE IF NOT EXISTS custody_rbf_orphan_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    transfer_id         VARCHAR(64)     NOT NULL DEFAULT '',
    gen_id              VARCHAR(36)     NOT NULL DEFAULT '',
    prev_gen_id         VARCHAR(36)     NOT NULL DEFAULT '',
    occ_id              VARCHAR(36)     NOT NULL DEFAULT '',
    kind                VARCHAR(32)     NOT NULL,
    state               VARCHAR(16)     NOT NULL DEFAULT 'open',
    quoted_hash         VARCHAR(64)     NOT NULL DEFAULT '',
    live_hash           VARCHAR(64)     NOT NULL DEFAULT '',
    quoted_gen          BIGINT          NOT NULL DEFAULT 0,
    live_gen            BIGINT          NOT NULL DEFAULT 0,
    held_occ_count      INT             NOT NULL DEFAULT 0,
    detail              VARCHAR(512)    NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    at_ts               DATETIME(3)     NOT NULL,
    closed_at           DATETIME(3)     NULL,
    created_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    KEY idx_fbp_xfer (transfer_id, state),
    KEY idx_fbp_kind (kind, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='RBF 孤儿/拒广播证明';

-- ── custody_feebump_audits ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS custody_feebump_audits (
    audit_id            VARCHAR(36)     NOT NULL,
    gen_id              VARCHAR(36)     NOT NULL DEFAULT '',
    transfer_id         VARCHAR(64)     NOT NULL DEFAULT '',
    action              VARCHAR(32)     NOT NULL DEFAULT '',
    from_state          VARCHAR(16)     NOT NULL DEFAULT '',
    to_state            VARCHAR(16)     NOT NULL DEFAULT '',
    actor               VARCHAR(64)     NOT NULL DEFAULT '',
    detail              VARCHAR(256)    NOT NULL DEFAULT '',
    at_ts               DATETIME(3)     NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_fba_gen (gen_id, at_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='抬费世代审计';

-- ── custody_feebump_epochs ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS custody_feebump_epochs (
    transfer_id         VARCHAR(64)     NOT NULL,
    generation          BIGINT          NOT NULL DEFAULT 0,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (transfer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='transfer 抬费世代计数';
