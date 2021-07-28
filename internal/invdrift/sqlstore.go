// @FileName: sqlstore.go
// @Summary: invdrift 包内 sqlstore 实现。
// SQL 存储实现：写入/读取偏离批次、条目、封堵建议与证明。
// - 使用独立表族（drift_batches / drift_items / seal_advices / drift_proofs），
// 不读写 aa_ledger_drift_block 或 leddiff 相关表。
// - 所有查询使用参数化语句，防止 SQL 注入。
// - InsertItems 使用批量 INSERT 以减少往返。
// @Author: Blaise
// @Created: 2021.07.27

package invdrift

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQLStore invdrift SQL 存储实现
type SQLStore struct {
	db *sql.DB
}

// NewSQLStore 创建 SQLStore；db 不能为 nil。
  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{db: db}
  }

// ---- DriftBatch ----

  func (s *SQLStore) InsertBatch(ctx context.Context, b DriftBatch) error {
	  if s == nil || s.db == nil {
		   return ErrStore
	  }
	  var gradedAt, sealedAt, resolvedAt any
	   if b.GradedAt != nil {
		   gradedAt = *b.GradedAt
	  }
	  if b.SealedAt != nil {
		  sealedAt = *b.SealedAt
	  }
	  if b.ResolvedAt != nil {
		  resolvedAt = *b.ResolvedAt
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO drift_batches
  (id, state, trigger_by, data_center, scan_at, graded_at, sealed_at, resolved_at,
   crit_count, warn_count, info_count, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   b.ID, string(b.State), b.TriggerBy, b.DataCenter,
		  b.ScanAt, gradedAt, sealedAt, resolvedAt,
		   b.CritCount, b.WarnCount, b.InfoCount,
		   b.CreatedAt, b.UpdatedAt)
	   return err
  }

  func (s *SQLStore) UpdateBatch(ctx context.Context, b DriftBatch) error {
	   if s == nil || s.db == nil {
		   return ErrStore
	   }
	   var gradedAt, sealedAt, resolvedAt any
	   if b.GradedAt != nil {
		  gradedAt = *b.GradedAt
	   }
	  if b.SealedAt != nil {
		   sealedAt = *b.SealedAt
	   }
	  if b.ResolvedAt != nil {
		   resolvedAt = *b.ResolvedAt
	   }
	  _, err := s.db.ExecContext(ctx, `
UPDATE drift_batches
SET state=?, graded_at=?, sealed_at=?, resolved_at=?,
    crit_count=?, warn_count=?, info_count=?, updated_at=?
WHERE id=?`,
		  string(b.State),
		   gradedAt, sealedAt, resolvedAt,
		   b.CritCount, b.WarnCount, b.InfoCount, b.UpdatedAt,
		   b.ID)
	  return err
  }

  func (s *SQLStore) GetBatch(ctx context.Context, id string) (DriftBatch, error) {
	  if s == nil || s.db == nil {
		  return DriftBatch{}, ErrStore
	  }
	   row := s.db.QueryRowContext(ctx, `
SELECT id, state, trigger_by, data_center, scan_at, graded_at, sealed_at, resolved_at,
       crit_count, warn_count, info_count, created_at, updated_at
FROM drift_batches WHERE id=?`, id)
	  return scanBatch(row)
   }

   func (s *SQLStore) ListBatches(ctx context.Context, limit, offset int) ([]DriftBatch, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStore
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT id, state, trigger_by, data_center, scan_at, graded_at, sealed_at, resolved_at,
       crit_count, warn_count, info_count, created_at, updated_at
FROM drift_batches ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  return scanBatches(rows)
  }

   func (s *SQLStore) ListOpenBatches(ctx context.Context) ([]DriftBatch, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStore
	   }
	   rows, err := s.db.QueryContext(ctx, `
SELECT id, state, trigger_by, data_center, scan_at, graded_at, sealed_at, resolved_at,
       crit_count, warn_count, info_count, created_at, updated_at
FROM drift_batches
WHERE state NOT IN ('resolved') ORDER BY created_at DESC`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanBatches(rows)
   }

// ---- DriftItem ----

  func (s *SQLStore) InsertItems(ctx context.Context, items []DriftItem) error {
	  if s == nil || s.db == nil {
		   return ErrStore
	  }
	  if len(items) == 0 {
		  return nil
	  }
	   // 批量插入：MySQL 支持 multi-row VALUES
	  const batchSize = 50
	   for start := 0; start < len(items); start += batchSize {
		  end := start + batchSize
		   if end > len(items) {
			   end = len(items)
		   }
		   chunk := items[start:end]
		  ph := make([]string, 0, len(chunk))
		  args := make([]any, 0, len(chunk)*18)
		  for _, it := range chunk {
			  ph = append(ph, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
			  var gradedAt, resolvedAt any
			   if it.GradedAt != nil {
				  gradedAt = *it.GradedAt
			  }
			  if it.ResolvedAt != nil {
				   resolvedAt = *it.ResolvedAt
			  }
			   args = append(args,
				   it.ID, it.BatchID, it.Asset, it.DataCenter,
				  it.LedgerHot, it.LedgerCold, it.LedgerWarm,
				   it.CustHot, it.CustCold,
				   it.InvHot, it.InvCold,
				  it.DeltaHot, it.DeltaCold, it.DeltaPct,
				   string(it.Grade), gradedAt, resolvedAt,
				   it.CreatedAt)
		   }
		   q := `INSERT INTO drift_items
  (id, batch_id, asset, data_center,
   ledger_hot, ledger_cold, ledger_warm,
   cust_hot, cust_cold, inv_hot, inv_cold,
   delta_hot, delta_cold, delta_pct,
   grade, graded_at, resolved_at, created_at)
VALUES ` + strings.Join(ph, ",")
		  if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			  return err
		  }
	   }
	   return nil
  }

   func (s *SQLStore) ListItemsByBatch(ctx context.Context, batchID string) ([]DriftItem, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStore
	   }
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, batch_id, asset, data_center,
       ledger_hot, ledger_cold, ledger_warm,
       cust_hot, cust_cold, inv_hot, inv_cold,
       delta_hot, delta_cold, delta_pct,
       grade, graded_at, resolved_at, created_at
FROM drift_items WHERE batch_id=? ORDER BY asset`, batchID)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanItems(rows)
  }

  func (s *SQLStore) ListItemsByAsset(ctx context.Context, asset string, limit int) ([]DriftItem, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStore
	   }
	   if limit <= 0 {
		  limit = 20
	   }
	   rows, err := s.db.QueryContext(ctx, ` 
SELECT id, batch_id, asset, data_center,
       ledger_hot, ledger_cold, ledger_warm,
       cust_hot, cust_cold, inv_hot, inv_cold,
       delta_hot, delta_cold, delta_pct,
       grade, graded_at, resolved_at, created_at
FROM drift_items WHERE asset=? ORDER BY created_at DESC LIMIT ?`, asset, limit)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanItems(rows)
   }

  func (s *SQLStore) MarkItemResolved(ctx context.Context, itemID string, resolvedAt time.Time) error {
	  if s == nil || s.db == nil {
		  return ErrStore
	   }
	  _, err := s.db.ExecContext(ctx, `
UPDATE drift_items SET resolved_at=? WHERE id=?`, resolvedAt, itemID)
	  return err
   }

// ---- SealAdvice ----

   func (s *SQLStore) InsertAdvices(ctx context.Context, advices []SealAdvice) error {
	   if s == nil || s.db == nil {
		   return ErrStore
	  }
	  for _, a := range advices {
		  var ackedAt, expiredAt any
		   if a.AckedAt != nil {
			  ackedAt = *a.AckedAt
		  }
		  if a.ExpiredAt != nil {
			   expiredAt = *a.ExpiredAt
		   }
		   _, err := s.db.ExecContext(ctx, ` 
INSERT INTO seal_advices
  (id, batch_id, item_id, asset, kind, state, grade,
   proposed_at, acked_at, acked_by, expired_at, expire_reason,
   ttl_seconds, note, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			  a.ID, a.BatchID, a.ItemID, a.Asset,
			   string(a.Kind),
			  string(a.State), string(a.Grade),
			   a.ProposedAt, ackedAt, a.AckedBy, expiredAt, a.ExpireReason,
			  a.TTLSeconds, a.Note, a.CreatedAt)
		   if err != nil {
			   return err
		  }
	   }
	   return nil
  }

  func (s *SQLStore) GetAdvice(ctx context.Context, id string) (SealAdvice, error) {
	   if s == nil || s.db == nil {
		   return SealAdvice{}, ErrStore
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT id, batch_id, item_id, asset, kind, state, grade,
       proposed_at, acked_at, acked_by, expired_at, expire_reason,
       ttl_seconds, note, created_at
FROM seal_advices WHERE id=?`, id)
	   return scanAdvice(row)
   }

   func (s *SQLStore) ListAdvicesByBatch(ctx context.Context, batchID string) ([]SealAdvice, error) {
	  if s == nil || s.db == nil {
		  return nil, ErrStore
	   }
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, batch_id, item_id, asset, kind, state, grade,
       proposed_at, acked_at, acked_by, expired_at, expire_reason,
       ttl_seconds, note, created_at
FROM seal_advices WHERE batch_id=? ORDER BY created_at`, batchID)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanAdvices(rows)
   }

  func (s *SQLStore) ListActiveAdvices(ctx context.Context) ([]SealAdvice, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStore
	  }
	   rows, err := s.db.QueryContext(ctx, `
SELECT id, batch_id, item_id, asset, kind, state, grade,
       proposed_at, acked_at, acked_by, expired_at, expire_reason,
       ttl_seconds, note, created_at
FROM seal_advices WHERE state='proposed' ORDER BY created_at DESC`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanAdvices(rows)
   }

   func (s *SQLStore) UpdateAdviceState(ctx context.Context, id string, state AdviceState,
	   ackedBy string, ackedAt, expiredAt *time.Time, expireReason string) error {
	   if s == nil || s.db == nil {
		   return ErrStore
	   }
	   var aa, ea any
	  if ackedAt != nil {
		  aa = *ackedAt
	  }
	   if expiredAt != nil {
		  ea = *expiredAt
	  }
	  _, err := s.db.ExecContext(ctx, ` 
UPDATE seal_advices
SET state=?, acked_by=?, acked_at=?, expired_at=?, expire_reason=?
WHERE id=?`,
		  string(state), ackedBy, aa, ea, expireReason, id)
	  return err
  }

