package fundmesh

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type SQLCancelRepo struct {
	db *persist.SQLDB
}

   func NewSQLCancelRepo(db *persist.SQLDB) *SQLCancelRepo {
	  return &SQLCancelRepo{db: db}
  }

  func (r *SQLCancelRepo) Put(ctx context.Context, rec *CancelRecord) error {
	   if r == nil || r.db == nil || rec == nil || rec.WithdrawID == "" {
		   return ErrInvalidInput
	   }
	   if rec.CreatedAt.IsZero() {
		  rec.CreatedAt = time.Now().UTC()
	   }
	   rec.UpdatedAt = time.Now().UTC()
	  q := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, `
INSERT INTO fundmesh_cancel_records
  (record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (withdraw_id) DO UPDATE SET
  state=EXCLUDED.state, phase=EXCLUDED.phase, code=EXCLUDED.code,
  remote_ok=EXCLUDED.remote_ok, note=EXCLUDED.note, updated_at=EXCLUDED.updated_at
`, rec.RecordID, rec.WithdrawID, rec.UserID, rec.Actor, rec.Reason, rec.IdempotencyKey,
			   string(rec.Entry), string(rec.State), string(rec.Phase), rec.Code, boolToInt(rec.RemoteOK), rec.Note,
			   rec.CreatedAt.UTC(), rec.UpdatedAt.UTC())
	   } else {
		   _, err = q.ExecContext(ctx, `
INSERT INTO fundmesh_cancel_records
  (record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), phase=VALUES(phase), code=VALUES(code),
  remote_ok=VALUES(remote_ok), note=VALUES(note), updated_at=VALUES(updated_at)
`, rec.RecordID, rec.WithdrawID, rec.UserID, rec.Actor, rec.Reason, rec.IdempotencyKey,
			  string(rec.Entry),
			  string(rec.State), string(rec.Phase), rec.Code, boolToInt(rec.RemoteOK), rec.Note,
			  rec.CreatedAt.UTC(), rec.UpdatedAt.UTC())
	  }
	   if err != nil {
		  return ErrRepoUnavailable
	  }
	   return nil
  }

  func (r *SQLCancelRepo) GetByWithdraw(ctx context.Context, withdrawID string) (*CancelRecord, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrRepoUnavailable
	  }
	   q := r.db.Conn(ctx)
	   row := q.QueryRowContext(ctx, `SELECT record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at
FROM fundmesh_cancel_records WHERE withdraw_id=?`, withdrawID)
	  if r.db.Dialect == persist.Postgres {
		  row = q.QueryRowContext(ctx, `SELECT record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at
FROM fundmesh_cancel_records WHERE withdraw_id=$1`, withdrawID)
	   }
	  return scanCancel(row)
  }

  func (r *SQLCancelRepo) GetByIdem(ctx context.Context, key string) (*CancelRecord, error) {
	  if r == nil || r.db == nil || key == "" {
		   return nil, ErrWithdrawNotFound
	  }
	  q := r.db.Conn(ctx)
	  row := q.QueryRowContext(ctx, `SELECT record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at
FROM fundmesh_cancel_records WHERE idempotency_key=?`, key)
	   if r.db.Dialect == persist.Postgres {
		  row = q.QueryRowContext(ctx, `SELECT record_id, withdraw_id, user_id, actor, reason, idempotency_key, entry, state, phase, code, remote_ok, note, created_at, updated_at
FROM fundmesh_cancel_records WHERE idempotency_key=$1`, key)
	   }
	  return scanCancel(row)
   }

   func scanCancel(row *sql.Row) (*CancelRecord, error) {
	  var rec CancelRecord
	   var remote int
	  var entry, state, phase string
	  err := row.Scan(&rec.RecordID, &rec.WithdrawID, &rec.UserID, &rec.Actor, &rec.Reason, &rec.IdempotencyKey,
		   &entry,
		   &state, &phase, &rec.Code, &remote, &rec.Note, &rec.CreatedAt, &rec.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrWithdrawNotFound
	   }
	   if err != nil {
		   return nil, ErrRepoUnavailable
	   }
	  rec.Entry = EntryKind(entry)
	   rec.State = CancelState(state)
	  rec.Phase = WithdrawPhase(phase)
	   rec.RemoteOK = remote != 0
	   return &rec, nil
  }

   func boolToInt(v bool) int {
	   if v {
		   return 1
	  }
	  return 0
   }
