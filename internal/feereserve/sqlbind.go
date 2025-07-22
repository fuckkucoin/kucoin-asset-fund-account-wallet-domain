package feereserve

// LeasesDDL 预留代次租约表。generation + token 联合校验避免过期令牌接着操作。
const LeasesDDL = `
CREATE TABLE IF NOT EXISTS fr_reserve_leases (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id     VARCHAR(64)  NOT NULL,
  user_id         VARCHAR(64)  NOT NULL,
  asset           VARCHAR(16)  NOT NULL,
  reserved_fee    VARCHAR(64)  NOT NULL,
  actual_fee      VARCHAR(64)  NOT NULL DEFAULT '',
  generation      BIGINT       NOT NULL DEFAULT 1,
  token           VARCHAR(64)  NOT NULL,
  state           VARCHAR(16)  NOT NULL DEFAULT 'reserved',
  terminal_leg    VARCHAR(16)  NOT NULL DEFAULT '',
  voucher_id      VARCHAR(128) NOT NULL DEFAULT '',
  block_reason    VARCHAR(256) NOT NULL DEFAULT '',
  lease_expiry    DATETIME(3)  NOT NULL,
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  settled_at      DATETIME(3)  NULL,
  released_at     DATETIME(3)  NULL,
  UNIQUE KEY uk_fr_lease_withdraw (withdraw_id)
);
`

// CasesDDL 异常案例表。
const CasesDDL = `
CREATE TABLE IF NOT EXISTS fr_reserve_cases (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id  VARCHAR(64)  NOT NULL,
  lease_id     VARCHAR(64)  NOT NULL,
  kind         VARCHAR(32)  NOT NULL,
  state        VARCHAR(16)  NOT NULL DEFAULT 'open',
  detail       TEXT         NOT NULL,
  proof_ids    TEXT         NOT NULL DEFAULT '',
  resolution   TEXT         NOT NULL DEFAULT '',
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  proven_at    DATETIME(3)  NULL,
  closed_at    DATETIME(3)  NULL
);
`

// ProofsDDL 差额证明表。sealed 之后不可再改。
const ProofsDDL = `
CREATE TABLE IF NOT EXISTS fr_delta_proofs (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  withdraw_id  VARCHAR(64)  NOT NULL,
  lease_id     VARCHAR(64)  NOT NULL,
  reserved_fee VARCHAR(64)  NOT NULL,
  actual_fee   VARCHAR(64)  NOT NULL,
  delta_amt    VARCHAR(64)  NOT NULL,
  delta_sign   VARCHAR(8)   NOT NULL,
  case_id      VARCHAR(64)  NOT NULL DEFAULT '',
  sealed       TINYINT      NOT NULL DEFAULT 0,
  sealed_at    DATETIME(3)  NULL,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);
`

// TerminalBindsDDL 终态绑定表。主键即 withdraw_id，插入一行即锁定方向。
const TerminalBindsDDL = `
CREATE TABLE IF NOT EXISTS fr_terminal_binds (
  withdraw_id VARCHAR(64) NOT NULL PRIMARY KEY,
  leg         VARCHAR(16) NOT NULL,
  lease_id    VARCHAR(64) NOT NULL,
  generation  BIGINT      NOT NULL,
  token       VARCHAR(64) NOT NULL,
  voucher_id  VARCHAR(128) NOT NULL DEFAULT '',
  bound_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
);
`

// AllDDL 一次性返回全部 DDL，方便迁移脚本或初始化。
  func AllDDL() []string {
	  return []string{LeasesDDL, CasesDDL, ProofsDDL, TerminalBindsDDL}
   }
