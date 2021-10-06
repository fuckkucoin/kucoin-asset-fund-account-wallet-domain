package pumpmarkfence

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

   func ph(d persist.Dialect, n int) string {
	   if d == persist.Postgres {
		   return "$" + strconv.Itoa(n)
	   }
	  return "?"
  }

  func nullTime(t time.Time) sql.NullTime {
	   if t.IsZero() {
		  return sql.NullTime{}
	  }
	   return sql.NullTime{Time: t.UTC(), Valid: true}
   }

   func timeVal(nt sql.NullTime) time.Time {
	  if !nt.Valid {
		  return time.Time{}
	   }
	  return nt.Time.UTC()
   }

  func boolInt(b bool) int {
	  if b {
		  return 1
	  }
	  return 0
  }

   func intBool(v int) bool { return v != 0 }

   func ttlNanos(d time.Duration) int64 { return int64(d) }

  func nanosTTL(n int64) time.Duration { return time.Duration(n) }

   func genCols() string {
	   return `gen_id, outbox_id, business_key, kind, payload_hash, effect_key, generation, state,
  ttl_nanos, deadline, issued_at, delivered_at, marked_at, expired_at, applied, handler_invoked, actor, note, idempotency_key`
  }

   func scanGen(sc interface{ Scan(dest ...any) error }) (*DeliverEffectGen, error) {
	   var row DeliverEffectGen
	   var ttl int64
	   var delivered, marked, expired sql.NullTime
	  var applied, invoked int
	   err := sc.Scan(
		   &row.GenID,
		   &row.OutboxID, &row.BusinessKey, &row.Kind, &row.PayloadHash, &row.EffectKey,
		  &row.Generation, &row.State, &ttl, &row.Deadline, &row.IssuedAt, &delivered, &marked, &expired,
		  &applied, &invoked, &row.Actor, &row.Note, &row.IdempotencyKey,
	   )
	  if err != nil {
		  return nil, err
	  }
	   row.TTL = nanosTTL(ttl)
	  row.Deadline = row.Deadline.UTC()
	  row.IssuedAt = row.IssuedAt.UTC()
	   row.DeliveredAt = timeVal(delivered)
	   row.MarkedAt = timeVal(marked)
	  row.ExpiredAt = timeVal(expired)
	   row.Applied = intBool(applied)
	   row.HandlerInvoked = intBool(invoked)
	   return &row, nil
  }

// SQLGenRepo 世代表。
type SQLGenRepo struct{ db *persist.SQLDB }

  func NewSQLGenRepo(db *persist.SQLDB) *SQLGenRepo { return &SQLGenRepo{db: db} }

   func (r *SQLGenRepo) Save(ctx context.Context, row *DeliverEffectGen) error {
	  if r == nil || r.db == nil || row == nil || row.GenID == "" {
		  return ErrInvalidInput
	  }
	  c := r.db.Conn(ctx)
	  var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = c.ExecContext(ctx, `
INSERT INTO outbox_deliver_gens
 (gen_id, outbox_id, business_key, kind, payload_hash, effect_key, generation, state,
  ttl_nanos, deadline, issued_at, delivered_at, marked_at, expired_at, applied, handler_invoked, actor, note, idempotency_key)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT (gen_id) DO UPDATE SET
  payload_hash=EXCLUDED.payload_hash, effect_key=EXCLUDED.effect_key, generation=EXCLUDED.generation,
  state=EXCLUDED.state, deadline=EXCLUDED.deadline, delivered_at=EXCLUDED.delivered_at,
  marked_at=EXCLUDED.marked_at, expired_at=EXCLUDED.expired_at, applied=EXCLUDED.applied,
  handler_invoked=EXCLUDED.handler_invoked, actor=EXCLUDED.actor, note=EXCLUDED.note
`, row.GenID, row.OutboxID, row.BusinessKey, row.Kind, row.PayloadHash, row.EffectKey, row.Generation,
			  string(row.State), ttlNanos(row.TTL), row.Deadline.UTC(), row.IssuedAt.UTC(), nullTime(row.DeliveredAt),
			  nullTime(row.MarkedAt), nullTime(row.ExpiredAt), boolInt(row.Applied), boolInt(row.HandlerInvoked),
			   row.Actor,
			  row.Note, row.IdempotencyKey)
	   } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO outbox_deliver_gens
 (gen_id, outbox_id, business_key, kind, payload_hash, effect_key, generation, state,
  ttl_nanos, deadline, issued_at, delivered_at, marked_at, expired_at, applied, handler_invoked, actor, note, idempotency_key)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  payload_hash=VALUES(payload_hash), effect_key=VALUES(effect_key), generation=VALUES(generation),
  state=VALUES(state), deadline=VALUES(deadline), delivered_at=VALUES(delivered_at),
  marked_at=VALUES(marked_at), expired_at=VALUES(expired_at), applied=VALUES(applied),
  handler_invoked=VALUES(handler_invoked), actor=VALUES(actor), note=VALUES(note)
