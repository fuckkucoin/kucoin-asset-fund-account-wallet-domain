package fxmarkleasegate

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
CREATE TABLE IF NOT EXISTS fx_mark_leases (
  id VARCHAR(64) PRIMARY KEY,
  from_asset VARCHAR(16) NOT NULL,
  to_asset VARCHAR(16) NOT NULL,
  pair_key VARCHAR(40) NOT NULL,
  rate VARCHAR(64) NOT NULL,
  source VARCHAR(16) NOT NULL,
  source_print VARCHAR(80) NOT NULL,
  generation BIGINT NOT NULL,
  state VARCHAR(16) NOT NULL,
  ttl_ms BIGINT NOT NULL,
  issued_at DATETIME NOT NULL,
  expires_at DATETIME NOT NULL,
  bound_at DATETIME NULL,
  consumed_at DATETIME NULL,
  expired_at DATETIME NULL,
  revoked_at DATETIME NULL,
  proof_id VARCHAR(64) NULL,
  idem VARCHAR(80) NULL,
  revoke_reason VARCHAR(255) NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  INDEX idx_fx_mark_leases_pair (pair_key, issued_at),
  INDEX idx_fx_mark_leases_state (state, expires_at),
  INDEX idx_fx_mark_leases_idem (idem)
);
CREATE TABLE IF NOT EXISTS fx_mark_drift_proofs (
  id VARCHAR(64) PRIMARY KEY,
  lease_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  body JSON NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_fx_mark_proofs_lease (lease_id, created_at)
);
CREATE TABLE IF NOT EXISTS fx_mark_audit (
  id VARCHAR(64) PRIMARY KEY,
  lease_id VARCHAR(64) NOT NULL,
  event VARCHAR(16) NOT NULL,
  from_state VARCHAR(16) NOT NULL,
  to_state VARCHAR(16) NOT NULL,
  actor VARCHAR(64) NOT NULL,
  body JSON NOT NULL,
  at DATETIME NOT NULL,
  INDEX idx_fx_mark_audit_lease (lease_id, at)
);
CREATE TABLE IF NOT EXISTS fx_mark_generations (
  asset VARCHAR(16) PRIMARY KEY,
  seq BIGINT NOT NULL,
  last_usd VARCHAR(64) NOT NULL,
  updated_at DATETIME NOT NULL
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

  func (s *SQLStore) SaveLease(ctx context.Context, l MarkLease) (MarkLease, error) {
	   if s == nil || s.db == nil {
		   return MarkLease{}, ErrStoreUnavailable
	   }
	   body, err := json.Marshal(l)
	  if err != nil {
		   return MarkLease{}, err
	   }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO fx_mark_leases (
  id, from_asset, to_asset, pair_key, rate, source, source_print, generation, state, ttl_ms,
  issued_at, expires_at, bound_at, consumed_at, expired_at, revoked_at, proof_id, idem,
  revoke_reason, body, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  state=VALUES(state), proof_id=VALUES(proof_id), bound_at=VALUES(bound_at),
  consumed_at=VALUES(consumed_at), expired_at=VALUES(expired_at), revoked_at=VALUES(revoked_at),
  revoke_reason=VALUES(revoke_reason), body=VALUES(body), updated_at=VALUES(updated_at)`,
		   l.ID, l.From, l.To, PairKey(l.From, l.To), l.Rate, l.Source, l.SourcePrint, l.Generation, string(l.State), l.TTL.Milliseconds(),
		   l.IssuedAt, l.ExpiresAt, nullTime(l.BoundAt), nullTime(l.ConsumedAt), nullTime(l.ExpiredAt), nullTime(l.RevokedAt),
		  l.ProofID, l.Idem, l.RevokeReason, body, l.CreatedAt, l.UpdatedAt,
	  )
	   if err != nil {
		  return MarkLease{}, err
	  }
	   return l, nil
  }

  func (s *SQLStore) GetLease(ctx context.Context, id string) (MarkLease, error) {
	   return s.scanLease(ctx, `SELECT body FROM fx_mark_leases WHERE id=?`, strings.TrimSpace(id))
  }

   func (s *SQLStore) GetLeaseByIdem(ctx context.Context, idem string) (MarkLease, error) {
	  return s.scanLease(ctx, `SELECT body FROM fx_mark_leases WHERE idem=? ORDER BY issued_at DESC LIMIT 1`, strings.TrimSpace(idem))
  }

   func (s *SQLStore) scanLease(ctx context.Context, q string, args ...any) (MarkLease, error) {
	  if s == nil || s.db == nil {
		  return MarkLease{}, ErrStoreUnavailable
	   }
	  var body []byte
	  if err := s.db.QueryRowContext(ctx, q, args...).Scan(&body); err != nil {
		   if err == sql.ErrNoRows {
			  return MarkLease{},
				  ErrLeaseNotFound
		  }
		  return MarkLease{}, err
	   }
	  var l MarkLease
	  if err := json.Unmarshal(body, &l); err != nil || l.ID == "" {
		   return MarkLease{},
			  ErrLeaseNotFound
	   }
	   return l, nil
   }

   func (s *SQLStore) listLeases(ctx context.Context, q string, args ...any) ([]MarkLease, error) {
	  if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	  }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]MarkLease, 0)
	   for rows.Next() {
		  var body []byte
		  if err := rows.Scan(&body); err != nil {
			   return nil, err
		   }
		   var l MarkLease
		  if json.Unmarshal(body, &l) != nil || l.ID == "" {
			  continue
		   }
		  out = append(out, l)
	  }
	   return out, rows.Err()
  }

   func (s *SQLStore) ListLeasesByPair(ctx context.Context, pairKey string, limit int) ([]MarkLease, error) {
	   if limit <= 0 {
		   limit = 50
	   }
	   return s.listLeases(ctx, `SELECT body FROM fx_mark_leases WHERE pair_key=? ORDER BY issued_at DESC LIMIT ?`, pairKey, limit)
   }

  func (s *SQLStore) ListLeasesByState(ctx context.Context, state LeaseState, limit int) ([]MarkLease, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	  return s.listLeases(ctx, `SELECT body FROM fx_mark_leases WHERE state=? ORDER BY issued_at DESC LIMIT ?`, string(state), limit)
  }

  func (s *SQLStore) ListLeasesByAsset(ctx context.Context, asset string, limit int) ([]MarkLease, error) {
	   if limit <= 0 {
		   limit = 80
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  return s.listLeases(ctx, `SELECT body FROM fx_mark_leases WHERE from_asset=? OR to_asset=? ORDER BY issued_at DESC LIMIT ?`, asset, asset, limit)
  }

   func (s *SQLStore) ListExpiredLive(ctx context.Context, before time.Time, limit int) ([]MarkLease, error) {
	   if limit <= 0 {
		  limit = DefaultExpiryBatch
	  }
	   return s.listLeases(ctx, `SELECT body FROM fx_mark_leases WHERE state IN ('issued','bound') AND expires_at<=? ORDER BY expires_at ASC LIMIT ?`, before, limit)
  }

   func (s *SQLStore) ListLive(ctx context.Context, limit int) ([]MarkLease, error) {
	   if limit <= 0 {
		   limit = DefaultPatrolBatch
	   }
	   return s.listLeases(ctx, `SELECT body FROM fx_mark_leases WHERE state IN ('issued','bound') ORDER BY issued_at ASC LIMIT ?`, limit)
   }

  func (s *SQLStore) CountByState(ctx context.Context, state LeaseState) (int, error) {
	   if s == nil || s.db == nil {
		   return 0, ErrStoreUnavailable
	  }
	   var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fx_mark_leases WHERE state=?`, string(state)).Scan(&n)
	  return n, err
   }

   func (s *SQLStore) SaveProof(ctx context.Context, p FxMarkDriftProof) (FxMarkDriftProof, error) {
	   if s == nil || s.db == nil {
		   return FxMarkDriftProof{}, ErrStoreUnavailable
	  }
	   body, err := json.Marshal(p)
	  if err != nil {
		   return FxMarkDriftProof{}, err
	  }
	  _, err = s.db.ExecContext(ctx, `
INSERT INTO fx_mark_drift_proofs (id, lease_id, kind, body, created_at)
VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE body=VALUES(body)`,
		   p.ID, p.LeaseID, string(p.Kind), body, p.CreatedAt)
	  if err != nil {
		   return FxMarkDriftProof{}, err
	  }
	   return p, nil
  }

   func (s *SQLStore) GetProof(ctx context.Context, id string) (FxMarkDriftProof, error) {
	   if s == nil || s.db == nil {
		  return FxMarkDriftProof{}, ErrStoreUnavailable
	   }
	   var body []byte
	  if err := s.db.QueryRowContext(ctx, `SELECT body FROM fx_mark_drift_proofs WHERE id=?`, strings.TrimSpace(id)).Scan(&body); err != nil {
		  if err == sql.ErrNoRows {
			   return FxMarkDriftProof{}, ErrProofNotFound
		   }
		  return FxMarkDriftProof{}, err
	  }
	  var p FxMarkDriftProof
	  if err := json.Unmarshal(body, &p); err != nil {
		   return FxMarkDriftProof{}, ErrProofNotFound
	  }
	   return p, nil
  }

   func (s *SQLStore) listProofs(ctx context.Context, q string, args ...any) ([]FxMarkDriftProof, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, q, args...)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]FxMarkDriftProof, 0)
	   for rows.Next() {
		  var body []byte
		  if err := rows.Scan(&body); err != nil {
			  return nil, err
		   }
		  var p FxMarkDriftProof
		   if json.Unmarshal(body, &p) != nil {
			   continue
		   }
		   out = append(out, p)
	   }
	   return out, rows.Err()
  }

  func (s *SQLStore) ListProofsByLease(ctx context.Context, leaseID string, limit int) ([]FxMarkDriftProof, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	   return s.listProofs(ctx, `SELECT body FROM fx_mark_drift_proofs WHERE lease_id=? ORDER BY created_at DESC LIMIT ?`, leaseID, limit)
   }

  func (s *SQLStore) ListProofs(ctx context.Context, kind DriftKind, limit int) ([]FxMarkDriftProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   if kind == "" {
		   return s.listProofs(ctx, `SELECT body FROM fx_mark_drift_proofs ORDER BY created_at DESC LIMIT ?`, limit)
	  }
	   return s.listProofs(ctx, `SELECT body FROM fx_mark_drift_proofs WHERE kind=? ORDER BY created_at DESC LIMIT ?`, string(kind), limit)
  }

  func (s *SQLStore) CountProofs(ctx context.Context) (int, error) {
	   if s == nil || s.db == nil {
		   return 0, ErrStoreUnavailable
	   }
	  var n int
	   err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fx_mark_drift_proofs`).Scan(&n)
	   return n, err
  }

  func (s *SQLStore) SaveAudit(ctx context.Context, a MarkLeaseAudit) (MarkLeaseAudit, error) {
	  if s == nil || s.db == nil {
		  return MarkLeaseAudit{}, ErrStoreUnavailable
	  }
	  body, err := json.Marshal(a)
	  if err != nil {
		   return MarkLeaseAudit{}, err
	   }
	   _, err = s.db.ExecContext(ctx, `
INSERT INTO fx_mark_audit (id, lease_id, event, from_state, to_state, actor, body, at)
VALUES (?,?,?,?,?,?,?,?)`,
		  a.ID, a.LeaseID, a.Event, string(a.FromState), string(a.ToState), a.Actor, body, a.At)
	   if err != nil {
		  return MarkLeaseAudit{}, err
	  }
	   return a, nil
   }

  func (s *SQLStore) ListAudit(ctx context.Context, leaseID string, limit int) ([]MarkLeaseAudit, error) {
	   if s == nil || s.db == nil {
		   return nil, ErrStoreUnavailable
	   }
	  if limit <= 0 {
		  limit = 50
	  }
	  q := `SELECT body FROM fx_mark_audit WHERE lease_id=? ORDER BY at DESC LIMIT ?`
	   args := []any{leaseID, limit}
	   if strings.TrimSpace(leaseID) == "" {
		   q = `SELECT body FROM fx_mark_audit ORDER BY at DESC LIMIT ?`
		  args = []any{limit}
	   }
	  rows,
		   err := s.db.QueryContext(ctx, q, args...)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  out := make([]MarkLeaseAudit, 0)
	   for rows.Next() {
		  var body []byte
		  if err := rows.Scan(&body); err != nil {
			   return nil, err
		   }
		  var a MarkLeaseAudit
		  if json.Unmarshal(body, &a) != nil {
			  continue
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
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fx_mark_audit`).Scan(&n)
	   return n, err
   }

  func (s *SQLStore) SaveGeneration(ctx context.Context, g MarkGeneration) (MarkGeneration, error) {
	  if s == nil || s.db == nil {
		  return MarkGeneration{}, ErrStoreUnavailable
	  }
	   _, err := s.db.ExecContext(ctx, `
INSERT INTO fx_mark_generations (asset, seq, last_usd, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE seq=VALUES(seq), last_usd=VALUES(last_usd), updated_at=VALUES(updated_at)`,
		  g.Asset, g.Seq, g.LastUSD, g.UpdatedAt)
	   if err != nil {
		  return MarkGeneration{}, err
	   }
	  return g, nil
  }

  func (s *SQLStore) GetGeneration(ctx context.Context, asset string) (MarkGeneration, error) {
	  if s == nil || s.db == nil {
		   return MarkGeneration{}, ErrStoreUnavailable
	   }
	   var g MarkGeneration
	  err := s.db.QueryRowContext(ctx, `SELECT asset, seq, last_usd, updated_at FROM fx_mark_generations WHERE asset=?`,
		   strings.ToUpper(strings.TrimSpace(asset))).Scan(&g.Asset,
		   &g.Seq, &g.LastUSD, &g.UpdatedAt)
	   if err == sql.ErrNoRows {
		   return MarkGeneration{}, ErrLeaseNotFound
	   }
	   return g, err
  }

   func (s *SQLStore) ListGenerations(ctx context.Context) ([]MarkGeneration, error) {
	   if s == nil || s.db == nil {
		  return nil, ErrStoreUnavailable
	   }
	  rows, err := s.db.QueryContext(ctx, `SELECT asset, seq, last_usd, updated_at FROM fx_mark_generations ORDER BY asset`)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   out := make([]MarkGeneration, 0)
	   for rows.Next() {
		  var g MarkGeneration
		  if err := rows.Scan(&g.Asset, &g.Seq, &g.LastUSD, &g.UpdatedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, g)
	   }
	   return out, rows.Err()
   }

  func (s *SQLStore) CountGenerations(ctx context.Context) (int, error) {
	   if s == nil || s.db == nil {
		   return 0, ErrStoreUnavailable
	   }
	  var n int
	  err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM fx_mark_generations`).Scan(&n)
	  return n, err
   }

  func nullTime(t *time.Time) any {
	   if t == nil || t.IsZero() {
		   return nil
	  }
	   return *t
  }
