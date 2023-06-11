// 功能：oracletickgate 包内 sqlstore 实现。
// SQL 表语句与占位实现。默认走 MemStore；迁移见 0137_oracle_tick_lease.sql。

// 作者：Finn
// 日期：2023.06.10

package oracletickgate

const (
	SQLCreateLeases = `
CREATE TABLE IF NOT EXISTS oracle_tick_leases (
    lease_id         VARCHAR(64)  NOT NULL COMMENT 'tick_lease_id',
    pair             VARCHAR(32)  NOT NULL,
    asset            VARCHAR(16)  NOT NULL,
    quote            VARCHAR(16)  NOT NULL,
    price            VARCHAR(64)  NOT NULL,
    tick_at          DATETIME(3)  NOT NULL,
    fingerprint      VARCHAR(64)  NOT NULL,
    generation       BIGINT       NOT NULL DEFAULT 1,
    valuation_kind   VARCHAR(32)  NOT NULL COMMENT 'convert|mark_to_market|margin_ltv',
    mode             VARCHAR(16)  NOT NULL COMMENT 'enforce|shadow|probe',
    quote_ccy        VARCHAR(16)  NOT NULL,
    amount           VARCHAR(64)  NOT NULL DEFAULT '',
    borrow_asset     VARCHAR(16)  NOT NULL DEFAULT '',
    borrow_amt       VARCHAR(64)  NOT NULL DEFAULT '',
    col_asset        VARCHAR(16)  NOT NULL DEFAULT '',
    col_amt          VARCHAR(64)  NOT NULL DEFAULT '',
    mtm_total        VARCHAR(64)  NOT NULL DEFAULT '',
    ltv_bps          INT          NOT NULL DEFAULT 0,
    state            VARCHAR(16)  NOT NULL DEFAULT 'issued'
                     COMMENT 'issued|bound|consumed|expired|revoked',
    ttl_seconds      BIGINT       NOT NULL DEFAULT 30,
    issued_at        DATETIME(3)  NOT NULL,
    expires_at       DATETIME(3)  NOT NULL,
    bound_at         DATETIME(3)  NULL,
    consumed_at      DATETIME(3)  NULL,
    revoked_at       DATETIME(3)  NULL,
    write_kind       VARCHAR(32)  NOT NULL DEFAULT '',
    write_ref        VARCHAR(64)  NOT NULL DEFAULT '',
    actor            VARCHAR(64)  NOT NULL DEFAULT '',
    note             VARCHAR(255) NOT NULL DEFAULT '',
    idem_key         VARCHAR(128) NOT NULL DEFAULT '',
    PRIMARY KEY (lease_id),
    KEY idx_otl_pair_state (pair, state),
    KEY idx_otl_state_exp (state, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值 Tick 资金写租约';`
	SQLCreateProofs = `
CREATE TABLE IF NOT EXISTS oracle_tick_drift_proofs (
    proof_id         VARCHAR(64)  NOT NULL,
    lease_id         VARCHAR(64)  NOT NULL DEFAULT '',
    pair             VARCHAR(32)  NOT NULL DEFAULT '',
    kind             VARCHAR(64)  NOT NULL,
    fail_closed      TINYINT(1)   NOT NULL DEFAULT 1,
    issued_price     VARCHAR(64)  NOT NULL DEFAULT '',
    live_price       VARCHAR(64)  NOT NULL DEFAULT '',
    issued_generation BIGINT      NOT NULL DEFAULT 0,
    live_generation  BIGINT       NOT NULL DEFAULT 0,
    issued_fp        VARCHAR(64)  NOT NULL DEFAULT '',
    live_fp          VARCHAR(64)  NOT NULL DEFAULT '',
    write_kind       VARCHAR(32)  NOT NULL DEFAULT '',
    summary          TEXT         NOT NULL,
    opened_at        DATETIME(3)  NOT NULL,
    exported_at      DATETIME(3)  NULL,
    detail           TEXT         NULL,
    fund_write_held  TINYINT(1)   NOT NULL DEFAULT 1,
    PRIMARY KEY (proof_id),
    KEY idx_otdp_lease (lease_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick 漂移拒写证明仓';`
	SQLCreateAudit = `
CREATE TABLE IF NOT EXISTS oracle_tick_audit (
    audit_id    VARCHAR(64)  NOT NULL,
    lease_id    VARCHAR(64)  NULL,
    pair        VARCHAR(32)  NOT NULL DEFAULT '',
    action      VARCHAR(32)  NOT NULL,
    actor       VARCHAR(64)  NOT NULL,
    from_state  VARCHAR(16)  NOT NULL DEFAULT '',
    to_state    VARCHAR(16)  NOT NULL DEFAULT '',
    detail      VARCHAR(255) NOT NULL DEFAULT '',
    created_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (audit_id),
    KEY idx_ota_lease (lease_id, created_at),
    KEY idx_ota_pair (pair, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值 Tick 租约审计';`
	SQLCreateGens = ` 
CREATE TABLE IF NOT EXISTS oracle_tick_generations (
    pair        VARCHAR(32)  NOT NULL,
    generation  BIGINT       NOT NULL,
    price       VARCHAR(64)  NOT NULL,
    tick_at     DATETIME(3)  NOT NULL,
    updated_at  DATETIME(3)  NOT NULL,
    PRIMARY KEY (pair)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick pair 世代';`
	SQLCreateWrites = `
CREATE TABLE IF NOT EXISTS oracle_tick_fund_writes (
    write_id    VARCHAR(64)  NOT NULL,
    lease_id    VARCHAR(64)  NOT NULL,
    write_kind  VARCHAR(32)  NOT NULL,
    write_ref   VARCHAR(64)  NOT NULL DEFAULT '',
    pair        VARCHAR(32)  NOT NULL,
    generation  BIGINT       NOT NULL,
    posted      TINYINT(1)   NOT NULL DEFAULT 1,
    actor       VARCHAR(64)  NOT NULL,
    posted_at   DATETIME(3)  NOT NULL,
    PRIMARY KEY (write_id),
    UNIQUE KEY uk_otfw_lease (lease_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='估值绑定资金写记录（一租约一写，防双花）';`
	SQLCreateIdem = `
CREATE TABLE IF NOT EXISTS oracle_tick_idem (
    idem_key   VARCHAR(128) NOT NULL,
    result_id  VARCHAR(64)  NOT NULL,
    PRIMARY KEY (idem_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tick 租约幂等';`
	SQLInsertLease = `INSERT INTO oracle_tick_leases
(lease_id, pair, asset, quote, price, tick_at, fingerprint, generation, valuation_kind, mode,
 quote_ccy, amount, borrow_asset, borrow_amt, col_asset, col_amt, mtm_total, ltv_bps, state,
 ttl_seconds, issued_at, expires_at, bound_at, consumed_at, revoked_at, write_kind, write_ref, actor, note, idem_key)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	SQLCASLease = `UPDATE oracle_tick_leases SET state=?, bound_at=?, consumed_at=?, revoked_at=?, write_kind=?, write_ref=?, note=?
WHERE lease_id=? AND generation=? AND state=?`
	SQLSelectLease = `SELECT lease_id, pair, asset, quote, price, tick_at, fingerprint, generation, valuation_kind, mode,
 quote_ccy, amount, borrow_asset, borrow_amt, col_asset, col_amt, mtm_total, ltv_bps, state,
 ttl_seconds, issued_at, expires_at, bound_at, consumed_at, revoked_at, write_kind, write_ref, actor, note, idem_key
 FROM oracle_tick_leases WHERE lease_id=?`
	SQLOpenByPair = `SELECT lease_id FROM oracle_tick_leases WHERE pair=? AND state IN ('issued','bound')`
	SQLExpiredOpen = `SELECT lease_id FROM oracle_tick_leases WHERE state IN ('issued','bound') AND expires_at<=? LIMIT ?`
)

// SQLStore 文档化 SQL 仓；未注入 DB 时回退 MemStore。
type SQLStore struct {
	*MemStore
	dialect string
}

// NewSQLStore 回退内存，保留 SQL 常量供迁移与运维。

   func NewSQLStore(dialect string) *SQLStore {
	  if dialect == "" {
		  dialect = "mysql"
	  }
	   return &SQLStore{MemStore: NewMemStore(), dialect: dialect}
  }

// Dialect 方言。
   func (s *SQLStore) Dialect() string {
	  if s == nil {
		   return "mysql"
	   }
	  return s.dialect
   }

// SchemaStatements 建表语句清单。
  func SchemaStatements() []string {
	  return []string{SQLCreateLeases,
		   SQLCreateProofs, SQLCreateAudit, SQLCreateGens, SQLCreateWrites, SQLCreateIdem}
  }
