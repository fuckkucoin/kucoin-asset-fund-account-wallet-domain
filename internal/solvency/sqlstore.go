// 功能：偿付证明的 SQL 仓储。迁移表只追加本包自己的行，不改审计仓分区结构。
// 发布迁移在单条更新里完成：building 行缺少封存摘要时，条件更新不会把它写成 published。

// 作者：Bronson
// 日期：2021.11.15

package solvency

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type SQLStore struct {
	db      *sql.DB
	dialect persist.Dialect
}

   func NewSQLStore(db *sql.DB, d persist.Dialect) *SQLStore {
	  return &SQLStore{db: db, dialect: d}
  }

   func (s *SQLStore) q(query string) string {
	  return persist.Rewrite(query, s.dialect)
   }

  func (s *SQLStore) avail() error {
	  if s == nil || s.db == nil {
		  return ErrUnavailable
	  }
	   return nil
  }

const attestCols = `id, schedule_id, status, class_name, root_hash, leaf_count, signature, signer,
window_start, window_end, fence_epoch, fence_token, book_digest, seal_digest, coverage, scope_assets,
disclaimer, reason, audit_ref, created_at, updated_at, sealed_at, published_at, revoked_at`

  func (s *SQLStore) InsertAttestation(ctx context.Context, a Attestation) error {
	   if err := s.avail(); err != nil {
		   return err
	   }
	   if a.ID == "" || a.Status != StatusBuilding || a.Root != "" || a.SealDigest != "" {
		   return ErrTransition
	   }
	  a.Solvency,
		   a.Deprecated = deriveSolvency(a.Class)
	  _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO aa_solvency_attestations (
id, schedule_id, status, class_name, root_hash, leaf_count, signature, signer,
window_start, window_end, fence_epoch, fence_token, book_digest, seal_digest, coverage, scope_assets,
disclaimer, reason, audit_ref, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		  a.ID, a.ScheduleID, string(a.Status), a.Class, a.Root, a.LeafCount, a.Signature, a.Signer,
		   utc(a.WindowStart), utc(a.WindowEnd), a.FenceEpoch, a.FenceToken, a.BookDigest, a.SealDigest,
		   a.Coverage, a.Scope, a.Disclaimer, a.Reason, a.AuditRef, utc(a.CreatedAt), utc(a.UpdatedAt),
	  )
	  return err
  }

  func (s *SQLStore) GetAttestation(ctx context.Context, id string) (Attestation, error) {
	   if err := s.avail(); err != nil {
		  return Attestation{}, err
	  }
	   row := s.db.QueryRowContext(ctx, s.q(`SELECT `+attestCols+` FROM aa_solvency_attestations WHERE id=?`), strings.TrimSpace(id))
	   return scanAttest(row)
  }

   func (s *SQLStore) ListAttestations(ctx context.Context,
	  scheduleID string, limit int) ([]Attestation, error) {
	   if err := s.avail(); err != nil {
		  return nil, err
	  }
	  if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	  var (
		   rows *sql.Rows
		   err  error
	  )
	  if strings.TrimSpace(scheduleID) == "" {
		  rows, err = s.db.QueryContext(ctx, s.q(`SELECT `+attestCols+` FROM aa_solvency_attestations ORDER BY created_at DESC LIMIT ?`), limit)
	   } else {
		   rows, err = s.db.QueryContext(ctx, s.q(`SELECT `+attestCols+` FROM aa_solvency_attestations WHERE schedule_id=? ORDER BY created_at DESC LIMIT ?`), scheduleID, limit)
	  }
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]Attestation, 0)
	  for rows.Next() {
		   a, err := scanAttest(rows)
		   if err != nil {
			  return nil, err
		  }
		  out = append(out, a)
	  }
	   return out, rows.Err()
   }

