// 功能：persist/sql.go
// 作者：Kinsley
// 日期：2020.09.09

package persist

import (
	"context"
	"database/sql"
	"errors"
	"os"
)

// SchemaSQL 审查案、占用、绑址、审批、充提唯一键、围栏、出金簿 memo、理财目录软删。
// 金额列用 DECIMAL(36,18)。入金自然键 (chain, tx_hash, output_index)；bootstrap 即含 output_index；0019 仅升级旧库。
// address_allowlist.memo / aa_earn_products.deleted_at 与 0032 一致；address_allowlist 写面列由 0039 ApplyOnce 加。aa_earn_product_audit 与 0033 一致；aa_earn_product_audit_cold 与 0035 一致；绿场 ApplySchema 即有基础表。

const SchemaSQL = `
CREATE TABLE IF NOT EXISTS aa_fences (
  row_id VARCHAR(191) NOT NULL PRIMARY KEY,
  epoch BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(64) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_cases (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  state VARCHAR(32) NOT NULL,
  assignee VARCHAR(64) NOT NULL DEFAULT '',
  decided_by VARCHAR(64) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_cases_move_open (movement_id, state)
);
CREATE TABLE IF NOT EXISTS aa_case_audit (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  case_id VARCHAR(64) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  event VARCHAR(32) NOT NULL,
  from_state VARCHAR(32) NOT NULL DEFAULT '',
  to_state VARCHAR(32) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_case_audit (case_id, at)
);
CREATE TABLE IF NOT EXISTS aa_usage_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL,
  idem VARCHAR(128) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_usage_idem (idem),
  KEY idx_aa_usage_user (kind, user_id, asset, at)
);
CREATE TABLE IF NOT EXISTS aa_addr_binds (
  bind_key VARCHAR(191) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  family VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  allocated TINYINT NOT NULL DEFAULT 1,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  bound_at DATETIME(3) NOT NULL,
  reuse_after DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_addr (address)
);
CREATE TABLE IF NOT EXISTS aa_approvals (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL DEFAULT '',
  dest VARCHAR(128) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  requested_by VARCHAR(64) NOT NULL,
  approved_by VARCHAR(64) NOT NULL DEFAULT '',
  state VARCHAR(16) NOT NULL,
  note VARCHAR(255) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_deposits (
  chain VARCHAR(16) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  step VARCHAR(16) NOT NULL DEFAULT '',
  from_address VARCHAR(128) NOT NULL DEFAULT '',
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (chain, tx_hash, output_index),
  KEY idx_aa_dep_xfer (transfer_id, step),
  KEY idx_aa_dep_user_asset_created (user_id, asset, created_at),
  KEY idx_aa_dep_step_updated (step, updated_at)
);
CREATE TABLE IF NOT EXISTS aa_withdraws (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  idem VARCHAR(128) NOT NULL,
  state VARCHAR(32) NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_wd_idem (idem)
);
CREATE TABLE IF NOT EXISTS aa_wd_outbound (
  transfer_id VARCHAR(64) NOT NULL PRIMARY KEY,
  run_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL DEFAULT '',
  amount DECIMAL(36,18) NOT NULL DEFAULT 0,
  chain VARCHAR(16) NOT NULL DEFAULT '',
  idem VARCHAR(128) NOT NULL DEFAULT '',
  step VARCHAR(16) NOT NULL DEFAULT '',
  kyt VARCHAR(16) NOT NULL DEFAULT '',
  policy_ver INT NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  debit_once TINYINT NOT NULL DEFAULT 0,
  fail_sealed TINYINT NOT NULL DEFAULT 0,
  compensate VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_wd_outbound_idem (idem),
  KEY idx_aa_wd_outbound_step (step, updated_at),
  KEY idx_aa_wd_outbound_user (user_id, asset, created_at)
);
CREATE TABLE IF NOT EXISTS aa_outbox (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  ev_key VARCHAR(128) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  payload TEXT NOT NULL,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  sent_at DATETIME(3) NULL,
  KEY idx_aa_outbox_pending (sent_at, created_at)
);
CREATE TABLE IF NOT EXISTS aa_seq (
  name VARCHAR(64) NOT NULL PRIMARY KEY,
  n BIGINT NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_kyt_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  movement_id VARCHAR(128) NOT NULL DEFAULT '',
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  decision VARCHAR(16) NOT NULL,
  reason VARCHAR(255) NOT NULL DEFAULT '',
  pack VARCHAR(32) NOT NULL DEFAULT '',
  pack_ver INT NOT NULL DEFAULT 0,
  mode VARCHAR(16) NOT NULL DEFAULT 'enforce',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_kyt_user (user_id, created_at)
);
CREATE TABLE IF NOT EXISTS aa_custody_inv (
  asset VARCHAR(16) NOT NULL,
  side VARCHAR(8) NOT NULL,
  available DECIMAL(36,18) NOT NULL,
  frozen DECIMAL(36,18) NOT NULL DEFAULT 0,
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  taken_at DATETIME(3) NOT NULL,
  PRIMARY KEY (asset, side)
);
CREATE TABLE IF NOT EXISTS aa_nonce_slots (
  slot_key VARCHAR(191) NOT NULL PRIMARY KEY,
  chain VARCHAR(16) NOT NULL,
  from_addr VARCHAR(128) NOT NULL,
  next_nonce BIGINT NOT NULL DEFAULT 0,
  last_hash VARCHAR(128) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  updated_at DATETIME(3) NOT NULL
);
CREATE TABLE IF NOT EXISTS aa_sign_jobs (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  movement_id VARCHAR(128) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  dest VARCHAR(128) NOT NULL,
  amount DECIMAL(36,18) NOT NULL,
  canonical TEXT NOT NULL,
  state VARCHAR(16) NOT NULL,
  adapter VARCHAR(32) NOT NULL DEFAULT '',
  fence_epoch BIGINT NOT NULL DEFAULT 0,
  fence_token VARCHAR(64) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_sign_move (movement_id)
);
CREATE TABLE IF NOT EXISTS aa_policy_hits (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  pack VARCHAR(32) NOT NULL,
  pack_ver INT NOT NULL,
  rule VARCHAR(64) NOT NULL,
  mode VARCHAR(16) NOT NULL,
  allow TINYINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  KEY idx_aa_policy_hits (pack, rule, created_at)
);
CREATE TABLE IF NOT EXISTS address_allowlist (
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(16) NOT NULL,
  address VARCHAR(128) NOT NULL,
  label VARCHAR(64) NOT NULL DEFAULT '',
  memo VARCHAR(128) NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, chain, address)
);
CREATE TABLE IF NOT EXISTS aa_earn_positions (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  principal VARCHAR(64) NOT NULL DEFAULT '0',
  accrued VARCHAR(64) NOT NULL DEFAULT '0',
  claimed VARCHAR(64) NOT NULL DEFAULT '0',
  penalty VARCHAR(64) NOT NULL DEFAULT '',
  opened_at DATETIME(3) NOT NULL,
  unlock_at DATETIME(3) NULL,
  last_accrue DATETIME(3) NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  transfer_id VARCHAR(64) NOT NULL DEFAULT '',
  idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
  auto_subscribe TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  KEY idx_aa_earn_pos_user (user_id, status),
  KEY idx_aa_earn_pos_idem (idempotency_key),
  KEY idx_aa_earn_pos_product (product_id, status)
);
CREATE TABLE IF NOT EXISTS aa_earn_products (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  asset VARCHAR(16) NOT NULL,
  category VARCHAR(32) NOT NULL DEFAULT 'flexible',
  daily_bps INT NOT NULL DEFAULT 0,
  lock_days INT NOT NULL DEFAULT 0,
  min_subscribe VARCHAR(64) NOT NULL DEFAULT '',
  max_position VARCHAR(64) NOT NULL DEFAULT '',
  early_bps INT NOT NULL DEFAULT 0,
  active TINYINT(1) NOT NULL DEFAULT 1,
  auto_subscribe TINYINT(1) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL,
  deleted_at DATETIME(3) NULL,
  KEY idx_aa_earn_prod_cat (category, active),
  KEY idx_aa_earn_prod_deleted (deleted_at)
);
CREATE TABLE IF NOT EXISTS aa_earn_product_audit (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  product_id VARCHAR(64) NOT NULL,
  action VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL DEFAULT '',
  note VARCHAR(255) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  KEY idx_aa_earn_prod_audit_prod (product_id, at),
  KEY idx_aa_earn_prod_audit_at (at)
);
CREATE TABLE IF NOT EXISTS aa_earn_product_audit_cold (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  archive_id VARCHAR(64) NOT NULL,
  product_id VARCHAR(64) NOT NULL DEFAULT '',
  path VARCHAR(512) NOT NULL,
  row_count INT NOT NULL DEFAULT 0,
  start_at DATETIME(3) NOT NULL,
  end_at DATETIME(3) NOT NULL,
  sha256 CHAR(64) NOT NULL DEFAULT '',
  actor VARCHAR(64) NOT NULL DEFAULT '',
  at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_earn_audit_cold_id (archive_id),
  KEY idx_aa_earn_audit_cold_prod (product_id, at),
  KEY idx_aa_earn_audit_cold_at (at)
);
CREATE TABLE IF NOT EXISTS aa_schema_version (
  version INT NOT NULL PRIMARY KEY,
  name VARCHAR(191) NOT NULL,
  checksum VARCHAR(64) NOT NULL DEFAULT '',
  applied_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_aa_schema_name (name)
);
`

