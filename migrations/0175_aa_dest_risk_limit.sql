-- 出金目的地风险图谱与动态限额。
-- 节点、边、簇、策略、占用、覆写、审计分表。
-- 不改 KYT 厂商表，不改审批矩阵表。
-- 作者：Axel

CREATE TABLE IF NOT EXISTS aa_dest_node (
  node_key VARCHAR(191) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  cluster_id VARCHAR(191) NOT NULL,
  tier VARCHAR(16) NOT NULL DEFAULT 'low',
  score VARCHAR(32) NOT NULL DEFAULT '0',
  kyt_hint VARCHAR(16) NOT NULL DEFAULT '',
  label VARCHAR(64) NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1,
  first_seen DATETIME(3) NOT NULL,
  last_seen DATETIME(3) NOT NULL,
  KEY idx_aa_dest_node_cluster (cluster_id)
);

CREATE TABLE IF NOT EXISTS aa_dest_edge (
  edge_id VARCHAR(64) NOT NULL PRIMARY KEY,
  left_key VARCHAR(191) NOT NULL,
  right_key VARCHAR(191) NOT NULL,
  anchor_weight VARCHAR(32) NOT NULL,
  weight VARCHAR(32) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  evidence VARCHAR(255) NOT NULL DEFAULT '',
  half_life_sec INT NOT NULL DEFAULT 3600,
  active INT NOT NULL DEFAULT 1,
  born_at DATETIME(3) NOT NULL,
  anchor_at DATETIME(3) NOT NULL,
  touched_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_dest_edge_pair (left_key, right_key),
  KEY idx_aa_dest_edge_active (active, touched_at)
);

CREATE TABLE IF NOT EXISTS aa_dest_cluster (
  cluster_id VARCHAR(191) NOT NULL,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  member_n INT NOT NULL DEFAULT 0,
  score VARCHAR(32) NOT NULL DEFAULT '0',
  tier VARCHAR(16) NOT NULL DEFAULT 'low',
  max_weight VARCHAR(32) NOT NULL DEFAULT '0',
  reserved_amt VARCHAR(64) NOT NULL DEFAULT '0',
  consumed_amt VARCHAR(64) NOT NULL DEFAULT '0',
  computed_at DATETIME(3) NOT NULL,
  PRIMARY KEY (cluster_id, asset)
);

CREATE TABLE IF NOT EXISTS aa_dest_limit_policy (
  policy_id VARCHAR(64) NOT NULL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  cluster_max VARCHAR(64) NOT NULL,
  user_max VARCHAR(64) NOT NULL,
  dest_max VARCHAR(64) NOT NULL,
  window_sec INT NOT NULL DEFAULT 86400,
  low_mul VARCHAR(16) NOT NULL DEFAULT '1',
  mid_mul VARCHAR(16) NOT NULL DEFAULT '0.5',
  high_mul VARCHAR(16) NOT NULL DEFAULT '0.25',
  severe_mul VARCHAR(16) NOT NULL DEFAULT '0.1',
  version BIGINT NOT NULL DEFAULT 1,
  enabled INT NOT NULL DEFAULT 1,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_dest_pol_asset (asset, enabled)
);

CREATE TABLE IF NOT EXISTS aa_dest_limit_hold (
  hold_id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  cluster_id VARCHAR(191) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  policy_id VARCHAR(64) NOT NULL DEFAULT '',
  tier VARCHAR(16) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  limit_amt VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  reserved_at DATETIME(3) NOT NULL,
  deadline DATETIME(3) NOT NULL,
  closed_at DATETIME(3) NULL,
  KEY idx_aa_dest_hold_user (user_id, asset, state),
  KEY idx_aa_dest_hold_cluster (cluster_id, asset, state),
  KEY idx_aa_dest_hold_req (request_id)
);

CREATE TABLE IF NOT EXISTS aa_dest_limit_override (
  override_id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  cluster_id VARCHAR(191) NOT NULL DEFAULT '',
  extra VARCHAR(64) NOT NULL,
  actor_a VARCHAR(64) NOT NULL,
  actor_b VARCHAR(64) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL,
  state VARCHAR(16) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  confirmed_at DATETIME(3) NULL,
  KEY idx_aa_dest_ov_user (user_id, asset, state)
);

CREATE TABLE IF NOT EXISTS aa_dest_risk_audit (
  audit_id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(32) NOT NULL,
  subject VARCHAR(191) NOT NULL,
  before_v VARCHAR(255) NOT NULL DEFAULT '',
  after_v VARCHAR(255) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  detail VARCHAR(512) NOT NULL DEFAULT '',
  at_time DATETIME(3) NOT NULL,
  KEY idx_aa_dest_audit_kind (kind, at_time)
);
