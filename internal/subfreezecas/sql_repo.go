package subfreezecas

import (
	"context"
	"database/sql"
	"encoding/json"
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

  func childJSON(ids []string) string {
	   ids = normalizeChildIDs(ids)
	   if len(ids) == 0 {
		   return "[]"
	   }
	   b, err := json.Marshal(ids)
	   if err != nil {
		   return "[]"
	   }
	   return string(b)
   }

  func parseChildJSON(raw string) []string {
	   raw = strings.TrimSpace(raw)
	  if raw == "" {
		   return nil
	  }
	  var out []string
	   if err := json.Unmarshal([]byte(raw), &out); err != nil {
		  return nil
	   }
	   return normalizeChildIDs(out)
  }

   func casCols() string {
	   return `freeze_cas_id, parent_id, child_set_fingerprint, child_ids_json, reason, generation, state,
  actor, opened_at, frozen_at, thawed_at, superseded_by, idempotency_key, note`
  }

  func scanCAS(sc interface{ Scan(dest ...any) error }) (*FreezeCAS, error) {
	   var row FreezeCAS
	  var childRaw string
	  var frozen, thawed sql.NullTime
	  err := sc.Scan(
		  &row.FreezeCASID, &row.ParentID, &row.ChildSetFingerprint, &childRaw, &row.Reason,
		  &row.Generation, &row.State, &row.Actor, &row.OpenedAt, &frozen, &thawed,
		   &row.SupersededBy, &row.IdempotencyKey, &row.Note,
	   )
	  if err != nil {
		   return nil, err
	  }
	   row.ChildIDs = parseChildJSON(childRaw)
	  row.OpenedAt = row.OpenedAt.UTC()
	  row.FrozenAt = timeVal(frozen)
	   row.ThawedAt = timeVal(thawed)
	  return &row, nil
  }

// SQLCASRepo CAS 表。

type SQLCASRepo struct{ db *persist.SQLDB }

  func NewSQLCASRepo(db *persist.SQLDB) *SQLCASRepo { return &SQLCASRepo{db: db} }

  func (r *SQLCASRepo) Save(ctx context.Context, row *FreezeCAS) error {
	  if r == nil || r.db == nil || row == nil || row.FreezeCASID == "" {
		  return ErrInvalidInput
	   }
	   c := r.db.Conn(ctx)
	  var err error
	   if r.db.Dialect == persist.Postgres {
		  _, err = c.ExecContext(ctx, `
INSERT INTO subacct_freeze_cas
 (freeze_cas_id, parent_id, child_set_fingerprint, child_ids_json, reason, generation, state,
  actor, opened_at, frozen_at, thawed_at, superseded_by, idempotency_key, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (freeze_cas_id) DO UPDATE SET
  child_set_fingerprint=EXCLUDED.child_set_fingerprint, child_ids_json=EXCLUDED.child_ids_json,
  reason=EXCLUDED.reason, generation=EXCLUDED.generation, state=EXCLUDED.state, actor=EXCLUDED.actor,
  frozen_at=EXCLUDED.frozen_at, thawed_at=EXCLUDED.thawed_at, superseded_by=EXCLUDED.superseded_by,
  note=EXCLUDED.note
`, row.FreezeCASID, row.ParentID, row.ChildSetFingerprint, childJSON(row.ChildIDs), row.Reason,
			  row.Generation, string(row.State), row.Actor, row.OpenedAt.UTC(), nullTime(row.FrozenAt),
			   nullTime(row.ThawedAt), row.SupersededBy, row.IdempotencyKey, row.Note)
	   } else {
		  _, err = c.ExecContext(ctx, `
INSERT INTO subacct_freeze_cas
 (freeze_cas_id, parent_id, child_set_fingerprint, child_ids_json, reason, generation, state,
  actor, opened_at, frozen_at, thawed_at, superseded_by, idempotency_key, note)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  child_set_fingerprint=VALUES(child_set_fingerprint), child_ids_json=VALUES(child_ids_json),
  reason=VALUES(reason), generation=VALUES(generation), state=VALUES(state), actor=VALUES(actor),
  frozen_at=VALUES(frozen_at), thawed_at=VALUES(thawed_at), superseded_by=VALUES(superseded_by),
  note=VALUES(note)
`, row.FreezeCASID, row.ParentID, row.ChildSetFingerprint, childJSON(row.ChildIDs), row.Reason,
			  row.Generation, string(row.State), row.Actor, row.OpenedAt.UTC(), nullTime(row.FrozenAt),
			  nullTime(row.ThawedAt), row.SupersededBy, row.IdempotencyKey, row.Note)
	   }
	  return err
  }

  func (r *SQLCASRepo) Get(ctx context.Context, casID string) (*FreezeCAS, error) {
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + casCols() + ` FROM subacct_freeze_cas WHERE freeze_cas_id=` + ph(r.db.Dialect, 1)
	  row := c.QueryRowContext(ctx, q, strings.TrimSpace(casID))
	  got, err := scanCAS(row)
	   if err == sql.ErrNoRows {
		  return nil, ErrCASNotFound
	  }
	  return got, err
   }

  func (r *SQLCASRepo) GetByIdem(ctx context.Context, parentID, idem string) (*FreezeCAS, error) {
	  idem = strings.TrimSpace(idem)
	   if idem == "" {
		   return nil, ErrCASNotFound
	   }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + casCols() + ` FROM subacct_freeze_cas WHERE parent_id=` + ph(r.db.Dialect, 1) +
		   ` AND idempotency_key=` + ph(r.db.Dialect, 2) + ` LIMIT 1`
	  row := c.QueryRowContext(ctx,
		   q, strings.TrimSpace(parentID), idem)
	  got, err := scanCAS(row)
	   if err == sql.ErrNoRows {
		   return nil, ErrCASNotFound
	   }
	   return got, err
  }

  func (r *SQLCASRepo) ListByParent(ctx context.Context, parentID string, limit int) ([]FreezeCAS, error) {
	  if limit <= 0 {
		   limit = 50
	   }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + casCols() + ` FROM subacct_freeze_cas WHERE parent_id=` + ph(r.db.Dialect, 1) +
		  ` ORDER BY generation DESC, opened_at DESC LIMIT ` + ph(r.db.Dialect, 2)
	  rows, err := c.QueryContext(ctx, q, strings.TrimSpace(parentID), limit)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  out := make([]FreezeCAS, 0)
	  for rows.Next() {
		  got, err := scanCAS(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, got.Copy())
	  }
	   return out, rows.Err()
  }

   func (r *SQLCASRepo) ListFrozen(ctx context.Context, limit int) ([]FreezeCAS, error) {
	  if limit <= 0 {
		  limit = 100
	   }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + casCols() + ` FROM subacct_freeze_cas WHERE state='frozen' ORDER BY frozen_at DESC LIMIT ` + ph(r.db.Dialect, 1)
	   rows, err := c.QueryContext(ctx, q, limit)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]FreezeCAS, 0)
	   for rows.Next() {
		   got, err := scanCAS(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, got.Copy())
	  }
	   return out, rows.Err()
   }

   func proofCols() string {
	   return `proof_id, freeze_cas_id, parent_id, child_id, owner_id, asset, amount, debit_idem, 
  detected_at, compensated, compensate_credit_key, state, acked_by, acked_at, note, rolled_back_available`
  }

   func scanProof(sc interface{ Scan(dest ...any) error }) (*FrozenSpendProof, error) {
	   var p FrozenSpendProof
	  var acked sql.NullTime
	  var compensated int
	  err := sc.Scan(
		   &p.ProofID, &p.FreezeCASID, &p.ParentID, &p.ChildID, &p.OwnerID, &p.Asset, &p.Amount, &p.DebitIdem,
		   &p.DetectedAt, &compensated, &p.CompensateCreditKey, &p.State, &p.AckedBy, &acked, &p.Note, &p.RolledBackAvail,
	   )
	   if err != nil {
		  return nil, err
	  }
	  p.Compensated = compensated != 0
	  p.DetectedAt = p.DetectedAt.UTC()
	  p.AckedAt = timeVal(acked)
	  return &p, nil
   }

