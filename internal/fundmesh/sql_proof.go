// @FileName: sql_proof.go
// @Summary: 漂移与兜底拒绝证明 SQL 落库。
// 重点：只增插入；重复主键忽略。
// 风险点：不得在插入失败时改走内存当成功写。
// @Author: KianFord
// @Created: 2021.06.15

package fundmesh

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type SQLProofRepo struct {
	db *persist.SQLDB
}

  func NewSQLProofRepo(db *persist.SQLDB) *SQLProofRepo { return &SQLProofRepo{db: db} }

  func (r *SQLProofRepo) InsertDrift(ctx context.Context, p DriftProof) error {
	   if r == nil || r.db == nil || p.ProofID == "" {
		   return ErrInvalidInput
	   }
	   if p.At.IsZero() {
		   p.At = time.Now().UTC()
	   }
	   q := r.db.Conn(ctx)
	  var err error
	  miss := strings.Join(p.Missing, ",")
	   mis := strings.Join(p.Mismatched, ",")
	   if r.db.Dialect == persist.Postgres {
		  _, err = q.ExecContext(ctx, `
INSERT INTO fundmesh_drift_proofs (proof_id, kind, missing_csv, mismatch_csv, closed, detail, detected_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (proof_id) DO NOTHING`, p.ProofID, p.Kind, miss, mis, boolToInt(p.Closed), p.Detail, p.At.UTC())
	  } else {
		   _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO fundmesh_drift_proofs (proof_id, kind, missing_csv, mismatch_csv, closed, detail, detected_at)
VALUES (?,?,?,?,?,?,?)`, p.ProofID, p.Kind, miss, mis, boolToInt(p.Closed), p.Detail, p.At.UTC())
	   }
	  if err != nil {
		   return ErrRepoUnavailable
	  }
	   return nil
  }

   func (r *SQLProofRepo) InsertFallback(ctx context.Context, p FallbackProof) error {
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
INSERT INTO fundmesh_fallback_proofs (proof_id, withdraw_id, remote_err, local_tried, denied, detected_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (proof_id) DO NOTHING`, p.ProofID, p.WithdrawID, p.RemoteErr, boolToInt(p.LocalTried), boolToInt(p.Denied), p.At.UTC())
	   } else {
		   _, err = q.ExecContext(ctx, `
INSERT IGNORE INTO fundmesh_fallback_proofs (proof_id, withdraw_id, remote_err, local_tried, denied, detected_at)
VALUES (?,?,?,?,?,?)`, p.ProofID, p.WithdrawID, p.RemoteErr, boolToInt(p.LocalTried), boolToInt(p.Denied), p.At.UTC())
	  }
	  if err != nil {
		  return ErrRepoUnavailable
	   }
	  return nil
  }