`, row.GenID, row.OutboxID, row.BusinessKey, row.Kind, row.PayloadHash, row.EffectKey, row.Generation,
			   string(row.State), ttlNanos(row.TTL), row.Deadline.UTC(), row.IssuedAt.UTC(), nullTime(row.DeliveredAt),
			  nullTime(row.MarkedAt), nullTime(row.ExpiredAt), boolInt(row.Applied), boolInt(row.HandlerInvoked),
			  row.Actor, row.Note, row.IdempotencyKey)
	   }
	   return err
  }

   func (r *SQLGenRepo) Get(ctx context.Context, genID string) (*DeliverEffectGen, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	  }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + genCols() + ` FROM outbox_deliver_gens WHERE gen_id=` + ph(r.db.Dialect, 1)
	   row := scanOrNotFound(scanGen, c.QueryRowContext(ctx, q, strings.TrimSpace(genID)))
	   if row == nil {
		   return nil, ErrGenNotFound
	   }
	  return row, nil
   }

   func (r *SQLGenRepo) GetByOutbox(ctx context.Context, outboxID string) (*DeliverEffectGen, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	   }
	  c := r.db.Conn(ctx)
	  q := `SELECT ` + genCols() + ` FROM outbox_deliver_gens WHERE outbox_id=` + ph(r.db.Dialect, 1) + ` ORDER BY generation DESC, issued_at DESC LIMIT 1`
	   row := scanOrNotFound(scanGen, c.QueryRowContext(ctx, q, strings.TrimSpace(outboxID)))
	   if row == nil {
		   return nil, ErrGenNotFound
	   }
	  return row, nil
   }

   func (r *SQLGenRepo) GetByEffect(ctx context.Context, effectKey string) (*DeliverEffectGen, error) {
	   if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	   }
	   c := r.db.Conn(ctx)
	  q := `SELECT ` + genCols() + ` FROM outbox_deliver_gens WHERE effect_key=` + ph(r.db.Dialect, 1) + ` ORDER BY generation DESC LIMIT 1`
	   row := scanOrNotFound(scanGen, c.QueryRowContext(ctx, q, strings.TrimSpace(effectKey)))
	   if row == nil {
		  return nil, ErrGenNotFound
	  }
	   return row, nil
   }

   func (r *SQLGenRepo) ListLive(ctx context.Context, limit int) ([]DeliverEffectGen, error) {
	   return r.listWhere(ctx, `state IN ('issued','delivered')`, limit)
  }

   func (r *SQLGenRepo) ListByState(ctx context.Context, state GenState, limit int) ([]DeliverEffectGen, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		   limit = 80
	   }
	   c := r.db.Conn(ctx)
	   q := `SELECT ` + genCols() + ` FROM outbox_deliver_gens WHERE state=` + ph(r.db.Dialect, 1) + ` ORDER BY issued_at ASC LIMIT ` + ph(r.db.Dialect, 2)
	   rows, err := c.QueryContext(ctx, q, string(state), limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanGenRows(rows)
  }

   func (r *SQLGenRepo) listWhere(ctx context.Context, where string, limit int) ([]DeliverEffectGen, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 80
	   }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + genCols() + ` FROM outbox_deliver_gens WHERE ` + where + ` ORDER BY issued_at ASC LIMIT ` + ph(r.db.Dialect, 1)
	   rows, err := c.QueryContext(ctx, q, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanGenRows(rows)
   }

  func scanGenRows(rows *sql.Rows) ([]DeliverEffectGen, error) {
	  out := make([]DeliverEffectGen, 0)
	   for rows.Next() {
		  row, err := scanGen(rows)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, *row)
	  }
	   return out, rows.Err()
  }

  func scanOrNotFound(scan func(interface{ Scan(dest ...any) error }) (*DeliverEffectGen, error), row *sql.Row) *DeliverEffectGen {
	  got, err := scan(row)
	   if err != nil {
		   return nil
	   }
	  return got
   }

   func halfCols() string {
	  return `case_id, gen_id, outbox_id, effect_key, state, reason, opened_at, resumed_at, abandoned_at,
  resume_token, resume_holder, resume_until, resume_count, peer_acked, note`
  }

  func scanHalf(sc interface{ Scan(dest ...any) error }) (*HalfMarkCase, error) {
	  var row HalfMarkCase
	   var resumed, abandoned, until sql.NullTime
	  var acked int
	  err := sc.Scan(
		  &row.CaseID, &row.GenID, &row.OutboxID, &row.EffectKey, &row.State, &row.Reason,
		  &row.OpenedAt,
		   &resumed, &abandoned, &row.ResumeToken, &row.ResumeHolder, &until,
		  &row.ResumeCount, &acked, &row.Note,
	  )
	   if err != nil {
		  return nil, err
	  }
	   row.OpenedAt = row.OpenedAt.UTC()
	  row.ResumedAt = timeVal(resumed)
	   row.AbandonedAt = timeVal(abandoned)
	  row.ResumeUntil = timeVal(until)
	   row.PeerAcked = intBool(acked)
	  return &row, nil
  }

// SQLHalfRepo 半截案表。
type SQLHalfRepo struct{ db *persist.SQLDB }

   func NewSQLHalfRepo(db *persist.SQLDB) *SQLHalfRepo { return &SQLHalfRepo{db: db} }

  func (r *SQLHalfRepo) Save(ctx context.Context, row *HalfMarkCase) error {
	  if r == nil || r.db == nil || row == nil || row.CaseID == "" {
		  return ErrInvalidInput
	  }
	  c := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = c.ExecContext(ctx, `
