package shardholdgen

import (
	"context"
	"database/sql"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore 基于 persist.SQLDB。
type SQLStore struct {
	db *persist.SQLDB
}

  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   if db == nil {
		   panic("shardholdgen: SQLStore requires non-nil SQLDB")
	   }
	  return &SQLStore{db: db}
   }

   func (s *SQLStore) q(ctx context.Context) persist.Queryer {
	  return s.db.Conn(ctx)
  }

const genCols = `gen_id, plan_id, leg_id, hold_id, idempotency_key, user_id, asset, amount, hold_amt,
		fingerprint, generation, state, ttl_seconds, issued_at, held_at, released_at, expired_at,
		broadcast_id, live_hash, release_amt, mutex_owner, advance_count, release_count, created_at, updated_at`

   func (s *SQLStore) SaveGen(ctx context.Context, g LegHoldGen) error {
	  const q = `INSERT INTO wdshard_leg_hold_gens (` + genCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.q(ctx).ExecContext(ctx, q,
		   g.GenID, g.PlanID, g.LegID, g.HoldID, g.IdempotencyKey, g.UserID, g.Asset, g.Amount, g.HoldAmt,
		   g.Fingerprint,
		  g.Generation, string(g.State), g.TTLSeconds, g.IssuedAt, nullTime(g.HeldAt),
		  nullTime(g.ReleasedAt), nullTime(g.ExpiredAt), g.BroadcastID, g.LiveHash, g.ReleaseAmt,
		  g.MutexOwner, g.AdvanceCount, g.ReleaseCount, g.CreatedAt, g.UpdatedAt,
	   )
	  if err != nil {
		   return WrapStore("save_gen", err)
	  }
	  return nil
  }

   func (s *SQLStore) GetGen(ctx context.Context, genID string) (LegHoldGen, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+genCols+` FROM wdshard_leg_hold_gens WHERE gen_id=?`, genID)
	   g, err := scanGen(row)
	  if err == sql.ErrNoRows {
		  return LegHoldGen{}, ErrGenNotFound
	  }
	   if err != nil {
		   return LegHoldGen{}, WrapStore("get_gen", err)
	  }
	   return g, nil
   }

  func (s *SQLStore) GetGenByIdem(ctx context.Context, key string) (LegHoldGen, bool, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+genCols+` FROM wdshard_leg_hold_gens WHERE idempotency_key=? LIMIT 1`, key)
	   g, err := scanGen(row)
	   if err == sql.ErrNoRows {
		   return LegHoldGen{}, false, nil
	   }
	   if err != nil {
		  return LegHoldGen{}, false, WrapStore("get_gen_idem", err)
	   }
	   return g, true, nil
  }

   func (s *SQLStore) GetGenByLeg(ctx context.Context, planID, legID string) (LegHoldGen, bool, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+genCols+` FROM wdshard_leg_hold_gens WHERE plan_id=? AND leg_id=? LIMIT 1`, planID, legID)
	  g, err := scanGen(row)
	  if err == sql.ErrNoRows {
		   return LegHoldGen{}, false, nil
	  }
	  if err != nil {
		  return LegHoldGen{}, false, WrapStore("get_gen_leg", err)
	  }
	   return g, true, nil
   }

  func (s *SQLStore) UpdateGen(ctx context.Context, g LegHoldGen) error {
	  const q = `UPDATE wdshard_leg_hold_gens SET hold_id=?, hold_amt=?, fingerprint=?, generation=?, state=?,
		held_at=?, released_at=?, expired_at=?, broadcast_id=?, live_hash=?, release_amt=?, mutex_owner=?,
		advance_count=?, release_count=?, updated_at=? WHERE gen_id=?`
	  _, err := s.q(ctx).ExecContext(ctx, q,
		  g.HoldID, g.HoldAmt, g.Fingerprint, g.Generation, string(g.State),
		  nullTime(g.HeldAt), nullTime(g.ReleasedAt), nullTime(g.ExpiredAt), g.BroadcastID, g.LiveHash,
		  g.ReleaseAmt, g.MutexOwner, g.AdvanceCount, g.ReleaseCount, g.UpdatedAt, g.GenID,
	   )
	   if err != nil {
		  return WrapStore("update_gen", err)
	  }
	  return nil
  }

  func (s *SQLStore) ListGensByState(ctx context.Context, state GenState, limit int) ([]LegHoldGen, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	   rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+genCols+` FROM wdshard_leg_hold_gens WHERE state=? LIMIT ?`, string(state), limit)
	   if err != nil {
		   return nil, WrapStore("list_gen_state", err)
	   }
	   defer rows.Close()
	   return scanGens(rows)
  }

  func (s *SQLStore) ListExpiredActive(ctx context.Context, now time.Time, limit int) ([]LegHoldGen, error) {
	   if limit <= 0 {
		  limit = ExpireBatchSize
	   }
	  rows, err := s.q(ctx).QueryContext(ctx,
		   `SELECT `+genCols+` FROM wdshard_leg_hold_gens WHERE state IN ('issued','held') AND DATE_ADD(issued_at, INTERVAL ttl_seconds SECOND) < ? LIMIT ?`,
		   now, limit)
	   if err != nil {
		  return nil, WrapStore("list_expired", err)
	  }
	  defer rows.Close()
	   return scanGens(rows)
   }

   func (s *SQLStore) TryLockLeg(ctx context.Context,
	   planID, legID, owner string) (bool, error) {
	   res, err := s.q(ctx).ExecContext(ctx,
		  `UPDATE wdshard_leg_hold_gens SET mutex_owner=? WHERE plan_id=? AND leg_id=? AND (mutex_owner='' OR mutex_owner=?)`,
		   owner, planID, legID, owner)
	   if err != nil {
		   return false, WrapStore("try_lock", err)
	   }
	   n, _ := res.RowsAffected()
	  return n > 0, nil
  }

  func (s *SQLStore) UnlockLeg(ctx context.Context, planID, legID, owner string) error {
	   _, err := s.q(ctx).ExecContext(ctx,
		   `UPDATE wdshard_leg_hold_gens SET mutex_owner='' WHERE plan_id=? AND leg_id=? AND (mutex_owner=? OR ?= '')`,
		  planID, legID, owner, owner)
	   if err != nil {
		  return WrapStore("unlock", err)
	  }
	   return nil
  }

