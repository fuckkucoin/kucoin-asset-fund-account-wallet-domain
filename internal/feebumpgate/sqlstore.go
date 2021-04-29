// @FileName: sqlstore.go
// @Summary: feebumpgate 包内 sqlstore 实现。
// 做什么：MySQL 世代/占用/证明仓。
// 不执行迁移 DDL；调用方保证表已存在。
// 不变量：INSERT 幂等；CAS 用 WHERE state=from。
// @Author: Bronson
// @Created: 2021.04.28

package feebumpgate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore MySQL 实现。
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 构造。

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
   }

  func (s *SQLStore) SaveGen(ctx context.Context, g FeeBumpGen) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `
INSERT INTO custody_feebump_gens (
  gen_id, transfer_id, generation, prev_broadcast_hash, new_broadcast_hash,
  new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to, amount,
  asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
  signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   g.GenID, g.TransferID, g.Generation, g.PrevBroadcastHash, g.NewBroadcastHash,
		   g.NewFee, g.PrevFee, g.DestinationTag, g.OccupancyFP, g.Chain, g.To, g.Amount,
		  g.Asset, g.CanonDigest, g.SigDigest, string(g.State), g.OccID, g.IdempotencyKey, g.Actor,
		   g.SignedAt, nullTime(g.BroadcastAt), nullTime(g.SupersededAt), nullTime(g.FinalAt),
		  nullTime(g.BlockedAt), g.CreatedAt, g.UpdatedAt)
	   if err != nil {
		  if isDup(err) {
			   return ErrIdem
		   }
		  return ErrStore
	   }
	  return nil
   }

  func (s *SQLStore) GetGen(ctx context.Context, genID string) (FeeBumpGen, error) {
	  if s == nil || s.db == nil {
		  return FeeBumpGen{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `SELECT gen_id, transfer_id, generation, prev_broadcast_hash, 
		new_broadcast_hash, new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to,
		amount, asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
		signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
		FROM custody_feebump_gens WHERE gen_id=?`, strings.TrimSpace(genID))
	   return scanGen(row)
   }

   func (s *SQLStore) GetGenByIdem(ctx context.Context, idem string) (FeeBumpGen, error) {
	  if s == nil || s.db == nil {
		   return FeeBumpGen{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `SELECT gen_id, transfer_id, generation, prev_broadcast_hash,
		new_broadcast_hash, new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to,
		amount, asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
		signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
		FROM custody_feebump_gens WHERE idempotency_key=?`, strings.TrimSpace(idem))
	   return scanGen(row)
   }

  func (s *SQLStore) LatestGen(ctx context.Context, transferID string) (FeeBumpGen, error) {
	   if s == nil || s.db == nil {
		   return FeeBumpGen{}, ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `SELECT gen_id, transfer_id, generation, prev_broadcast_hash,
		new_broadcast_hash, new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to,
		amount, asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
		signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
		FROM custody_feebump_gens WHERE transfer_id=? ORDER BY generation DESC LIMIT 1`,
		  strings.TrimSpace(transferID))
	  return scanGen(row)
  }

   func (s *SQLStore) ListGens(ctx context.Context, transferID string) ([]FeeBumpGen, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, `SELECT gen_id, transfer_id, generation, prev_broadcast_hash,
		new_broadcast_hash, new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to,
		amount, asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
		signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
		FROM custody_feebump_gens WHERE transfer_id=?`, strings.TrimSpace(transferID))
	   if err != nil {
		   return nil, ErrStore
	   }
	   defer rows.Close()
	  return scanGens(rows)
   }

  func (s *SQLStore) ListActiveGens(ctx context.Context, limit int) ([]FeeBumpGen, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 64
	   }
	   rows, err := s.db.QueryContext(ctx, `SELECT gen_id, transfer_id, generation, prev_broadcast_hash,
		new_broadcast_hash, new_fee, prev_fee, destination_tag, occupancy_fp, chain_code, dest_to,
		amount, asset, canon_digest, sig_digest, state, occ_id, idempotency_key, actor,
		signed_at, broadcast_at, superseded_at, final_at, blocked_at, created_at, updated_at
		FROM custody_feebump_gens WHERE state IN ('signed','broadcast') LIMIT ?`, limit)
	  if err != nil {
		  return nil, ErrStore
	   }
	  defer rows.Close()
	   return scanGens(rows)
   }

   func (s *SQLStore) CASGen(ctx context.Context, genID string, from, to GenState, patch FeeBumpGen) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	   }
	   if !AllowGenTransition(from, to) {
		  return ErrInvalidTransition
	   }
	   res, err := s.db.ExecContext(ctx, `UPDATE custody_feebump_gens SET
		state=?, new_broadcast_hash=?, occupancy_fp=?, occ_id=?,
		broadcast_at=?, superseded_at=?, final_at=?, blocked_at=?, updated_at=?
		WHERE gen_id=? AND state=?`,
		   string(to), patch.NewBroadcastHash, patch.OccupancyFP, patch.OccID,
		   nullTime(patch.BroadcastAt), nullTime(patch.SupersededAt), nullTime(patch.FinalAt),
		   nullTime(patch.BlockedAt), patch.UpdatedAt, strings.TrimSpace(genID), string(from))
	   if err != nil {
		   return ErrStore
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	  }
	   return nil
   }

   func (s *SQLStore) SaveOcc(ctx context.Context, o BroadcastOccupancy) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, `INSERT INTO custody_broadcast_occs (
		occ_id, gen_id, transfer_id, generation, broadcast_hash, occupancy_fp,
		asset, amount, fee, state, held_at, retired_at, orphaned_at, expire_at,
		actor, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   o.OccID, o.GenID, o.TransferID, o.Generation, o.BroadcastHash, o.OccupancyFP,
		   o.Asset,
		  o.Amount, o.Fee, string(o.State), o.HeldAt, nullTime(o.RetiredAt),
		   nullTime(o.OrphanedAt), o.ExpireAt, o.Actor, o.CreatedAt, o.UpdatedAt)
	   if err != nil {
		   if isDup(err) {
			   return ErrIdem
		   }
		   return ErrStore
	  }
	   return nil
   }

   func (s *SQLStore) GetOcc(ctx context.Context, occID string) (BroadcastOccupancy, error) {
	  if s == nil || s.db == nil {
		  return BroadcastOccupancy{}, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, occSelect+` WHERE occ_id=?`, strings.TrimSpace(occID))
	  return scanOcc(row)
   }

   func (s *SQLStore) GetOccByGen(ctx context.Context, genID string) (BroadcastOccupancy, error) {
	   if s == nil || s.db == nil {
		  return BroadcastOccupancy{},
			   ErrUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, occSelect+` WHERE gen_id=?`, strings.TrimSpace(genID))
	  return scanOcc(row)
  }

   func (s *SQLStore) ListHeldByTransfer(ctx context.Context, transferID string) ([]BroadcastOccupancy, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, occSelect+` WHERE transfer_id=? AND state='held'`, strings.TrimSpace(transferID))
	  if err != nil {
		   return nil, ErrStore
	   }
	  defer rows.Close()
	   return scanOccs(rows)
  }

   func (s *SQLStore) ListHeldExpired(ctx context.Context, now time.Time, limit int) ([]BroadcastOccupancy, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 64
	   }
	   rows, err := s.db.QueryContext(ctx, occSelect+` WHERE state='held' AND expire_at<=? LIMIT ?`, now, limit)
	  if err != nil {
		  return nil, ErrStore
	  }
	   defer rows.Close()
	   return scanOccs(rows)
  }

   func (s *SQLStore) CASOcc(ctx context.Context, occID string, from, to OccState, patch BroadcastOccupancy) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	  if !AllowOccTransition(from, to) {
		   return ErrInvalidTransition
	  }
	   res, err := s.db.ExecContext(ctx, `UPDATE custody_broadcast_occs SET
		state=?, retired_at=?, orphaned_at=?, updated_at=? WHERE occ_id=? AND state=?`,
		   string(to),
		   nullTime(patch.RetiredAt), nullTime(patch.OrphanedAt), patch.UpdatedAt,
		   strings.TrimSpace(occID),
		   string(from))
	  if err != nil {
		  return ErrStore
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrInvalidTransition
	   }
	  return nil
  }

   func (s *SQLStore) CountHeld(ctx context.Context, transferID string) (int, error) {
	   if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	  }
	  var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custody_broadcast_occs WHERE transfer_id=? AND state='held'`,
		   strings.TrimSpace(transferID)).Scan(&n)
	   if err != nil {
		  return 0, ErrStore
	   }
	   return n, nil
  }

   func (s *SQLStore) SaveProof(ctx context.Context, p RbfOrphanProof) error {
	   if s == nil || s.db == nil {
		   return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `INSERT IGNORE INTO custody_rbf_orphan_proofs (
		proof_id, transfer_id, gen_id, prev_gen_id, occ_id, kind, state,
		quoted_hash, live_hash, quoted_gen, live_gen, held_occ_count, detail,
		actor, at_ts, closed_at, created_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  p.ProofID, p.TransferID, p.GenID, p.PrevGenID, p.OccID, string(p.Kind), string(p.State),
		  p.QuotedHash, p.LiveHash, p.QuotedGen, p.LiveGen, p.HeldOccCount, p.Detail,
		   p.Actor, p.At, nullTime(p.ClosedAt), p.CreatedAt)
	  if err != nil {
		   return ErrStore
	   }
	  return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (RbfOrphanProof, error) {
	  if s == nil || s.db == nil {
		  return RbfOrphanProof{}, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, proofSelect+` WHERE proof_id=?`, strings.TrimSpace(proofID))
	  return scanProof(row)
  }

  func (s *SQLStore) ListProofsByTransfer(ctx context.Context, transferID string) ([]RbfOrphanProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   rows, err := s.db.QueryContext(ctx, proofSelect+` WHERE transfer_id=?`, strings.TrimSpace(transferID))
	  if err != nil {
		  return nil, ErrStore
	  }
	   defer rows.Close()
	   return scanProofs(rows)
  }

   func (s *SQLStore) ListOpenOrphans(ctx context.Context, limit int) ([]RbfOrphanProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 64
	  }
	  rows, err := s.db.QueryContext(ctx, proofSelect+` WHERE state='open' LIMIT ?`, limit)
	  if err != nil {
		  return nil, ErrStore
	   }
	  defer rows.Close()
	  return scanProofs(rows)
  }

   func (s *SQLStore) ListProofsByKind(ctx context.Context, kind ProofKind, limit int) ([]RbfOrphanProof, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	  }
	   rows, err := s.db.QueryContext(ctx, proofSelect+` WHERE kind=? LIMIT ?`, string(kind), limit)
	  if err != nil {
		  return nil, ErrStore
	  }
	   defer rows.Close()
	  return scanProofs(rows)
   }

  func (s *SQLStore) CASProofState(ctx context.Context, proofID string, from, to OrphanState, patch RbfOrphanProof) error {
	  if s == nil || s.db == nil {
		   return ErrUnavailable
	   }
	  res, err := s.db.ExecContext(ctx, `UPDATE custody_rbf_orphan_proofs SET state=?, closed_at=?, actor=?
		WHERE proof_id=? AND state=?`,
		   string(to), nullTime(patch.ClosedAt), patch.Actor, strings.TrimSpace(proofID), string(from))
	   if err != nil {
		   return ErrStore
	   }
	  n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrInvalidTransition
	   }
	  return nil
  }

   func (s *SQLStore) AppendAudit(ctx context.Context, a GenAudit) error {
	   if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	  _, err := s.db.ExecContext(ctx, `INSERT INTO custody_feebump_audits (
		audit_id, gen_id, transfer_id, action, from_state, to_state, actor, detail, at_ts
	) VALUES (?,?,?,?,?,?,?,?,?)`,
		   a.AuditID, a.GenID, a.TransferID, a.Action, a.FromState, a.ToState, a.Actor, a.Detail, a.At)
	   if err != nil {
		   return ErrStore
	   }
	  return nil
  }

  func (s *SQLStore) ListAudit(ctx context.Context, genID string) ([]GenAudit, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, `SELECT audit_id, gen_id, transfer_id, action, from_state, to_state, actor, detail, at_ts
		FROM custody_feebump_audits WHERE gen_id=?`, strings.TrimSpace(genID))
	   if err != nil {
		   return nil, ErrStore
	  }
	   defer rows.Close()
	   out := make([]GenAudit, 0)
	  for rows.Next() {
		   var a GenAudit
		   if err := rows.Scan(&a.AuditID, &a.GenID, &a.TransferID, &a.Action, &a.FromState, &a.ToState, &a.Actor, &a.Detail, &a.At); err != nil {
			   return nil, ErrStore
		  }
		  out = append(out, a)
	   }
	  return out, nil
   }

   func (s *SQLStore) NextGeneration(ctx context.Context, transferID string) (int64, error) {
	   if s == nil || s.db == nil {
		  return 0, ErrUnavailable
	  }
	   tid := strings.TrimSpace(transferID)
	   _, err := s.db.ExecContext(ctx, `INSERT INTO custody_feebump_epochs (transfer_id, generation, updated_at)
		VALUES (?,1,UTC_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE generation=generation+1, updated_at=UTC_TIMESTAMP(3)`, tid)
	  if err != nil {
		   return 0, ErrStore
	   }
	   var n int64
	  if err := s.db.QueryRowContext(ctx, `SELECT generation FROM custody_feebump_epochs WHERE transfer_id=?`, tid).Scan(&n); err != nil {
		   return 0, ErrStore
	   }
	   return n, nil
  }

  func (s *SQLStore) CurrentGeneration(ctx context.Context, transferID string) (int64, error) {
	   if s == nil || s.db == nil {
		   return 0, ErrUnavailable
	   }
	  var n int64
	   err := s.db.QueryRowContext(ctx, `SELECT generation FROM custody_feebump_epochs WHERE transfer_id=?`,
		   strings.TrimSpace(transferID)).Scan(&n)
	   if err == sql.ErrNoRows {
		   return 0, nil
	   }
	  if err != nil {
		  return 0, ErrStore
	  }
	   return n, nil
   }

