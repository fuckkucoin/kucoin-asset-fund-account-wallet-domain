-- 回收迟贷记持久互斥与强制围栏世代收口引擎迁移。
--
-- 表：
--   late_credit_gens         — 按 output_key 持久化 late_credit_gen
--   late_credit_race_events  — 可复放竞态事件（open→acked）
--   late_credit_proofs       — LateCreditRaceProof 证明仓
--
-- 原则：
--   1. (chain, tx_hash, output_index) UNIQUE，Occupy/Credit 互斥由应用 CAS + 唯一约束保证。
--   2. generation 单调递增，禁止回拨。
--   3. 写路径 fence_epoch>0 且 fence_token 非空由应用强制；库层不隐式放行。
--   4. 不引用错链回收编排主链外键，隔离边界在应用层。
--
-- 迁移标识：0131（与 main 既有 0070_aa_reclaim 冲突后重编号）
-- 作者：Styx
-- 日期：2021.06.20

-- +migrate Up

CREATE TABLE IF NOT EXISTS late_credit_gens (
    gen_id          VARCHAR(64)   NOT NULL COMMENT '世代记录 ID',
    chain           VARCHAR(32)   NOT NULL COMMENT '链标识',
    tx_hash         VARCHAR(128)  NOT NULL COMMENT '交易哈希',
    output_index    INT           NOT NULL COMMENT '输出序号',
    generation      BIGINT        NOT NULL COMMENT '单调世代号',
    state           VARCHAR(16)   NOT NULL DEFAULT 'free'
                        COMMENT 'free|occupied|credited|reclaimed',
    holder_kind     VARCHAR(32)   NOT NULL DEFAULT '' COMMENT 'occupy|credit|auto_credit',
    holder_actor    VARCHAR(128)  NOT NULL DEFAULT '' COMMENT '持有方',
    occupancy_id    VARCHAR(64)   NULL COMMENT '只读关联 ReclaimOccupancy',
    case_id         VARCHAR(64)   NULL COMMENT '只读关联 ReclaimCase',
    asset           VARCHAR(32)   NULL,
    amount          VARCHAR(64)   NULL,
    fence_epoch     BIGINT        NOT NULL DEFAULT 0 COMMENT '围栏世代',
    fence_token     VARCHAR(128)  NOT NULL DEFAULT '' COMMENT '围栏令牌',
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (gen_id),
    UNIQUE KEY uk_lcg_output (chain, tx_hash, output_index),
    KEY idx_lcg_state (state, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='迟贷记持久世代（Occupy/Credit 互斥）';

CREATE TABLE IF NOT EXISTS late_credit_race_events (
    event_id        VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    tx_hash         VARCHAR(128)  NOT NULL,
    output_index    INT           NOT NULL,
    gen_id          VARCHAR(64)   NOT NULL,
    generation      BIGINT        NOT NULL,
    kind            VARCHAR(64)   NOT NULL COMMENT 'occupy_vs_credit 等',
    state           VARCHAR(16)   NOT NULL DEFAULT 'open' COMMENT 'open|acked',
    actor_a         VARCHAR(128)  NOT NULL DEFAULT '',
    actor_b         VARCHAR(128)  NOT NULL DEFAULT '',
    detail          TEXT          NULL,
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    acked_at        DATETIME(3)   NULL,
    PRIMARY KEY (event_id),
    KEY idx_lcre_state (state, created_at),
    KEY idx_lcre_output (chain, tx_hash, output_index, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='迟贷记竞态事件（重启可 Drain）';

CREATE TABLE IF NOT EXISTS late_credit_proofs (
    proof_id        VARCHAR(64)   NOT NULL,
    chain           VARCHAR(32)   NOT NULL,
    tx_hash         VARCHAR(128)  NOT NULL,
    output_index    INT           NOT NULL,
    gen_id          VARCHAR(64)   NOT NULL,
    generation      BIGINT        NOT NULL,
    event_id        VARCHAR(64)   NOT NULL,
    kind            VARCHAR(64)   NOT NULL,
    occupy_actor    VARCHAR(128)  NOT NULL DEFAULT '',
    credit_actor    VARCHAR(128)  NOT NULL DEFAULT '',
    fail_closed     TINYINT(1)    NOT NULL DEFAULT 1,
    summary         TEXT          NOT NULL,
    opened_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    exported_at     DATETIME(3)   NULL,
    PRIMARY KEY (proof_id),
    KEY idx_lcp_output (chain, tx_hash, output_index),
    KEY idx_lcp_event (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='迟贷记竞态证明仓';

-- +migrate Down

DROP TABLE IF EXISTS late_credit_proofs;
DROP TABLE IF EXISTS late_credit_race_events;
DROP TABLE IF EXISTS late_credit_gens;
