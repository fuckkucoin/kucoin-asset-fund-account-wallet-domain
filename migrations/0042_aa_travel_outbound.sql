-- 旅行规则对手目录与资金账户出站案件。VASP 无密钥列；案件状态 required/submitted/acknowledged/rejected/manual-review。缺走廊或未确认案件不得广播。
CREATE TABLE IF NOT EXISTS aa_travel_vasps (
  code VARCHAR(16) NOT NULL PRIMARY KEY,
-- 作者：Axel
  name VARCHAR(64) NOT NULL,
  country VARCHAR(8) NOT NULL DEFAULT '',
  endpoint VARCHAR(255) NOT NULL DEFAULT '',
  did VARCHAR(128) NOT NULL DEFAULT '',
  lei VARCHAR(32) NOT NULL DEFAULT '',
  protocol VARCHAR(16) NOT NULL DEFAULT 'LOCAL',
  enabled TINYINT NOT NULL DEFAULT 1,
  unhosted TINYINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_travel_vasp_did (did),
  KEY idx_aa_travel_vasp_lei (lei)
);

CREATE TABLE IF NOT EXISTS aa_travel_cases (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  msg_id VARCHAR(64) NOT NULL DEFAULT '',
  chain VARCHAR(32) NOT NULL DEFAULT '',
  asset VARCHAR(16) NOT NULL,
  amount VARCHAR(64) NOT NULL,
  dest VARCHAR(128) NOT NULL DEFAULT '',
  peer_vasp VARCHAR(16) NOT NULL DEFAULT '',
  corridor VARCHAR(64) NOT NULL DEFAULT '',
  originator_name VARCHAR(128) NOT NULL DEFAULT '',
  originator_account VARCHAR(128) NOT NULL DEFAULT '',
  originator_vasp VARCHAR(32) NOT NULL DEFAULT '',
  beneficiary_name VARCHAR(128) NOT NULL DEFAULT '',
  beneficiary_account VARCHAR(128) NOT NULL DEFAULT '',
  beneficiary_vasp VARCHAR(32) NOT NULL DEFAULT '',
  checksum CHAR(64) NOT NULL DEFAULT '',
  envelope_json TEXT NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_travel_case_move (movement_id),
  KEY idx_aa_travel_case_user (user_id, updated_at),
  KEY idx_aa_travel_case_status (status, updated_at)
);

CREATE TABLE IF NOT EXISTS aa_travel_case_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  case_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  from_status VARCHAR(32) NOT NULL DEFAULT '',
  to_status VARCHAR(32) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_travel_case_audit (case_id, at)
);