// SQLProofRepo 冻后借记证明表。
type SQLProofRepo struct{ db *persist.SQLDB }

   func NewSQLProofRepo(db *persist.SQLDB) *SQLProofRepo { return &SQLProofRepo{db: db} }

  func (r *SQLProofRepo) Save(ctx context.Context, row *FrozenSpendProof) error {
	  if r == nil || r.db == nil || row == nil || row.ProofID == "" {
		  return ErrInvalidInput
	  }
	   comp := 0
	  if row.Compensated {
		   comp = 1
	   }
	   c := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = c.ExecContext(ctx, `
INSERT INTO subacct_frozen_spend_proofs
 (proof_id, freeze_cas_id, parent_id, child_id, owner_id, asset, amount, debit_idem,
  detected_at, compensated, compensate_credit_key, state, acked_by, acked_at, note, rolled_back_available)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (proof_id) DO UPDATE SET
  compensated=EXCLUDED.compensated, compensate_credit_key=EXCLUDED.compensate_credit_key,
  state=EXCLUDED.state, acked_by=EXCLUDED.acked_by, acked_at=EXCLUDED.acked_at,
  note=EXCLUDED.note, rolled_back_available=EXCLUDED.rolled_back_available
`, row.ProofID, row.FreezeCASID, row.ParentID, row.ChildID, row.OwnerID, row.Asset, row.Amount, row.DebitIdem,
			   row.DetectedAt.UTC(), comp, row.CompensateCreditKey, string(row.State), row.AckedBy,
			   nullTime(row.AckedAt), row.Note, row.RolledBackAvail)
	  } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO subacct_frozen_spend_proofs
 (proof_id, freeze_cas_id, parent_id, child_id, owner_id, asset, amount, debit_idem,
  detected_at, compensated, compensate_credit_key, state, acked_by, acked_at, note, rolled_back_available)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  compensated=VALUES(compensated), compensate_credit_key=VALUES(compensate_credit_key),
  state=VALUES(state), acked_by=VALUES(acked_by), acked_at=VALUES(acked_at),
  note=VALUES(note), rolled_back_available=VALUES(rolled_back_available)