// ---- DriftProof ----

   func (s *SQLStore) InsertProof(ctx context.Context, p DriftProof) error {
	  if s == nil || s.db == nil {
		  return ErrStore
	   }
	  _, err := s.db.ExecContext(ctx, ` 
INSERT INTO drift_proofs
  (id, batch_id, asset,
   ledger_digest, custody_digest, hotinv_digest,
   ledger_snap, custody_snap, hotinv_snap,
   delta_hot, delta_cold, composite_hash,
   grade, recorded_at, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   p.ID, p.BatchID, p.Asset,
		  p.LedgerDigest,
		   p.CustodyDigest, p.HotInvDigest,
		   p.LedgerSnap, p.CustodySnap, p.HotInvSnap,
		   p.DeltaHot, p.DeltaCold, p.CompositeHash,
		   string(p.Grade), p.RecordedAt, p.CreatedAt)
	   return err
  }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (DriftProof, error) {
	  if s == nil || s.db == nil {
		   return DriftProof{}, ErrStore
	   }
	  row := s.db.QueryRowContext(ctx, `
SELECT id, batch_id, asset,
       ledger_digest, custody_digest, hotinv_digest,
       ledger_snap, custody_snap, hotinv_snap,
       delta_hot, delta_cold, composite_hash,
       grade, recorded_at, created_at
FROM drift_proofs WHERE id=?`, id)
	   return scanProof(row)
  }

   func (s *SQLStore) ListProofsByBatch(ctx context.Context, batchID string) ([]DriftProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStore
	  }
	  rows, err := s.db.QueryContext(ctx, `
SELECT id, batch_id, asset,
       ledger_digest, custody_digest, hotinv_digest,
       ledger_snap, custody_snap, hotinv_snap,
       delta_hot, delta_cold, composite_hash,
       grade, recorded_at, created_at
FROM drift_proofs WHERE batch_id=? ORDER BY asset`, batchID)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  return scanProofs(rows)
  }

// ---- scan helpers ----

type rowScanner interface {
	Scan(dest ...any) error
}

   func scanBatch(row rowScanner) (DriftBatch, error) {
	   var b DriftBatch
	  var state string
	  var gradedAt, sealedAt, resolvedAt sql.NullTime
	  err := row.Scan(
		  &b.ID, &state, &b.TriggerBy, &b.DataCenter,
		  &b.ScanAt, &gradedAt, &sealedAt, &resolvedAt,
		   &b.CritCount, &b.WarnCount, &b.InfoCount,
		  &b.CreatedAt, &b.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return DriftBatch{}, ErrBatchNotFound
	   }
	  if err != nil {
		   return DriftBatch{}, err
	   }
	  b.State = BatchState(state)
	   if gradedAt.Valid {
		   t := gradedAt.Time
		   b.GradedAt = &t
	  }
	   if sealedAt.Valid {
		  t := sealedAt.Time
		   b.SealedAt = &t
	  }
	  if resolvedAt.Valid {
		  t := resolvedAt.Time
		   b.ResolvedAt = &t
	  }
	   return b, nil
  }

  func scanBatches(rows *sql.Rows) ([]DriftBatch, error) {
	  var out []DriftBatch
	   for rows.Next() {
		   b, err := scanBatch(rows)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, b)
	   }
	   return out, rows.Err()
  }

   func scanItems(rows *sql.Rows) ([]DriftItem, error) {
	   var out []DriftItem
	  for rows.Next() {
		  var it DriftItem
		   var grade string
		  var gradedAt, resolvedAt sql.NullTime
		   err := rows.Scan(
			  &it.ID, &it.BatchID, &it.Asset, &it.DataCenter,
			   &it.LedgerHot, &it.LedgerCold, &it.LedgerWarm,
			  &it.CustHot, &it.CustCold, &it.InvHot, &it.InvCold,
			   &it.DeltaHot,
			  &it.DeltaCold, &it.DeltaPct,
			   &grade, &gradedAt, &resolvedAt, &it.CreatedAt)
		  if err != nil {
			   return nil, err
		   }
		   it.Grade = DriftGrade(grade)
		  if gradedAt.Valid {
			  t := gradedAt.Time
			   it.GradedAt = &t
		  }
		   if resolvedAt.Valid {
			  t := resolvedAt.Time
			  it.ResolvedAt = &t
		  }
		   out = append(out, it)
	  }
	   return out, rows.Err()
  }

   func scanAdvice(row rowScanner) (SealAdvice, error) {
	   var a SealAdvice
	  var kind, state, grade string
	  var ackedAt, expiredAt sql.NullTime
	  err := row.Scan(
		   &a.ID, &a.BatchID, &a.ItemID, &a.Asset,
		  &kind, &state, &grade,
		  &a.ProposedAt, &ackedAt, &a.AckedBy, &expiredAt, &a.ExpireReason,
		   &a.TTLSeconds,
		  &a.Note, &a.CreatedAt)
	  if err == sql.ErrNoRows {
		   return SealAdvice{}, ErrAdviceNotFound
	   }
	   if err != nil {
		   return SealAdvice{}, err
	   }
	   a.Kind = SealKind(kind)
	  a.State = AdviceState(state)
	   a.Grade = DriftGrade(grade)
	   if ackedAt.Valid {
		  t := ackedAt.Time
		  a.AckedAt = &t
	  }
	   if expiredAt.Valid {
		  t := expiredAt.Time
		  a.ExpiredAt = &t
	  }
	   return a, nil
  }

  func scanAdvices(rows *sql.Rows) ([]SealAdvice, error) {
	   var out []SealAdvice
	   for rows.Next() {
		  a, err := scanAdvice(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, a)
	   }
	   return out, rows.Err()
   }

   func scanProof(row rowScanner) (DriftProof, error) {
	  var p DriftProof
	  var grade string
	  err := row.Scan(
		  &p.ID, &p.BatchID, &p.Asset,
		  &p.LedgerDigest, &p.CustodyDigest, &p.HotInvDigest,
		   &p.LedgerSnap, &p.CustodySnap, &p.HotInvSnap,
		  &p.DeltaHot,
		   &p.DeltaCold, &p.CompositeHash,
		  &grade, &p.RecordedAt, &p.CreatedAt)
	  if err == sql.ErrNoRows {
		   return DriftProof{}, ErrProofNotFound
	  }
	   if err != nil {
		   return DriftProof{}, fmt.Errorf("scan proof: %w", err)
	  }
	   p.Grade = DriftGrade(grade)
	  return p, nil
   }

  func scanProofs(rows *sql.Rows) ([]DriftProof, error) {
	  var out []DriftProof
	   for rows.Next() {
		   p, err := scanProof(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, p)
	  }
	  return out, rows.Err()
   }
