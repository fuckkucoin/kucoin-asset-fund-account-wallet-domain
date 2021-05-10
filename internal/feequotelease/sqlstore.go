// 功能：手续费报价租约漂移封堵引擎：SQL 持久化存储实现。
// 实现 Store 接口，支持 MySQL 与 PostgreSQL（通过 persist.SQLDB 方言切换）。
// 表：fee_quote_leases / fee_sch_generations / fee_quote_proofs。

// 作者：Cason
// 日期：2021.05.09

package feequotelease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// SQLStore SQL 持久化实现。
type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 创建 SQL 存储实例。
  func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  return &SQLStore{db: db}
  }

   func (s *SQLStore) ready() bool {
	  return s != nil && s.db != nil && s.db.DB != nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// FeeQuoteLease CRUD
// ─────────────────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveLease(ctx context.Context, l FeeQuoteLease) error {
	   if !s.ready() {
		  return ErrUnavailable
	   }
	   actInt := func(t *time.Time) interface{} {
		  if t == nil {
			   return nil
		  }
		   return t.UTC().Format("2006-01-02 15:04:05.000")
	   }
	   stateStr := string(l.State)
	   expiresAt := l.ExpiresAt.UTC().Format("2006-01-02 15:04:05.000")
	   quotedAt := l.QuotedAt.UTC().Format("2006-01-02 15:04:05.000")
	  now := time.Now().UTC().Format("2006-01-02 15:04:05.000")
	   q := `INSERT INTO fee_quote_leases
	  (id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net,
	   schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
	   applied_at, expired_at, revoked_at, revoke_reason, idempotency_key, updated_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON DUPLICATE KEY UPDATE
	  state=VALUES(state), applied_at=VALUES(applied_at), expired_at=VALUES(expired_at),
	  revoked_at=VALUES(revoked_at), revoke_reason=VALUES(revoke_reason),
	  idempotency_key=VALUES(idempotency_key), updated_at=VALUES(updated_at)`
	  if s.db.Dialect == persist.Postgres {
		  q = `INSERT INTO fee_quote_leases
		  (id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net,
		   schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
		   applied_at, expired_at, revoked_at, revoke_reason, idempotency_key, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
		  state=EXCLUDED.state, applied_at=EXCLUDED.applied_at, expired_at=EXCLUDED.expired_at,
		  revoked_at=EXCLUDED.revoked_at, revoke_reason=EXCLUDED.revoke_reason,
		  idempotency_key=EXCLUDED.idempotency_key, updated_at=EXCLUDED.updated_at`
	  }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   l.ID, l.Kind, l.Asset, l.Gross, l.Volume,
		  l.QuotedBPS, l.QuotedFee, l.QuotedNet,
		  l.ScheduleID, l.SchGen, l.TiersFingerprint,
		   stateStr, expiresAt, quotedAt,
		   actInt(l.AppliedAt), actInt(l.ExpiredAt), actInt(l.RevokedAt),
		  l.RevokeReason, l.IdempotencyKey, now,
	   )
	  return err
   }

   func (s *SQLStore) GetLease(ctx context.Context, id string) (FeeQuoteLease, error) {
	   if !s.ready() {
		   return FeeQuoteLease{}, ErrUnavailable
	  }
	   row := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net,
		        schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
		        applied_at, expired_at, revoked_at, revoke_reason, idempotency_key
		 FROM fee_quote_leases WHERE id=?`, id)
	  return scanLease(row)
  }

  func (s *SQLStore) GetLeaseByIdempotencyKey(ctx context.Context, key string) (FeeQuoteLease, bool, error) {
	  if !s.ready() {
		   return FeeQuoteLease{}, false, ErrUnavailable
	  }
	   if strings.TrimSpace(key) == "" {
		  return FeeQuoteLease{}, false, nil
	  }
	  row := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net, 
		        schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
		        applied_at, expired_at, revoked_at, revoke_reason, idempotency_key
		 FROM fee_quote_leases WHERE idempotency_key=? LIMIT 1`, key)
	   l, err := scanLease(row)
	  if errors.Is(err, ErrLeaseNotFound) {
		   return FeeQuoteLease{}, false, nil
	   }
	   if err != nil {
		   return FeeQuoteLease{}, false, err
	   }
	  return l, true, nil
  }

  func (s *SQLStore) ListActiveLeasesByKindAsset(ctx context.Context, kind, asset string) ([]FeeQuoteLease, error) {
	  if !s.ready() {
		   return nil, ErrUnavailable
	   }
	  rows, err := s.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net,
		        schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
		        applied_at, expired_at, revoked_at, revoke_reason, idempotency_key
		 FROM fee_quote_leases WHERE kind=? AND asset=? AND state='quoted'`, kind, asset)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanLeases(rows)
  }

  func (s *SQLStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]FeeQuoteLease, error) {
	  if !s.ready() {
		  return nil, ErrUnavailable
	   }
	  rows, err := s.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, kind, asset, gross, volume, quoted_bps, quoted_fee, quoted_net,
		        schedule_id, sch_gen, tiers_fingerprint, state, expires_at, quoted_at,
		        applied_at, expired_at, revoked_at, revoke_reason, idempotency_key
		 FROM fee_quote_leases WHERE state='quoted' AND expires_at < ? LIMIT ?`,
		   now.UTC().Format("2006-01-02 15:04:05.000"), limit)
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanLeases(rows)
  }

