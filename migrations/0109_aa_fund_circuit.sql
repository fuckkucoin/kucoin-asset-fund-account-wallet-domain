-- 功能：资金域统一限流熔断总线之信号、熔断器、策略版本、案件与半开探针。
-- 重点：熔断键 row_key 恒为 fc: 前缀，与 dep:/wd: 速度键并存互不覆盖；策略文档不含解冻动作。
-- 风险点：熔断器落库失败不得当成未熔断；案件未双人结案前不得清空熔断行。
-- 作者：Kinsley

CREATE TABLE IF NOT EXISTS aa_fund_signals (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  path VARCHAR(32) NOT NULL DEFAULT '',
  scope_key VARCHAR(128) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  weight INT NOT NULL DEFAULT 0,
  source_key VARCHAR(160) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_fund_sig_scope (scope_key, created_at),
  KEY idx_aa_fund_sig_user (user_id, created_at),
  KEY idx_aa_fund_sig_kind (kind, created_at)
);

CREATE TABLE IF NOT EXISTS aa_fund_circuits (
  row_key VARCHAR(128) NOT NULL PRIMARY KEY,
  scope VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  score INT NOT NULL DEFAULT 0,
  severity VARCHAR(16) NOT NULL DEFAULT 'none',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  trips INT NOT NULL DEFAULT 0,
  probes_issued INT NOT NULL DEFAULT 0,
  forced TINYINT NOT NULL DEFAULT 0,
  cross_path TINYINT NOT NULL DEFAULT 0,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  case_id VARCHAR(64) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  opened_at DATETIME(3) NULL,
  half_open_at DATETIME(3) NULL,
  closed_at DATETIME(3) NULL,
  next_advance_at DATETIME(3) NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_fund_cir_state (state, updated_at),
  KEY idx_aa_fund_cir_scope (scope, state)
);

CREATE TABLE IF NOT EXISTS aa_fund_policies (
  version INT NOT NULL PRIMARY KEY,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  doc_json TEXT NOT NULL,
  updated_at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_fund_cases (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  circuit_key VARCHAR(128) NOT NULL,
  scope VARCHAR(16) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  score INT NOT NULL DEFAULT 0,
  severity VARCHAR(16) NOT NULL DEFAULT 'none',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  opened_by VARCHAR(64) NOT NULL DEFAULT '',
  reviewer VARCHAR(64) NOT NULL DEFAULT '',
  clear_req VARCHAR(64) NOT NULL DEFAULT '',
  clear_votes VARCHAR(255) NOT NULL DEFAULT '',
  audit_json TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_fund_case_key (circuit_key, state),
  KEY idx_aa_fund_case_state (state, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_fund_probes (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  circuit_key VARCHAR(128) NOT NULL,
  path VARCHAR(32) NOT NULL,
  state VARCHAR(16) NOT NULL,
  token VARCHAR(64) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  round INT NOT NULL DEFAULT 0,
  note VARCHAR(255) NOT NULL DEFAULT '',
  issued_at DATETIME(3) NOT NULL,
  expires_at DATETIME(3) NULL,
  decided_at DATETIME(3) NULL,
  KEY idx_aa_fund_probe_key (circuit_key, state),
  KEY idx_aa_fund_probe_state (state, issued_at)
);
