package fiatbalgate

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// SQLStore 基于 *sql.DB 的 Store 实现（MySQL/TiDB）。
type SQLStore struct {
	db *sql.DB
	clock func() time.Time
}

// NewSQLStore 构造 SQLStore。

  func NewSQLStore(db *sql.DB) *SQLStore {
	  return &SQLStore{
		   db: db,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (s *SQLStore) WithClock(fn func() time.Time) *SQLStore {
	  if fn != nil {
		  s.clock = fn
	   }
	   return s
  }

   // isDup 判断 MySQL/TiDB 唯一索引冲突（Error 1062）。
   func isDup(err error) bool {
	   if err == nil {
		  return false
	  }
	   msg := err.Error()
	   return strings.Contains(msg, "Duplicate entry") ||
		  strings.Contains(msg, "duplicate key") ||
		   strings.Contains(msg, "1062")
   }

// nullTime 将 *time.Time 转为 interface{}（nil → SQL NULL）。
   func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}

// ── JournalGeneration ─────────────────────────────────────────────────────────

const sqlInsertGen = `
INSERT IGNORE INTO fiat_jnl_gens
  (gen_id, seq, entry_id, kind, asset, bumped_at, synced_at)
VALUES (?,?,?,?,?,?,?)`

   func (s *SQLStore) SaveGeneration(ctx context.Context, g *JournalGeneration) error {
	   if s == nil || g == nil {
		  return ErrUnavailable
	   }
	  _, err := s.db.ExecContext(ctx, sqlInsertGen,
		   g.GenID, g.Seq, g.EntryID, g.Kind, g.Asset, g.BumpedAt, g.SyncedAt)
	   return err
   }

const sqlGetGenBySeq = `
SELECT gen_id, seq, entry_id, kind, asset, bumped_at, synced_at
FROM fiat_jnl_gens WHERE seq=? LIMIT 1`

  func (s *SQLStore) GetGenerationBySeq(ctx context.Context, seq uint64) (*JournalGeneration, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, sqlGetGenBySeq, seq)
	   g := &JournalGeneration{}
	  err := row.Scan(&g.GenID, &g.Seq, &g.EntryID, &g.Kind, &g.Asset, &g.BumpedAt, &g.SyncedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrGenNotFound
	  }
	  return g, err
  }

const sqlMaxGenSeq = `SELECT COALESCE(MAX(seq),0) FROM fiat_jnl_gens`

   func (s *SQLStore) MaxGenSeq(ctx context.Context) (uint64, error) {
	   if s == nil {
		  return 0, ErrUnavailable
	   }
	  var v uint64
	  err := s.db.QueryRowContext(ctx, sqlMaxGenSeq).Scan(&v)
	  return v, err
  }

// ── FiatImbalanceCase ─────────────────────────────────────────────────────────

const sqlInsertCase = `
INSERT INTO fiat_imbalance_cases
  (case_id, asset, scan_gen_seq, detected_entry_id,
   sum_debit, sum_credit, imbalance_kind, state, scanner_id,
   opened_at, blocking_at, cleared_at, cleared_by, clear_note, proof_id,
   created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

   func (s *SQLStore) SaveCase(ctx context.Context, c *FiatImbalanceCase) error {
	   if s == nil || c == nil {
		   return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertCase,
		   c.CaseID, c.Asset, c.ScanGenSeq, c.DetectedEntryID,
		  c.SumDebit, c.SumCredit, string(c.ImbalanceKind), string(c.State), c.ScannerID,
		   c.OpenedAt, nullTime(c.BlockingAt), nullTime(c.ClearedAt), c.ClearedBy, c.ClearNote, c.ProofID,
		  c.CreatedAt, c.UpdatedAt)
	  return err
   }

const sqlGetCase = `
SELECT case_id, asset, scan_gen_seq, detected_entry_id,
       sum_debit, sum_credit, imbalance_kind, state, scanner_id,
       opened_at, blocking_at, cleared_at, cleared_by, clear_note, proof_id,
       created_at, updated_at