INSERT INTO outbox_half_mark_cases
 (case_id, gen_id, outbox_id, effect_key, state, reason, opened_at, resumed_at, abandoned_at,
  resume_token, resume_holder, resume_until, resume_count, peer_acked, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (case_id) DO UPDATE SET
  state=EXCLUDED.state, reason=EXCLUDED.reason, resumed_at=EXCLUDED.resumed_at, abandoned_at=EXCLUDED.abandoned_at,
  resume_token=EXCLUDED.resume_token, resume_holder=EXCLUDED.resume_holder, resume_until=EXCLUDED.resume_until,
  resume_count=EXCLUDED.resume_count, peer_acked=EXCLUDED.peer_acked, note=EXCLUDED.note
`, row.CaseID, row.GenID, row.OutboxID, row.EffectKey, string(row.State), row.Reason, row.OpenedAt.UTC(),
			  nullTime(row.ResumedAt), nullTime(row.AbandonedAt), row.ResumeToken, row.ResumeHolder,
			   nullTime(row.ResumeUntil), row.ResumeCount, boolInt(row.PeerAcked), row.Note)
	  } else {
		   _, err = c.ExecContext(ctx, ` 
INSERT INTO outbox_half_mark_cases
 (case_id, gen_id, outbox_id, effect_key, state, reason, opened_at, resumed_at, abandoned_at,
  resume_token, resume_holder, resume_until, resume_count, peer_acked, note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), reason=VALUES(reason), resumed_at=VALUES(resumed_at), abandoned_at=VALUES(abandoned_at),
  resume_token=VALUES(resume_token), resume_holder=VALUES(resume_holder), resume_until=VALUES(resume_until),
  resume_count=VALUES(resume_count), peer_acked=VALUES(peer_acked), note=VALUES(note)
`, row.CaseID, row.GenID, row.OutboxID, row.EffectKey, string(row.State), row.Reason, row.OpenedAt.UTC(),
			   nullTime(row.ResumedAt), nullTime(row.AbandonedAt), row.ResumeToken, row.ResumeHolder,
			  nullTime(row.ResumeUntil),
			   row.ResumeCount, boolInt(row.PeerAcked), row.Note)
	   }
	   return err
  }

   func (r *SQLHalfRepo) Get(ctx context.Context, caseID string) (*HalfMarkCase, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	   }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + halfCols() + ` FROM outbox_half_mark_cases WHERE case_id=` + ph(r.db.Dialect, 1)
	  row, err := scanHalf(c.QueryRowContext(ctx, q, strings.TrimSpace(caseID)))
	  if err != nil {
		   return nil, ErrHalfMarkNotFound
	   }
	  return row, nil
   }

   func (r *SQLHalfRepo) GetByGen(ctx context.Context, genID string) (*HalfMarkCase, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	  }
	   c := r.db.Conn(ctx)
	  q := `SELECT ` + halfCols() + ` FROM outbox_half_mark_cases WHERE gen_id=` + ph(r.db.Dialect, 1) + ` ORDER BY opened_at DESC LIMIT 1`
	  row, err := scanHalf(c.QueryRowContext(ctx, q, strings.TrimSpace(genID)))
	  if err != nil {
		   return nil, ErrHalfMarkNotFound
	  }
	   return row, nil
   }

  func (r *SQLHalfRepo) GetOpenByOutbox(ctx context.Context, outboxID string) (*HalfMarkCase, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	  }
	   c := r.db.Conn(ctx)
	   q := `SELECT ` + halfCols() + ` FROM outbox_half_mark_cases WHERE outbox_id=` + ph(r.db.Dialect, 1) + ` AND state='open' ORDER BY opened_at DESC LIMIT 1`
	  row,
		   err := scanHalf(c.QueryRowContext(ctx, q, strings.TrimSpace(outboxID)))
	  if err != nil {
		   return nil, ErrHalfMarkNotFound
	  }
	   return row, nil
  }

   func (r *SQLHalfRepo) ListOpen(ctx context.Context, limit int) ([]HalfMarkCase, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 80
	  }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + halfCols() + ` FROM outbox_half_mark_cases WHERE state='open' ORDER BY opened_at ASC LIMIT ` + ph(r.db.Dialect, 1)
	   rows, err := c.QueryContext(ctx, q, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]HalfMarkCase, 0)
	   for rows.Next() {
		   row, err := scanHalf(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, *row)
	   }
	  return out, rows.Err()
  }

  func proofCols() string {
	  return `proof_id, gen_id, outbox_id, effect_key, kind, generation, payload_hash, actor, detail, detected_at, exported, exported_at, note`
  }

  func scanProof(sc interface{ Scan(dest ...any) error }) (*OutboxDupProof, error) {
	   var row OutboxDupProof
	  var exported int
	   var expAt sql.NullTime
	  err := sc.Scan(
		  &row.ProofID, &row.GenID, &row.OutboxID, &row.EffectKey, &row.Kind, &row.Generation,
		   &row.PayloadHash, &row.Actor, &row.Detail, &row.DetectedAt, &exported, &expAt, &row.Note,
	  )
	  if err != nil {
		  return nil, err
	   }
	   row.DetectedAt = row.DetectedAt.UTC()
	   row.Exported = intBool(exported)
	  row.ExportedAt = timeVal(expAt)
	  return &row, nil
   }

// SQLProofRepo 双效果证明表。

type SQLProofRepo struct{ db *persist.SQLDB }

   func NewSQLProofRepo(db *persist.SQLDB) *SQLProofRepo { return &SQLProofRepo{db: db} }

   func (r *SQLProofRepo) Save(ctx context.Context, row *OutboxDupProof) error {
	   if r == nil || r.db == nil || row == nil || row.ProofID == "" {
		  return ErrInvalidInput
	   }
	   c := r.db.Conn(ctx)
	  var err error
	  if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, `
INSERT INTO outbox_dup_proofs
 (proof_id, gen_id, outbox_id, effect_key, kind, generation, payload_hash, actor, detail, detected_at, exported, exported_at, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (proof_id) DO UPDATE SET
  exported=EXCLUDED.exported, exported_at=EXCLUDED.exported_at, note=EXCLUDED.note
`, row.ProofID, row.GenID, row.OutboxID, row.EffectKey, string(row.Kind), row.Generation, row.PayloadHash,
			  row.Actor, row.Detail, row.DetectedAt.UTC(), boolInt(row.Exported), nullTime(row.ExportedAt), row.Note)
	  } else {
		  _, err = c.ExecContext(ctx, `
INSERT INTO outbox_dup_proofs
 (proof_id, gen_id, outbox_id, effect_key, kind, generation, payload_hash, actor, detail, detected_at, exported, exported_at, note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  exported=VALUES(exported), exported_at=VALUES(exported_at), note=VALUES(note)
`, row.ProofID, row.GenID, row.OutboxID, row.EffectKey, string(row.Kind), row.Generation, row.PayloadHash,
			  row.Actor, row.Detail, row.DetectedAt.UTC(), boolInt(row.Exported), nullTime(row.ExportedAt), row.Note)
	  }
	  return err
  }

   func (r *SQLProofRepo) Get(ctx context.Context, proofID string) (*OutboxDupProof, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	  }
	  c := r.db.Conn(ctx)
	  q := `SELECT ` + proofCols() + ` FROM outbox_dup_proofs WHERE proof_id=` + ph(r.db.Dialect, 1)
	  row, err := scanProof(c.QueryRowContext(ctx, q, strings.TrimSpace(proofID)))
	   if err != nil {
		   return nil, ErrProofNotFound
	   }
	   return row, nil
  }

  func (r *SQLProofRepo) ListUnexported(ctx context.Context, limit int) ([]OutboxDupProof, error) {
	   return r.list(ctx, `exported=0`, "", limit)
  }

  func (r *SQLProofRepo) ListByOutbox(ctx context.Context, outboxID string, limit int) ([]OutboxDupProof, error) {
	   return r.list(ctx, `outbox_id=`+ph(r.db.Dialect, 1), strings.TrimSpace(outboxID), limit)
  }

   func (r *SQLProofRepo) ListByKind(ctx context.Context, kind ProofKind, limit int) ([]OutboxDupProof, error) {
	   return r.list(ctx, `kind=`+ph(r.db.Dialect, 1), string(kind), limit)
   }

   func (r *SQLProofRepo) list(ctx context.Context, where, arg string, limit int) ([]OutboxDupProof, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 80
	   }
	   c := r.db.Conn(ctx)
	  var (
		   rows *sql.Rows
		  err  error
	   )
	   if arg == "" {
		  q := `SELECT ` + proofCols() + ` FROM outbox_dup_proofs WHERE ` + where + ` ORDER BY detected_at ASC LIMIT ` + ph(r.db.Dialect, 1)
		   rows, err = c.QueryContext(ctx, q, limit)
	  } else {
		  q := `SELECT ` + proofCols() + ` FROM outbox_dup_proofs WHERE ` + where + ` ORDER BY detected_at ASC LIMIT ` + ph(r.db.Dialect, 2)
		  rows,
			   err = c.QueryContext(ctx, q, arg, limit)
	  }
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   out := make([]OutboxDupProof, 0)
	   for rows.Next() {
		   row, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, *row)
	  }
	   return out, rows.Err()
   }

// SQLAppliedRepo 已落地效果键。
type SQLAppliedRepo struct{ db *persist.SQLDB }

  func NewSQLAppliedRepo(db *persist.SQLDB) *SQLAppliedRepo { return &SQLAppliedRepo{db: db} }

  func (r *SQLAppliedRepo) Put(ctx context.Context, row *AppliedEffect) error {
	  if r == nil || r.db == nil || row == nil || row.EffectKey == "" {
		   return ErrInvalidInput
	   }
	   c := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, `
INSERT INTO outbox_applied_effects (effect_key, gen_id, outbox_id, generation, applied_at)
VALUES ($1,$2,$3,$4,$5)
`, row.EffectKey, row.GenID, row.OutboxID, row.Generation, row.AppliedAt.UTC())
	   } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO outbox_applied_effects (effect_key, gen_id, outbox_id, generation, applied_at)
VALUES (?,?,?,?,?)
`, row.EffectKey, row.GenID, row.OutboxID, row.Generation, row.AppliedAt.UTC())
	   }
	  if err != nil {
		  if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			  return ErrAlreadyApplied
		   }
		   return err
	  }
	   return nil
  }

  func (r *SQLAppliedRepo) Get(ctx context.Context, effectKey string) (*AppliedEffect, error) {
	   if r == nil || r.db == nil {
		  return nil, ErrUnavailable
	  }
	  c := r.db.Conn(ctx)
	   q := `SELECT effect_key, gen_id, outbox_id, generation, applied_at FROM outbox_applied_effects WHERE effect_key=` + ph(r.db.Dialect, 1)
	  var row AppliedEffect
	   err := c.QueryRowContext(ctx, q, strings.TrimSpace(effectKey)).Scan(&row.EffectKey, &row.GenID, &row.OutboxID, &row.Generation, &row.AppliedAt)
	  if err != nil {
		  return nil, ErrEffectKey
	  }
	   row.AppliedAt = row.AppliedAt.UTC()
	  return &row, nil
   }

