package utamodegate

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// SQLStore 是 Store 的 SQL 持久化实现。

type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 构造 SQLStore。

   func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   return &SQLStore{db: db}
   }

// ── ModeGeneration ────────────────────────────────────────────────────────────

  func (s *SQLStore) CurrentGeneration(ctx context.Context, userID string) (int64, error) {
	  var gen int64
	  err := s.db.DB.QueryRowContext(ctx,
		  `SELECT generation FROM uta_mode_generations WHERE user_id=?`, userID,
	  ).Scan(&gen)
	   if err == sql.ErrNoRows {
		   return 0, nil
	  }
	  return gen, err
   }

  func (s *SQLStore) BumpGeneration(ctx context.Context, userID string, expected int64, newMode MarginMode) (int64, error) {
	  if expected == 0 {
		   // 首次：INSERT ON CONFLICT DO NOTHING，然后重试 UPDATE
		  id := uuid.NewString()
		   _, _ = s.db.DB.ExecContext(ctx,
			   `INSERT INTO uta_mode_generations (id, user_id, generation, current_mode, updated_at)
			 VALUES (?, ?, 1, ?, NOW(3))
			 ON DUPLICATE KEY UPDATE id=id`,
			  id, userID, string(newMode),
		   )
		   // 读取当前值（不论 INSERT 是否成功）
		   var gen int64
		   if err := s.db.DB.QueryRowContext(ctx,
			   `SELECT generation FROM uta_mode_generations WHERE user_id=?`, userID,
		  ).Scan(&gen); err != nil {
			   return 0, err
		  }
		   if gen != 1 {
			  // 对方已写入，world gen=1；我们期望 0，当前已是 1 → StaleGen
			   return 0, ErrStaleGen
		  }
		   return 1, nil
	  }
	   res, err := s.db.DB.ExecContext(ctx,
		  `UPDATE uta_mode_generations
		 SET generation=generation+1, current_mode=?, updated_at=NOW(3)
		 WHERE user_id=? AND generation=?`,
		   string(newMode), userID, expected,
	  )
	  if err != nil {
		   return 0, err
	  }
	  rows, err := res.RowsAffected()
	   if err != nil {
		  return 0, err
	   }
	  if rows == 0 {
		  return 0, ErrStaleGen
	  }
	   return expected + 1, nil
   }

  func (s *SQLStore) GetGeneration(ctx context.Context, userID string) (*ModeGeneration, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, user_id, generation, current_mode, updated_at
		 FROM uta_mode_generations WHERE user_id=?`, userID,
	   )
	  var mg ModeGeneration
	   var modeStr string
	   var updatedAt time.Time
	  if err := row.Scan(&mg.ID, &mg.UserID, &mg.Generation, &modeStr, &updatedAt); err != nil {
		  if err == sql.ErrNoRows {
			  return &ModeGeneration{UserID: userID, Generation: 0, CurrentMode: MarginModeUnified}, nil
		   }
		  return nil, err
	   }
	  mg.CurrentMode = MarginMode(modeStr)
	  mg.UpdatedAt = updatedAt
	   return &mg, nil
   }

// ── ModeLease ─────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveLease(ctx context.Context, l *ModeLease) error {
	   _, err := s.db.DB.ExecContext(ctx,
		   `INSERT INTO uta_mode_leases
		   (id, user_id, from_mode, to_mode, generation, trigger_kind, trigger_ref, status, note, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   status=VALUES(status), generation=VALUES(generation), note=VALUES(note), updated_at=VALUES(updated_at)`,
		   l.ID, l.UserID, string(l.FromMode), string(l.ToMode), l.Generation,
		  l.TriggerKind, l.TriggerRef, string(l.Status), l.Note,
		  l.CreatedAt, l.UpdatedAt,
	   )
	   return err
   }

   func (s *SQLStore) GetLease(ctx context.Context, leaseID string) (*ModeLease, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, user_id, from_mode, to_mode, generation, trigger_kind, trigger_ref, status, note, created_at, updated_at
		 FROM uta_mode_leases WHERE id=?`, leaseID,
	  )
	   return scanLease(row)
   }

  func (s *SQLStore) GetLeaseByTriggerRef(ctx context.Context, userID, triggerRef string) (*ModeLease, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, user_id, from_mode, to_mode, generation, trigger_kind, trigger_ref, status, note, created_at, updated_at
		 FROM uta_mode_leases WHERE user_id=? AND trigger_ref=? AND status != 'switched'
		 ORDER BY created_at DESC LIMIT 1`, userID, triggerRef,
	  )
	   l, err := scanLease(row)
	  if err == ErrLeaseNotFound {
		  return nil, nil
	  }
	   return l, err
  }

  func (s *SQLStore) ListActiveLeasesByUser(ctx context.Context, userID string) ([]*ModeLease, error) {
	   rows, err := s.db.DB.QueryContext(ctx,
		   `SELECT id, user_id, from_mode, to_mode, generation, trigger_kind, trigger_ref, status, note, created_at, updated_at
		 FROM uta_mode_leases WHERE user_id=? AND status != 'switched'
		 ORDER BY created_at DESC`, userID,
	  )
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   return scanLeases(rows)
   }

   func (s *SQLStore) ListLeasesByStatus(ctx context.Context, status LeaseStatus, limit int) ([]*ModeLease, error) {
	   rows, err := s.db.DB.QueryContext(ctx,
		   `SELECT id, user_id, from_mode, to_mode, generation, trigger_kind, trigger_ref, status, note, created_at, updated_at
		 FROM uta_mode_leases WHERE status=? ORDER BY created_at ASC LIMIT ?`,
		   string(status), limit,
	  )
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanLeases(rows)
  }

   func scanLease(row *sql.Row) (*ModeLease, error) {
	  var l ModeLease
	   var fromMode, toMode, status string
	   err := row.Scan(&l.ID, &l.UserID, &fromMode, &toMode, &l.Generation,
		   &l.TriggerKind, &l.TriggerRef, &status, &l.Note, &l.CreatedAt, &l.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrLeaseNotFound
	   }
	  if err != nil {
		  return nil, err
	   }
	   l.FromMode = MarginMode(fromMode)
	  l.ToMode = MarginMode(toMode)
	   l.Status = LeaseStatus(status)
	  return &l, nil
   }

   func scanLeases(rows *sql.Rows) ([]*ModeLease, error) {
	  var out []*ModeLease
	  for rows.Next() {
		  var l ModeLease
		  var fromMode, toMode, status string
		   if err := rows.Scan(&l.ID, &l.UserID, &fromMode, &toMode, &l.Generation,
			  &l.TriggerKind, &l.TriggerRef, &status, &l.Note, &l.CreatedAt, &l.UpdatedAt); err != nil {
			  return nil, err
		   }
		  l.FromMode = MarginMode(fromMode)
		   l.ToMode = MarginMode(toMode)
		  l.Status = LeaseStatus(status)
		  out = append(out, &l)
	  }
	   return out, rows.Err()
  }

// ── LiabilityBind ─────────────────────────────────────────────────────────────

  func (s *SQLStore) SaveBind(ctx context.Context, b *LiabilityBind) error {
	   _, err := s.db.DB.ExecContext(ctx,
		   `INSERT INTO uta_liability_binds
		   (id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   status=VALUES(status), updated_at=VALUES(updated_at)`,
		   b.ID, b.UserID, string(b.BindKind), b.Currency, string(b.MarginMode),
		   b.Amount,
		   b.Generation, b.RefID, string(b.Status),
		   b.ExpiresAt, b.CreatedAt, b.UpdatedAt,
	  )
	  return err
   }

  func (s *SQLStore) GetBind(ctx context.Context, bindID string) (*LiabilityBind, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at
		 FROM uta_liability_binds WHERE id=?`, bindID,
	   )
	  return scanBind(row)
  }

  func (s *SQLStore) GetBindByRef(ctx context.Context, userID, refID string) (*LiabilityBind, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at 
		 FROM uta_liability_binds WHERE user_id=? AND ref_id=? ORDER BY created_at DESC LIMIT 1`,
		   userID, refID,
	   )
	   return scanBind(row)
  }

   func (s *SQLStore) ListOpenBindsByUser(ctx context.Context, userID string) ([]*LiabilityBind, error) {
	   var q string
	  var args []any
	  if userID == "" {
		   q = `SELECT id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at
		     FROM uta_liability_binds WHERE status='open' ORDER BY created_at ASC`
	  } else {
		  q = `SELECT id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at
		     FROM uta_liability_binds WHERE user_id=? AND status='open' ORDER BY created_at ASC`
		  args = []any{userID}
	   }
	   rows, err := s.db.DB.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanBinds(rows)
   }

   func (s *SQLStore) ListExpiredOpenBinds(ctx context.Context, before time.Time, limit int) ([]*LiabilityBind, error) {
	  rows, err := s.db.DB.QueryContext(ctx,
		   `SELECT id, user_id, bind_kind, currency, margin_mode, amount, generation, ref_id, status, expires_at, created_at, updated_at
		 FROM uta_liability_binds WHERE status='open' AND expires_at < ? ORDER BY expires_at ASC LIMIT ?`,
		  before, limit,
	  )
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   return scanBinds(rows)
   }

  func scanBind(row *sql.Row) (*LiabilityBind, error) {
	   var b LiabilityBind
	  var bindKind, marginMode, status string
	  err := row.Scan(&b.ID, &b.UserID, &bindKind, &b.Currency, &marginMode,
		  &b.Amount, &b.Generation, &b.RefID, &status,
		   &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return nil, ErrBindNotFound
	  }
	  if err != nil {
		  return nil, err
	  }
	   b.BindKind = BindKind(bindKind)
	  b.MarginMode = MarginMode(marginMode)
	  b.Status = BindStatus(status)
	   return &b, nil
   }

   func scanBinds(rows *sql.Rows) ([]*LiabilityBind, error) {
	   var out []*LiabilityBind
	   for rows.Next() {
		  var b LiabilityBind
		   var bindKind, marginMode, status string
		  if err := rows.Scan(&b.ID,
			   &b.UserID, &bindKind, &b.Currency, &marginMode,
			  &b.Amount, &b.Generation, &b.RefID, &status,
			  &b.ExpiresAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			   return nil, err
		   }
		   b.BindKind = BindKind(bindKind)
		   b.MarginMode = MarginMode(marginMode)
		  b.Status = BindStatus(status)
		  out = append(out, &b)
	  }
	   return out, rows.Err()
   }

// ── ModeDriftCase ─────────────────────────────────────────────────────────────

  func (s *SQLStore) SaveDriftCase(ctx context.Context, dc *ModeDriftCase) error {
	  _, err := s.db.DB.ExecContext(ctx,
		   `INSERT INTO uta_mode_drift_cases 
		   (id, user_id, asset, kind, old_mode, new_mode, old_generation, new_generation,
		    liability_amount, bind_id, lease_id, proof_id, status,
		    confirm1_by, confirm2_by, confirm1_at, confirm2_at, note, detected_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   status=VALUES(status), proof_id=VALUES(proof_id),
		   confirm1_by=VALUES(confirm1_by), confirm2_by=VALUES(confirm2_by),
		   confirm1_at=VALUES(confirm1_at), confirm2_at=VALUES(confirm2_at),
		   updated_at=VALUES(updated_at)`,
		  dc.ID,
		  dc.UserID, dc.Asset, string(dc.Kind),
		   string(dc.OldMode), string(dc.NewMode), dc.OldGeneration, dc.NewGeneration,
		   dc.LiabilityAmount, dc.BindID, dc.LeaseID, dc.ProofID, string(dc.Status),
		  dc.Confirm1By, dc.Confirm2By, dc.Confirm1At, dc.Confirm2At,
		  dc.Note, dc.DetectedAt, dc.UpdatedAt,
	   )
	  return err
   }

   func (s *SQLStore) GetDriftCase(ctx context.Context, caseID string) (*ModeDriftCase, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, user_id, asset, kind, old_mode, new_mode, old_generation, new_generation, 
		        liability_amount, bind_id, lease_id, proof_id, status,
		        confirm1_by, confirm2_by, confirm1_at, confirm2_at, note, detected_at, updated_at
		 FROM uta_mode_drift_cases WHERE id=?`, caseID,
	  )
	  return scanDriftCase(row)
  }

   func (s *SQLStore) FindOpenDriftCase(ctx context.Context, userID, asset string, kind DriftKind) (*ModeDriftCase, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		   `SELECT id, user_id, asset, kind, old_mode, new_mode, old_generation, new_generation,
		        liability_amount, bind_id, lease_id, proof_id, status,
		        confirm1_by, confirm2_by, confirm1_at, confirm2_at, note, detected_at, updated_at
		 FROM uta_mode_drift_cases WHERE user_id=? AND asset=? AND kind=? AND status='open'
		 ORDER BY detected_at DESC LIMIT 1`,
		  userID, strings.ToUpper(asset), string(kind),
	  )
	   dc, err := scanDriftCase(row)
	  if err == ErrDriftCaseNotFound {
		   return nil, nil
	  }
	   return dc, err
   }

  func (s *SQLStore) ListBlockingDriftCases(ctx context.Context, userID string) ([]*ModeDriftCase, error) {
	   rows,
		   err := s.db.DB.QueryContext(ctx,
		   `SELECT id, user_id, asset, kind, old_mode, new_mode, old_generation, new_generation,
		        liability_amount, bind_id, lease_id, proof_id, status,
		        confirm1_by, confirm2_by, confirm1_at, confirm2_at, note, detected_at, updated_at
		 FROM uta_mode_drift_cases WHERE user_id=? AND status IN ('open','proven')
		 ORDER BY detected_at DESC`, userID,
	  )
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	  return scanDriftCases(rows)
   }

  func (s *SQLStore) ListOpenDriftCases(ctx context.Context, limit int) ([]*ModeDriftCase, error) {
	   rows, err := s.db.DB.QueryContext(ctx,
		   `SELECT id, user_id, asset, kind, old_mode, new_mode, old_generation, new_generation,
		        liability_amount, bind_id, lease_id, proof_id, status,
		        confirm1_by, confirm2_by, confirm1_at, confirm2_at, note, detected_at, updated_at
		 FROM uta_mode_drift_cases WHERE status='open' ORDER BY detected_at ASC LIMIT ?`, limit,
	   )
	   if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	   return scanDriftCases(rows)
  }

  func scanDriftCase(row *sql.Row) (*ModeDriftCase, error) {
	  var dc ModeDriftCase
	  var kind, oldMode, newMode, status string
	   err := row.Scan(
		  &dc.ID,
		  &dc.UserID, &dc.Asset, &kind,
		  &oldMode, &newMode, &dc.OldGeneration, &dc.NewGeneration,
		   &dc.LiabilityAmount, &dc.BindID, &dc.LeaseID, &dc.ProofID, &status,
		  &dc.Confirm1By, &dc.Confirm2By, &dc.Confirm1At, &dc.Confirm2At,
		   &dc.Note, &dc.DetectedAt, &dc.UpdatedAt,
	   )
	  if err == sql.ErrNoRows {
		   return nil, ErrDriftCaseNotFound
	  }
	   if err != nil {
		  return nil, err
	  }
	   dc.Kind = DriftKind(kind)
	  dc.OldMode = MarginMode(oldMode)
	   dc.NewMode = MarginMode(newMode)
	   dc.Status = DriftCaseStatus(status)
	  return &dc, nil
  }

   func scanDriftCases(rows *sql.Rows) ([]*ModeDriftCase, error) {
	  var out []*ModeDriftCase
	   for rows.Next() {
		   var dc ModeDriftCase
		   var kind, oldMode, newMode, status string
		   if err := rows.Scan(
			  &dc.ID, &dc.UserID, &dc.Asset, &kind,
			  &oldMode, &newMode, &dc.OldGeneration, &dc.NewGeneration,
			  &dc.LiabilityAmount,
			  &dc.BindID, &dc.LeaseID, &dc.ProofID, &status,
			  &dc.Confirm1By, &dc.Confirm2By, &dc.Confirm1At, &dc.Confirm2At,
			  &dc.Note, &dc.DetectedAt, &dc.UpdatedAt,
		  ); err != nil {
			   return nil, err
		  }
		  dc.Kind = DriftKind(kind)
		  dc.OldMode = MarginMode(oldMode)
		   dc.NewMode = MarginMode(newMode)
		  dc.Status = DriftCaseStatus(status)
		  out = append(out, &dc)
	  }
	   return out, rows.Err()
   }