// ─────────────────────────────────────────────────────────────────────────────
// ScheduleGeneration CRUD
// ─────────────────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveGen(ctx context.Context, g ScheduleGeneration) error {
	   if !s.ready() {
		   return ErrUnavailable
	  }
	   putAt := g.PutAt.UTC().Format("2006-01-02 15:04:05.000")
	   q := `INSERT INTO fee_sch_generations (id, kind, asset, gen, schedule_id, bps, tiers_fingerprint, put_at, put_by)
	  VALUES (?,?,?,?,?,?,?,?,?)
	  ON DUPLICATE KEY UPDATE gen=VALUES(gen), schedule_id=VALUES(schedule_id), bps=VALUES(bps),
	    tiers_fingerprint=VALUES(tiers_fingerprint), put_at=VALUES(put_at), put_by=VALUES(put_by)`
	  if s.db.Dialect == persist.Postgres {
		  q = `INSERT INTO fee_sch_generations (id, kind, asset, gen, schedule_id, bps, tiers_fingerprint, put_at, put_by)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		  ON CONFLICT (kind, asset) DO UPDATE SET
		    gen=EXCLUDED.gen, schedule_id=EXCLUDED.schedule_id, bps=EXCLUDED.bps,
		    tiers_fingerprint=EXCLUDED.tiers_fingerprint, put_at=EXCLUDED.put_at, put_by=EXCLUDED.put_by`
	   }
	   _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  g.ID,
		   g.Kind, g.Asset, g.Gen, g.ScheduleID, g.BPS, g.TiersFingerprint, putAt, g.PutBy)
	   return err
   }

  func (s *SQLStore) GetCurrentGen(ctx context.Context, kind, asset string) (ScheduleGeneration, error) {
	  if !s.ready() {
		  return ScheduleGeneration{}, ErrUnavailable
	   }
	   row := s.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, kind, asset, gen, schedule_id, bps, tiers_fingerprint, put_at, put_by
		 FROM fee_sch_generations WHERE kind=? AND asset=?`, kind, asset)
	   return scanGen(row)
  }

// ─────────────────────────────────────────────────────────────────────────────
// FeeQuoteDriftProof CRUD
// ─────────────────────────────────────────────────────────────────────────────

   func (s *SQLStore) SaveProof(ctx context.Context, p FeeQuoteDriftProof) error {
	  if !s.ready() {
		  return ErrUnavailable
	  }
	  detectedAt := p.DetectedAt.UTC().Format("2006-01-02 15:04:05.000")
	   q := `INSERT IGNORE INTO fee_quote_proofs
	  (id, kind, lease_id, fee_kind, asset, gross, bind_sch_gen, current_sch_gen,
	   bind_fingerprint, current_fingerprint, bind_bps, current_bps, evidence, detected_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  if s.db.Dialect == persist.Postgres {
		  q = `INSERT INTO fee_quote_proofs
		  (id, kind, lease_id, fee_kind, asset, gross, bind_sch_gen, current_sch_gen,
		   bind_fingerprint, current_fingerprint, bind_bps, current_bps, evidence, detected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`
	  }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   p.ID, string(p.Kind), p.LeaseID, p.FeeKind, p.Asset, p.Gross,
		  p.BindSchGen, p.CurrentSchGen, p.BindFingerprint, p.CurrentFingerprint,
		   p.BindBPS, p.CurrentBPS, p.Evidence, detectedAt)
	   return err
  }

   func (s *SQLStore) ListProofs(ctx context.Context, limit int) ([]FeeQuoteDriftProof, error) {
	   if !s.ready() {
		  return nil, ErrUnavailable
	   }
	  rows, err := s.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, kind, lease_id, fee_kind, asset, gross, bind_sch_gen, current_sch_gen, 
		        bind_fingerprint, current_fingerprint, bind_bps, current_bps, evidence, detected_at, exported_at
		 FROM fee_quote_proofs WHERE exported_at IS NULL ORDER BY detected_at ASC LIMIT ?`, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanProofs(rows)
  }

  func (s *SQLStore) MarkProofExported(ctx context.Context, id string, exportedAt time.Time) error {
	  if !s.ready() {
		   return ErrUnavailable
	   }
	  at := exportedAt.UTC().Format("2006-01-02 15:04:05.000")
	  _,
		   err := s.db.Conn(ctx).ExecContext(ctx,
		  `UPDATE fee_quote_proofs SET exported_at=? WHERE id=?`, at, id)
	   return err
   }

// ─────────────────────────────────────────────────────────────────────────────
// 扫描辅助函数
// ─────────────────────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...interface{}) error
}

  func scanLease(row rowScanner) (FeeQuoteLease, error) {
	  var l FeeQuoteLease
	   var state string
	  var expiresAt, quotedAt string
	  var appliedAt,
		  expiredAt, revokedAt sql.NullString
	   err := row.Scan(
		  &l.ID, &l.Kind, &l.Asset, &l.Gross, &l.Volume,
		   &l.QuotedBPS, &l.QuotedFee, &l.QuotedNet,
		   &l.ScheduleID, &l.SchGen, &l.TiersFingerprint,
		  &state, &expiresAt, &quotedAt,
		  &appliedAt, &expiredAt, &revokedAt,
		  &l.RevokeReason, &l.IdempotencyKey,
	   )
	   if errors.Is(err, sql.ErrNoRows) {
		   return FeeQuoteLease{}, ErrLeaseNotFound
	  }
	   if err != nil {
		  return FeeQuoteLease{}, err
	  }
	  l.State = LeaseState(state)
	   l.ExpiresAt = parseTime(expiresAt)
	   l.QuotedAt = parseTime(quotedAt)
	  if appliedAt.Valid {
		   t := parseTime(appliedAt.String)
		   l.AppliedAt = &t
	  }
	  if expiredAt.Valid {
		  t := parseTime(expiredAt.String)
		  l.ExpiredAt = &t
	   }
	  if revokedAt.Valid {
		  t := parseTime(revokedAt.String)
		  l.RevokedAt = &t
	  }
	  return l, nil
   }

   func scanLeases(rows *sql.Rows) ([]FeeQuoteLease, error) {
	   var out []FeeQuoteLease
	   for rows.Next() {
		  var l FeeQuoteLease
		  var state string
		  var expiresAt, quotedAt string
		  var appliedAt, expiredAt, revokedAt sql.NullString
		   if err := rows.Scan(
			  &l.ID, &l.Kind, &l.Asset, &l.Gross, &l.Volume,
			   &l.QuotedBPS, &l.QuotedFee, &l.QuotedNet,
			   &l.ScheduleID, &l.SchGen, &l.TiersFingerprint,
			   &state, &expiresAt, &quotedAt,
			  &appliedAt, &expiredAt, &revokedAt,
			   &l.RevokeReason, &l.IdempotencyKey,
		   ); err != nil {
			   return nil, err
		   }
		   l.State = LeaseState(state)
		   l.ExpiresAt = parseTime(expiresAt)
		  l.QuotedAt = parseTime(quotedAt)
		  if appliedAt.Valid {
			   t := parseTime(appliedAt.String)
			  l.AppliedAt = &t
		   }
		   if expiredAt.Valid {
			   t := parseTime(expiredAt.String)
			  l.ExpiredAt = &t
		   }
		  if revokedAt.Valid {
			  t := parseTime(revokedAt.String)
			  l.RevokedAt = &t
		   }
		   out = append(out, l)
	   }
	   return out, rows.Err()
  }

   func scanGen(row rowScanner) (ScheduleGeneration, error) {
	  var g ScheduleGeneration
	   var putAt string
	   err := row.Scan(&g.ID, &g.Kind, &g.Asset, &g.Gen, &g.ScheduleID, &g.BPS, &g.TiersFingerprint, &putAt, &g.PutBy)
	  if errors.Is(err, sql.ErrNoRows) {
		   return ScheduleGeneration{},
			  ErrGenNotFound
	  }
	   if err != nil {
		   return ScheduleGeneration{}, err
	   }
	  g.PutAt = parseTime(putAt)
	   return g, nil
  }

  func scanProofs(rows *sql.Rows) ([]FeeQuoteDriftProof, error) {
	   var out []FeeQuoteDriftProof
	  for rows.Next() {
		  var p FeeQuoteDriftProof
		   var kind, detectedAt string
		  var exportedAt sql.NullString
		   if err := rows.Scan(
			   &p.ID, &kind, &p.LeaseID, &p.FeeKind, &p.Asset, &p.Gross,
			  &p.BindSchGen, &p.CurrentSchGen,
			  &p.BindFingerprint, &p.CurrentFingerprint,
			   &p.BindBPS, &p.CurrentBPS,
			   &p.Evidence, &detectedAt, &exportedAt,
		   ); err != nil {
			   return nil, err
		   }
		   p.Kind = DriftProofKind(kind)
		  p.DetectedAt = parseTime(detectedAt)
		  if exportedAt.Valid {
			  t := parseTime(exportedAt.String)
			  p.ExportedAt = &t
		  }
		   out = append(out, p)
	   }
	  return out, rows.Err()
  }

   func parseTime(s string) time.Time {
	   if s == "" {
		   return time.Time{}
	   }
	  formats := []string{
		   "2006-01-02 15:04:05.000",
		   "2006-01-02 15:04:05",
		   "2006-01-02T15:04:05.999999999Z07:00",
		   "2006-01-02T15:04:05Z",
	   }
	  for _, f := range formats {
		  if t, err := time.Parse(f, s); err == nil {
			  return t
		  }
	  }
	  return time.Time{}
   }

// sqlOn 辅助：检查 db 可用

  func sqlOn(db *persist.SQLDB) bool {
	   return db != nil && db.DB != nil
   }

// formatErr 包装 SQL 错误
   func formatErr(op string, err error) error {
	   return fmt.Errorf("feequotelease.%s: %w", op, err)
  }