type SQLDB struct {
	DB *sql.DB
	Dialect Dialect
}

   func OpenSQL(db *sql.DB) *SQLDB { return &SQLDB{DB: db, Dialect: MySQL} }

  func OpenSQLDialect(db *sql.DB, d Dialect) *SQLDB {
	  return &SQLDB{DB: db, Dialect: d}
  }

   func DSNFromEnv() string {
	  return os.Getenv("ACCOUNT_ASSETS_DSN")
   }

// FundingMySQLDSN 站内 MySQL 连接串：先读 ACCOUNT_ASSETS_DSN，否则与 deploy 中 mysql-0.kucoin.net 一致。
// 不在代码中写环回地址或常用端口常量；端口由驱动在解析主机名时默认处理。
func FundingMySQLDSN() string {
	if v := DSNFromEnv(); v != "" {
		return v
	}
	return "spotwallet:localdev@tcp(mysql-0.kucoin.net)/spotwallet?parseTime=true&charset=utf8mb4&loc=UTC"
}

// PGDSNFromEnv 读 ACCOUNT_ASSETS_PG_DSN，否则 ACCOUNT_ASSETS_DSN。主机 pg-0.kucoin.net（SG 10.18 / TYO 10.19）。

  func PGDSNFromEnv() string {
	   if v := os.Getenv("ACCOUNT_ASSETS_PG_DSN"); v != "" {
		  return v
	   }
	  return os.Getenv("ACCOUNT_ASSETS_DSN")
   }