`, row.ProofID, row.FreezeCASID, row.ParentID, row.ChildID, row.OwnerID, row.Asset, row.Amount, row.DebitIdem,
			  row.DetectedAt.UTC(), comp, row.CompensateCreditKey, string(row.State), row.AckedBy,
			  nullTime(row.AckedAt), row.Note, row.RolledBackAvail)
	   }
	   return err
   }

   func (r *SQLProofRepo) Get(ctx context.Context, proofID string) (*FrozenSpendProof, error) {
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + proofCols() + ` FROM subacct_frozen_spend_proofs WHERE proof_id=` + ph(r.db.Dialect, 1)
	  got, err := scanProof(c.QueryRowContext(ctx, q, strings.TrimSpace(proofID)))
	   if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	  }
	  return got, err
   }

  func (r *SQLProofRepo) GetByDebitIdem(ctx context.Context, idem string) (*FrozenSpendProof, error) {
	   idem = strings.TrimSpace(idem)
	  if idem == "" {
		   return nil, ErrProofNotFound
	   }
	   c := r.db.Conn(ctx)
	   q := `SELECT ` + proofCols() + ` FROM subacct_frozen_spend_proofs WHERE debit_idem=` + ph(r.db.Dialect, 1) + ` LIMIT 1`
	  got, err := scanProof(c.QueryRowContext(ctx, q, idem))
	   if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	  }
	   return got, err
  }

   func (r *SQLProofRepo) ListOpen(ctx context.Context, limit int) ([]FrozenSpendProof, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	   c := r.db.Conn(ctx)
	   q := `SELECT ` + proofCols() + ` FROM subacct_frozen_spend_proofs WHERE state='open' ORDER BY detected_at DESC LIMIT ` + ph(r.db.Dialect, 1)
	  rows,
		   err := c.QueryContext(ctx, q, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]FrozenSpendProof, 0)
	  for rows.Next() {
		  got, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, got.Copy())
	   }
	   return out, rows.Err()
  }

  func (r *SQLProofRepo) ListByCAS(ctx context.Context, casID string, limit int) ([]FrozenSpendProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + proofCols() + ` FROM subacct_frozen_spend_proofs WHERE freeze_cas_id=` + ph(r.db.Dialect, 1) +
		  ` ORDER BY detected_at DESC LIMIT ` + ph(r.db.Dialect, 2)
	  rows, err := c.QueryContext(ctx, q, strings.TrimSpace(casID), limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]FrozenSpendProof, 0)
	  for rows.Next() {
		   got, err := scanProof(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, got.Copy())
	  }
	   return out, rows.Err()
  }

  func revokeCols() string {
	   return `revoke_id, freeze_cas_id, parent_id, owner_id, generation, frozen_at, window_from, window_to, hit_count, active, note`
  }

   func scanRevoke(sc interface{ Scan(dest ...any) error }) (*VelocityRevocation, error) {
	  var v VelocityRevocation
	  var active int
	   err := sc.Scan(&v.RevokeID, &v.FreezeCASID, &v.ParentID, &v.OwnerID, &v.Generation,
		   &v.FrozenAt, &v.WindowFrom, &v.WindowTo, &v.HitCount, &active, &v.Note)
	   if err != nil {
		  return nil, err
	   }
	  v.Active = active != 0
	   v.FrozenAt = v.FrozenAt.UTC()
	  v.WindowFrom = v.WindowFrom.UTC()
	   v.WindowTo = v.WindowTo.UTC()
	  return &v, nil
   }

// SQLRevokeRepo 速度窗吊销表。

type SQLRevokeRepo struct{ db *persist.SQLDB }

   func NewSQLRevokeRepo(db *persist.SQLDB) *SQLRevokeRepo { return &SQLRevokeRepo{db: db} }

  func (r *SQLRevokeRepo) Save(ctx context.Context, row *VelocityRevocation) error {
	  if r == nil || r.db == nil || row == nil || row.RevokeID == "" {
		  return ErrInvalidInput
	   }
	   act := 0
	   if row.Active {
		  act = 1
	   }
	  c := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = c.ExecContext(ctx, `