const proofCols = `proof_id, kind, gen_id, plan_id, leg_id, hold_id, idempotency_key, amount, bind_gen, live_gen, 
		bind_hash, live_hash, digest, evidence, detected_at, exported_at, created_at`

   func (s *SQLStore) SaveProof(ctx context.Context, p ShardHoldProof) error {
	   const q = `INSERT INTO wdshard_hold_proofs (` + proofCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.q(ctx).ExecContext(ctx, q,
		   p.ProofID, string(p.Kind), p.GenID, p.PlanID, p.LegID, p.HoldID, p.IdemKey, p.Amount,
		   p.BindGen, p.LiveGen, p.BindHash, p.LiveHash, p.Digest, p.Evidence, p.DetectedAt,
		   nullTime(p.ExportedAt), p.CreatedAt,
	   )
	  if err != nil {
		   return WrapStore("save_proof", err)
	   }
	  return nil
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (ShardHoldProof, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+proofCols+` FROM wdshard_hold_proofs WHERE proof_id=?`, proofID)
	  p, err := scanProof(row)
	   if err == sql.ErrNoRows {
		  return ShardHoldProof{}, ErrProofNotFound
	  }
	  if err != nil {
		  return ShardHoldProof{}, WrapStore("get_proof", err)
	   }
	  return p, nil
  }

   func (s *SQLStore) ListProofs(ctx context.Context, filter ExportFilter) ([]ShardHoldProof, error) {
	   if filter.Limit <= 0 {
		  filter.Limit = ProofExportBatch
	   }
	   q := `SELECT ` + proofCols + ` FROM wdshard_hold_proofs WHERE 1=1`
	   args := make([]any, 0)
	  if filter.Kind != "" {
		   q += ` AND kind=?`
		   args = append(args, string(filter.Kind))
	  }
	  if filter.GenID != "" {
		   q += ` AND gen_id=?`
		  args = append(args, filter.GenID)
	   }
	   if filter.LegID != "" {
		  q += ` AND leg_id=?`
		  args = append(args, filter.LegID)
	   }
	  if filter.UnexportedOnly {
		   q += ` AND exported_at IS NULL`
	  }
	  q += ` ORDER BY detected_at DESC LIMIT ?`
	   args = append(args, filter.Limit)
	   rows, err := s.q(ctx).QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, WrapStore("list_proofs", err)
	   }
	   defer rows.Close()
	  out := make([]ShardHoldProof, 0)
	  for rows.Next() {
		  p, err := scanProof(rows)
		   if err != nil {
			  return nil, WrapStore("scan_proof", err)
		   }
		   out = append(out, p)
	  }
	  return out, rows.Err()
  }

  func (s *SQLStore) MarkProofsExported(ctx context.Context, ids []string, at time.Time) error {
	   for _, id := range ids {
		  if _, err := s.q(ctx).ExecContext(ctx, `UPDATE wdshard_hold_proofs SET exported_at=? WHERE proof_id=?`, at, id); err != nil {
			  return WrapStore("mark_export", err)
		   }
	   }
	   return nil
  }

