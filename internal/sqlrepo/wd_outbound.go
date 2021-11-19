package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// WDOutbound 出站 saga 行。自然键 transfer_id；idem 防重复扣款。
type WDOutbound struct {
	db *persist.SQLDB
}

   func NewWDOutbound(db *persist.SQLDB) *WDOutbound { return &WDOutbound{db: db} }

type WDOutboundRun struct {
	TransferID string
	RunID      string
	UserID     string
	Asset      string
	Amount string
	Chain string
	Idem string
	Step      string
	KYT       string
	PolicyVer int
	LastError  string
	DebitOnce  bool
	FailSealed bool
	Compensate string
	CreatedAt time.Time
	UpdatedAt time.Time
}

  func (r *WDOutbound) PersistRun(ctx context.Context, row WDOutboundRun) error {
	   if r == nil || r.db == nil {
		   return ErrUnavailable
	  }
	   row.TransferID = strings.TrimSpace(row.TransferID)
	  row.Idem = strings.TrimSpace(row.Idem)
	  if row.TransferID == "" {
		   return ErrConflict
	   }
	   if row.Idem == "" {
		   row.Idem = "xfer:" + row.TransferID
	   }
	  now := time.Now().UTC()
	   if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = now
	   }
	   if row.CreatedAt.IsZero() {
		  row.CreatedAt = row.UpdatedAt
	   }
	  debit, seal := 0, 0
	   if row.DebitOnce {
		   debit = 1
	   }
	   if row.FailSealed {
		   seal = 1
	   }
	   stmt := `INSERT INTO aa_wd_outbound (transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE run_id=VALUES(run_id), user_id=VALUES(user_id), asset=VALUES(asset), amount=VALUES(amount), chain=VALUES(chain), idem=VALUES(idem), step=VALUES(step), kyt=VALUES(kyt), policy_ver=VALUES(policy_ver), last_error=VALUES(last_error), debit_once=VALUES(debit_once), fail_sealed=VALUES(fail_sealed), compensate=VALUES(compensate), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_wd_outbound (transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (transfer_id) DO UPDATE SET run_id=EXCLUDED.run_id, user_id=EXCLUDED.user_id, asset=EXCLUDED.asset, amount=EXCLUDED.amount, chain=EXCLUDED.chain, idem=EXCLUDED.idem, step=EXCLUDED.step, kyt=EXCLUDED.kyt, policy_ver=EXCLUDED.policy_ver, last_error=EXCLUDED.last_error, debit_once=EXCLUDED.debit_once, fail_sealed=EXCLUDED.fail_sealed, compensate=EXCLUDED.compensate, updated_at=EXCLUDED.updated_at`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  row.TransferID, row.RunID, row.UserID, row.Asset, row.Amount, row.Chain, row.Idem,
		  row.Step, row.KYT, row.PolicyVer, row.LastError, debit, seal, row.Compensate, row.CreatedAt, row.UpdatedAt)
	   return err
  }

  func (r *WDOutbound) Get(ctx context.Context, transferID string) (WDOutboundRun, error) {
	   if r == nil || r.db == nil {
		   return WDOutboundRun{}, ErrUnavailable
	  }
	  return scanWDOutbound(r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at FROM aa_wd_outbound WHERE transfer_id=?`,
		  strings.TrimSpace(transferID)))
  }

  func (r *WDOutbound) GetByIdem(ctx context.Context, idem string) (WDOutboundRun, error) {
	  if r == nil || r.db == nil {
		  return WDOutboundRun{}, ErrUnavailable
	  }
	  return scanWDOutbound(r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at FROM aa_wd_outbound WHERE idem=?`,
		  strings.TrimSpace(idem)))
  }

  func (r *WDOutbound) ListAll(ctx context.Context, limit int) ([]WDOutboundRun, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 4000
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT transfer_id, run_id, user_id, asset, amount, chain, idem, step, kyt, policy_ver, last_error, debit_once, fail_sealed, compensate, created_at, updated_at FROM aa_wd_outbound ORDER BY updated_at LIMIT ?`, limit)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]WDOutboundRun, 0)
	  for rows.Next() {
		   row, err := scanWDOutboundRow(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, row)
	   }
	  return out, rows.Err()
  }

   func scanWDOutbound(row *sql.Row) (WDOutboundRun, error) {
	  var o WDOutboundRun
	   var debit, seal int
	  err := row.Scan(&o.TransferID, &o.RunID, &o.UserID, &o.Asset, &o.Amount, &o.Chain, &o.Idem, &o.Step, &o.KYT, &o.PolicyVer, &o.LastError, &debit, &seal, &o.Compensate, &o.CreatedAt, &o.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return WDOutboundRun{},
			   ErrNotFound
	   }
	  if err != nil {
		  return WDOutboundRun{}, err
	  }
	  o.DebitOnce = debit != 0
	  o.FailSealed = seal != 0
	   return o, nil
   }

  func scanWDOutboundRow(rows *sql.Rows) (WDOutboundRun, error) {
	  var o WDOutboundRun
	   var debit, seal int
	   err := rows.Scan(&o.TransferID, &o.RunID, &o.UserID, &o.Asset, &o.Amount, &o.Chain, &o.Idem, &o.Step, &o.KYT, &o.PolicyVer, &o.LastError, &debit, &seal, &o.Compensate, &o.CreatedAt, &o.UpdatedAt)
	   if err != nil {
		   return WDOutboundRun{}, err
	   }
	  o.DebitOnce = debit != 0
	  o.FailSealed = seal != 0
	  return o, nil
  }