// SQLAuditRepo 审计表。
type SQLAuditRepo struct{ db *persist.SQLDB }

  func NewSQLAuditRepo(db *persist.SQLDB) *SQLAuditRepo { return &SQLAuditRepo{db: db} }

  func (r *SQLAuditRepo) Append(ctx context.Context, row FenceAudit) error {
	   if r == nil || r.db == nil || row.AuditID == "" {
		   return ErrInvalidInput
	  }
	   c := r.db.Conn(ctx)
	  if r.db.Dialect == persist.Postgres {
		  _, err := c.ExecContext(ctx, ` 
INSERT INTO outbox_pump_fence_audit
 (audit_id, gen_id, outbox_id, actor, action, outcome, detail, generation, at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
`, row.AuditID, row.GenID, row.OutboxID, row.Actor, row.Action, row.Outcome, row.Detail, row.Generation, row.At.UTC())
		  return err
	   }
	  _, err := c.ExecContext(ctx, `
INSERT INTO outbox_pump_fence_audit
 (audit_id, gen_id, outbox_id, actor, action, outcome, detail, generation, at)
VALUES (?,?,?,?,?,?,?,?,?)
`, row.AuditID, row.GenID, row.OutboxID, row.Actor, row.Action, row.Outcome, row.Detail, row.Generation, row.At.UTC())
	   return err
  }

  func (r *SQLAuditRepo) ListByGen(ctx context.Context, genID string, limit int) ([]FenceAudit, error) {
	  return r.list(ctx, `gen_id=`+ph(r.db.Dialect, 1), strings.TrimSpace(genID), limit)
  }

   func (r *SQLAuditRepo) ListByOutbox(ctx context.Context, outboxID string, limit int) ([]FenceAudit, error) {
	  return r.list(ctx, `outbox_id=`+ph(r.db.Dialect, 1), strings.TrimSpace(outboxID), limit)
  }

   func (r *SQLAuditRepo) list(ctx context.Context, where, arg string, limit int) ([]FenceAudit, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 80
	  }
	  c := r.db.Conn(ctx)
	  q := `SELECT audit_id, gen_id, outbox_id, actor, action, outcome, detail, generation, at FROM outbox_pump_fence_audit WHERE ` + where + ` ORDER BY at ASC LIMIT ` + ph(r.db.Dialect, 2)
	  rows, err := c.QueryContext(ctx, q, arg, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]FenceAudit, 0)
	  for rows.Next() {
		  var row FenceAudit
		  if err := rows.Scan(&row.AuditID, &row.GenID, &row.OutboxID, &row.Actor, &row.Action, &row.Outcome, &row.Detail, &row.Generation, &row.At); err != nil {
			   return nil, err
		  }
		   row.At = row.At.UTC()
		  out = append(out, row)
	  }
	  return out, rows.Err()
   }
