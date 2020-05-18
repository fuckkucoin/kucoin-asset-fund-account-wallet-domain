-- 多链手续费抽象、动态报价版本与出金手续费储备入账。
-- 表前缀 fee_ 避免与历史表冲突。
-- 金额列 DECIMAL(36,18)；时间列 DATETIME(3)。
-- 作者：Kinsley
-- 不改 aa_fee_schedules、transfer、swap 任何已有列和约束。

CREATE TABLE IF NOT EXISTS fee_oracle_sources (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  name         VARCHAR(64)  NOT NULL DEFAULT '',
  kind         VARCHAR(16)  NOT NULL DEFAULT 'http',
  chain        VARCHAR(16)  NOT NULL,
  family       VARCHAR(16)  NOT NULL,
  endpoint     VARCHAR(512) NOT NULL DEFAULT '',
  weight       INT          NOT NULL DEFAULT 1,
  enabled      TINYINT      NOT NULL DEFAULT 1,
  stale_sec    INT          NOT NULL DEFAULT 120,
  note         VARCHAR(255) NOT NULL DEFAULT '',
  created_at   DATETIME(3)  NOT NULL,
  updated_at   DATETIME(3)  NOT NULL,
  KEY idx_fos_chain (chain, enabled)
);

CREATE TABLE IF NOT EXISTS fee_oracle_ticks (
  id           VARCHAR(64)    NOT NULL PRIMARY KEY,
  source_id    VARCHAR(64)    NOT NULL DEFAULT '',
  chain        VARCHAR(16)    NOT NULL,
  family       VARCHAR(16)    NOT NULL,
  fee_rate     DECIMAL(36,18) NOT NULL DEFAULT 0,
  fee_unit     VARCHAR(16)    NOT NULL DEFAULT '',
  gas_limit    BIGINT         NOT NULL DEFAULT 0,
  base_fee     DECIMAL(36,18) NOT NULL DEFAULT 0,
  priority     DECIMAL(36,18) NOT NULL DEFAULT 0,
  energy       BIGINT         NOT NULL DEFAULT 0,
  bandwidth    BIGINT         NOT NULL DEFAULT 0,
  native_fee   DECIMAL(36,18) NOT NULL DEFAULT 0,
  usd_fee      DECIMAL(36,18) NOT NULL DEFAULT 0,
  ticked_at    DATETIME(3)    NOT NULL,
  recorded_at  DATETIME(3)    NOT NULL,
  KEY idx_fot_chain (chain, ticked_at),
  KEY idx_fot_src   (source_id, ticked_at)
);

CREATE TABLE IF NOT EXISTS fee_quote_versions (
  id           VARCHAR(64)    NOT NULL PRIMARY KEY,
  withdraw_id  VARCHAR(64)    NOT NULL DEFAULT '',
  chain        VARCHAR(16)    NOT NULL,
  family       VARCHAR(16)    NOT NULL,
  asset        VARCHAR(16)    NOT NULL,
  gross        DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_fee    DECIMAL(36,18) NOT NULL DEFAULT 0,
  quote_net    DECIMAL(36,18) NOT NULL DEFAULT 0,
  fee_rate     DECIMAL(36,18) NOT NULL DEFAULT 0,
  gas_limit    BIGINT         NOT NULL DEFAULT 0,
  base_fee     DECIMAL(36,18) NOT NULL DEFAULT 0,
  priority     DECIMAL(36,18) NOT NULL DEFAULT 0,
  native_fee   DECIMAL(36,18) NOT NULL DEFAULT 0,
  tick_id      VARCHAR(64)    NOT NULL DEFAULT '',
  tick_json    MEDIUMTEXT     NOT NULL DEFAULT '',
  schedule_id  VARCHAR(64)    NOT NULL DEFAULT '',
  bps          INT            NOT NULL DEFAULT 0,
  state        VARCHAR(16)    NOT NULL DEFAULT 'active',
  expires_at   DATETIME(3)    NOT NULL,
  bound_at     DATETIME(3),
  created_at   DATETIME(3)    NOT NULL,
  KEY idx_fqv_wd  (withdraw_id, state),
  KEY idx_fqv_exp (expires_at, state),
  KEY idx_fqv_created (created_at)
);

CREATE TABLE IF NOT EXISTS fee_reserves (
  id              VARCHAR(64)    NOT NULL PRIMARY KEY,
  withdraw_id     VARCHAR(64)    NOT NULL,
  version_id      VARCHAR(64)    NOT NULL DEFAULT '',
  user_id         VARCHAR(64)    NOT NULL,
  asset           VARCHAR(16)    NOT NULL,
  chain           VARCHAR(16)    NOT NULL,
  reserved        DECIMAL(36,18) NOT NULL DEFAULT 0,
  actual          DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta           DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta_dir       VARCHAR(8)     NOT NULL DEFAULT 'zero',
  state           VARCHAR(16)    NOT NULL DEFAULT 'reserved',
  ledger_reserve  VARCHAR(64)    NOT NULL DEFAULT '',
  ledger_settle   VARCHAR(64)    NOT NULL DEFAULT '',
  ledger_delta    VARCHAR(64)    NOT NULL DEFAULT '',
  settled_at      DATETIME(3),
  created_at      DATETIME(3)    NOT NULL,
  updated_at      DATETIME(3)    NOT NULL,
  UNIQUE KEY uk_fr_wd (withdraw_id),
  KEY idx_fr_state (state, created_at),
  KEY idx_fr_user  (user_id, state),
  KEY idx_fr_ver   (version_id)
);