FROM fiat_imbalance_cases WHERE case_id=? LIMIT 1`

   func (s *SQLStore) GetCase(ctx context.Context, caseID string) (*FiatImbalanceCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, sqlGetCase, caseID)
	   return scanCase(row)
   }

   func scanCase(row *sql.Row) (*FiatImbalanceCase, error) {
	   c := &FiatImbalanceCase{}
	   var stateStr, kindStr string
	  var blockingAt,
		  clearedAt sql.NullTime
	  err := row.Scan(
		   &c.CaseID, &c.Asset, &c.ScanGenSeq, &c.DetectedEntryID,
		   &c.SumDebit, &c.SumCredit, &kindStr, &stateStr, &c.ScannerID,
		   &c.OpenedAt, &blockingAt, &clearedAt, &c.ClearedBy, &c.ClearNote, &c.ProofID,
		   &c.CreatedAt, &c.UpdatedAt,
	   )
	   if err == sql.ErrNoRows {
		  return nil, ErrCaseNotFound
	  }
	  if err != nil {
		   return nil, err
	  }
	   c.State = CaseState(stateStr)
	  c.ImbalanceKind = ImbalanceKind(kindStr)
	  if blockingAt.Valid {
		   t := blockingAt.Time
		   c.BlockingAt = &t
	  }
	   if clearedAt.Valid {
		  t := clearedAt.Time
		  c.ClearedAt = &t
	   }
	   return c, nil
   }

const sqlUpdateCaseState = `
UPDATE fiat_imbalance_cases
SET state=?, blocking_at=?, cleared_at=?, cleared_by=?, clear_note=?, proof_id=?, updated_at=?
WHERE case_id=? AND state=?`

   func (s *SQLStore) UpdateCaseState(ctx context.Context, caseID string, from, to CaseState, patch *FiatImbalanceCase) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   now := s.clock()
	  var blockingAt,
		   clearedAt interface{}
	  clearedBy, clearNote, proofID := "", "", ""
	   if patch != nil {
		  blockingAt = nullTime(patch.BlockingAt)
		   clearedAt = nullTime(patch.ClearedAt)
		  clearedBy = patch.ClearedBy
		   clearNote = patch.ClearNote
		   proofID = patch.ProofID
	  }
	  res, err := s.db.ExecContext(ctx, sqlUpdateCaseState,
		   string(to), blockingAt, clearedAt, clearedBy, clearNote, proofID, now,
		  caseID, string(from))
	  if err != nil {
		   return err
	   }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	   }
	   return nil
   }

const sqlListOpenBlocking = ` 
SELECT case_id, asset, scan_gen_seq, detected_entry_id,
       sum_debit, sum_credit, imbalance_kind, state, scanner_id,
       opened_at, blocking_at, cleared_at, cleared_by, clear_note, proof_id,
       created_at, updated_at
FROM fiat_imbalance_cases
WHERE state IN ('open','blocking') %s
ORDER BY opened_at DESC
LIMIT ?`

  func (s *SQLStore) ListOpenOrBlockingCases(ctx context.Context, asset string, limit int) ([]*FiatImbalanceCase, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	   var (
		  query string
		   args  []interface{}
	   )
	  if asset != "" {
		  query = strings.Replace(sqlListOpenBlocking, "%s", "AND asset=?", 1)
		  args = []interface{}{asset, limit}
	   } else {
		  query = strings.Replace(sqlListOpenBlocking, "%s", "", 1)
		  args = []interface{}{limit}
	  }
	  return queryCases(ctx, s.db, query, args...)
  }

const sqlListCasesByAsset = `
SELECT case_id, asset, scan_gen_seq, detected_entry_id,
       sum_debit, sum_credit, imbalance_kind, state, scanner_id,
       opened_at, blocking_at, cleared_at, cleared_by, clear_note, proof_id,
       created_at, updated_at
