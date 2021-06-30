// 功能：出站幂等仓与扣减流水 SQL 仓储。
// 重点：幂等键唯一 upsert；流水成功行不可覆盖。
// 风险点：方言占位符必须按库切换。

// 作者：Likeo
// 日期：2021.06.14

package fundmesh

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type SQLOutboundRepo struct {
	db *persist.SQLDB
}

  func NewSQLOutboundRepo(db *persist.SQLDB) *SQLOutboundRepo {
	  return &SQLOutboundRepo{db: db}
   }

   func (r *SQLOutboundRepo) Put(ctx context.Context, rec *OutboundRecord) error {
	   if r == nil || r.db == nil || rec == nil || rec.IdempotencyKey == "" {
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
INSERT INTO fundmesh_outbound_records
  (record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
   idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
ON CONFLICT (idempotency_key) DO UPDATE SET
  state=EXCLUDED.state, phase=EXCLUDED.phase, code=EXCLUDED.code,
  remote_ok=EXCLUDED.remote_ok, debit_count=EXCLUDED.debit_count,
  note=EXCLUDED.note, updated_at=EXCLUDED.updated_at
`, rec.RecordID, rec.WithdrawID, rec.UserID, rec.Actor, rec.Asset, rec.Amount, rec.Chain, rec.Dest, rec.Memo, rec.Tag,
			  rec.IdempotencyKey, rec.Scheme, rec.Kind, rec.Fingerprint, rec.Canon,
			  string(rec.State),
			  string(rec.Phase), rec.Code, boolToInt(rec.RemoteOK), rec.DebitCount, rec.Note,
			  rec.CreatedAt.UTC(), rec.UpdatedAt.UTC())
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT INTO fundmesh_outbound_records
  (record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
   idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), phase=VALUES(phase), code=VALUES(code),
  remote_ok=VALUES(remote_ok), debit_count=VALUES(debit_count),
  note=VALUES(note), updated_at=VALUES(updated_at)
`, rec.RecordID, rec.WithdrawID, rec.UserID, rec.Actor, rec.Asset, rec.Amount, rec.Chain, rec.Dest, rec.Memo, rec.Tag,
			  rec.IdempotencyKey, rec.Scheme, rec.Kind, rec.Fingerprint, rec.Canon,
			  string(rec.State), string(rec.Phase), rec.Code, boolToInt(rec.RemoteOK), rec.DebitCount, rec.Note,
			  rec.CreatedAt.UTC(),
			  rec.UpdatedAt.UTC())
	  }
	   if err != nil {
		   return ErrOutboundUnavailable
	   }
	   return nil
  }

  func (r *SQLOutboundRepo) GetByIdem(ctx context.Context, key string) (*OutboundRecord, error) {
	  if r == nil || r.db == nil || key == "" {
		   return nil, ErrWithdrawNotFound
	   }
	  q := r.db.Conn(ctx)
	   query := `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE idempotency_key=?`
	  if r.db.Dialect == persist.Postgres {
		  query = `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE idempotency_key=$1`
	  }
	  return scanOutbound(q.QueryRowContext(ctx, query, key))
   }

  func (r *SQLOutboundRepo) GetByWithdraw(ctx context.Context, withdrawID string) (*OutboundRecord, error) {
	  if r == nil || r.db == nil || withdrawID == "" {
		  return nil, ErrWithdrawNotFound
	  }
	   q := r.db.Conn(ctx)
	  query := `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE withdraw_id=?`
	   if r.db.Dialect == persist.Postgres {
		   query = `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE withdraw_id=$1`
	   }
	   return scanOutbound(q.QueryRowContext(ctx, query, withdrawID))
  }

  func (r *SQLOutboundRepo) ListByUser(ctx context.Context, userID string, limit int) ([]OutboundRecord, error) {
	  if r == nil || r.db == nil {
		  return nil, ErrOutboundUnavailable
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	  q := r.db.Conn(ctx)
	  query := `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE user_id=? ORDER BY updated_at DESC LIMIT ?`
	  if r.db.Dialect == persist.Postgres {
		  query = `SELECT record_id, withdraw_id, user_id, actor, asset, amount, chain, dest, memo, tag,
 idempotency_key, scheme, kind, fingerprint, canon, state, phase, code, remote_ok, debit_count, note, created_at, updated_at
FROM fundmesh_outbound_records WHERE user_id=$1 ORDER BY updated_at DESC LIMIT $2`
	  }
	  rows, err := q.QueryContext(ctx, query, userID, limit)
	   if err != nil {
		  return nil, ErrOutboundUnavailable
	   }
	  defer rows.Close()
	  var out []OutboundRecord
	   for rows.Next() {
		  rec, err := scanOutboundRow(rows)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, *rec)
	   }
	  return out, rows.Err()
  }

   func scanOutbound(row *sql.Row) (*OutboundRecord, error) {
	  var rec OutboundRecord
	  var remote int
	   var state, phase string
	   err := row.Scan(&rec.RecordID, &rec.WithdrawID, &rec.UserID, &rec.Actor, &rec.Asset, &rec.Amount, &rec.Chain, &rec.Dest, &rec.Memo, &rec.Tag,
		   &rec.IdempotencyKey, &rec.Scheme, &rec.Kind, &rec.Fingerprint, &rec.Canon, &state, &phase, &rec.Code, &remote, &rec.DebitCount, &rec.Note,
		  &rec.CreatedAt, &rec.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrWithdrawNotFound
	   }
	  if err != nil {
		  return nil, ErrOutboundUnavailable
	  }
	  rec.State = OutboundState(state)
	   rec.Phase = WithdrawPhase(phase)
	   rec.RemoteOK = remote != 0
	  return &rec, nil
  }

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanOutboundRow(row rowScanner) (*OutboundRecord, error) {
	  var rec OutboundRecord
	  var remote int
	  var state, phase string
	   err := row.Scan(&rec.RecordID, &rec.WithdrawID, &rec.UserID, &rec.Actor, &rec.Asset, &rec.Amount, &rec.Chain, &rec.Dest, &rec.Memo, &rec.Tag,
		   &rec.IdempotencyKey, &rec.Scheme, &rec.Kind, &rec.Fingerprint, &rec.Canon, &state, &phase, &rec.Code, &remote, &rec.DebitCount, &rec.Note,
		  &rec.CreatedAt, &rec.UpdatedAt)
	   if err != nil {
		  return nil, ErrOutboundUnavailable
	   }
	   rec.State = OutboundState(state)
	   rec.Phase = WithdrawPhase(phase)
	  rec.RemoteOK = remote != 0
	   return &rec, nil
   }

