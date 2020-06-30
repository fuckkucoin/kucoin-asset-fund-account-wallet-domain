-- 法币凭证不平衡挡写引擎表族。
-- 含：fiat_jnl_gens（世代快照）、fiat_imbalance_cases（不平衡案）、
--      fiat_imbalance_proofs（证明包）、fiat_clear_approvals（四眼确认）。
-- 不含：法币上分 Confirm（fiatadd 主链）、轧差编排（fiatrail）、
--        上币/行情/网关相关表。
--
-- 作者：Eco
-- 日期：2025.11.08

-- ── fiat_jnl_gens ─────────────────────────────────────────────────────────────
-- 记录 fiatjnl 每次 Post/Adjust 的世代快照，供 fiatbalgate 扫描器绑定世代。
-- seq 严格单调递增，不可跳跃或回退；(seq) 全局唯一。
-- INSERT IGNORE 保证幂等写入；不提供 DELETE。
CREATE TABLE IF NOT EXISTS fiat_jnl_gens (
    gen_id          VARCHAR(36)     NOT NULL,
    seq             BIGINT UNSIGNED NOT NULL,   -- 单调递增世代号（从 fiatjnl.GenTracker）
    entry_id        VARCHAR(36)     NOT NULL,   -- 触发抬升的 fiatjnl.Entry.ID
    kind            VARCHAR(16)     NOT NULL,   -- post / adjust
    asset           VARCHAR(16)     NOT NULL,   -- 资产代码（大写）
    bumped_at       DATETIME(3)     NOT NULL,   -- 世代抬升时间（UTC，来自 fiatjnl）
    synced_at       DATETIME(3)     NOT NULL,   -- 写入本表时间（UTC）
    PRIMARY KEY (gen_id),
    UNIQUE KEY uq_fjg_seq (seq),
    KEY idx_fjg_asset_seq (asset, seq),
    KEY idx_fjg_entry_id (entry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='法币凭证世代快照';

-- ── fiat_imbalance_cases ──────────────────────────────────────────────────────
-- 记录不平衡发现事件与处置状态机（open → blocking → cleared）。
-- 同 asset 最多一条 open/blocking 案（由应用层幂等保证）。
-- 清障后状态不可逆转（cleared 为终态）。
CREATE TABLE IF NOT EXISTS fiat_imbalance_cases (
    case_id             VARCHAR(36)     NOT NULL,
    asset               VARCHAR(16)     NOT NULL,   -- 资产代码（大写）
    scan_gen_seq        BIGINT UNSIGNED NOT NULL,   -- 开案时绑定的世代号
    detected_entry_id   VARCHAR(36)     NOT NULL DEFAULT '', -- 触发失衡的凭证 ID
    sum_debit           DECIMAL(36,18)  NOT NULL DEFAULT 0,  -- 扫描区间 ΣDebit
    sum_credit          DECIMAL(36,18)  NOT NULL DEFAULT 0,  -- 扫描区间 ΣCredit
    imbalance_kind      VARCHAR(32)     NOT NULL,   -- balanced_mismatch / conserve_fail / empty_lines
    state               VARCHAR(16)     NOT NULL DEFAULT 'open', -- open / blocking / cleared
    scanner_id          VARCHAR(64)     NOT NULL DEFAULT '', -- 发现不平衡的 Worker/Scanner ID
    opened_at           DATETIME(3)     NOT NULL,
    blocking_at         DATETIME(3)     NULL,
    cleared_at          DATETIME(3)     NULL,
    cleared_by          VARCHAR(256)    NOT NULL DEFAULT '',
    clear_note          VARCHAR(1024)   NOT NULL DEFAULT '',
    proof_id            VARCHAR(36)     NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (case_id),
    KEY idx_fic_asset_state (asset, state),
    KEY idx_fic_scan_gen (scan_gen_seq),
    KEY idx_fic_opened_at (opened_at),
    KEY idx_fic_state_opened (state, opened_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='法币凭证不平衡案';

-- ── fiat_imbalance_proofs ─────────────────────────────────────────────────────
-- 记录不平衡证明包；每条 case 最多一条 proof（uq_proof_case_id）。
-- 只追加，内容字段不可修改；state 仅允许 pending → exported → verified 推进。
CREATE TABLE IF NOT EXISTS fiat_imbalance_proofs (
    proof_id            VARCHAR(36)     NOT NULL,
    case_id             VARCHAR(36)     NOT NULL,   -- 关联的不平衡案 ID
    asset               VARCHAR(16)     NOT NULL,
    scan_gen_seq        BIGINT UNSIGNED NOT NULL,
    entries_snapshot    MEDIUMTEXT      NOT NULL DEFAULT '', -- 凭证 ID 列表 JSON
    lines_snapshot      MEDIUMTEXT      NOT NULL DEFAULT '', -- 凭证 Lines JSON
    sum_debit           DECIMAL(36,18)  NOT NULL DEFAULT 0,
    sum_credit          DECIMAL(36,18)  NOT NULL DEFAULT 0,
    digest_sha256       VARCHAR(64)     NOT NULL DEFAULT '', -- SHA-256 hex 摘要
    state               VARCHAR(16)     NOT NULL DEFAULT 'pending', -- pending / exported / verified
    exported_at         DATETIME(3)     NULL,
    verified_at         DATETIME(3)     NULL,
    verified_by         VARCHAR(256)    NOT NULL DEFAULT '',
    created_at          DATETIME(3)     NOT NULL,
    updated_at          DATETIME(3)     NOT NULL,
    PRIMARY KEY (proof_id),
    UNIQUE KEY uq_proof_case_id (case_id),
    KEY idx_fip_asset_state (asset, state),
    KEY idx_fip_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='法币凭证不平衡证明包';

-- ── fiat_clear_approvals ──────────────────────────────────────────────────────
-- 记录四眼清障确认记录（每 step 一行，按时间升序）。
-- 只追加，不删除；同 case 允许多条记录（最少两条：step1 + step2）。
CREATE TABLE IF NOT EXISTS fiat_clear_approvals (
    approval_id     VARCHAR(36)     NOT NULL,
    case_id         VARCHAR(36)     NOT NULL,
    step            VARCHAR(16)     NOT NULL,   -- first_approved / done
    actor           VARCHAR(256)    NOT NULL,   -- ops user ID
    note            VARCHAR(1024)   NOT NULL DEFAULT '',
    created_at      DATETIME(3)     NOT NULL,
    PRIMARY KEY (approval_id),
    KEY idx_fca_case_id (case_id),
    KEY idx_fca_actor (actor),
    KEY idx_fca_case_created (case_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='法币凭证不平衡四眼清障确认';
