-- 托管熔断 Clear↔Engage 夹缝充提滑窗收口引擎迁移。
--
-- 表：
--   custody_halt_epochs          — 按作用域持久化 halt_epoch
--   custody_slip_proofs          — SlipProof 滑窗证明仓
--   custody_clear_window_cases   — Clear 成功后的封窗案
--   custody_halt_intents         — 绑定世代的充/提/清扫在途意图
--   custody_halt_quorum          — Clear 法定人数投票（非 r10 误清四眼）
--   custody_halt_epoch_audits    — 世代审计轨迹
--
-- 原则：
--   1. generation 单调递增，禁止回拨。
--   2. Clear 窗内充/提/清扫拒写并落 SlipProof。
--   3. Engage 与 Clear 互斥；EngageOnly 桥不得 Clear。
--   4. 不引用误清审批 / 禁提策略 / haltclear 外键。
--
-- 迁移标识：0143（自 destin4 0074 重编号，避开既有 0074_aa_wd_shard_plan）
-- 作者：Styx
-- 日期：2021.09.08

-- +migrate Up

CREATE TABLE IF NOT EXISTS custody_halt_epochs (
    epoch_id        VARCHAR(64)   NOT NULL COMMENT '熔断世代记录 ID',
    scope_key       VARCHAR(128)  NOT NULL COMMENT 'global|chain:X|asset:Y|pair:X/Y',
    scope           VARCHAR(16)   NOT NULL DEFAULT 'global',
    chain           VARCHAR(32)   NOT NULL DEFAULT '',
    asset           VARCHAR(32)   NOT NULL DEFAULT '',
    generation      BIGINT        NOT NULL COMMENT '单调 halt_epoch',
    state           VARCHAR(16)   NOT NULL DEFAULT 'idle'
                        COMMENT 'idle|engaged|clearing|cleared|reengaged',
    prev_gen        BIGINT        NOT NULL DEFAULT 0,
    lane_open       TINYINT(1)    NOT NULL DEFAULT 0,
    mutex_holder    VARCHAR(128)  NULL,
    mutex_kind      VARCHAR(16)   NULL COMMENT 'engage|clear',
    actor           VARCHAR(128)  NOT NULL DEFAULT '',
    reason          VARCHAR(512)  NOT NULL DEFAULT '',
    quorum_id       VARCHAR(64)   NULL,
    idem_key        VARCHAR(128)  NULL,
    window_case_id  VARCHAR(64)   NULL,
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                        ON UPDATE CURRENT_TIMESTAMP(3),
    cleared_at      DATETIME(3)   NULL,
    engaged_at      DATETIME(3)   NULL,
    PRIMARY KEY (epoch_id),
    UNIQUE KEY uk_che_scope (scope_key),
    KEY idx_che_state (state, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='托管熔断世代（Engage/Clear 升降 halt_epoch）';

CREATE TABLE IF NOT EXISTS custody_slip_proofs (
    proof_id        VARCHAR(64)   NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL DEFAULT '',
    scope_key       VARCHAR(128)  NOT NULL,
    scope           VARCHAR(16)   NOT NULL DEFAULT 'global',
    chain           VARCHAR(32)   NOT NULL DEFAULT '',
    asset           VARCHAR(32)   NOT NULL DEFAULT '',
    generation      BIGINT        NOT NULL,
    carried_gen     BIGINT        NOT NULL DEFAULT 0,
    kind            VARCHAR(64)   NOT NULL COMMENT 'clear_window_deposit 等',
    state           VARCHAR(16)   NOT NULL DEFAULT 'open' COMMENT 'open|proven|closed',
    lane            VARCHAR(16)   NOT NULL DEFAULT 'deposit',
    intent_id       VARCHAR(64)   NULL,
    case_id         VARCHAR(64)   NULL,
    actor           VARCHAR(128)  NULL,
    summary         TEXT          NOT NULL,
    fail_closed     TINYINT(1)    NOT NULL DEFAULT 1,
    disposition     VARCHAR(16)   NOT NULL DEFAULT 'reject',
    digest          VARCHAR(64)   NOT NULL DEFAULT '',
    opened_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    proven_at       DATETIME(3)   NULL,
    closed_at       DATETIME(3)   NULL,
    exported_at     DATETIME(3)   NULL,
    PRIMARY KEY (proof_id),
    KEY idx_csp_scope (scope_key, state),
    KEY idx_csp_kind (kind, opened_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Clear↔Engage 滑窗证明仓';

CREATE TABLE IF NOT EXISTS custody_clear_window_cases (
    case_id         VARCHAR(64)   NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL,
    scope_key       VARCHAR(128)  NOT NULL,
    scope           VARCHAR(16)   NOT NULL DEFAULT 'global',
    chain           VARCHAR(32)   NOT NULL DEFAULT '',
    asset           VARCHAR(32)   NOT NULL DEFAULT '',
    clear_gen       BIGINT        NOT NULL,
    prev_gen        BIGINT        NOT NULL DEFAULT 0,
    state           VARCHAR(16)   NOT NULL DEFAULT 'open' COMMENT 'open|sealing|sealed|conflict',
    revoked_intents INT           NOT NULL DEFAULT 0,
    slip_count      INT           NOT NULL DEFAULT 0,
    actor           VARCHAR(128)  NOT NULL DEFAULT '',
    reason          VARCHAR(512)  NOT NULL DEFAULT '',
    opened_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    sealed_at       DATETIME(3)   NULL,
    PRIMARY KEY (case_id),
    KEY idx_ccw_scope (scope_key, state),
    KEY idx_ccw_epoch (epoch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Clear 成功后封窗案';

CREATE TABLE IF NOT EXISTS custody_halt_intents (
    intent_id       VARCHAR(64)   NOT NULL,
    scope_key       VARCHAR(128)  NOT NULL,
    scope           VARCHAR(16)   NOT NULL DEFAULT 'global',
    chain           VARCHAR(32)   NOT NULL DEFAULT '',
    asset           VARCHAR(32)   NOT NULL DEFAULT '',
    lane            VARCHAR(16)   NOT NULL COMMENT 'deposit|withdraw|sweep',
    generation      BIGINT        NOT NULL,
    state           VARCHAR(16)   NOT NULL DEFAULT 'allowed'
                        COMMENT 'allowed|revoked|broadcast|credited|rejected',
    account         VARCHAR(64)   NULL,
    asset_code      VARCHAR(32)   NULL,
    amount          VARCHAR(64)   NULL,
    idem_key        VARCHAR(128)  NULL,
    allow_ticket    VARCHAR(128)  NULL,
    proof_id        VARCHAR(64)   NULL,
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (intent_id),
    UNIQUE KEY uk_chi_idem (idem_key),
    KEY idx_chi_scope (scope_key, state, generation)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='绑定 halt_epoch 的在途充提意图';

CREATE TABLE IF NOT EXISTS custody_halt_quorum (
    quorum_id       VARCHAR(64)   NOT NULL,
    epoch_id        VARCHAR(64)   NOT NULL,
    scope_key       VARCHAR(128)  NOT NULL DEFAULT 'global',
    threshold       INT           NOT NULL DEFAULT 2,
    votes_json      TEXT          NOT NULL,
    satisfied       TINYINT(1)    NOT NULL DEFAULT 0,
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (quorum_id),
    UNIQUE KEY uk_chq_epoch (epoch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Clear 法定人数（非误清四眼主链）';

CREATE TABLE IF NOT EXISTS custody_halt_epoch_audits (
    event_id        VARCHAR(64)   NOT NULL,
    kind            VARCHAR(32)   NOT NULL,
    scope_key       VARCHAR(128)  NOT NULL,
    actor           VARCHAR(128)  NOT NULL DEFAULT '',
    detail          TEXT          NULL,
    generation      BIGINT        NOT NULL DEFAULT 0,
    occurred_at     DATETIME(3)   NOT NULL,
    PRIMARY KEY (event_id),
    KEY idx_chea_scope (scope_key, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='熔断世代审计轨迹';

-- +migrate Down

DROP TABLE IF EXISTS custody_halt_epoch_audits;
DROP TABLE IF EXISTS custody_halt_quorum;
DROP TABLE IF EXISTS custody_halt_intents;
DROP TABLE IF EXISTS custody_clear_window_cases;
DROP TABLE IF EXISTS custody_slip_proofs;
DROP TABLE IF EXISTS custody_halt_epochs;