type SQLDebitJournal struct {
	db *persist.SQLDB
}

   func NewSQLDebitJournal(db *persist.SQLDB) *SQLDebitJournal {
	   return &SQLDebitJournal{db: db}
   }

   func (j *SQLDebitJournal) Append(ctx context.Context, line DebitLine) error {
	   if j == nil || j.db == nil || line.IdempotencyKey == "" {
		   return ErrInvalidInput
	   }
	  if line.At.IsZero() {
		  line.At = time.Now().UTC()
	  }
	   q := j.db.Conn(ctx)
	   var err error
	   if j.db.Dialect == persist.Postgres {
		   _, err = q.ExecContext(ctx, ` 
INSERT INTO fundmesh_debit_journal
  (line_id, withdraw_id, idempotency_key, fingerprint, amount, asset, user_id, ok, remote_ok, note, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (idempotency_key) DO NOTHING`,
			   line.LineID, line.WithdrawID, line.IdempotencyKey, line.Fingerprint, line.Amount, line.Asset, line.UserID,
			  boolToInt(line.OK), boolToInt(line.Remote), line.Note, line.At.UTC())
	  } else {
		  _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO fundmesh_debit_journal
  (line_id, withdraw_id, idempotency_key, amount, asset, user_id, ok, remote_ok, note, created_at, fingerprint)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			  line.LineID, line.WithdrawID, line.IdempotencyKey, line.Amount, line.Asset, line.UserID,
			   boolToInt(line.OK),
			  boolToInt(line.Remote), line.Note, line.At.UTC(), line.Fingerprint)
	  }
	  if err != nil {
		   return ErrOutboundUnavailable
	  }
	  return nil
   }

  func (j *SQLDebitJournal) GetByIdem(ctx context.Context, key string) (*DebitLine, error) {
	  if j == nil || j.db == nil || key == "" {
		   return nil, ErrWithdrawNotFound
	   }
	   q := j.db.Conn(ctx)
	  query := `SELECT line_id, withdraw_id, idempotency_key, fingerprint, amount, asset, user_id, ok, remote_ok, note, created_at
FROM fundmesh_debit_journal WHERE idempotency_key=?`
	   if j.db.Dialect == persist.Postgres {
		  query = `SELECT line_id, withdraw_id, idempotency_key, fingerprint, amount, asset, user_id, ok, remote_ok, note, created_at
FROM fundmesh_debit_journal WHERE idempotency_key=$1`
	   }
	   var ln DebitLine
	  var okv, remote int
	   err := q.QueryRowContext(ctx, query, key).Scan(&ln.LineID, &ln.WithdrawID, &ln.IdempotencyKey, &ln.Fingerprint, &ln.Amount, &ln.Asset, &ln.UserID, &okv, &remote, &ln.Note, &ln.At)
	   if err == sql.ErrNoRows {
		  return nil, ErrWithdrawNotFound
	   }
	   if err != nil {
		  return nil, ErrOutboundUnavailable
	   }
	   ln.OK = okv != 0
	   ln.Remote = remote != 0
	  return &ln, nil
  }

   func (j *SQLDebitJournal) CountByWithdraw(ctx context.Context, withdrawID string) (int, error) {
	   return j.count(ctx, `SELECT COUNT(*) FROM fundmesh_debit_journal WHERE withdraw_id=? AND ok=1`,
		   `SELECT COUNT(*) FROM fundmesh_debit_journal WHERE withdraw_id=$1 AND ok=1`, withdrawID)
  }

  func (j *SQLDebitJournal) CountOK(ctx context.Context, key string) (int, error) {
	   return j.count(ctx, `SELECT COUNT(*) FROM fundmesh_debit_journal WHERE idempotency_key=? AND ok=1`,
		  `SELECT COUNT(*) FROM fundmesh_debit_journal WHERE idempotency_key=$1 AND ok=1`, key)
   }

   func (j *SQLDebitJournal) count(ctx context.Context, mysqlQ, pgQ, arg string) (int, error) {
	   if j == nil || j.db == nil {
		  return 0, ErrOutboundUnavailable
	   }
	  q := j.db.Conn(ctx)
	  query := mysqlQ
	   if j.db.Dialect == persist.Postgres {
		   query = pgQ
	  }
	   var n int
	  if err := q.QueryRowContext(ctx, query, arg).Scan(&n); err != nil {
		   return 0, ErrOutboundUnavailable
	  }
	  return n, nil
   }