// Queryer 事务内或连接上的执行面。Postgres 路径会改写 ? 占位。

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Conn 给仓储用。已在 InTx 里则走同一事务，围栏和业务行同提交。

  func (s *SQLDB) Conn(ctx context.Context) Queryer {
	   return s.conn(ctx)
   }

type sqlTxKey struct{}

// 作者: Knox
// InTx 以可重复读开启事务。上下文里已有事务时直接复用。
// 冻结与占用必须在同一次提交里完成。严禁先提交冻结、再另起事务记占用。
// 死锁或快照冲突时整笔回滚后重开，最多三次。余额不足、校验失败等业务错误不重试。

   func (s *SQLDB) InTx(ctx context.Context, fn func(context.Context) error) error {
	   if s == nil || s.DB == nil {
		  return ErrTxClosed
	   }
	   if _, ok := ctx.Value(sqlTxKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := SleepTxRetry(ctx, attempt); err != nil {
				return err
			}
		}
		last = s.inTxOnce(ctx, fn)
		if last == nil || !RetryableTxConflict(last) {
			return last
		}
	}
	return last
}

func (s *SQLDB) inTxOnce(ctx context.Context, fn func(context.Context) error) error {
	tx, err := s.DB.BeginTx(ctx, MoneyTxOptions())
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, sqlTxKey{}, tx)
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

  func (s *SQLDB) conn(ctx context.Context) Queryer {
	  var inner Queryer
	   if tx, ok := ctx.Value(sqlTxKey{}).(*sql.Tx); ok && tx != nil {
		   inner = tx
	  } else {
		  inner = s.DB
	   }
	   if s != nil && s.Dialect == Postgres {
		  return rewritingQueryer{inner: inner, d: Postgres}
	  }
	   return inner
   }

// BumpFence 行锁后 +1 epoch 并换 token。旧 token 立刻失效。

   func (s *SQLDB) BumpFence(ctx context.Context, id, token string) (Fence, error) {
	   if token == "" {
		   token = NewToken()
	  }
	   q := s.conn(ctx)
	   upsert := `INSERT INTO aa_fences (row_id, epoch, token, updated_at) VALUES (?, 1, ?, CURRENT_TIMESTAMP(3)) 
ON DUPLICATE KEY UPDATE epoch=epoch+1, token=VALUES(token), updated_at=CURRENT_TIMESTAMP(3)`
	  if s != nil && s.Dialect == Postgres {
		  upsert = `INSERT INTO aa_fences (row_id, epoch, token, updated_at) VALUES (?, 1, ?, CURRENT_TIMESTAMP)
ON CONFLICT (row_id) DO UPDATE SET epoch=aa_fences.epoch+1, token=EXCLUDED.token, updated_at=CURRENT_TIMESTAMP`
	   }
	  _,
		   err := q.ExecContext(ctx, upsert, id, token)
	  if err != nil {
		  return Fence{}, err
	  }
	  var f Fence
	   err = q.QueryRowContext(ctx, `SELECT epoch, token FROM aa_fences WHERE row_id=?`, id).Scan(&f.Epoch, &f.Token)
	   return f, err
   }

   func (s *SQLDB) CheckFence(ctx context.Context, id string, epoch int64, token string) error {
	   var f Fence
	  err := s.conn(ctx).QueryRowContext(ctx, `SELECT epoch, token FROM aa_fences WHERE row_id=? FOR UPDATE`, id).Scan(&f.Epoch, &f.Token)
	  if errors.Is(err, sql.ErrNoRows) {
		  return ErrNotFound
	  }
	  if err != nil {
		  return err
	   }
	   if !f.Match(epoch, token) {
		   return ErrStaleFence
	   }
	  return nil
  }

  func (s *SQLDB) ApplySchema(ctx context.Context) error {
	   if s == nil || s.DB == nil {
		   return ErrTxClosed
	   }
	   if s.Dialect == Postgres {
		  return ApplyStatements(ctx, s.DB, SchemaPostgres)
	  }
	  _, err := s.DB.ExecContext(ctx, SchemaSQL)
	  return err
   }