INSERT INTO subacct_vel_revocations
 (revoke_id, freeze_cas_id, parent_id, owner_id, generation, frozen_at, window_from, window_to, hit_count, active, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (revoke_id) DO UPDATE SET
  active=EXCLUDED.active, hit_count=EXCLUDED.hit_count, note=EXCLUDED.note
`, row.RevokeID, row.FreezeCASID, row.ParentID, row.OwnerID, row.Generation, row.FrozenAt.UTC(),
			  row.WindowFrom.UTC(),
			   row.WindowTo.UTC(), row.HitCount, act, row.Note)
	  } else {
		   _, err = c.ExecContext(ctx, `
INSERT INTO subacct_vel_revocations
 (revoke_id, freeze_cas_id, parent_id, owner_id, generation, frozen_at, window_from, window_to, hit_count, active, note)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  active=VALUES(active), hit_count=VALUES(hit_count), note=VALUES(note)
`, row.RevokeID, row.FreezeCASID, row.ParentID, row.OwnerID, row.Generation, row.FrozenAt.UTC(),
			  row.WindowFrom.UTC(),
			  row.WindowTo.UTC(), row.HitCount, act, row.Note)
	   }
	  return err
  }

  func (r *SQLRevokeRepo) Get(ctx context.Context, revokeID string) (*VelocityRevocation, error) {
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + revokeCols() + ` FROM subacct_vel_revocations WHERE revoke_id=` + ph(r.db.Dialect, 1)
	  got, err := scanRevoke(c.QueryRowContext(ctx, q, strings.TrimSpace(revokeID)))
	   if err == sql.ErrNoRows {
		  return nil, ErrRevokeNotFound
	   }
	  return got, err
   }

  func (r *SQLRevokeRepo) ListActiveByOwner(ctx context.Context, ownerID string) ([]VelocityRevocation, error) {
	  c := r.db.Conn(ctx)
	   q := `SELECT ` + revokeCols() + ` FROM subacct_vel_revocations WHERE owner_id=` + ph(r.db.Dialect, 1) + ` AND active=1 ORDER BY frozen_at DESC`
	   rows, err := c.QueryContext(ctx, q, strings.TrimSpace(ownerID))
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]VelocityRevocation, 0)
	   for rows.Next() {
		  got, err := scanRevoke(rows)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, got.Copy())
	   }
	  return out, rows.Err()
  }

  func (r *SQLRevokeRepo) ListByCAS(ctx context.Context, casID string) ([]VelocityRevocation, error) {
	  c := r.db.Conn(ctx)
	  q := `SELECT ` + revokeCols() + ` FROM subacct_vel_revocations WHERE freeze_cas_id=` + ph(r.db.Dialect, 1)
	  rows, err := c.QueryContext(ctx, q, strings.TrimSpace(casID))
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	  out := make([]VelocityRevocation, 0)
	   for rows.Next() {
		  got, err := scanRevoke(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, got.Copy())
	   }
	  return out, rows.Err()
  }

   func (r *SQLRevokeRepo) ListActive(ctx context.Context, limit int) ([]VelocityRevocation, error) {
	  if limit <= 0 {
		  limit = 200
	  }
	   c := r.db.Conn(ctx)
	  q := `SELECT ` + revokeCols() + ` FROM subacct_vel_revocations WHERE active=1 ORDER BY frozen_at DESC LIMIT ` + ph(r.db.Dialect, 1)
	   rows, err := c.QueryContext(ctx, q, limit)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]VelocityRevocation, 0)
	   for rows.Next() {
		  got, err := scanRevoke(rows)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, got.Copy())
	  }
	   return out, rows.Err()
   }

// SQLAuditRepo 冻结 CAS 审计表。
type SQLAuditRepo struct{ db *persist.SQLDB }

   func NewSQLAuditRepo(db *persist.SQLDB) *SQLAuditRepo { return &SQLAuditRepo{db: db} }

   func (r *SQLAuditRepo) Append(ctx context.Context, row FreezeAudit) error {
	  if r == nil || r.db == nil || row.AuditID == "" {
		   return ErrInvalidInput
	  }
	  c := r.db.Conn(ctx)
	   q := `INSERT INTO subacct_freeze_cas_audit
 (audit_id, freeze_cas_id, parent_id, child_id, actor, action, outcome, detail, generation, at)
VALUES (` + strings.Join(func() []string {
		  out := make([]string, 10)
		  for i := range out {
			   out[i] = ph(r.db.Dialect, i+1)
		  }
		   return out
	  }(), ",") + `)`
	   _, err := c.ExecContext(ctx, q, row.AuditID, row.FreezeCASID, row.ParentID, row.ChildID,
		  row.Actor, row.Action, row.Outcome, row.Detail, row.Generation, row.At.UTC())
	  return err
   }

   func (r *SQLAuditRepo) list(ctx context.Context, field, key string, limit int) ([]FreezeAudit, error) {
	  if limit <= 0 {
		   limit = 50
	   }
	   c := r.db.Conn(ctx)
	  q := `SELECT audit_id, freeze_cas_id, parent_id, child_id, actor, action, outcome, detail, generation, at
 FROM subacct_freeze_cas_audit WHERE ` + field + `=` + ph(r.db.Dialect, 1) + ` ORDER BY at DESC LIMIT ` + ph(r.db.Dialect, 2)
	   rows, err := c.QueryContext(ctx, q, key, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]FreezeAudit, 0)
	   for rows.Next() {
		   var a FreezeAudit
		  if err := rows.Scan(&a.AuditID, &a.FreezeCASID, &a.ParentID, &a.ChildID, &a.Actor, &a.Action,
			  &a.Outcome, &a.Detail, &a.Generation, &a.At); err != nil {
			  return nil, err
		  }
		   a.At = a.At.UTC()
		  out = append(out, a)
	  }
	  return out, rows.Err()
   }

   func (r *SQLAuditRepo) ListByParent(ctx context.Context, parentID string, limit int) ([]FreezeAudit, error) {
	  return r.list(ctx, "parent_id", strings.TrimSpace(parentID), limit)
  }

  func (r *SQLAuditRepo) ListByCAS(ctx context.Context, casID string, limit int) ([]FreezeAudit, error) {
	  return r.list(ctx, "freeze_cas_id", strings.TrimSpace(casID), limit)
   }

   func journalCols() string {
	   return `journal_id, freeze_cas_id, owner_id, parent_id, child_id, asset, amount, idempotency_key, ok, refused, at, after_frozen`
   }

  func scanJournal(sc interface{ Scan(dest ...any) error }) (*DebitJournal, error) {
	  var j DebitJournal
	  var ok, after int
	   err := sc.Scan(&j.JournalID, &j.FreezeCASID, &j.OwnerID, &j.ParentID, &j.ChildID, &j.Asset, &j.Amount,
		   &j.Idem, &ok, &j.Refused, &j.At, &after)
	  if err != nil {
		   return nil, err
	  }
	   j.OK = ok != 0
	  j.AfterFrozen = after != 0
	  j.At = j.At.UTC()
	  return &j, nil
   }

// SQLJournalRepo 借记轨迹表。
type SQLJournalRepo struct{ db *persist.SQLDB }

  func NewSQLJournalRepo(db *persist.SQLDB) *SQLJournalRepo { return &SQLJournalRepo{db: db} }

  func (r *SQLJournalRepo) Save(ctx context.Context, row *DebitJournal) error {
	  if r == nil || r.db == nil || row == nil || row.JournalID == "" {
		   return ErrInvalidInput
	  }
	  ok, after := 0, 0
	   if row.OK {
		   ok = 1
	   }
	   if row.AfterFrozen {
		   after = 1
	  }
	   c := r.db.Conn(ctx)
	   var err error
	  if r.db.Dialect == persist.Postgres {
		   _, err = c.ExecContext(ctx, ` 
INSERT INTO subacct_freeze_cas_journals
 (journal_id, freeze_cas_id, owner_id, parent_id, child_id, asset, amount, idempotency_key, ok, refused, at, after_frozen)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (journal_id) DO NOTHING
`, row.JournalID, row.FreezeCASID, row.OwnerID, row.ParentID, row.ChildID, row.Asset, row.Amount,
			  row.Idem,
			  ok, row.Refused, row.At.UTC(), after)
	  } else {
		  _, err = c.ExecContext(ctx, `
INSERT INTO subacct_freeze_cas_journals
 (journal_id, freeze_cas_id, owner_id, parent_id, child_id, asset, amount, idempotency_key, ok, refused, at, after_frozen)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE journal_id=journal_id
`, row.JournalID, row.FreezeCASID, row.OwnerID, row.ParentID, row.ChildID, row.Asset, row.Amount,
			   row.Idem,
			   ok, row.Refused, row.At.UTC(), after)
	   }
	  return err
   }

  func (r *SQLJournalRepo) ListByOwner(ctx context.Context, ownerID string, since time.Time, limit int) ([]DebitJournal, error) {
	   if limit <= 0 {
		  limit = 50
	   }
	  if since.IsZero() {
		   since = time.Unix(0, 0).UTC()
	   }
	   c := r.db.Conn(ctx)
	  q := `SELECT ` + journalCols() + ` FROM subacct_freeze_cas_journals WHERE owner_id=` + ph(r.db.Dialect, 1) +
		  ` AND at>=` + ph(r.db.Dialect, 2) + ` ORDER BY at DESC LIMIT ` + ph(r.db.Dialect, 3)
	  rows, err := c.QueryContext(ctx, q, strings.TrimSpace(ownerID), since.UTC(), limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]DebitJournal, 0)
	   for rows.Next() {
		  got, err := scanJournal(rows)
		   if err != nil {
			  return nil, err
		  }
		   out = append(out, got.Copy())
	   }
	  return out, rows.Err()
   }

   func (r *SQLJournalRepo) ListAfter(ctx context.Context, since time.Time, limit int) ([]DebitJournal, error) {
	  if limit <= 0 {
		  limit = 100
	  }
	  if since.IsZero() {
		  since = time.Unix(0, 0).UTC()
	   }
	   c := r.db.Conn(ctx)
	   q := `SELECT ` + journalCols() + ` FROM subacct_freeze_cas_journals WHERE at>=` + ph(r.db.Dialect, 1) +
		  ` ORDER BY at DESC LIMIT ` + ph(r.db.Dialect, 2)
	  rows, err := c.QueryContext(ctx, q, since.UTC(), limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]DebitJournal, 0)
	   for rows.Next() {
		  got, err := scanJournal(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, got.Copy())
	   }
	   return out, rows.Err()
   }
