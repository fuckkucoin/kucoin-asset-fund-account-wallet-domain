// - 读写 inc_recover_leases、inc_dup_cases、inc_effect_proofs、inc_stuck_generations 四张表。
// - AppendProof 使用 INSERT IGNORE（MySQL）/ INSERT … ON CONFLICT DO NOTHING（PG）保证 append-only。
// - 不依赖 ORM；使用 database/sql 口径接口。
// 不变量：
// - 同 transfer_id 的 CurrentGen 由 UPDATE … SET current_gen = current_gen + 1 原子递增。
// - ActiveLeaseByTransfer 通过 WHERE state = 'acting' LIMIT 1 实现。

// Author: jakson
// Date: 2021.11.22

package stucksettle

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SQLStore Store 接口的 SQL 实现。
type SQLStore struct {
	db *sql.DB
	driver string // "postgres" or "mysql"
}

// NewSQLStore 构造 SQL 存储。driver 取 "postgres" 或 "mysql"。

  func NewSQLStore(db *sql.DB, driver string) *SQLStore {
	  return &SQLStore{db: db,
		   driver: driver}
  }

  func (s *SQLStore) placeholder(n int) string {
	  if s.driver == "postgres" {
		  return fmt.Sprintf("$%d", n)
	   }
	  return "?"
  }

// ── StuckGeneration ──────────────────────────────────────────────────────────

  func (s *SQLStore) GetOrCreateGen(transferID string, now time.Time) (*StuckGeneration, error) {
	  ctx := context.Background()
	  var g StuckGeneration
	   row := s.db.QueryRowContext(ctx,
		   `SELECT stuck_gen_id, transfer_id, current_gen, last_acting_at, terminated_at, created_at, updated_at
		   FROM inc_stuck_generations WHERE transfer_id = `+s.placeholder(1), transferID)
	   var lastActing, terminated sql.NullTime
	   err := row.Scan(&g.StuckGenID, &g.TransferID, &g.CurrentGen, &lastActing, &terminated, &g.CreatedAt, &g.UpdatedAt)
	  if err == sql.ErrNoRows {
		  // 首次插入
		  g = StuckGeneration{
			  StuckGenID: fmt.Sprintf("gen:%s", transferID),
			  TransferID: transferID,
			  CurrentGen: 0,
			   CreatedAt:  now,
			  UpdatedAt:  now,
		   }
		   _, err2 := s.db.ExecContext(ctx,
			  `INSERT INTO inc_stuck_generations(stuck_gen_id, transfer_id, current_gen, created_at, updated_at)
			 VALUES(`+s.placeholder(1)+`,`+s.placeholder(2)+`,0,`+s.placeholder(3)+`,`+s.placeholder(4)+`)`,
			  g.StuckGenID,
			  g.TransferID, now, now)
		  if err2 != nil {
			   // 并发插入竞态：再查一次
			   row2 := s.db.QueryRowContext(ctx,
				   `SELECT stuck_gen_id, transfer_id, current_gen, last_acting_at, terminated_at, created_at, updated_at
				   FROM inc_stuck_generations WHERE transfer_id = `+s.placeholder(1), transferID)
			  _ = row2.Scan(&g.StuckGenID, &g.TransferID, &g.CurrentGen, &lastActing, &terminated, &g.CreatedAt, &g.UpdatedAt)
		  }
		   return &g, nil
	   }
	   if err != nil {
		   return nil, err
	   }
	   if lastActing.Valid {
		   g.LastActingAt = lastActing.Time
	  }
	   if terminated.Valid {
		   g.TerminatedAt = terminated.Time
	   }
	   return &g, nil
  }

   func (s *SQLStore) BumpGen(transferID string, now time.Time) (*StuckGeneration, error) {
	   ctx := context.Background()
	  // GetOrCreate 确保行存在
	   if _, err := s.GetOrCreateGen(transferID, now); err != nil {
		   return nil, err
	   }
	   _, err := s.db.ExecContext(ctx,
		  `UPDATE inc_stuck_generations
		    SET current_gen = current_gen + 1,
		        last_acting_at = `+s.placeholder(1)+`,
		        updated_at = `+s.placeholder(2)+`
		  WHERE transfer_id = `+s.placeholder(3),
		  now, now, transferID)
	  if err != nil {
		  return nil, err
	   }
	   return s.GetGen(transferID)
   }

  func (s *SQLStore) GetGen(transferID string) (*StuckGeneration, error) {
	   ctx := context.Background()
	   var g StuckGeneration
	  var lastActing, terminated sql.NullTime
	   row := s.db.QueryRowContext(ctx,
		  `SELECT stuck_gen_id, transfer_id, current_gen, last_acting_at, terminated_at, created_at, updated_at
		   FROM inc_stuck_generations WHERE transfer_id = `+s.placeholder(1), transferID)
	   if err := row.Scan(&g.StuckGenID, &g.TransferID, &g.CurrentGen, &lastActing, &terminated, &g.CreatedAt, &g.UpdatedAt); err != nil {
		  return nil, err
	   }
	   if lastActing.Valid {
		   g.LastActingAt = lastActing.Time
	  }
	   if terminated.Valid {
		  g.TerminatedAt = terminated.Time
	   }
	  return &g, nil
  }

