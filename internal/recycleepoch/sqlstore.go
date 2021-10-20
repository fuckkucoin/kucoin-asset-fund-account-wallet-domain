package recycleepoch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SQLStore struct {
	db *sql.DB
	dialect string
}

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

  func (s *SQLStore) GetEpoch(ctx context.Context, slot SlotKey) (*RecycleEpoch, error) {
	  q := `SELECT epoch_id, chain, address, generation, state, prev_user, next_user,
	             cool_until, marked_at, ready_at, assigned_at, actor_id, idem_key,
	             recycle_ref, created_at, updated_at
	      FROM addrpool_recycle_epochs WHERE chain=? AND address=?`
	  return scanEpoch(s.db.QueryRowContext(ctx, s.rewrite(q), slot.Chain, slot.Address))
   }

   func (s *SQLStore) GetEpochByID(ctx context.Context, epochID string) (*RecycleEpoch, error) {
	   q := `SELECT epoch_id, chain, address, generation, state, prev_user, next_user,
	             cool_until, marked_at, ready_at, assigned_at, actor_id, idem_key,
	             recycle_ref, created_at, updated_at
	      FROM addrpool_recycle_epochs WHERE epoch_id=?`
	  return scanEpoch(s.db.QueryRowContext(ctx, s.rewrite(q), epochID))
  }

   func scanEpoch(row *sql.Row) (*RecycleEpoch, error) {
	  var e RecycleEpoch
	  var next, actor, idem, ref sql.NullString
	  var ready, assigned sql.NullTime
	   err := row.Scan(
		   &e.EpochID, &e.Slot.Chain, &e.Slot.Address, &e.Generation, &e.State,
		  &e.PrevUser, &next, &e.CoolUntil, &e.MarkedAt, &ready, &assigned,
		  &actor, &idem, &ref, &e.CreatedAt, &e.UpdatedAt,
	   )
	   if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrEpochNotFound
		   }
		   return nil, err
	  }
	  if next.Valid {
		   e.NextUser = next.String
	   }
	   if actor.Valid {
		  e.ActorID = actor.String
	  }
	   if idem.Valid {
		  e.IdemKey = idem.String
	   }
	   if ref.Valid {
		   e.RecycleRef = ref.String
	   }
	   if ready.Valid {
		   t := ready.Time
		  e.ReadyAt = &t
	  }
	   if assigned.Valid {
		   t := assigned.Time
		   e.AssignedAt = &t
	  }
	  return &e, nil
   }

   func (s *SQLStore) InsertEpoch(ctx context.Context, e RecycleEpoch) error {
	   q := `INSERT INTO addrpool_recycle_epochs
	      (epoch_id, chain, address, generation, state, prev_user, next_user,
	       cool_until, marked_at, ready_at, assigned_at, actor_id, idem_key,
	       recycle_ref, created_at, updated_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   e.EpochID, e.Slot.Chain, e.Slot.Address, e.Generation, string(e.State),
		  e.PrevUser, nullStr(e.NextUser), e.CoolUntil, e.MarkedAt, nullTime(e.ReadyAt),
		   nullTime(e.AssignedAt), e.ActorID, nullStr(e.IdemKey), nullStr(e.RecycleRef),
		   e.CreatedAt, e.UpdatedAt,
	  )
	  return err
   }

  func (s *SQLStore) CASUpdateEpoch(ctx context.Context, epochID string, expectedGen int64, expectedState EpochState, next RecycleEpoch) error {
	  q := `UPDATE addrpool_recycle_epochs SET
	        generation=?, state=?, prev_user=?, next_user=?, cool_until=?,
	        marked_at=?, ready_at=?, assigned_at=?, actor_id=?, idem_key=?,
	        recycle_ref=?, updated_at=?
	      WHERE epoch_id=? AND generation=? AND state=?`
	   res, err := s.db.ExecContext(ctx, s.rewrite(q),
		  next.Generation, string(next.State), next.PrevUser, nullStr(next.NextUser),
		  next.CoolUntil, next.MarkedAt, nullTime(next.ReadyAt), nullTime(next.AssignedAt),
		  next.ActorID,
		   nullStr(next.IdemKey), nullStr(next.RecycleRef), next.UpdatedAt,
		  epochID, expectedGen, string(expectedState),
	   )
	  if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrEpochMismatch
	   }
	  if next.Generation < expectedGen {
		   return ErrEpochRollback
	  }
	  return nil
  }

  func (s *SQLStore) ListEpochsByState(ctx context.Context, state EpochState, limit int) ([]RecycleEpoch, error) {
	  if limit <= 0 {
		   limit = 100
	   }
	  q := `SELECT epoch_id, chain, address, generation, state, prev_user, next_user,
	             cool_until, marked_at, ready_at, assigned_at, actor_id, idem_key,
	             recycle_ref, created_at, updated_at
	      FROM addrpool_recycle_epochs WHERE state=? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), string(state), limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   return scanEpochRows(rows)
  }

  func (s *SQLStore) ListCoolingElapsed(ctx context.Context, now time.Time, limit int) ([]RecycleEpoch, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	   q := `SELECT epoch_id, chain, address, generation, state, prev_user, next_user,
	             cool_until, marked_at, ready_at, assigned_at, actor_id, idem_key,
	             recycle_ref, created_at, updated_at
	      FROM addrpool_recycle_epochs WHERE state='cooling' AND cool_until<=? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), now, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanEpochRows(rows)
   }

   func scanEpochRows(rows *sql.Rows) ([]RecycleEpoch, error) {
	  var out []RecycleEpoch
	  for rows.Next() {
		  var e RecycleEpoch
		   var next,
			   actor, idem, ref sql.NullString
		   var ready, assigned sql.NullTime
		  if err := rows.Scan(
			  &e.EpochID,
			   &e.Slot.Chain, &e.Slot.Address, &e.Generation, &e.State,
			  &e.PrevUser, &next, &e.CoolUntil, &e.MarkedAt, &ready, &assigned,
			   &actor,
			   &idem, &ref, &e.CreatedAt, &e.UpdatedAt,
		  ); err != nil {
			   return nil, err
		   }
		   if next.Valid {
			  e.NextUser = next.String
		   }
		   if actor.Valid {
			   e.ActorID = actor.String
		  }
		  if idem.Valid {
			   e.IdemKey = idem.String
		   }
		  if ref.Valid {
			   e.RecycleRef = ref.String
		  }
		  if ready.Valid {
			   t := ready.Time
			   e.ReadyAt = &t
		   }
		   if assigned.Valid {
			  t := assigned.Time
			   e.AssignedAt = &t
		   }
		   out = append(out, e)
	  }
	   return out, rows.Err()
  }

  func (s *SQLStore) GetBind(ctx context.Context, slot SlotKey) (*ObserveBind, error) {
	   q := `SELECT bind_id, chain, address, epoch_id, generation, user_id, state,
	             bound_at, revoked_at, revoke_reason
	      FROM addrpool_observe_binds WHERE chain=? AND address=?`
	  return scanBind(s.db.QueryRowContext(ctx, s.rewrite(q), slot.Chain, slot.Address))
  }

  func (s *SQLStore) GetBindByID(ctx context.Context, bindID string) (*ObserveBind, error) {
	  q := `SELECT bind_id, chain, address, epoch_id, generation, user_id, state, 
	             bound_at, revoked_at, revoke_reason
	      FROM addrpool_observe_binds WHERE bind_id=?`
	  return scanBind(s.db.QueryRowContext(ctx,
		  s.rewrite(q), bindID))
  }

   func scanBind(row *sql.Row) (*ObserveBind, error) {
	  var b ObserveBind
	   var rev sql.NullTime
	   var reason sql.NullString
	   err := row.Scan(
		   &b.BindID, &b.Slot.Chain, &b.Slot.Address, &b.EpochID, &b.Generation,
		  &b.UserID, &b.State, &b.BoundAt, &rev, &reason,
	  )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrBindNotFound
		  }
		  return nil, err
	   }
	  if rev.Valid {
		   t := rev.Time
		   b.RevokedAt = &t
	   }
	   if reason.Valid {
		   b.RevokeReason = reason.String
	   }
	   return &b, nil
   }

   func (s *SQLStore) UpsertBind(ctx context.Context, b ObserveBind) error {
	   q := `INSERT INTO addrpool_observe_binds
	      (bind_id, chain, address, epoch_id, generation, user_id, state, bound_at, revoked_at, revoke_reason)
	      VALUES (?,?,?,?,?,?,?,?,?,?)
	      ON DUPLICATE KEY UPDATE bind_id=VALUES(bind_id), epoch_id=VALUES(epoch_id),
	        generation=VALUES(generation), user_id=VALUES(user_id), state=VALUES(state),
	        bound_at=VALUES(bound_at), revoked_at=VALUES(revoked_at), revoke_reason=VALUES(revoke_reason)`
	   if s.dialect == "postgres" {
		  q = `INSERT INTO addrpool_observe_binds
		      (bind_id, chain, address, epoch_id, generation, user_id, state, bound_at, revoked_at, revoke_reason)
		      VALUES (?,?,?,?,?,?,?,?,?,?)
		      ON CONFLICT (chain, address) DO UPDATE SET
		        bind_id=EXCLUDED.bind_id, epoch_id=EXCLUDED.epoch_id,
		        generation=EXCLUDED.generation, user_id=EXCLUDED.user_id, state=EXCLUDED.state,
		        bound_at=EXCLUDED.bound_at, revoked_at=EXCLUDED.revoked_at, revoke_reason=EXCLUDED.revoke_reason`
	   }
	   _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   b.BindID,
		  b.Slot.Chain, b.Slot.Address, b.EpochID, b.Generation, b.UserID,
		  string(b.State), b.BoundAt, nullTime(b.RevokedAt), nullStr(b.RevokeReason),
	  )
	   return err
  }

   func (s *SQLStore) RevokeBind(ctx context.Context, slot SlotKey, reason string, now time.Time) error {
	  q := `UPDATE addrpool_observe_binds SET state='revoked', revoked_at=?, revoke_reason=?
	      WHERE chain=? AND address=? AND state='active'`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q), now, reason, slot.Chain, slot.Address)
	  return err
   }

  func (s *SQLStore) ListBindsByUser(ctx context.Context, userID string, limit int) ([]ObserveBind, error) {
	   if limit <= 0 {
		   limit = 100
	   }
	  q := `SELECT bind_id, chain, address, epoch_id, generation, user_id, state,
	             bound_at, revoked_at, revoke_reason
	      FROM addrpool_observe_binds WHERE user_id=? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), userID, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []ObserveBind
	  for rows.Next() {
		  var b ObserveBind
		   var rev sql.NullTime
		  var reason sql.NullString
		   if err := rows.Scan(
			   &b.BindID, &b.Slot.Chain, &b.Slot.Address, &b.EpochID, &b.Generation,
			   &b.UserID, &b.State, &b.BoundAt, &rev, &reason,
		  ); err != nil {
			  return nil, err
		   }
		   if rev.Valid {
			   t := rev.Time
			   b.RevokedAt = &t
		  }
		  if reason.Valid {
			   b.RevokeReason = reason.String
		   }
		   out = append(out, b)
	  }
	  return out, rows.Err()
   }

  func (s *SQLStore) InsertProof(ctx context.Context, p MiscreditProof) error {
	  q := `INSERT INTO addrpool_miscredit_proofs
	      (proof_id, chain, address, epoch_id, generation, carried_gen, kind, state,
	       prev_user, attempt_user, current_user, tx_hash, asset, amount,
	       disposition, fail_closed, summary, opened_at, acked_at, exported_at, observe_id)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		   p.ProofID,
		  p.Slot.Chain, p.Slot.Address, p.EpochID, p.Generation, p.CarriedGen,
		   string(p.Kind), string(p.State), p.PrevUser, p.AttemptUser, p.CurrentUser,
		   nullStr(p.TxHash), nullStr(p.Asset), nullStr(p.Amount), string(p.Disposition),
		   boolToInt(p.FailClosed), p.Summary, p.OpenedAt, nullTime(p.AckedAt),
		  nullTime(p.ExportedAt), nullStr(p.ObserveID),
	  )
	   return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*MiscreditProof, error) {
	  q := `SELECT proof_id, chain, address, epoch_id, generation, carried_gen, kind, state, 
	             prev_user, attempt_user, current_user, tx_hash, asset, amount,
	             disposition, fail_closed, summary, opened_at, acked_at, exported_at, observe_id
	      FROM addrpool_miscredit_proofs WHERE proof_id=?`
	  return scanProof(s.db.QueryRowContext(ctx, s.rewrite(q), proofID))
   }

  func scanProof(row *sql.Row) (*MiscreditProof, error) {
	   var p MiscreditProof
	  var tx, asset, amt, obs sql.NullString
	   var acked, exp sql.NullTime
	   var fc int
	  err := row.Scan(
		   &p.ProofID, &p.Slot.Chain, &p.Slot.Address, &p.EpochID, &p.Generation, &p.CarriedGen,
		  &p.Kind, &p.State, &p.PrevUser, &p.AttemptUser, &p.CurrentUser,
		   &tx,
		  &asset, &amt, &p.Disposition, &fc, &p.Summary, &p.OpenedAt, &acked, &exp, &obs,
	   )
	   if err != nil {
		   if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrProofNotFound
		  }
		   return nil, err
	   }
	  p.FailClosed = fc != 0
	  if tx.Valid {
		   p.TxHash = tx.String
	  }
	  if asset.Valid {
		  p.Asset = asset.String
	   }
	   if amt.Valid {
		  p.Amount = amt.String
	   }
	  if obs.Valid {
		   p.ObserveID = obs.String
	   }
	   if acked.Valid {
		  t := acked.Time
		  p.AckedAt = &t
	  }
	  if exp.Valid {
		   t := exp.Time
		   p.ExportedAt = &t
	  }
	  return &p, nil
   }

   func (s *SQLStore) ListOpenProofs(ctx context.Context, limit int) ([]MiscreditProof, error) {
	  if limit <= 0 {
		   limit = 200
	   }
	  q := `SELECT proof_id, chain, address, epoch_id, generation, carried_gen, kind, state,
	             prev_user, attempt_user, current_user, tx_hash, asset, amount,
	             disposition, fail_closed, summary, opened_at, acked_at, exported_at, observe_id
	      FROM addrpool_miscredit_proofs WHERE state='open' LIMIT ?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), limit)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanProofRows(rows)
  }

  func (s *SQLStore) ListProofsBySlot(ctx context.Context, slot SlotKey) ([]MiscreditProof, error) {
	  q := `SELECT proof_id, chain, address, epoch_id, generation, carried_gen, kind, state,
	             prev_user, attempt_user, current_user, tx_hash, asset, amount,
	             disposition, fail_closed, summary, opened_at, acked_at, exported_at, observe_id
	      FROM addrpool_miscredit_proofs WHERE chain=? AND address=?`
	  rows, err := s.db.QueryContext(ctx, s.rewrite(q), slot.Chain, slot.Address)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanProofRows(rows)
   }

  func scanProofRows(rows *sql.Rows) ([]MiscreditProof, error) {
	  var out []MiscreditProof
	  for rows.Next() {
		  var p MiscreditProof
		  var tx, asset, amt, obs sql.NullString
		  var acked, exp sql.NullTime
		  var fc int
		   if err := rows.Scan(
			  &p.ProofID, &p.Slot.Chain, &p.Slot.Address, &p.EpochID, &p.Generation, &p.CarriedGen,
			   &p.Kind, &p.State, &p.PrevUser, &p.AttemptUser, &p.CurrentUser,
			   &tx, &asset, &amt, &p.Disposition, &fc, &p.Summary, &p.OpenedAt, &acked, &exp, &obs,
		  ); err != nil {
			  return nil, err
		  }
		   p.FailClosed = fc != 0
		  if tx.Valid {
			  p.TxHash = tx.String
		  }
		   if asset.Valid {
			   p.Asset = asset.String
		  }
		   if amt.Valid {
			   p.Amount = amt.String
		  }
		   if obs.Valid {
			  p.ObserveID = obs.String
		  }
		   if acked.Valid {
			   t := acked.Time
			  p.AckedAt = &t
		  }
		  if exp.Valid {
			   t := exp.Time
			  p.ExportedAt = &t
		   }
		  out = append(out, p)
	  }
	   return out, rows.Err()
   }

  func (s *SQLStore) AckProof(ctx context.Context, proofID string, now time.Time) error {
	   q := `UPDATE addrpool_miscredit_proofs SET state='acked', acked_at=?
	      WHERE proof_id=? AND state='open'`
	  res, err := s.db.ExecContext(ctx, s.rewrite(q), now, proofID)
	  if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   p, gerr := s.GetProof(ctx, proofID)
		  if gerr != nil {
			  return gerr
		   }
		   if p.State == ProofAcked {
			  return ErrProofAlreadyAcked
		  }
		   return ErrProofNotFound
	   }
	  return nil
   }

  func (s *SQLStore) MarkProofExported(ctx context.Context, proofID string, now time.Time) error {
	  q := `UPDATE addrpool_miscredit_proofs SET exported_at=? WHERE proof_id=?`
	   _, err := s.db.ExecContext(ctx, s.rewrite(q), now, proofID)
	   return err
  }

   func (s *SQLStore) InsertObserve(ctx context.Context, r ObserveRecord) error {
	   q := `INSERT INTO addrpool_observe_records
	      (observe_id, chain, address, epoch_id, generation, carried_gen, user_id,
	       tx_hash, asset, amount, outcome, proof_id, idem_key, created_at)
	      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  r.ObserveID, r.Slot.Chain, r.Slot.Address, r.EpochID, r.Generation, r.CarriedGen,
		  r.UserID, r.TxHash, r.Asset, r.Amount, string(r.Outcome), nullStr(r.ProofID),
		  r.IdemKey, r.CreatedAt,
	   )
	  return err
   }

  func (s *SQLStore) GetObserveByIdem(ctx context.Context, idemKey string) (*ObserveRecord, error) {
	  q := `SELECT observe_id, chain, address, epoch_id, generation, carried_gen, user_id,
	             tx_hash, asset, amount, outcome, proof_id, idem_key, created_at
	      FROM addrpool_observe_records WHERE idem_key=?`
	  row := s.db.QueryRowContext(ctx,
		  s.rewrite(q), idemKey)
	   var r ObserveRecord
	  var proof sql.NullString
	   err := row.Scan(
		  &r.ObserveID,
		   &r.Slot.Chain, &r.Slot.Address, &r.EpochID, &r.Generation, &r.CarriedGen,
		   &r.UserID, &r.TxHash, &r.Asset, &r.Amount, &r.Outcome, &proof, &r.IdemKey, &r.CreatedAt,
	   )
	  if err != nil {
		  if errors.Is(err, sql.ErrNoRows) {
			   return nil, ErrBindNotFound
		  }
		   return nil, err
	  }
	  if proof.Valid {
		   r.ProofID = proof.String
	  }
	  return &r, nil
   }

  func (s *SQLStore) ListObservesBySlot(ctx context.Context, slot SlotKey, limit int) ([]ObserveRecord, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  q := `SELECT observe_id, chain, address, epoch_id, generation, carried_gen, user_id,
	             tx_hash, asset, amount, outcome, proof_id, idem_key, created_at
	      FROM addrpool_observe_records WHERE chain=? AND address=? ORDER BY created_at DESC LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), slot.Chain, slot.Address, limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []ObserveRecord
	  for rows.Next() {
		   var r ObserveRecord
		   var proof sql.NullString
		   if err := rows.Scan(
			   &r.ObserveID, &r.Slot.Chain, &r.Slot.Address, &r.EpochID, &r.Generation, &r.CarriedGen,
			   &r.UserID, &r.TxHash, &r.Asset, &r.Amount, &r.Outcome, &proof, &r.IdemKey, &r.CreatedAt,
		  ); err != nil {
			   return nil, err
		  }
		   if proof.Valid {
			   r.ProofID = proof.String
		   }
		   out = append(out, r)
	  }
	  return out, rows.Err()
  }

  func (s *SQLStore) InsertAudit(ctx context.Context, ev AuditEvent) error {
	   q := `INSERT INTO addrpool_recycle_epoch_audits
	      (event_id, chain, address, kind, actor, detail, occurred_at)
	      VALUES (?,?,?,?,?,?,?)`
	  _, err := s.db.ExecContext(ctx, s.rewrite(q),
		  ev.EventID, ev.Slot.Chain, ev.Slot.Address, ev.Kind, ev.Actor, ev.Detail, ev.OccurredAt,
	   )
	  return err
   }

   func (s *SQLStore) ListAudit(ctx context.Context, slot SlotKey, limit int) ([]AuditEvent, error) {
	   if limit <= 0 {
		  limit = 100
	   }
	  q := `SELECT event_id, chain, address, kind, actor, detail, occurred_at
	      FROM addrpool_recycle_epoch_audits WHERE chain=? AND address=? LIMIT ?`
	   rows, err := s.db.QueryContext(ctx, s.rewrite(q), slot.Chain, slot.Address, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []AuditEvent
	   for rows.Next() {
		   var ev AuditEvent
		  if err := rows.Scan(&ev.EventID, &ev.Slot.Chain, &ev.Slot.Address, &ev.Kind, &ev.Actor, &ev.Detail, &ev.OccurredAt); err != nil {
			  return nil, err
		  }
		   out = append(out, ev)
	  }
	   return out, rows.Err()
   }

  func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

   func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

   func boolToInt(b bool) int {
	  if b {
		   return 1
	   }
	   return 0
  }
