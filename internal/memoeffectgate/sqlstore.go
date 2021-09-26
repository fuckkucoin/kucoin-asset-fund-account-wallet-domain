// fundsurf_memo_effects / fundsurf_memo_half_proofs / fundsurf_ticket_credit_fences SQL。

// Author: Vance
// Date: 2021.09.02

package memoeffectgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type SQLStore struct {
	db *sql.DB
}

   func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

const schemaSQL = `
CREATE TABLE IF NOT EXISTS fundsurf_memo_effects (
  id VARCHAR(64) PRIMARY KEY,
  ticket_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  chain VARCHAR(32) NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  output_index INT NOT NULL DEFAULT 0,
  output_key VARCHAR(200) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  state VARCHAR(24) NOT NULL,
  holder VARCHAR(32) NOT NULL,
  fence_id VARCHAR(64) NULL,
  proof_id VARCHAR(64) NULL,
  idem VARCHAR(80) NULL,
  ttl_ms BIGINT NOT NULL,
  bound_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  credited_at DATETIME NULL,
  rejected_at DATETIME NULL,
  expired_at DATETIME NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_memo_eff_ticket (ticket_id, updated_at),
  INDEX idx_memo_eff_output (output_key),
  INDEX idx_memo_eff_state (state, expires_at),
  INDEX idx_memo_eff_idem (idem)
);
CREATE TABLE IF NOT EXISTS fundsurf_memo_half_proofs (
  id VARCHAR(64) PRIMARY KEY,
  effect_id VARCHAR(64) NOT NULL,
  ticket_id VARCHAR(64) NOT NULL,
  fence_id VARCHAR(64) NULL,
  kind VARCHAR(40) NOT NULL,
  state VARCHAR(16) NOT NULL,
  blocked TINYINT NOT NULL,
  sealed TINYINT NOT NULL,
  silent_keep TINYINT NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_memo_half_ticket (ticket_id, created_at),
  INDEX idx_memo_half_state (state, created_at),
  INDEX idx_memo_half_kind (kind)
);
CREATE TABLE IF NOT EXISTS fundsurf_ticket_credit_fences (
  id VARCHAR(64) PRIMARY KEY,
  effect_id VARCHAR(64) NOT NULL,
  ticket_id VARCHAR(64) NOT NULL,
  phase VARCHAR(24) NOT NULL,
  observe_ok TINYINT NOT NULL,
  apply_ok TINYINT NOT NULL,
  ticket_ok TINYINT NOT NULL,
  half_cut TINYINT NOT NULL,
  credit_count INT NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_memo_fence_ticket (ticket_id),
  INDEX idx_memo_fence_half (half_cut, updated_at)
);
CREATE TABLE IF NOT EXISTS fundsurf_memo_effect_audit (
  id VARCHAR(64) PRIMARY KEY,
  effect_id VARCHAR(64) NOT NULL,
  event VARCHAR(24) NOT NULL,
  from_state VARCHAR(24) NOT NULL,
  to_state VARCHAR(24) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  body JSON NOT NULL,
  at DATETIME NOT NULL,
  INDEX idx_memo_eff_audit (effect_id, at)
);
`

  func SchemaSQL() string { return schemaSQL }

   func (s *SQLStore) EnsureSchema(ctx context.Context) error {
	   if s == nil || s.db == nil {
		   return ErrStoreUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, schemaSQL)
	   return err
  }

   func (s *SQLStore) SaveEffect(ctx context.Context, e MemoEffect) (MemoEffect, error) {
	  if s == nil || s.db == nil {
		   return MemoEffect{}, ErrStoreUnavailable
	   }
	   body, err := json.Marshal(e)
	  if err != nil {
		  return MemoEffect{}, err
	  }
	   _, err = s.db.ExecContext(ctx, ` 
INSERT INTO fundsurf_memo_effects
(id, ticket_id, user_id, chain, tx_hash, output_index, output_key, asset, state, holder, fence_id, proof_id, idem, ttl_ms, bound_at, expires_at, credited_at, rejected_at, expired_at, body, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 ticket_id=VALUES(ticket_id), user_id=VALUES(user_id), chain=VALUES(chain), tx_hash=VALUES(tx_hash),
 output_index=VALUES(output_index), output_key=VALUES(output_key), asset=VALUES(asset), state=VALUES(state),
 holder=VALUES(holder), fence_id=VALUES(fence_id), proof_id=VALUES(proof_id), idem=VALUES(idem),
 ttl_ms=VALUES(ttl_ms), bound_at=VALUES(bound_at), expires_at=VALUES(expires_at), credited_at=VALUES(credited_at),
 rejected_at=VALUES(rejected_at), expired_at=VALUES(expired_at), body=VALUES(body), updated_at=VALUES(updated_at)`,
		  e.ID, e.TicketID, e.UserID, e.Chain, e.TxHash, e.OutputIndex, e.OutputKey, e.Asset, string(e.State), e.Holder,
		  nullStr(e.FenceID), nullStr(e.ProofID), nullStr(e.Idem), int64(e.TTL.Duration()/time.Millisecond),
		   e.BoundAt, e.ExpiresAt, nullTime(e.CreditedAt), nullTime(e.RejectedAt), nullTime(e.ExpiredAt),
		  body, e.CreatedAt, e.UpdatedAt)
	   if err != nil {
		  return MemoEffect{}, err
	  }
	  return e, nil
  }

  func (s *SQLStore) scanEffect(row interface{ Scan(...any) error }) (MemoEffect, error) {
	   var body []byte
	  var e MemoEffect
	  var state, holder string
	   if err := row.Scan(&e.ID, &body, &state, &holder); err != nil {
		  if err == sql.ErrNoRows {
			   return MemoEffect{}, ErrEffectNotFound
		  }
		  return MemoEffect{}, err
	   }
	   if err := json.Unmarshal(body, &e); err != nil {
		   return MemoEffect{}, err
	   }
	   e.State = EffectState(state)
	  e.Holder = holder
	   return e, nil
   }

   func (s *SQLStore) GetEffect(ctx context.Context, id string) (MemoEffect, error) {
	   if s == nil || s.db == nil {
		   return MemoEffect{}, ErrStoreUnavailable
	   }
	  row := s.db.QueryRowContext(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE id=?`, strings.TrimSpace(id))
	   return s.scanEffect(row)
  }

  func (s *SQLStore) GetEffectByTicket(ctx context.Context, ticketID string) (MemoEffect, error) {
	   if s == nil || s.db == nil {
		   return MemoEffect{}, ErrStoreUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE ticket_id=? ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(ticketID))
	   return s.scanEffect(row)
   }

  func (s *SQLStore) GetEffectByIdem(ctx context.Context, idem string) (MemoEffect, error) {
	  if s == nil || s.db == nil {
		  return MemoEffect{}, ErrStoreUnavailable
	  }
	   row := s.db.QueryRowContext(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE idem=? LIMIT 1`, strings.TrimSpace(idem))
	  return s.scanEffect(row)
   }

   func (s *SQLStore) GetEffectByOutput(ctx context.Context, outputKey string) (MemoEffect, error) {
	   if s == nil || s.db == nil {
		  return MemoEffect{}, ErrStoreUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE output_key=? ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(outputKey))
	  return s.scanEffect(row)
  }

   func (s *SQLStore) listEffectsQuery(ctx context.Context, q string, args ...any) ([]MemoEffect, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]MemoEffect, 0)
	   for rows.Next() {
		   e, err := s.scanEffect(rows)
		  if err != nil {
			   return nil, err
		   }
		   out = append(out, e)
	  }
	  return out, rows.Err()
  }

  func (s *SQLStore) ListEffectsByState(ctx context.Context, state EffectState, limit int) ([]MemoEffect, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	   return s.listEffectsQuery(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE state=? ORDER BY updated_at DESC LIMIT ?`, string(state), limit)
   }

  func (s *SQLStore) ListExpiredBound(ctx context.Context, before time.Time, limit int) ([]MemoEffect, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	  }
	  if limit <= 0 {
		  limit = 100
	  }
	  return s.listEffectsQuery(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects WHERE state=? AND expires_at<=? ORDER BY expires_at ASC LIMIT ?`, string(EffectBound), before, limit)
  }

   func (s *SQLStore) ListEffects(ctx context.Context, limit int) ([]MemoEffect, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 {
		  limit = 100
	   }
	  return s.listEffectsQuery(ctx, `SELECT id, body, state, holder FROM fundsurf_memo_effects ORDER BY created_at DESC LIMIT ?`, limit)
  }

  func (s *SQLStore) CountEffects(ctx context.Context, state EffectState) (int, error) {
	   if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	   }
	   var n int
	  var err error
	  if state == "" {
		  err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fundsurf_memo_effects`).Scan(&n)
	  } else {
		  err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fundsurf_memo_effects WHERE state=?`, string(state)).Scan(&n)
	  }
	   return n, err
  }

  func (s *SQLStore) SaveFence(ctx context.Context, f TicketCreditFence) (TicketCreditFence, error) {
	   if s == nil || s.db == nil {
		  return TicketCreditFence{}, ErrStoreUnavailable
	  }
	  body, err := json.Marshal(f)
	   if err != nil {
		  return TicketCreditFence{}, err
	   }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO fundsurf_ticket_credit_fences
(id, effect_id, ticket_id, phase, observe_ok, apply_ok, ticket_ok, half_cut, credit_count, body, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 effect_id=VALUES(effect_id), ticket_id=VALUES(ticket_id), phase=VALUES(phase),
 observe_ok=VALUES(observe_ok), apply_ok=VALUES(apply_ok), ticket_ok=VALUES(ticket_ok),
 half_cut=VALUES(half_cut), credit_count=VALUES(credit_count), body=VALUES(body), updated_at=VALUES(updated_at)`,
		  f.ID, f.EffectID, f.TicketID, string(f.Phase), b2i(f.ObserveOK), b2i(f.ApplyOK), b2i(f.TicketOK), b2i(f.HalfCut), f.CreditCount, body, f.CreatedAt, f.UpdatedAt)
	  if err != nil {
		  return TicketCreditFence{}, err
	  }
	  return f, nil
  }

   func (s *SQLStore) scanFence(row interface{ Scan(...any) error }) (TicketCreditFence, error) {
	  var body []byte
	  var f TicketCreditFence
	   if err := row.Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			  return TicketCreditFence{}, ErrFenceUnavailable
		  }
		   return TicketCreditFence{}, err
	  }
	  if err := json.Unmarshal(body, &f); err != nil {
		   return TicketCreditFence{}, err
	  }
	  return f, nil
   }

   func (s *SQLStore) GetFence(ctx context.Context, id string) (TicketCreditFence, error) {
	   if s == nil || s.db == nil {
		  return TicketCreditFence{}, ErrStoreUnavailable
	  }
	  row := s.db.QueryRowContext(ctx, `SELECT body FROM fundsurf_ticket_credit_fences WHERE id=?`, strings.TrimSpace(id))
	   return s.scanFence(row)
  }

   func (s *SQLStore) GetFenceByTicket(ctx context.Context, ticketID string) (TicketCreditFence, error) {
	  if s == nil || s.db == nil {
		  return TicketCreditFence{}, ErrStoreUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `SELECT body FROM fundsurf_ticket_credit_fences WHERE ticket_id=? ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(ticketID))
	  return s.scanFence(row)
   }

   func (s *SQLStore) ListHalfCutFences(ctx context.Context, limit int) ([]TicketCreditFence, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		  limit = 80
	   }
	  rows, err := s.db.QueryContext(ctx, `SELECT body FROM fundsurf_ticket_credit_fences WHERE half_cut=1 AND phase<>? ORDER BY updated_at ASC LIMIT ?`, string(FenceSealed), limit)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]TicketCreditFence, 0)
	   for rows.Next() {
		   f, err := s.scanFence(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, f)
	   }
	   return out, rows.Err()
   }

  func (s *SQLStore) ListFences(ctx context.Context, limit int) ([]TicketCreditFence, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 {
		   limit = 80
	   }
	  rows, err := s.db.QueryContext(ctx, `SELECT body FROM fundsurf_ticket_credit_fences ORDER BY created_at DESC LIMIT ?`, limit)
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]TicketCreditFence, 0)
	   for rows.Next() {
		   f, err := s.scanFence(rows)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, f)
	   }
	  return out, rows.Err()
  }

  func (s *SQLStore) SaveProof(ctx context.Context, p MemoHalfCreditProof) (MemoHalfCreditProof, error) {
	   if s == nil || s.db == nil {
		  return MemoHalfCreditProof{}, ErrStoreUnavailable
	  }
	  body, err := json.Marshal(p)
	  if err != nil {
		  return MemoHalfCreditProof{}, err
	   }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO fundsurf_memo_half_proofs
(id, effect_id, ticket_id, fence_id, kind, state, blocked, sealed, silent_keep, body, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 effect_id=VALUES(effect_id), ticket_id=VALUES(ticket_id), fence_id=VALUES(fence_id),
 kind=VALUES(kind), state=VALUES(state), blocked=VALUES(blocked), sealed=VALUES(sealed),
 silent_keep=VALUES(silent_keep), body=VALUES(body)`,
		  p.ID,
		   p.EffectID, p.TicketID, nullStr(p.FenceID), string(p.Kind), string(p.State),
		  b2i(p.Blocked), b2i(p.Sealed), b2i(p.SilentKeep), body, p.CreatedAt)
	   if err != nil {
		   return MemoHalfCreditProof{}, err
	   }
	   return p, nil
   }

   func (s *SQLStore) scanProof(row interface{ Scan(...any) error }) (MemoHalfCreditProof, error) {
	  var body []byte
	   var p MemoHalfCreditProof
	  if err := row.Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			   return MemoHalfCreditProof{}, ErrProofNotFound
		  }
		   return MemoHalfCreditProof{}, err
	   }
	  if err := json.Unmarshal(body, &p); err != nil {
		   return MemoHalfCreditProof{}, err
	   }
	   return p, nil
  }

  func (s *SQLStore) GetProof(ctx context.Context, id string) (MemoHalfCreditProof, error) {
	  if s == nil || s.db == nil {
		  return MemoHalfCreditProof{}, ErrStoreUnavailable
	   }
	   row := s.db.QueryRowContext(ctx, `SELECT body FROM fundsurf_memo_half_proofs WHERE id=?`, strings.TrimSpace(id))
	   return s.scanProof(row)
   }

  func (s *SQLStore) listProofs(ctx context.Context, q string, args ...any) ([]MemoHalfCreditProof, error) {
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]MemoHalfCreditProof, 0)
	  for rows.Next() {
		   p, err := s.scanProof(rows)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, p)
	   }
	  return out, rows.Err()
   }

   func (s *SQLStore) ListProofsByTicket(ctx context.Context, ticketID string, limit int) ([]MemoHalfCreditProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	  return s.listProofs(ctx, `SELECT body FROM fundsurf_memo_half_proofs WHERE ticket_id=? ORDER BY created_at DESC LIMIT ?`, ticketID, limit)
  }

  func (s *SQLStore) ListProofsByState(ctx context.Context, state HalfState, limit int) ([]MemoHalfCreditProof, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	  return s.listProofs(ctx, `SELECT body FROM fundsurf_memo_half_proofs WHERE state=? ORDER BY created_at DESC LIMIT ?`, string(state), limit)
   }

   func (s *SQLStore) ListProofs(ctx context.Context, kind ProofKind, limit int) ([]MemoHalfCreditProof, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	   if limit <= 0 {
		  limit = 50
	  }
	   if kind == "" {
		   return s.listProofs(ctx, `SELECT body FROM fundsurf_memo_half_proofs ORDER BY created_at DESC LIMIT ?`, limit)
	  }
	   return s.listProofs(ctx, `SELECT body FROM fundsurf_memo_half_proofs WHERE kind=? ORDER BY created_at DESC LIMIT ?`, string(kind), limit)
   }

  func (s *SQLStore) CountProofs(ctx context.Context) (int, error) {
	  if s == nil || s.db == nil {
		  return 0, ErrStoreUnavailable
	  }
	  var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fundsurf_memo_half_proofs`).Scan(&n)
	   return n, err
   }

   func (s *SQLStore) SaveAudit(ctx context.Context, a EffectAudit) (EffectAudit, error) {
	   if s == nil || s.db == nil {
		   return EffectAudit{}, ErrStoreUnavailable
	  }
	  body, err := json.Marshal(a)
	  if err != nil {
		  return EffectAudit{}, err
	   }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO fundsurf_memo_effect_audit (id, effect_id, event, from_state, to_state, actor, body, at)
VALUES (?,?,?,?,?,?,?,?)`, a.ID, a.EffectID, a.Event, string(a.FromState), string(a.ToState), a.Actor, body, a.At)
	  if err != nil {
		   return EffectAudit{}, err
	   }
	  return a, nil
  }

  func (s *SQLStore) ListAudit(ctx context.Context, effectID string, limit int) ([]EffectAudit, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	  rows, err := s.db.QueryContext(ctx, `SELECT body FROM fundsurf_memo_effect_audit WHERE effect_id=? OR ?='' ORDER BY at DESC LIMIT ?`, effectID, effectID, limit)
	  if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]EffectAudit, 0)
	   for rows.Next() {
		   var body []byte
		  if err := rows.Scan(&body); err != nil {
			  return nil, err
		   }
		  var a EffectAudit
		  if err := json.Unmarshal(body, &a); err != nil {
			   return nil, err
		  }
		   out = append(out, a)
	   }
	  return out, rows.Err()
   }

  func (s *SQLStore) CountAudits(ctx context.Context) (int, error) {
	  if s == nil || s.db == nil {
		   return 0, ErrStoreUnavailable
	   }
	   var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fundsurf_memo_effect_audit`).Scan(&n)
	  return n, err
   }

  func b2i(v bool) int {
	   if v {
		   return 1
	  }
	  return 0
   }

   func nullStr(s string) any {
	  if strings.TrimSpace(s) == "" {
		   return nil
	   }
	  return s
   }

   func nullTime(t *time.Time) any {
	  if t == nil || t.IsZero() {
		   return nil
	  }
	  return *t
  }