type rowScanner interface{ Scan(dest ...any) error }

  func scanAttest(sc rowScanner) (Attestation, error) {
	   var a Attestation
	   var status string
	  var sealed, published, revoked sql.NullTime
	   err := sc.Scan(
		   &a.ID, &a.ScheduleID, &status, &a.Class, &a.Root, &a.LeafCount, &a.Signature, &a.Signer,
		  &a.WindowStart,
		   &a.WindowEnd, &a.FenceEpoch, &a.FenceToken, &a.BookDigest, &a.SealDigest,
		   &a.Coverage, &a.Scope, &a.Disclaimer, &a.Reason, &a.AuditRef, &a.CreatedAt, &a.UpdatedAt,
		   &sealed, &published, &revoked,
	   )
	   if err == sql.ErrNoRows {
		   return Attestation{}, ErrNotFound
	   }
	  if err != nil {
		  return Attestation{}, err
	   }
	  a.Status = Status(status)
	   a.WindowStart = utc(a.WindowStart)
	   a.WindowEnd = utc(a.WindowEnd)
	   a.CreatedAt = utc(a.CreatedAt)
	   a.UpdatedAt = utc(a.UpdatedAt)
	  a.SealedAt = nullToPtr(sealed)
	   a.PublishedAt = nullToPtr(published)
	  a.RevokedAt = nullToPtr(revoked)
	   a.Solvency, a.Deprecated = deriveSolvency(a.Class)
	  return a, nil
  }

   func nullToPtr(n sql.NullTime) *time.Time {
	   if !n.Valid {
		  return nil
	   }
	   t := utc(n.Time)
	   return &t
  }

   func ptrArg(t *time.Time) any {
	   if t == nil || t.IsZero() {
		  return nil
	   }
	   return utc(*t)
  }

  func (s *SQLStore) SaveCollection(ctx context.Context, a Attestation, leaves []LiabilityLeaf) error {
	  if err := s.avail(); err != nil {
		   return err
	  }
	  tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		   return err
	  }
	  defer func() { _ = tx.Rollback() }()
	  res, err := tx.ExecContext(ctx, s.q(`UPDATE aa_solvency_attestations
SET root_hash=?, leaf_count=?, book_digest=?, fence_epoch=?, updated_at=?
WHERE id=? AND status=? AND root_hash='' AND seal_digest=''`),
		   a.Root, a.LeafCount, a.BookDigest, a.FenceEpoch, utc(a.UpdatedAt), a.ID, string(StatusBuilding))
	   if err != nil {
		  return err
	   }
	  n, _ := res.RowsAffected()
	   if n != 1 {
		   return ErrAlreadyCollected
	   }
	  if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM aa_solvency_leaves WHERE attestation_id=?`), a.ID); err != nil {
		  return err
	  }
	  for _, l := range leaves {
		  _, err := tx.ExecContext(ctx, s.q(`INSERT INTO aa_solvency_leaves ( 
id, attestation_id, user_id, book, asset, available, frozen, total, fence_epoch, fence_token, leaf_hash, collected_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			  l.ID,
			  a.ID, l.UserID, l.Book, l.Asset, l.Available, l.Frozen, l.Total,
			   l.FenceEpoch, l.FenceToken, l.LeafHash, utc(l.CollectedAt))
		  if err != nil {
			  return err
		   }
	  }
	  return tx.Commit()
   }

  func (s *SQLStore) ListLeaves(ctx context.Context, attestationID string) ([]LiabilityLeaf, error) {
	  if err := s.avail(); err != nil {
		   return nil, err
	   }
	  if _, err := s.GetAttestation(ctx, attestationID); err != nil {
		  return nil, err
	  }
	   rows, err := s.db.QueryContext(ctx, s.q(`SELECT id, attestation_id, user_id, book, asset, available, frozen, total, fence_epoch, fence_token, leaf_hash, collected_at
FROM aa_solvency_leaves WHERE attestation_id=? ORDER BY user_id, book, asset`), attestationID)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]LiabilityLeaf, 0)
	  for rows.Next() {
		  var l LiabilityLeaf
		   if err := rows.Scan(&l.ID, &l.AttestationID, &l.UserID, &l.Book, &l.Asset, &l.Available, &l.Frozen, &l.Total, &l.FenceEpoch, &l.FenceToken, &l.LeafHash, &l.CollectedAt); err != nil {
			  return nil, err
		   }
		   l.CollectedAt = utc(l.CollectedAt)
		  out = append(out, l)
	   }
	   return out, rows.Err()
  }

   func (s *SQLStore) SaveCompare(ctx context.Context, a Attestation, snaps []ReserveSnapshot, drifts []DriftCase) error {
	  if err := s.avail(); err != nil {
		   return err
	   }
	  tx, err := s.db.BeginTx(ctx, nil)
	   if err != nil {
		   return err
	  }
	   defer func() { _ = tx.Rollback() }()
	  var stuck int
	  if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(1) FROM aa_solvency_drifts WHERE attestation_id=? AND status<>?`), a.ID, string(DriftOpen)).Scan(&stuck); err != nil {
		  return err
	  }
	   if stuck > 0 {
		   return ErrDrift
	  }
	   res, err := tx.ExecContext(ctx, s.q(`UPDATE aa_solvency_attestations
SET coverage=?, class_name=?, disclaimer=?, updated_at=?
WHERE id=? AND status=? AND root_hash<>''`),
		   a.Coverage, a.Class, a.Disclaimer, utc(a.UpdatedAt), a.ID, string(StatusBuilding))
	   if err != nil {
		   return err
	  }
	  n, _ := res.RowsAffected()
	  if n != 1 {
		   return ErrImmutable
	  }
	   if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM aa_solvency_reserve_snaps WHERE attestation_id=?`), a.ID); err != nil {
		   return err
	   }
	  if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM aa_solvency_drifts WHERE attestation_id=? AND status=?`), a.ID, string(DriftOpen)); err != nil {
		   return err
	   }
	   for _, sn := range snaps {
		   _, err := tx.ExecContext(ctx, s.q(`INSERT INTO aa_solvency_reserve_snaps (
id, attestation_id, asset, hot, warm, cold, observed, liability, surplus, deficit, source_name, taken_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`),
			   sn.ID, a.ID, sn.Asset, sn.Hot, sn.Warm, sn.Cold, sn.Observed, sn.Liability, sn.Surplus, sn.Deficit, sn.Source, utc(sn.TakenAt))
		  if err != nil {
			   return err
		   }
	   }
	   for _, d := range drifts {
		  _, err := tx.ExecContext(ctx, s.q(`INSERT INTO aa_solvency_drifts ( 
id, attestation_id, asset, status, severity, liability, observed, hot, warm, cold, deficit, ratio_bps, explain_text, opened_at, signaled
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
			  d.ID,
			  a.ID, d.Asset, string(d.Status), d.Severity, d.Liability, d.Observed, d.Hot, d.Warm, d.Cold, d.Deficit, d.RatioBps, d.Explain, utc(d.OpenedAt), boolArg(d.Signaled))
		  if err != nil {
			  return err
		  }
	   }
	   return tx.Commit()
   }

   func boolArg(v bool) int {
	  if v {
		   return 1
	   }
	   return 0
  }

   func (s *SQLStore) ListReserve(ctx context.Context, attestationID string) ([]ReserveSnapshot, error) {
	  if err := s.avail(); err != nil {
		   return nil, err
	  }
	  if _, err := s.GetAttestation(ctx, attestationID); err != nil {
		  return nil, err
	   }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT id, attestation_id, asset, hot, warm, cold, observed, liability, surplus, deficit, source_name, taken_at
FROM aa_solvency_reserve_snaps WHERE attestation_id=? ORDER BY asset`), attestationID)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]ReserveSnapshot, 0)
	   for rows.Next() {
		   var sn ReserveSnapshot
		  if err := rows.Scan(&sn.ID, &sn.AttestationID, &sn.Asset, &sn.Hot, &sn.Warm, &sn.Cold, &sn.Observed, &sn.Liability, &sn.Surplus, &sn.Deficit, &sn.Source, &sn.TakenAt); err != nil {
			   return nil, err
		   }
		  sn.TakenAt = utc(sn.TakenAt)
		  out = append(out, sn)
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) CasAttestation(ctx context.Context, id string, from Status, patch AttestPatch) (Attestation, error) {
	   if err := s.avail(); err != nil {
		   return Attestation{}, err
	  }
	  cur, err := s.GetAttestation(ctx, id)
	  if err != nil {
		   return Attestation{}, err
	  }
	  if cur.Status != from || !AllowAttest(cur.Status, patch.Status) {
		  return cur, ErrTransition
	  }
	  if patch.Status == StatusPublished && (cur.Root == "" || cur.SealDigest == "" || cur.Signature == "") {
		  return cur, ErrPartial
	  }
	  if patch.Status == StatusSealed && (cur.Root == "" || patch.SealDigest == "" || patch.Signature == "" || patch.SealedAt == nil) {
		  return cur, ErrPartial
	  }
	   next := applyPatch(cur, patch)
	   if next.Root != cur.Root || next.FenceEpoch != cur.FenceEpoch || next.FenceToken != cur.FenceToken || next.BookDigest != cur.BookDigest {
		  return cur, ErrImmutable
	  }
	  res, err := s.db.ExecContext(ctx, s.q(`UPDATE aa_solvency_attestations SET 
status=?, class_name=?, signature=?, signer=?, seal_digest=?, disclaimer=?, reason=?, audit_ref=?,
updated_at=?, sealed_at=?, published_at=?, revoked_at=?
WHERE id=? AND status=? AND root_hash=? AND seal_digest=?`),
		  string(next.Status), next.Class, next.Signature, next.Signer, next.SealDigest, next.Disclaimer, next.Reason, next.AuditRef,
		  utc(next.UpdatedAt), ptrArg(next.SealedAt), ptrArg(next.PublishedAt), ptrArg(next.RevokedAt),
		   id, string(from), cur.Root, cur.SealDigest)
	   if err != nil {
		   return cur, err
	  }
	  n, _ := res.RowsAffected()
	  if n != 1 {
		  return cur, ErrTransition
	  }
	   return s.GetAttestation(ctx, id)
   }

  func (s *SQLStore) NoteAudit(ctx context.Context, id, ref string) error {
	   if err := s.avail(); err != nil {
		   return err
	  }
	   ref = strings.TrimSpace(ref)
	   if ref == "" {
		   return ErrAudit
	  }
	  res, err := s.db.ExecContext(ctx, s.q(`UPDATE aa_solvency_attestations SET audit_ref=? WHERE id=? AND status IN (?, ?) AND root_hash<>''`),
		  ref, id, string(StatusSealed), string(StatusPublished))
	   if err != nil {
		   return err
	   }
	   n, _ := res.RowsAffected()
	  if n != 1 {
		   return ErrTransition
	  }
	  return nil
   }

  func (s *SQLStore) GetDrift(ctx context.Context, id string) (DriftCase, error) {
	  if err := s.avail(); err != nil {
		  return DriftCase{}, err
	   }
	  row := s.db.QueryRowContext(ctx, s.q(`SELECT id, attestation_id, asset, status, severity, liability, observed, hot, warm, cold, deficit, ratio_bps, explain_text,
opened_at, acked_at, closed_at, acked_by, closed_by, note, signaled
FROM aa_solvency_drifts WHERE id=?`), strings.TrimSpace(id))
	  return scanDrift(row)
  }

   func (s *SQLStore) ListDrifts(ctx context.Context, status DriftStatus, limit int) ([]DriftCase, error) {
	   if err := s.avail(); err != nil {
		  return nil, err
	   }
	  if limit <= 0 || limit > 500 {
		   limit = 100
	  }
	   var (
		  rows *sql.Rows
		   err  error
	   )
	   q := `SELECT id, attestation_id, asset, status, severity, liability, observed, hot, warm, cold, deficit, ratio_bps, explain_text,
opened_at, acked_at, closed_at, acked_by, closed_by, note, signaled FROM aa_solvency_drifts`
	  if status == "" {
		   rows, err = s.db.QueryContext(ctx, s.q(q+` ORDER BY opened_at DESC LIMIT ?`), limit)
	   } else {
		  rows, err = s.db.QueryContext(ctx, s.q(q+` WHERE status=? ORDER BY opened_at DESC LIMIT ?`), string(status), limit)
	  }
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]DriftCase, 0)
	   for rows.Next() {
		   d, err := scanDrift(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, d)
	  }
	  return out, rows.Err()
  }

  func scanDrift(sc rowScanner) (DriftCase, error) {
	  var d DriftCase
	  var status string
	   var acked, closed sql.NullTime
	   var signaled int
	   err := sc.Scan(&d.ID,
		   &d.AttestationID, &d.Asset, &status, &d.Severity, &d.Liability, &d.Observed, &d.Hot, &d.Warm, &d.Cold,
		   &d.Deficit, &d.RatioBps, &d.Explain, &d.OpenedAt, &acked, &closed, &d.AckedBy, &d.ClosedBy, &d.Note, &signaled)
	   if err == sql.ErrNoRows {
		   return DriftCase{},
			   ErrDriftNotFound
	  }
	  if err != nil {
		   return DriftCase{}, err
	  }
	  d.Status = DriftStatus(status)
	   d.OpenedAt = utc(d.OpenedAt)
	   d.AckedAt = nullToPtr(acked)
	   d.ClosedAt = nullToPtr(closed)
	   d.Signaled = signaled != 0
	  return d, nil
   }

   func (s *SQLStore) CasDrift(ctx context.Context, id string, from DriftStatus, next DriftCase) (DriftCase, error) {
	  if err := s.avail(); err != nil {
		  return DriftCase{}, err
	  }
	   cur, err := s.GetDrift(ctx, id)
	  if err != nil {
		   return DriftCase{}, err
	  }
	  if cur.Status != from || !AllowDrift(from, next.Status) {
		  return cur, ErrDrift
	   }
	   if cur.Deficit != next.Deficit || cur.Liability != next.Liability || cur.Asset != next.Asset {
		  return cur, ErrImmutable
	   }
	   res, err := s.db.ExecContext(ctx, s.q(`UPDATE aa_solvency_drifts SET status=?, acked_at=?, closed_at=?, acked_by=?, closed_by=?, note=?, signaled=? 
WHERE id=? AND status=? AND deficit=?`),
		   string(next.Status), ptrArg(next.AckedAt), ptrArg(next.ClosedAt), next.AckedBy, next.ClosedBy, next.Note, boolArg(next.Signaled),
		  id, string(from), cur.Deficit)
	  if err != nil {
		  return cur, err
	   }
	  n, _ := res.RowsAffected()
	   if n != 1 {
		   return cur, ErrDrift
	  }
	   return s.GetDrift(ctx, id)
   }

  func (s *SQLStore) PutSchedule(ctx context.Context, sch Schedule) error {
	   if err := s.avail(); err != nil {
		   return err
	   }
	  if sch.ID == "" || sch.Name == "" || sch.Window <= 0 {
		   return ErrSchedule
	   }
	   args := []any{
		  sch.ID, sch.Name, int64(sch.Interval / time.Second), int64(sch.Window / time.Second), joinScope(sch.Assets),
		  boolArg(sch.Enabled), boolArg(sch.AutoPublish), utc(sch.NextDue), sch.LastRunID, utc(sch.CreatedAt), utc(sch.UpdatedAt),
	   }
	  q := `INSERT INTO aa_solvency_schedules (
id, name, interval_sec, window_sec, assets, enabled, auto_publish, next_due, last_run_id, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE name=VALUES(name), interval_sec=VALUES(interval_sec), window_sec=VALUES(window_sec),
assets=VALUES(assets), enabled=VALUES(enabled), auto_publish=VALUES(auto_publish), next_due=VALUES(next_due),
last_run_id=VALUES(last_run_id), updated_at=VALUES(updated_at)`
	  if s.dialect == persist.Postgres {
		  q = `INSERT INTO aa_solvency_schedules (
id, name, interval_sec, window_sec, assets, enabled, auto_publish, next_due, last_run_id, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, interval_sec=EXCLUDED.interval_sec, window_sec=EXCLUDED.window_sec,
assets=EXCLUDED.assets, enabled=EXCLUDED.enabled, auto_publish=EXCLUDED.auto_publish, next_due=EXCLUDED.next_due,
last_run_id=EXCLUDED.last_run_id, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := s.db.ExecContext(ctx, s.q(q), args...)
	   return err
  }

  func (s *SQLStore) MarkSignaled(ctx context.Context, id string) error {
	   if err := s.avail(); err != nil {
		  return err
	  }
	  res, err := s.db.ExecContext(ctx, s.q(`UPDATE aa_solvency_drifts SET signaled=1 WHERE id=?`), strings.TrimSpace(id))
	  if err != nil {
		  return err
	  }
	  n, _ := res.RowsAffected()
	  if n != 1 {
		   return ErrDriftNotFound
	   }
	   return nil
   }

  func (s *SQLStore) GetSchedule(ctx context.Context, id string) (Schedule, error) {
	  if err := s.avail(); err != nil {
		  return Schedule{}, err
	  }
	  row := s.db.QueryRowContext(ctx, s.q(`SELECT id, name, interval_sec, window_sec, assets, enabled, auto_publish, next_due, last_run_id, created_at, updated_at
FROM aa_solvency_schedules WHERE id=?`), strings.TrimSpace(id))
	  return scanSched(row)
  }

   func (s *SQLStore) ListSchedules(ctx context.Context) ([]Schedule, error) {
	  if err := s.avail(); err != nil {
		  return nil, err
	  }
	  rows, err := s.db.QueryContext(ctx, s.q(`SELECT id, name, interval_sec, window_sec, assets, enabled, auto_publish, next_due, last_run_id, created_at, updated_at
FROM aa_solvency_schedules ORDER BY id`))
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]Schedule, 0)
	   for rows.Next() {
		   sch, err := scanSched(rows)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, sch)
	  }
	  return out, rows.Err()
  }

   func scanSched(sc rowScanner) (Schedule, error) {
	  var sch Schedule
	  var interval, window int64
	  var assets string
	  var enabled, auto int
	  err := sc.Scan(&sch.ID, &sch.Name, &interval, &window, &assets, &enabled, &auto, &sch.NextDue, &sch.LastRunID, &sch.CreatedAt, &sch.UpdatedAt)
	  if err == sql.ErrNoRows {
		  return Schedule{},
			  ErrScheduleNotFound
	   }
	  if err != nil {
		  return Schedule{}, err
	  }
	  sch.Interval = time.Duration(interval) * time.Second
	   sch.Window = time.Duration(window) * time.Second
	   sch.Enabled = enabled != 0
	   sch.AutoPublish = auto != 0
	   sch.NextDue = utc(sch.NextDue)
	  sch.CreatedAt = utc(sch.CreatedAt)
	   sch.UpdatedAt = utc(sch.UpdatedAt)
	   if strings.TrimSpace(assets) != "" {
		  sch.Assets = strings.Split(assets, ",")
	  }
	   return sch, nil
  }
