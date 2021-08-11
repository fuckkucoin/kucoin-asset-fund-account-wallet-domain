// SQL 存储：late_credit_gens / late_credit_race_events / late_credit_proofs。

// Author: Cason
// Date: 2021.08.07

package latecreditgate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLStore MySQL / PostgreSQL 实现。
type SQLStore struct {
	db      *sql.DB
	dialect string
}

// NewSQLStore 创建 SQL 仓。
   func NewSQLStore(db *sql.DB, dialect string) *SQLStore {
	  return &SQLStore{db: db, dialect: dialect}
  }

   func (s *SQLStore) rewrite(q string) string {
	  if s.dialect != "postgres" {
		  return q
	   }
	  n := 0
	  out := make([]byte, 0, len(q)+16)
	   for i := 0; i < len(q); i++ {
		   if q[i] == '?' {
			  n++
			  out = append(out, '$')
			  out = append(out, []byte(fmt.Sprintf("%d", n))...)
		  } else {
			  out = append(out, q[i])
		   }
	   }
	  return string(out)
  }

   func (s *SQLStore) GetGen(ctx context.Context, key OutputKey) (*LateCreditGeneration, error) {
	   q := `SELECT gen_id, chain, tx_hash, output_index, generation, state,
	             holder_kind, holder_actor, occupancy_id, case_id, asset, amount,
	             fence_epoch, fence_token, created_at, updated_at
	      FROM late_credit_gens WHERE chain=? AND tx_hash=? AND output_index=?`
	  row := s.db.QueryRowContext(ctx, s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	  return scanGen(row)
  }

   func (s *SQLStore) GetGenByID(ctx context.Context, genID string) (*LateCreditGeneration, error) {
	  q := `SELECT gen_id, chain, tx_hash, output_index, generation, state,
	             holder_kind, holder_actor, occupancy_id, case_id, asset, amount,
	             fence_epoch, fence_token, created_at, updated_at
	      FROM late_credit_gens WHERE gen_id=?`
	   return scanGen(s.db.QueryRowContext(ctx, s.rewrite(q), genID))
  }

  func scanGen(row *sql.Row) (*LateCreditGeneration, error) {
	  var g LateCreditGeneration
	   var occ, caseID, asset, amount sql.NullString
	   err := row.Scan(
		   &g.GenID,
		   &g.OutputKey.Chain, &g.OutputKey.TxHash, &g.OutputKey.OutputIndex,
		  &g.Generation, &g.State, &g.HolderKind, &g.HolderActor,
		   &occ, &caseID, &asset, &amount,
		  &g.FenceEpoch,
		  &g.FenceToken, &g.CreatedAt, &g.UpdatedAt,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrGenNotFound
		   }
		   return nil, err
	   }
	  if occ.Valid {
		   g.OccupancyID = occ.String
	  }
	   if caseID.Valid {
		   g.CaseID = caseID.String
	  }
	   if asset.Valid {
		   g.Asset = asset.String
	   }
	  if amount.Valid {
		   g.Amount = amount.String
	   }
	  return &g, nil
  }

   func (s *SQLStore) InsertGen(ctx context.Context, g LateCreditGeneration) error {
	   q := `INSERT INTO late_credit_gens
	      (gen_id, chain, tx_hash, output_index, generation, state,
	       holder_kind, holder_actor, occupancy_id, case_id, asset, amount,
	       fence_epoch, fence_token, created_at, updated_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _,
		   err := s.db.ExecContext(ctx, s.rewrite(q),
		  g.GenID, g.OutputKey.Chain, g.OutputKey.TxHash, g.OutputKey.OutputIndex,
		  g.Generation,
		  string(g.State), g.HolderKind, g.HolderActor,
		  nullStr(g.OccupancyID), nullStr(g.CaseID), nullStr(g.Asset), nullStr(g.Amount),
		   g.FenceEpoch, g.FenceToken, g.CreatedAt, g.UpdatedAt,
	  )
	  if err != nil && isDup(err) {
		  return ErrOccupyConflict
	  }
	   return err
   }

  func (s *SQLStore) CASUpdateGen(ctx context.Context, genID string, expectedGen int64, expectedState GenState, next LateCreditGeneration) error {
	   if next.Generation < expectedGen {
		  return ErrGenRollback
	   }
	   q := `UPDATE late_credit_gens SET generation=?, state=?, holder_kind=?, holder_actor=?,
	             occupancy_id=?, case_id=?, asset=?, amount=?, fence_epoch=?, fence_token=?, updated_at=?
	      WHERE gen_id=? AND generation=? AND state=?`
	   res,
		   err := s.db.ExecContext(ctx, s.rewrite(q),
		  next.Generation,
		   string(next.State), next.HolderKind, next.HolderActor,
		   nullStr(next.OccupancyID), nullStr(next.CaseID), nullStr(next.Asset), nullStr(next.Amount),
		   next.FenceEpoch, next.FenceToken, next.UpdatedAt,
		  genID, expectedGen, string(expectedState),
	  )
	  if err != nil {
		  return err
	   }
	   return requireOneRow(res, ErrGenMismatch)
  }

  func (s *SQLStore) ListGensByState(ctx context.Context, state GenState, limit int) ([]LateCreditGeneration, error) {
	  q := `SELECT gen_id, chain, tx_hash, output_index, generation, state, 
	             holder_kind, holder_actor, occupancy_id, case_id, asset, amount,
	             fence_epoch, fence_token, created_at, updated_at
	      FROM late_credit_gens WHERE state=? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), string(state), limit)
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanGenRows(rows)
  }

  func (s *SQLStore) ListOccupiedExpired(ctx context.Context, now time.Time, ttl time.Duration, limit int) ([]LateCreditGeneration, error) {
	   cut := now.Add(-ttl)
	   q := `SELECT gen_id, chain, tx_hash, output_index, generation, state,
	             holder_kind, holder_actor, occupancy_id, case_id, asset, amount,
	             fence_epoch, fence_token, created_at, updated_at
	      FROM late_credit_gens WHERE state='occupied' AND updated_at < ? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), cut, limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  return scanGenRows(rows)
   }

  func scanGenRows(rows *sql.Rows) ([]LateCreditGeneration, error) {
	  var out []LateCreditGeneration
	   for rows.Next() {
		   var g LateCreditGeneration
		   var occ, caseID, asset, amount sql.NullString
		  if err := rows.Scan(
			   &g.GenID,
			   &g.OutputKey.Chain, &g.OutputKey.TxHash, &g.OutputKey.OutputIndex,
			   &g.Generation, &g.State, &g.HolderKind, &g.HolderActor,
			  &occ, &caseID, &asset, &amount,
			   &g.FenceEpoch, &g.FenceToken, &g.CreatedAt, &g.UpdatedAt,
		  ); err != nil {
			  return nil, err
		   }
		  if occ.Valid {
			  g.OccupancyID = occ.String
		  }
		  if caseID.Valid {
			  g.CaseID = caseID.String
		   }
		  if asset.Valid {
			  g.Asset = asset.String
		   }
		  if amount.Valid {
			   g.Amount = amount.String
		   }
		  out = append(out, g)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) InsertRaceEvent(ctx context.Context, ev RaceEvent) error {
	  q := `INSERT INTO late_credit_race_events
	      (event_id, chain, tx_hash, output_index, gen_id, generation, kind, state,
	       actor_a, actor_b, detail, created_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   ev.EventID, ev.OutputKey.Chain, ev.OutputKey.TxHash, ev.OutputKey.OutputIndex,
		   ev.GenID, ev.Generation, string(ev.Kind), string(ev.State),
		   ev.ActorA, ev.ActorB, ev.Detail, ev.CreatedAt,
	   )
	   return err
   }

   func (s *SQLStore) GetRaceEvent(ctx context.Context, eventID string) (*RaceEvent, error) {
	  q := `SELECT event_id, chain, tx_hash, output_index, gen_id, generation, kind, state,
	             actor_a, actor_b, detail, created_at, acked_at
	      FROM late_credit_race_events WHERE event_id=?`
	  row := s.db.QueryRowContext(ctx, s.rewrite(q), eventID)
	   var ev RaceEvent
	  var acked sql.NullTime
	   err := row.Scan(
		   &ev.EventID, &ev.OutputKey.Chain, &ev.OutputKey.TxHash, &ev.OutputKey.OutputIndex,
		   &ev.GenID, &ev.Generation, &ev.Kind, &ev.State,
		  &ev.ActorA, &ev.ActorB, &ev.Detail, &ev.CreatedAt, &acked,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil,
				  ErrRaceEventNotFound
		  }
		   return nil, err
	   }
	  if acked.Valid {
		  t := acked.Time
		  ev.AckedAt = &t
	   }
	   return &ev, nil
   }

   func (s *SQLStore) ListOpenRaceEvents(ctx context.Context, limit int) ([]RaceEvent, error) {
	   q := `SELECT event_id, chain, tx_hash, output_index, gen_id, generation, kind, state,
	             actor_a, actor_b, detail, created_at, acked_at
	      FROM late_credit_race_events WHERE state='open' LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanRaceRows(rows)
  }

  func (s *SQLStore) ListOpenRaceEventsByKey(ctx context.Context, key OutputKey) ([]RaceEvent, error) {
	   q := `SELECT event_id, chain, tx_hash, output_index, gen_id, generation, kind, state,
	             actor_a, actor_b, detail, created_at, acked_at
	      FROM late_credit_race_events
	      WHERE chain=? AND tx_hash=? AND output_index=? AND state='open'`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanRaceRows(rows)
  }

   func scanRaceRows(rows *sql.Rows) ([]RaceEvent, error) {
	   var out []RaceEvent
	  for rows.Next() {
		   var ev RaceEvent
		   var acked sql.NullTime
		   if err := rows.Scan(
			   &ev.EventID, &ev.OutputKey.Chain, &ev.OutputKey.TxHash, &ev.OutputKey.OutputIndex,
			   &ev.GenID,
			   &ev.Generation, &ev.Kind, &ev.State,
			   &ev.ActorA, &ev.ActorB, &ev.Detail, &ev.CreatedAt, &acked,
		  ); err != nil {
			   return nil, err
		  }
		  if acked.Valid {
			   t := acked.Time
			   ev.AckedAt = &t
		  }
		   out = append(out, ev)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) AckRaceEvent(ctx context.Context, eventID string, now time.Time) error {
	   q := `UPDATE late_credit_race_events SET state='acked', acked_at=?
	      WHERE event_id=? AND state='open'`
	   res, err := s.db.ExecContext(ctx, s.rewrite(q), now, eventID)
	   if err != nil {
		   return err
	  }
	   n, err := res.RowsAffected()
	  if err != nil {
		   return err
	   }
	  if n == 0 {
		   ev, gerr := s.GetRaceEvent(ctx, eventID)
		   if gerr != nil {
			  return gerr
		  }
		   if ev.State == RaceAcked {
			  return ErrRaceEventAlreadyAcked
		   }
		   return ErrRaceEventNotFound
	  }
	   return nil
  }

  func (s *SQLStore) InsertProof(ctx context.Context, p LateCreditRaceProof) error {
	  q := `INSERT INTO late_credit_proofs
	      (proof_id, chain, tx_hash, output_index, gen_id, generation, event_id, kind,
	       occupy_actor, credit_actor, fail_closed, summary, opened_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   _,
		  err := s.db.ExecContext(ctx, s.rewrite(q),
		  p.ProofID, p.OutputKey.Chain, p.OutputKey.TxHash, p.OutputKey.OutputIndex,
		  p.GenID,
		  p.Generation, p.EventID, string(p.Kind),
		   p.OccupyActor, p.CreditActor, boolToInt(p.FailClosed), p.Summary, p.OpenedAt,
	   )
	  return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*LateCreditRaceProof, error) {
	   q := `SELECT proof_id, chain, tx_hash, output_index, gen_id, generation, event_id, kind,
	             occupy_actor, credit_actor, fail_closed, summary, opened_at, exported_at
	      FROM late_credit_proofs WHERE proof_id=?`
	   row := s.db.QueryRowContext(ctx, s.rewrite(q), proofID)
	  var p LateCreditRaceProof
	  var exported sql.NullTime
	  var fail int
	  err := row.Scan(
		   &p.ProofID,
		  &p.OutputKey.Chain, &p.OutputKey.TxHash, &p.OutputKey.OutputIndex,
		  &p.GenID, &p.Generation, &p.EventID, &p.Kind,
		   &p.OccupyActor, &p.CreditActor, &fail, &p.Summary, &p.OpenedAt, &exported,
	   )
	  if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil, ErrProofNotFound
		   }
		  return nil, err
	  }
	   p.FailClosed = fail != 0
	  if exported.Valid {
		   t := exported.Time
		   p.ExportedAt = &t
	  }
	   return &p, nil
   }

   func (s *SQLStore) ListProofs(ctx context.Context, limit int) ([]LateCreditRaceProof, error) {
	  q := `SELECT proof_id, chain, tx_hash, output_index, gen_id, generation, event_id, kind,
	             occupy_actor, credit_actor, fail_closed, summary, opened_at, exported_at
	      FROM late_credit_proofs ORDER BY opened_at DESC LIMIT ?`
	  rows,
		  err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanProofRows(rows)
   }

   func (s *SQLStore) ListProofsByKey(ctx context.Context, key OutputKey) ([]LateCreditRaceProof, error) {
	   q := `SELECT proof_id, chain, tx_hash, output_index, gen_id, generation, event_id, kind,
	             occupy_actor, credit_actor, fail_closed, summary, opened_at, exported_at
	      FROM late_credit_proofs WHERE chain=? AND tx_hash=? AND output_index=?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), key.Chain, key.TxHash, key.OutputIndex)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanProofRows(rows)
   }

   func scanProofRows(rows *sql.Rows) ([]LateCreditRaceProof, error) {
	  var out []LateCreditRaceProof
	   for rows.Next() {
		  var p LateCreditRaceProof
		  var exported sql.NullTime
		   var fail int
		  if err := rows.Scan(
			  &p.ProofID, &p.OutputKey.Chain, &p.OutputKey.TxHash, &p.OutputKey.OutputIndex,
			  &p.GenID, &p.Generation, &p.EventID, &p.Kind,
			  &p.OccupyActor, &p.CreditActor, &fail, &p.Summary, &p.OpenedAt, &exported,
		   ); err != nil {
			   return nil, err
		  }
		   p.FailClosed = fail != 0
		   if exported.Valid {
			  t := exported.Time
			  p.ExportedAt = &t
		  }
		   out = append(out, p)
	  }
	  return out, rows.Err()
   }

   func (s *SQLStore) MarkProofExported(ctx context.Context, proofID string, now time.Time) error {
	   q := `UPDATE late_credit_proofs SET exported_at=? WHERE proof_id=?`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q), now, proofID)
	   if err != nil {
		   return err
	  }
	   return requireOneRow(res, ErrProofNotFound)
   }

   func nullStr(s string) sql.NullString {
	   if s == "" {
		  return sql.NullString{}
	  }
	  return sql.NullString{String: s, Valid: true}
  }

  func requireOneRow(res sql.Result, notFound error) error {
	  n, err := res.RowsAffected()
	  if err != nil {
		  return err
	   }
	   if n == 0 {
		  return notFound
	  }
	  return nil
  }

  func isDup(err error) bool {
	  if err == nil {
		   return false
	  }
	  msg := err.Error()
	   return contains(msg, "Duplicate entry") || contains(msg, "duplicate key") || contains(msg, "UNIQUE constraint")
   }

  func contains(s, sub string) bool {
	  return len(s) >= len(sub) && (s == sub || containsStr(s, sub))
  }

   func containsStr(s, sub string) bool {
	   for i := 0; i <= len(s)-len(sub); i++ {
		  if s[i:i+len(sub)] == sub {
			   return true
		  }
	   }
	  return false
   }

   func boolToInt(b bool) int {
	  if b {
		   return 1
	  }
	  return 0
   }
