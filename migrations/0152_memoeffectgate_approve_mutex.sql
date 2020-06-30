-- Memo 找回审批贷记与输出效果互斥。
-- 作者：King
-- 日期：2021.11.04

CREATE TABLE IF NOT EXISTS fundsurf_memo_effects (
  id VARCHAR(64) PRIMARY KEY,
  ticket_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  output_key VARCHAR(200) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  state VARCHAR(24) NOT NULL,
  holder VARCHAR(32) NOT NULL,
  fence_id VARCHAR(64) NULL,
  proof_id VARCHAR(64) NULL,
  idem VARCHAR(80) NULL,
  ttl_ms BIGINT NOT NULL,
  bound_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  credited_at DATETIME NULL,
  rejected_at DATETIME NULL,
  expired_at DATETIME NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS fundsurf_memo_half_proofs (
  id VARCHAR(64) PRIMARY KEY,
  effect_id VARCHAR(64) NOT NULL,
  ticket_id VARCHAR(64) NOT NULL,
  fence_id VARCHAR(64) NULL,
  kind VARCHAR(40) NOT NULL,
  state VARCHAR(16) NOT NULL,
  blocked TINYINT NOT NULL,
  sealed TINYINT NOT NULL,
  silent_keep TINYINT NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS fundsurf_ticket_credit_fences (
  id VARCHAR(64) PRIMARY KEY,
  effect_id VARCHAR(64) NOT NULL,
  ticket_id VARCHAR(64) NOT NULL,
  phase VARCHAR(24) NOT NULL,
  observe_ok TINYINT NOT NULL,
  apply_ok TINYINT NOT NULL,
  ticket_ok TINYINT NOT NULL,
  half_cut TINYINT NOT NULL,
  credit_count INT NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