const halfCols = `case_id, gen_id, plan_id, leg_id, hold_id, kind, state, actor, resume_mutex, note, created_at, updated_at`

  func (s *SQLStore) SaveHalf(ctx context.Context, c HalfLegCase) error {
	  const q = `INSERT INTO wdshard_half_leg_cases (` + halfCols + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	   _,
		  err := s.q(ctx).ExecContext(ctx, q,
		   c.CaseID, c.GenID, c.PlanID, c.LegID, c.HoldID, string(c.Kind), string(c.State),
		  c.Actor, boolInt(c.ResumeMu), c.Note, c.CreatedAt, c.UpdatedAt,
	   )
	  if err != nil {
		  return WrapStore("save_half", err)
	   }
	   return nil
   }

   func (s *SQLStore) GetHalf(ctx context.Context, caseID string) (HalfLegCase, error) {
	   row := s.q(ctx).QueryRowContext(ctx, `SELECT `+halfCols+` FROM wdshard_half_leg_cases WHERE case_id=?`, caseID)
	  c, err := scanHalf(row)
	  if err == sql.ErrNoRows {
		  return HalfLegCase{}, ErrHalfNotFound
	   }
	   if err != nil {
		   return HalfLegCase{}, WrapStore("get_half", err)
	  }
	  return c, nil
  }

  func (s *SQLStore) GetOpenHalfByGen(ctx context.Context, genID string) (HalfLegCase, bool, error) {
	  row := s.q(ctx).QueryRowContext(ctx, `SELECT `+halfCols+` FROM wdshard_half_leg_cases WHERE gen_id=? AND state='open' LIMIT 1`, genID)
	  c, err := scanHalf(row)
	  if err == sql.ErrNoRows {
		   return HalfLegCase{}, false, nil
	  }
	  if err != nil {
		  return HalfLegCase{}, false, WrapStore("get_open_half", err)
	  }
	  return c, true, nil
   }

  func (s *SQLStore) UpdateHalf(ctx context.Context, c HalfLegCase) error {
	  _, err := s.q(ctx).ExecContext(ctx,
		  `UPDATE wdshard_half_leg_cases SET state=?, actor=?, resume_mutex=?, note=?, updated_at=? WHERE case_id=?`,
		  string(c.State), c.Actor, boolInt(c.ResumeMu), c.Note, c.UpdatedAt, c.CaseID)
	  if err != nil {
		  return WrapStore("update_half", err)
	  }
	  return nil
  }

   func (s *SQLStore) ListOpenHalf(ctx context.Context, limit int) ([]HalfLegCase, error) {
	   if limit <= 0 {
		   limit = PatrolBatchSize
	   }
	   rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+halfCols+` FROM wdshard_half_leg_cases WHERE state='open' LIMIT ?`, limit)
	   if err != nil {
		  return nil, WrapStore("list_open_half", err)
	  }
	   defer rows.Close()
	   out := make([]HalfLegCase, 0)
	   for rows.Next() {
		  c, err := scanHalf(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, c)
	   }
	  return out, rows.Err()
  }

  func (s *SQLStore) SaveAudit(ctx context.Context, a GenAudit) error {
	  _, err := s.q(ctx).ExecContext(ctx,
		   `INSERT INTO wdshard_hold_gen_audit (audit_id, gen_id, action, actor, from_state, to_state, note, at) VALUES (?,?,?,?,?,?,?,?)`,
		  a.AuditID, a.GenID, string(a.Action), a.Actor, string(a.FromState), string(a.ToState), a.Note, a.At)
	  if err != nil {
		   return WrapStore("save_audit", err)
	   }
	   return nil
   }

   func (s *SQLStore) ListAudit(ctx context.Context, genID string, limit int) ([]GenAudit, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	   rows, err := s.q(ctx).QueryContext(ctx,
		  `SELECT audit_id, gen_id, action, actor, from_state, to_state, note, at FROM wdshard_hold_gen_audit WHERE gen_id=? ORDER BY at DESC LIMIT ?`,
		   genID, limit)
	   if err != nil {
		  return nil, WrapStore("list_audit", err)
	   }
	  defer rows.Close()
	   out := make([]GenAudit, 0)
	  for rows.Next() {
		  var a GenAudit
		  var act, from, to string
		  if err := rows.Scan(&a.AuditID, &a.GenID, &act, &a.Actor, &from, &to, &a.Note, &a.At); err != nil {
			  return nil, WrapStore("scan_audit", err)
		  }
		   a.Action = AuditAction(act)
		   a.FromState = GenState(from)
		   a.ToState = GenState(to)
		  out = append(out, a)
	  }
	  return out, rows.Err()
  }

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanGen(row rowScanner) (LegHoldGen, error) {
	   var g LegHoldGen
	  var state string
	   var held, rel, exp sql.NullTime
	  if err := row.Scan(
		  &g.GenID, &g.PlanID, &g.LegID, &g.HoldID, &g.IdempotencyKey, &g.UserID, &g.Asset, &g.Amount, &g.HoldAmt,
		  &g.Fingerprint, &g.Generation, &state, &g.TTLSeconds, &g.IssuedAt, &held, &rel, &exp,
		  &g.BroadcastID,
		  &g.LiveHash, &g.ReleaseAmt, &g.MutexOwner, &g.AdvanceCount, &g.ReleaseCount, &g.CreatedAt, &g.UpdatedAt,
	  ); err != nil {
		   return LegHoldGen{}, err
	   }
	   g.State = GenState(state)
	   g.HeldAt = nullTimePtr(held)
	  g.ReleasedAt = nullTimePtr(rel)
	   g.ExpiredAt = nullTimePtr(exp)
	  return g, nil
   }

   func scanGens(rows *sql.Rows) ([]LegHoldGen, error) {
	  out := make([]LegHoldGen, 0)
	   for rows.Next() {
		   g, err := scanGen(rows)
		  if err != nil {
			   return nil, WrapStore("scan_gen", err)
		  }
		  out = append(out, g)
	   }
	   return out, rows.Err()
   }

  func scanProof(row rowScanner) (ShardHoldProof, error) {
	   var p ShardHoldProof
	   var kind string
	   var exp sql.NullTime
	  if err := row.Scan(
		   &p.ProofID, &kind, &p.GenID, &p.PlanID, &p.LegID, &p.HoldID, &p.IdemKey, &p.Amount,
		   &p.BindGen,
		   &p.LiveGen, &p.BindHash, &p.LiveHash, &p.Digest, &p.Evidence, &p.DetectedAt, &exp, &p.CreatedAt,
	   ); err != nil {
		   return ShardHoldProof{}, err
	   }
	  p.Kind = ProofKind(kind)
	  p.ExportedAt = nullTimePtr(exp)
	   return p, nil
  }

  func scanHalf(row rowScanner) (HalfLegCase, error) {
	  var c HalfLegCase
	   var kind, state string
	  var mu int
	   if err := row.Scan(&c.CaseID, &c.GenID, &c.PlanID, &c.LegID, &c.HoldID, &kind, &state, &c.Actor, &mu, &c.Note, &c.CreatedAt, &c.UpdatedAt); err != nil {
		   return HalfLegCase{}, err
	   }
	  c.Kind = HalfKind(kind)
	  c.State = HalfState(state)
	  c.ResumeMu = mu != 0
	   return c, nil
   }

   func nullTime(t *time.Time) any {
	   if t == nil {
		  return nil
	   }
	   return *t
  }

   func nullTimePtr(n sql.NullTime) *time.Time {
	  if !n.Valid {
		   return nil
	  }
	  t := n.Time
	   return &t
   }

   func boolInt(v bool) int {
	   if v {
		   return 1
	   }
	  return 0
  }
