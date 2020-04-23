-- 制裁名单、VIP/资金费率、风险敞口与升级单、运营配置包。 Open 以这些表为权威。

CREATE TABLE IF NOT EXISTS aa_sanctions (
-- 作者：Axel
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  added_at DATETIME(3) NOT NULL,
  PRIMARY KEY (chain, address)
);

CREATE TABLE IF NOT EXISTS aa_vip_schedules (
  id VARCHAR(64) NOT NULL,
  tier VARCHAR(16) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  bps INT NOT NULL,
  active TINYINT NOT NULL DEFAULT 1,
  version INT NOT NULL,
  PRIMARY KEY (tier, kind, asset),
  UNIQUE KEY uk_aa_vip_schedules_id (id)
);

CREATE TABLE IF NOT EXISTS aa_vip_pins (
  user_id VARCHAR(64) NOT NULL PRIMARY KEY,
  tier VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  note VARCHAR(128) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL
);

CREATE TABLE IF NOT EXISTS aa_vip_fills (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  notional VARCHAR(64) NOT NULL,
  day VARCHAR(16) NOT NULL,
  at DATETIME(3) NOT NULL,
  KEY idx_aa_vip_fills_user (user_id, at)
);

CREATE TABLE IF NOT EXISTS aa_fee_schedules (
  id VARCHAR(64) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  bps INT NOT NULL,
  min_fee VARCHAR(64) NOT NULL DEFAULT '',
  max_fee VARCHAR(64) NOT NULL DEFAULT '',
  flat VARCHAR(64) NOT NULL DEFAULT '',
  active TINYINT NOT NULL DEFAULT 1,
  version INT NOT NULL,
  PRIMARY KEY (kind, asset),
  UNIQUE KEY uk_aa_fee_schedules_id (id)
);

CREATE TABLE IF NOT EXISTS aa_risk_caps (
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  max_amt VARCHAR(64) NOT NULL,
  PRIMARY KEY (user_id, asset)
);

CREATE TABLE IF NOT EXISTS aa_risk_items (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  reason VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount VARCHAR(64) NOT NULL DEFAULT '',
  dest VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  KEY idx_aa_risk_items_status (status, opened_at)
);

CREATE TABLE IF NOT EXISTS aa_cfg_packs (
  name VARCHAR(64) NOT NULL,
  version INT NOT NULL,
  max_single VARCHAR(64) NOT NULL DEFAULT '',
  daily_withdraw VARCHAR(64) NOT NULL DEFAULT '',
  hourly_vel INT NOT NULL DEFAULT 0,
  daily_vel INT NOT NULL DEFAULT 0,
  asset_max TEXT,
  killed TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (name, version)
);

CREATE TABLE IF NOT EXISTS aa_cfg_flags (
  name VARCHAR(64) NOT NULL PRIMARY KEY,
  pin_version INT NOT NULL DEFAULT 0,
  killed TINYINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS aa_intel_tags (
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  tag VARCHAR(64) NOT NULL,
  PRIMARY KEY (chain, address),
  KEY idx_aa_intel_tags_tag (tag)
);

CREATE TABLE IF NOT EXISTS aa_intel_marks (
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  kind VARCHAR(16) NOT NULL,
  PRIMARY KEY (chain, address)
);