const occSelect = `SELECT occ_id, gen_id, transfer_id, generation, broadcast_hash, occupancy_fp,
	asset, amount, fee, state, held_at, retired_at, orphaned_at, expire_at, actor, created_at, updated_at
	FROM custody_broadcast_occs`

const proofSelect = `SELECT proof_id, transfer_id, gen_id, prev_gen_id, occ_id, kind, state,
	quoted_hash, live_hash, quoted_gen, live_gen, held_occ_count, detail, actor, at_ts, closed_at, created_at
	FROM custody_rbf_orphan_proofs`

type rowScanner interface {
	Scan(dest ...any) error
}

  func scanGen(row rowScanner) (FeeBumpGen, error) {
	   var g FeeBumpGen
	   var state string
	   var bcast, super, fin, blk sql.NullTime
	   err := row.Scan(&g.GenID, &g.TransferID, &g.Generation, &g.PrevBroadcastHash, &g.NewBroadcastHash,
		  &g.NewFee, &g.PrevFee, &g.DestinationTag, &g.OccupancyFP, &g.Chain, &g.To, &g.Amount,
		   &g.Asset, &g.CanonDigest, &g.SigDigest, &state, &g.OccID, &g.IdempotencyKey, &g.Actor,
		  &g.SignedAt, &bcast, &super, &fin, &blk, &g.CreatedAt, &g.UpdatedAt)
	  if err == sql.ErrNoRows {
		   return FeeBumpGen{}, ErrGenNotFound
	   }
	   if err != nil {
		   return FeeBumpGen{}, ErrStore
	   }
	   g.State = GenState(state)
	   g.BroadcastAt = ptrTime(bcast)
	  g.SupersededAt = ptrTime(super)
	   g.FinalAt = ptrTime(fin)
	  g.BlockedAt = ptrTime(blk)
	   return g, nil
  }

  func scanGens(rows *sql.Rows) ([]FeeBumpGen, error) {
	  out := make([]FeeBumpGen, 0)
	   for rows.Next() {
		   g, err := scanGen(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, g)
	  }
	  return out, nil
  }

   func scanOcc(row rowScanner) (BroadcastOccupancy, error) {
	   var o BroadcastOccupancy
	   var state string
	  var ret, orp sql.NullTime
	   err := row.Scan(&o.OccID, &o.GenID, &o.TransferID, &o.Generation, &o.BroadcastHash, &o.OccupancyFP,
		   &o.Asset, &o.Amount, &o.Fee, &state, &o.HeldAt, &ret, &orp, &o.ExpireAt, &o.Actor, &o.CreatedAt, &o.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return BroadcastOccupancy{}, ErrOccNotFound
	  }
	  if err != nil {
		  return BroadcastOccupancy{}, ErrStore
	  }
	   o.State = OccState(state)
	   o.RetiredAt = ptrTime(ret)
	  o.OrphanedAt = ptrTime(orp)
	  return o, nil
  }

   func scanOccs(rows *sql.Rows) ([]BroadcastOccupancy, error) {
	  out := make([]BroadcastOccupancy,
		  0)
	  for rows.Next() {
		   o, err := scanOcc(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, o)
	   }
	  return out, nil
   }

   func scanProof(row rowScanner) (RbfOrphanProof, error) {
	  var p RbfOrphanProof
	  var kind, state string
	  var closed sql.NullTime
	  err := row.Scan(&p.ProofID, &p.TransferID, &p.GenID, &p.PrevGenID, &p.OccID, &kind, &state,
		  &p.QuotedHash, &p.LiveHash, &p.QuotedGen, &p.LiveGen, &p.HeldOccCount, &p.Detail,
		   &p.Actor, &p.At, &closed, &p.CreatedAt)
	   if err == sql.ErrNoRows {
		  return RbfOrphanProof{},
			  ErrProofNotFound
	  }
	   if err != nil {
		  return RbfOrphanProof{}, ErrStore
	   }
	   p.Kind = ProofKind(kind)
	  p.State = OrphanState(state)
	  p.ClosedAt = ptrTime(closed)
	  return p, nil
  }

  func scanProofs(rows *sql.Rows) ([]RbfOrphanProof, error) {
	   out := make([]RbfOrphanProof, 0)
	  for rows.Next() {
		  p, err := scanProof(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	  }
	  return out, nil
  }

   func nullTime(t *time.Time) any {
	   if t == nil || t.IsZero() {
		   return nil
	   }
	   return *t
   }

   func ptrTime(n sql.NullTime) *time.Time {
	  if !n.Valid {
		  return nil
	   }
	  t := n.Time
	  return &t
   }

   func isDup(err error) bool {
	   if err == nil {
		   return false
	   }
	  s := err.Error()
	  return strings.Contains(s, "Duplicate") || strings.Contains(s, "UNIQUE")
   }