// ── RecoverLease ─────────────────────────────────────────────────────────────

  func (s *SQLStore) PutLease(l *RecoverLease) error {
	   ctx := context.Background()
	  var acting, recovered, blocked, expired sql.NullTime
	  if !l.ActingAt.IsZero() {
		  acting = sql.NullTime{Time: l.ActingAt, Valid: true}
	   }
	   if !l.RecoveredAt.IsZero() {
		   recovered = sql.NullTime{Time: l.RecoveredAt, Valid: true}
	   }
	  if !l.BlockedAt.IsZero() {
		  blocked = sql.NullTime{Time: l.BlockedAt, Valid: true}
	  }
	  if !l.ExpiredAt.IsZero() {
		  expired = sql.NullTime{Time: l.ExpiredAt, Valid: true}
	  }
	   effectProofID := sql.NullString{String: l.EffectProofID, Valid: l.EffectProofID != ""}
	   blockReason := sql.NullString{String: l.BlockReason, Valid: l.BlockReason != ""}
	  if s.driver == "postgres" {
		   _, err := s.db.ExecContext(ctx, `
			INSERT INTO inc_recover_leases
			  (lease_id, transfer_id, generation, state, holder_key,
			   opened_at, acting_at, recovered_at, blocked_at, expired_at, expires_at,
			   effect_proof_id, block_reason, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (lease_id) DO UPDATE SET
			  state = EXCLUDED.state,
			  acting_at = EXCLUDED.acting_at,
			  recovered_at = EXCLUDED.recovered_at,
			  blocked_at = EXCLUDED.blocked_at,
			  expired_at = EXCLUDED.expired_at,
			  effect_proof_id = EXCLUDED.effect_proof_id,
			  block_reason = EXCLUDED.block_reason,
			  updated_at = EXCLUDED.updated_at`,
			   l.LeaseID, l.TransferID, l.Generation, string(l.State), l.HolderKey,
			   l.OpenedAt, acting, recovered, blocked, expired, l.ExpiresAt,
			  effectProofID,
			  blockReason, l.CreatedAt, l.UpdatedAt)
		   return err
	   }
	  _, err := s.db.ExecContext(ctx, `
		INSERT INTO inc_recover_leases
		  (lease_id, transfer_id, generation, state, holder_key,
		   opened_at, acting_at, recovered_at, blocked_at, expired_at, expires_at,
		   effect_proof_id, block_reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  state = VALUES(state),
		  acting_at = VALUES(acting_at),
		  recovered_at = VALUES(recovered_at),
		  blocked_at = VALUES(blocked_at),
		  expired_at = VALUES(expired_at),
		  effect_proof_id = VALUES(effect_proof_id),
		  block_reason = VALUES(block_reason),
		  updated_at = VALUES(updated_at)`,
		  l.LeaseID, l.TransferID, l.Generation, string(l.State), l.HolderKey,
		  l.OpenedAt,
		   acting, recovered, blocked, expired, l.ExpiresAt,
		   effectProofID, blockReason, l.CreatedAt, l.UpdatedAt)
	   return err
   }

   func (s *SQLStore) GetLease(leaseID string) (*RecoverLease, error) {
	   ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		   `SELECT lease_id, transfer_id, generation, state, holder_key,
		        opened_at, acting_at, recovered_at, blocked_at, expired_at, expires_at,
		        effect_proof_id, block_reason, created_at, updated_at
		   FROM inc_recover_leases WHERE lease_id = `+s.placeholder(1), leaseID)
	  return scanLease(row)
   }

  func (s *SQLStore) ActiveLeaseByTransfer(transferID string) (*RecoverLease, error) {
	   ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		   `SELECT lease_id, transfer_id, generation, state, holder_key,
		        opened_at, acting_at, recovered_at, blocked_at, expired_at, expires_at,
		        effect_proof_id, block_reason, created_at, updated_at
		   FROM inc_recover_leases
		  WHERE transfer_id = `+s.placeholder(1)+` AND state = 'acting'
		  ORDER BY acting_at DESC LIMIT 1`, transferID)
	  l, err := scanLease(row)
	  if err == sql.ErrNoRows {
		  return nil, nil // nolint
	   }
	  return l, err
   }

  func scanLease(row *sql.Row) (*RecoverLease, error) {
	   var l RecoverLease
	   var acting, recovered, blocked, expired sql.NullTime
	  var effectProofID,
		  blockReason sql.NullString
	   var state string
	   err := row.Scan(&l.LeaseID, &l.TransferID, &l.Generation, &state, &l.HolderKey,
		  &l.OpenedAt, &acting, &recovered, &blocked, &expired, &l.ExpiresAt,
		   &effectProofID,
		  &blockReason, &l.CreatedAt, &l.UpdatedAt)
	  if err != nil {
		  return nil, err
	  }
	  l.State = RecoverLeaseState(state)
	  if acting.Valid {
		  l.ActingAt = acting.Time
	  }
	   if recovered.Valid {
		   l.RecoveredAt = recovered.Time
	   }
	  if blocked.Valid {
		  l.BlockedAt = blocked.Time
	  }
	   if expired.Valid {
		  l.ExpiredAt = expired.Time
	   }
	  if effectProofID.Valid {
		   l.EffectProofID = effectProofID.String
	  }
	   if blockReason.Valid {
		   l.BlockReason = blockReason.String
	   }
	  return &l, nil
  }