FROM fiat_imbalance_cases
WHERE asset=?
ORDER BY opened_at DESC
LIMIT ?`

   func (s *SQLStore) ListCasesByAsset(ctx context.Context, asset string, limit int) ([]*FiatImbalanceCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	  return queryCases(ctx, s.db, sqlListCasesByAsset, asset, limit)
   }

const sqlListExpiredOpen = `
SELECT case_id, asset, scan_gen_seq, detected_entry_id,
       sum_debit, sum_credit, imbalance_kind, state, scanner_id,
       opened_at, blocking_at, cleared_at, cleared_by, clear_note, proof_id,
       created_at, updated_at
FROM fiat_imbalance_cases
WHERE state='open' AND opened_at <= ?
ORDER BY opened_at ASC
LIMIT ?`

   func (s *SQLStore) ListExpiredOpenCases(ctx context.Context, staleThreshold time.Time, limit int) ([]*FiatImbalanceCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	   return queryCases(ctx, s.db, sqlListExpiredOpen, staleThreshold, limit)
   }

   func queryCases(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]*FiatImbalanceCase, error) {
	  rows, err := db.QueryContext(ctx, query, args...)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  var out []*FiatImbalanceCase
	   for rows.Next() {
		   c := &FiatImbalanceCase{}
		  var stateStr, kindStr string
		  var blockingAt,
			  clearedAt sql.NullTime
		  if err = rows.Scan(
			  &c.CaseID, &c.Asset, &c.ScanGenSeq, &c.DetectedEntryID,
			  &c.SumDebit, &c.SumCredit, &kindStr, &stateStr, &c.ScannerID,
			  &c.OpenedAt, &blockingAt, &clearedAt, &c.ClearedBy, &c.ClearNote, &c.ProofID,
			  &c.CreatedAt, &c.UpdatedAt,
		  ); err != nil {
			   return nil, err
		  }
		  c.State = CaseState(stateStr)
		  c.ImbalanceKind = ImbalanceKind(kindStr)
		  if blockingAt.Valid {
			  t := blockingAt.Time
			   c.BlockingAt = &t
		  }
		  if clearedAt.Valid {
			   t := clearedAt.Time
			   c.ClearedAt = &t
		  }
		  out = append(out, c)
	  }
	   return out, rows.Err()
   }

// ── FiatImbalanceProof ────────────────────────────────────────────────────────

const sqlInsertProof = `
INSERT INTO fiat_imbalance_proofs
  (proof_id, case_id, asset, scan_gen_seq, entries_snapshot, lines_snapshot,
   sum_debit, sum_credit, digest_sha256, state,
   exported_at, verified_at, verified_by, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

   func (s *SQLStore) SaveProof(ctx context.Context, p *FiatImbalanceProof) error {
	  if s == nil || p == nil {
		  return ErrUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, sqlInsertProof,
		   p.ProofID, p.CaseID, p.Asset, p.ScanGenSeq, p.EntriesSnapshot, p.LinesSnapshot,
		   p.SumDebit, p.SumCredit, p.DigestSHA256, string(p.State),
		   nullTime(p.ExportedAt), nullTime(p.VerifiedAt), p.VerifiedBy,
		   p.CreatedAt, p.UpdatedAt)
	  if isDup(err) {
		   return ErrAlreadyProven
	   }
	   return err
  }

const sqlGetProofByCase = `
SELECT proof_id, case_id, asset, scan_gen_seq, entries_snapshot, lines_snapshot,
       sum_debit, sum_credit, digest_sha256, state,
       exported_at, verified_at, verified_by, created_at, updated_at
FROM fiat_imbalance_proofs WHERE case_id=? LIMIT 1`

const sqlGetProofByID = `
SELECT proof_id, case_id, asset, scan_gen_seq, entries_snapshot, lines_snapshot,
       sum_debit, sum_credit, digest_sha256, state,
       exported_at, verified_at, verified_by, created_at, updated_at
FROM fiat_imbalance_proofs WHERE proof_id=? LIMIT 1`

  func (s *SQLStore) GetProofByCase(ctx context.Context, caseID string) (*FiatImbalanceProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   return scanProof(s.db.QueryRowContext(ctx, sqlGetProofByCase, caseID))
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*FiatImbalanceProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   return scanProof(s.db.QueryRowContext(ctx, sqlGetProofByID, proofID))
  }

  func scanProof(row *sql.Row) (*FiatImbalanceProof, error) {
	   p := &FiatImbalanceProof{}
	   var stateStr string
	   var exportedAt, verifiedAt sql.NullTime
	  err := row.Scan(
		  &p.ProofID, &p.CaseID, &p.Asset, &p.ScanGenSeq, &p.EntriesSnapshot, &p.LinesSnapshot,
		  &p.SumDebit,
		   &p.SumCredit, &p.DigestSHA256, &stateStr,
		  &exportedAt, &verifiedAt, &p.VerifiedBy, &p.CreatedAt, &p.UpdatedAt,
	  )
	   if err == sql.ErrNoRows {
		   return nil, ErrProofNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	   p.State = ProofState(stateStr)
	   if exportedAt.Valid {
		  t := exportedAt.Time
		  p.ExportedAt = &t
	  }
	   if verifiedAt.Valid {
		   t := verifiedAt.Time
		   p.VerifiedAt = &t
	   }
	   return p, nil
  }

const sqlUpdateProofState = ` 
UPDATE fiat_imbalance_proofs
SET state=?, exported_at=?, verified_at=?, verified_by=?, updated_at=?
WHERE proof_id=? AND state=?`

  func (s *SQLStore) UpdateProofState(ctx context.Context, proofID string, from, to ProofState, patch *FiatImbalanceProof) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  now := s.clock()
	   var exportedAt, verifiedAt interface{}
	   verifiedBy := ""
	  if patch != nil {
		   exportedAt = nullTime(patch.ExportedAt)
		  verifiedAt = nullTime(patch.VerifiedAt)
		   verifiedBy = patch.VerifiedBy
	   }
	  res, err := s.db.ExecContext(ctx, sqlUpdateProofState,
		   string(to), exportedAt, verifiedAt, verifiedBy, now,
		  proofID, string(from))
	   if err != nil {
		  return err
	   }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		  return ErrInvalidTransition
	   }
	  return nil
   }