CREATE TABLE IF NOT EXISTS fee_settlements (
  id                  VARCHAR(64)    NOT NULL PRIMARY KEY,
  reserve_id          VARCHAR(64)    NOT NULL,
  withdraw_id         VARCHAR(64)    NOT NULL,
  version_id          VARCHAR(64)    NOT NULL DEFAULT '',
  user_id             VARCHAR(64)    NOT NULL,
  asset               VARCHAR(16)    NOT NULL,
  chain               VARCHAR(16)    NOT NULL,
  reserved_fee        DECIMAL(36,18) NOT NULL DEFAULT 0,
  actual_fee          DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta_amt           DECIMAL(36,18) NOT NULL DEFAULT 0,
  delta_bps           INT            NOT NULL DEFAULT 0,
  tolerance_bps       INT            NOT NULL DEFAULT 50,
  state               VARCHAR(16)    NOT NULL DEFAULT 'pending',
  journal_note        VARCHAR(255)   NOT NULL DEFAULT '',
  ledger_compensation VARCHAR(64)    NOT NULL DEFAULT '',
  actor               VARCHAR(64)    NOT NULL DEFAULT '',
  settled_at          DATETIME(3),
  created_at          DATETIME(3)    NOT NULL,
  updated_at          DATETIME(3)    NOT NULL,
  UNIQUE KEY uk_fs_wd (withdraw_id),
  KEY idx_fs_state    (state, created_at),
  KEY idx_fs_res      (reserve_id),
  KEY idx_fs_user     (user_id, state)
);

CREATE TABLE IF NOT EXISTS fee_chain_specs (
  id                      VARCHAR(64)    NOT NULL PRIMARY KEY,
  chain                   VARCHAR(16)    NOT NULL,
  family                  VARCHAR(16)    NOT NULL,
  default_fee_rate_sat    DECIMAL(36,18) NOT NULL DEFAULT 0,
  default_vbytes          BIGINT         NOT NULL DEFAULT 0,
  default_base_fee_gwei   DECIMAL(36,18) NOT NULL DEFAULT 0,
  default_priority_gwei   DECIMAL(36,18) NOT NULL DEFAULT 0,
  default_gas_limit       BIGINT         NOT NULL DEFAULT 0,
  default_energy_units    BIGINT         NOT NULL DEFAULT 0,
  default_energy_price_sun DECIMAL(36,18) NOT NULL DEFAULT 0,
  default_bandwidth_bytes BIGINT         NOT NULL DEFAULT 0,
  default_bw_price_sun    DECIMAL(36,18) NOT NULL DEFAULT 0,
  default_base_lamports   BIGINT         NOT NULL DEFAULT 0,
  default_compute_units   BIGINT         NOT NULL DEFAULT 0,
  default_micro_lamports_cu BIGINT       NOT NULL DEFAULT 0,
  min_fee_user            VARCHAR(64)    NOT NULL DEFAULT '',
  max_fee_user            VARCHAR(64)    NOT NULL DEFAULT '',
  median_fee_native       DECIMAL(36,18) NOT NULL DEFAULT 0,
  active                  TINYINT        NOT NULL DEFAULT 1,
  created_at              DATETIME(3)    NOT NULL,
  updated_at              DATETIME(3)    NOT NULL,
  UNIQUE KEY uk_fcs_chain (chain),
  KEY idx_fcs_active (active)
);

CREATE TABLE IF NOT EXISTS fee_oracle_policy (
  id                  VARCHAR(64)  NOT NULL PRIMARY KEY,
  version             INT          NOT NULL DEFAULT 1,
  global_stale_sec    INT          NOT NULL DEFAULT 120,
  global_deviation_bps INT         NOT NULL DEFAULT 500,
  quote_ttl_sec       INT          NOT NULL DEFAULT 600,
  fallback_enabled    TINYINT      NOT NULL DEFAULT 0,
  fallback_audit_note VARCHAR(512) NOT NULL DEFAULT '',
  active              TINYINT      NOT NULL DEFAULT 1,
  created_by          VARCHAR(64)  NOT NULL DEFAULT '',
  created_at          DATETIME(3)  NOT NULL,
  KEY idx_fop_active (active, version)
);

CREATE TABLE IF NOT EXISTS fee_reserve_audit (
  id          VARCHAR(64)  NOT NULL PRIMARY KEY,
  reserve_id  VARCHAR(64)  NOT NULL,
  withdraw_id VARCHAR(64)  NOT NULL,
  event       VARCHAR(32)  NOT NULL,
  from_state  VARCHAR(16)  NOT NULL DEFAULT '',
  to_state    VARCHAR(16)  NOT NULL DEFAULT '',
  actor       VARCHAR(64)  NOT NULL DEFAULT '',
  note        VARCHAR(255) NOT NULL DEFAULT '',
  at          DATETIME(3)  NOT NULL,
  KEY idx_fra_res (reserve_id, at),
  KEY idx_fra_wd  (withdraw_id, at)
);