// ── ModeDriftProof ────────────────────────────────────────────────────────────

  func (s *SQLStore) SaveProof(ctx context.Context, p *ModeDriftProof) error {
	   _, err := s.db.DB.ExecContext(ctx,
		  `INSERT INTO uta_mode_drift_proofs
		   (id, drift_case_id, old_key_snapshot, new_agg_snapshot, delta_digest, built_by, verified_at, created_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE delta_digest=VALUES(delta_digest), verified_at=VALUES(verified_at)`,
		  p.ID, p.DriftCaseID, p.OldKeySnapshot, p.NewAggSnapshot,
		  p.DeltaDigest, p.BuiltBy, p.VerifiedAt, p.CreatedAt,
	   )
	  return err
   }

  func (s *SQLStore) GetProof(ctx context.Context, proofID string) (*ModeDriftProof, error) {
	  row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, drift_case_id, old_key_snapshot, new_agg_snapshot, delta_digest, built_by, verified_at, created_at
		 FROM uta_mode_drift_proofs WHERE id=?`, proofID,
	   )
	   return scanProof(row)
  }

   func (s *SQLStore) GetProofByCaseID(ctx context.Context, caseID string) (*ModeDriftProof, error) {
	   row := s.db.DB.QueryRowContext(ctx,
		  `SELECT id, drift_case_id, old_key_snapshot, new_agg_snapshot, delta_digest, built_by, verified_at, created_at 
		 FROM uta_mode_drift_proofs WHERE drift_case_id=? LIMIT 1`, caseID,
	   )
	   return scanProof(row)
  }

   func scanProof(row *sql.Row) (*ModeDriftProof, error) {
	  var p ModeDriftProof
	  err := row.Scan(&p.ID, &p.DriftCaseID, &p.OldKeySnapshot, &p.NewAggSnapshot,
		   &p.DeltaDigest, &p.BuiltBy, &p.VerifiedAt, &p.CreatedAt)
	  if err == sql.ErrNoRows {
		  return nil, ErrProofNotFound
	   }
	   return &p, err
  }
