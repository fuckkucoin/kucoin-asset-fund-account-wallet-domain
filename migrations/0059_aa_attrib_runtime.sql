-- 归因结论、信封、碰撞案和未绑定币工单。
-- 不改对账三方公式，不改审计仓表，不改 dep: 速度键。

-- 作者：Kinsley
CREATE TABLE IF NOT EXISTS aa_attrib_decision (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  disposition VARCHAR(16) NOT NULL,
  credit_intent INT NOT NULL DEFAULT 0,
  confidence INT NOT NULL DEFAULT 0,
  policy_version VARCHAR(64) NOT NULL DEFAULT '',
  family VARCHAR(16) NOT NULL DEFAULT '',
  reasons VARCHAR(512) NOT NULL DEFAULT '',
  envelope_id VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_attrib_dec_tx (chain, tx_hash, output_index)
);

CREATE TABLE IF NOT EXISTS aa_attrib_envelope (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  decision_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL DEFAULT '',
  output_index INT NOT NULL DEFAULT 0,
  prev_hash VARCHAR(64) NOT NULL DEFAULT '',
  payload_hash VARCHAR(64) NOT NULL,
  credit_intent INT NOT NULL DEFAULT 0,
  disposition VARCHAR(16) NOT NULL,
  policy_version VARCHAR(64) NOT NULL DEFAULT '',
  reasons VARCHAR(512) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_attrib_env_chain (chain, created_at)
);

CREATE TABLE IF NOT EXISTS aa_attrib_collision (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  namespace VARCHAR(191) NOT NULL DEFAULT '',
  fold_key VARCHAR(191) NOT NULL,
  state VARCHAR(16) NOT NULL,
  origin_user VARCHAR(64) NOT NULL DEFAULT '',
  winner VARCHAR(64) NOT NULL DEFAULT '',
  claimants VARCHAR(512) NOT NULL DEFAULT '',
  reason VARCHAR(255) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_attrib_fold (chain, namespace, fold_key)
);

CREATE TABLE IF NOT EXISTS aa_attrib_coin_ticket (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  vout INT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  amount VARCHAR(64) NOT NULL DEFAULT '',
  address VARCHAR(128) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_attrib_coin (chain, tx_hash)
);