type SQLOutboundProofRepo struct {
	db *persist.SQLDB
}

  func NewSQLOutboundProofRepo(db *persist.SQLDB) *SQLOutboundProofRepo {
	   return &SQLOutboundProofRepo{db: db}
   }

  func (r *SQLOutboundProofRepo) Insert(ctx context.Context, p OutboundProof) error {
	   if r == nil || r.db == nil || p.ProofID == "" {
		  return ErrInvalidInput
	   }
	  if p.At.IsZero() {
		  p.At = time.Now().UTC()
	   }
	   q := r.db.Conn(ctx)
	   var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO fundmesh_outbound_proofs
  (proof_id, kind, withdraw_id, idempotency_key, fingerprint, prior_phase, prior_code, debit_attempt, blocked, detail, detected_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (proof_id) DO NOTHING`,
			  p.ProofID,
			   p.Kind, p.WithdrawID, p.IdempotencyKey, p.Fingerprint, p.PriorPhase, p.PriorCode, p.DebitAttempt, boolToInt(p.Blocked), p.Detail, p.At.UTC())
	  } else {
		  _, err = q.ExecContext(ctx, ` 
INSERT IGNORE INTO fundmesh_outbound_proofs
  (proof_id, kind, withdraw_id, idempotency_key, fingerprint, prior_phase, prior_code, debit_attempt, blocked, detail, detected_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			  p.ProofID, p.Kind, p.WithdrawID, p.IdempotencyKey, p.Fingerprint, p.PriorPhase, p.PriorCode, p.DebitAttempt, boolToInt(p.Blocked), p.Detail, p.At.UTC())
	   }
	  if err != nil {
		  return ErrOutboundUnavailable
	  }
	  return nil
   }