// ── DupRecoverCase ───────────────────────────────────────────────────────────

   func (s *SQLStore) PutDupCase(c *DupRecoverCase) error {
	  ctx := context.Background()
	  var proven, closed sql.NullTime
	  if !c.ProvenAt.IsZero() {
		  proven = sql.NullTime{Time: c.ProvenAt, Valid: true}
	  }
	   if !c.ClosedAt.IsZero() {
		   closed = sql.NullTime{Time: c.ClosedAt, Valid: true}
	   }
	  proofID := sql.NullString{String: c.DupProofID, Valid: c.DupProofID != ""}
	   confirmedBy := sql.NullString{String: c.ConfirmedBy, Valid: c.ConfirmedBy != ""}
	   coConfirmedBy := sql.NullString{String: c.CoConfirmedBy, Valid: c.CoConfirmedBy != ""}
	   closeNote := sql.NullString{String: c.CloseNote, Valid: c.CloseNote != ""}
	  if s.driver == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
			INSERT INTO inc_dup_cases
			  (dup_case_id, transfer_id, generation, first_lease_id, dup_lease_id,
			   state, dup_proof_id, confirmed_by, co_confirmed_by,
			   detected_at, proven_at, closed_at, close_note, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (dup_case_id) DO UPDATE SET
			  state = EXCLUDED.state,
			  dup_proof_id = EXCLUDED.dup_proof_id,
			  confirmed_by = EXCLUDED.confirmed_by,
			  co_confirmed_by = EXCLUDED.co_confirmed_by,
			  proven_at = EXCLUDED.proven_at,
			  closed_at = EXCLUDED.closed_at,
			  close_note = EXCLUDED.close_note,
			  updated_at = EXCLUDED.updated_at`,
			  c.DupCaseID, c.TransferID, c.Generation, c.FirstLeaseID, c.DupLeaseID,
			   string(c.State), proofID, confirmedBy, coConfirmedBy,
			   c.DetectedAt,
			  proven, closed, closeNote, c.CreatedAt, c.UpdatedAt)
		   return err
	  }
	   _, err := s.db.ExecContext(ctx, `
		INSERT INTO inc_dup_cases
		  (dup_case_id, transfer_id, generation, first_lease_id, dup_lease_id,
		   state, dup_proof_id, confirmed_by, co_confirmed_by,
		   detected_at, proven_at, closed_at, close_note, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  state = VALUES(state),
		  dup_proof_id = VALUES(dup_proof_id),
		  confirmed_by = VALUES(confirmed_by),
		  co_confirmed_by = VALUES(co_confirmed_by),
		  proven_at = VALUES(proven_at),
		  closed_at = VALUES(closed_at),
		  close_note = VALUES(close_note),
		  updated_at = VALUES(updated_at)`,
		   c.DupCaseID, c.TransferID, c.Generation, c.FirstLeaseID, c.DupLeaseID,
		  string(c.State), proofID, confirmedBy, coConfirmedBy,
		  c.DetectedAt, proven, closed, closeNote, c.CreatedAt, c.UpdatedAt)
	  return err
  }

   func (s *SQLStore) GetDupCase(dupCaseID string) (*DupRecoverCase, error) {
	  ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		  `SELECT dup_case_id, transfer_id, generation, first_lease_id, dup_lease_id,
		        state, dup_proof_id, confirmed_by, co_confirmed_by,
		        detected_at, proven_at, closed_at, close_note, created_at, updated_at
		   FROM inc_dup_cases WHERE dup_case_id = `+s.placeholder(1), dupCaseID)
	  return scanDupCase(row)
   }

  func (s *SQLStore) DupCasesByTransfer(transferID string) ([]DupRecoverCase, error) {
	  ctx := context.Background()
	  rows, err := s.db.QueryContext(ctx,
		  `SELECT dup_case_id, transfer_id, generation, first_lease_id, dup_lease_id,
		        state, dup_proof_id, confirmed_by, co_confirmed_by,
		        detected_at, proven_at, closed_at, close_note, created_at, updated_at
		   FROM inc_dup_cases WHERE transfer_id = `+s.placeholder(1)+` ORDER BY detected_at`, transferID)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   var out []DupRecoverCase
	  for rows.Next() {
		   c, err := scanDupCaseRow(rows)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, *c)
	   }
	   return out, rows.Err()
   }

  func scanDupCase(row *sql.Row) (*DupRecoverCase, error) {
	  var c DupRecoverCase
	  var proven, closed sql.NullTime
	  var proofID, confirmedBy, coConfirmedBy, closeNote sql.NullString
	  var state string
	  err := row.Scan(&c.DupCaseID,
		   &c.TransferID, &c.Generation, &c.FirstLeaseID, &c.DupLeaseID,
		  &state, &proofID, &confirmedBy, &coConfirmedBy,
		   &c.DetectedAt, &proven, &closed, &closeNote, &c.CreatedAt, &c.UpdatedAt)
	  if err != nil {
		   return nil, err
	   }
	   c.State = DupRecoverCaseState(state)
	  if proven.Valid {
		  c.ProvenAt = proven.Time
	   }
	   if closed.Valid {
		  c.ClosedAt = closed.Time
	   }
	   if proofID.Valid {
		   c.DupProofID = proofID.String
	  }
	   if confirmedBy.Valid {
		   c.ConfirmedBy = confirmedBy.String
	  }
	   if coConfirmedBy.Valid {
		   c.CoConfirmedBy = coConfirmedBy.String
	  }
	  if closeNote.Valid {
		  c.CloseNote = closeNote.String
	   }
	   return &c, nil
  }

   func scanDupCaseRow(rows *sql.Rows) (*DupRecoverCase, error) {
	   var c DupRecoverCase
	  var proven, closed sql.NullTime
	   var proofID, confirmedBy, coConfirmedBy, closeNote sql.NullString
	   var state string
	   err := rows.Scan(&c.DupCaseID,
		  &c.TransferID, &c.Generation, &c.FirstLeaseID, &c.DupLeaseID,
		   &state, &proofID, &confirmedBy, &coConfirmedBy,
		  &c.DetectedAt,
		  &proven, &closed, &closeNote, &c.CreatedAt, &c.UpdatedAt)
	  if err != nil {
		   return nil, err
	   }
	   c.State = DupRecoverCaseState(state)
	  if proven.Valid {
		  c.ProvenAt = proven.Time
	   }
	   if closed.Valid {
		   c.ClosedAt = closed.Time
	  }
	   if proofID.Valid {
		   c.DupProofID = proofID.String
	   }
	   if confirmedBy.Valid {
		  c.ConfirmedBy = confirmedBy.String
	  }
	   if coConfirmedBy.Valid {
		   c.CoConfirmedBy = coConfirmedBy.String
	   }
	   if closeNote.Valid {
		   c.CloseNote = closeNote.String
	   }
	   return &c, nil
  }

// ── EffectProof (append-only) ─────────────────────────────────────────────────

   func (s *SQLStore) AppendProof(p *EffectProof) error {
	   ctx := context.Background()
	   ledgerRef := sql.NullString{String: p.LedgerRef, Valid: p.LedgerRef != ""}
	   if s.driver == "postgres" {
		  _, err := s.db.ExecContext(ctx, `
			INSERT INTO inc_effect_proofs
			  (effect_proof_id, transfer_id, lease_id, generation,
			   intent, action, ledger_ref, checksum, issued_by, issued_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (effect_proof_id) DO NOTHING`,
			   p.EffectProofID, p.TransferID, p.LeaseID, p.Generation,
			   p.Intent, p.Action, ledgerRef, p.Checksum, p.IssuedBy, p.IssuedAt)
		   return err
	   }
	   _, err := s.db.ExecContext(ctx, `
		INSERT IGNORE INTO inc_effect_proofs
		  (effect_proof_id, transfer_id, lease_id, generation,
		   intent, action, ledger_ref, checksum, issued_by, issued_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		  p.EffectProofID,
		  p.TransferID, p.LeaseID, p.Generation,
		  p.Intent, p.Action, ledgerRef, p.Checksum, p.IssuedBy, p.IssuedAt)
	  return err
   }

  func (s *SQLStore) GetProof(proofID string) (*EffectProof, error) {
	   ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		  `SELECT effect_proof_id, transfer_id, lease_id, generation,
		        intent, action, ledger_ref, checksum, issued_by, issued_at
		   FROM inc_effect_proofs WHERE effect_proof_id = `+s.placeholder(1), proofID)
	   return scanProof(row)
   }

   func (s *SQLStore) ProofByLease(leaseID string) (*EffectProof, error) {
	   ctx := context.Background()
	   row := s.db.QueryRowContext(ctx,
		   `SELECT effect_proof_id, transfer_id, lease_id, generation,
		        intent, action, ledger_ref, checksum, issued_by, issued_at
		   FROM inc_effect_proofs WHERE lease_id = `+s.placeholder(1)+` LIMIT 1`, leaseID)
	   return scanProof(row)
   }

// proofByTransferSQL 通过 transfer_id 查最新一条证明（按 issued_at 降序）。
  func (s *SQLStore) proofByTransferSQL(transferID string) (*EffectProof, error) {
	   ctx := context.Background()
	  row := s.db.QueryRowContext(ctx,
		  `SELECT effect_proof_id, transfer_id, lease_id, generation,
		        intent, action, ledger_ref, checksum, issued_by, issued_at
		   FROM inc_effect_proofs
		  WHERE transfer_id = `+s.placeholder(1)+`
		  ORDER BY issued_at DESC LIMIT 1`, transferID)
	  return scanProof(row)
   }

  func scanProof(row *sql.Row) (*EffectProof, error) {
	   var p EffectProof
	  var ledgerRef sql.NullString
	  err := row.Scan(&p.EffectProofID, &p.TransferID, &p.LeaseID, &p.Generation,
		   &p.Intent, &p.Action, &ledgerRef, &p.Checksum, &p.IssuedBy, &p.IssuedAt)
	  if err != nil {
		  return nil, err
	  }
	   if ledgerRef.Valid {
		  p.LedgerRef = ledgerRef.String
	   }
	   return &p, nil
   }