// ── ClearApproval ─────────────────────────────────────────────────────────────

const sqlInsertApproval = `
INSERT INTO fiat_clear_approvals
  (approval_id, case_id, step, actor, note, created_at)
VALUES (?,?,?,?,?,?)`

   func (s *SQLStore) SaveApproval(ctx context.Context, a *ClearApproval) error {
	  if s == nil || a == nil {
		   return ErrUnavailable
	   }
	   _, err := s.db.ExecContext(ctx, sqlInsertApproval,
		  a.ApprovalID, a.CaseID, string(a.Step), a.Actor, a.Note, a.CreatedAt)
	   return err
  }

const sqlListApprovals = `
SELECT approval_id, case_id, step, actor, note, created_at
FROM fiat_clear_approvals
WHERE case_id=? ORDER BY created_at ASC`

  func (s *SQLStore) ListApprovalsByCase(ctx context.Context, caseID string) ([]*ClearApproval, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   rows, err := s.db.QueryContext(ctx, sqlListApprovals, caseID)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   var out []*ClearApproval
	   for rows.Next() {
		   a := &ClearApproval{}
		   var stepStr string
		   if err = rows.Scan(&a.ApprovalID, &a.CaseID, &stepStr, &a.Actor, &a.Note, &a.CreatedAt); err != nil {
			   return nil, err
		   }
		   a.Step = ClearStep(stepStr)
		  out = append(out, a)
	   }
	  return out, rows.Err()
   }

// ── 辅助 ──────────────────────────────────────────────────────────────────────

const sqlHasActiveBlocking = ` 
SELECT COUNT(1) FROM fiat_imbalance_cases
WHERE asset=? AND state IN ('open','blocking') LIMIT 1`

  func (s *SQLStore) HasActiveBlockingForAsset(ctx context.Context, asset string) (bool, error) {
	  if s == nil {
		  return false, ErrUnavailable
	   }
	   var cnt int
	  err := s.db.QueryRowContext(ctx,
		  sqlHasActiveBlocking, asset).Scan(&cnt)
	   return cnt > 0, err
   }
