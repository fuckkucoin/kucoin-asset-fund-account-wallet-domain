package porsnapgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Store interface
// ─────────────────────────────────────────────────────────────────────────────
// Store 定义 porsnapgate 域的持久化操作。
type Store interface {
	persist.TxRunner
	// ── SnapGen ──────────────────────────────────────────────────────────────
	// InsertGen 插入新世代行；SnapGenID 不可重复，否则返回 ErrGenSealed。
	InsertGen(ctx context.Context, gen *PorSnapshotGen) error
	// GetGen 按 ID 读取世代；不存在返回 ErrGenNotFound。
	GetGen(ctx context.Context, snapGenID string) (*PorSnapshotGen, error)
	// GetGenBySnapshotID 按 SnapshotID 读取世代；不存在返回 ErrGenNotFound。
	GetGenBySnapshotID(ctx context.Context, snapshotID string) (*PorSnapshotGen, error)
	// UpdateGenState 更新世代状态与相关时间戳；乐观锁保护（期望旧状态匹配）。
	UpdateGenState(ctx context.Context, snapGenID string, from, to SnapGenState, now time.Time) error
	// ListGensByState 按状态分页列举世代；供 Worker 轮询使用。
	ListGensByState(ctx context.Context,
		state SnapGenState, limit int) ([]*PorSnapshotGen, error)
	// ListExpiredGens 列举 SealedAt + TTLSeconds < now 且 State=sealed 的世代。
	ListExpiredGens(ctx context.Context, now time.Time, limit int) ([]*PorSnapshotGen, error)
	// ── Attestation ──────────────────────────────────────────────────────────
	// InsertAttest 插入新公证行；AttestID 不可重复。
	InsertAttest(ctx context.Context, a *AttestationBundle) error
	// GetAttest 按 ID 读取公证；不存在返回 ErrAttestNotFound。
	GetAttest(ctx context.Context, attestID string) (*AttestationBundle, error)
	// GetAttestByGen 按 SnapGenID 读取已发布公证；不存在返回 ErrAttestNotFound。
	GetAttestByGen(ctx context.Context, snapGenID string) (*AttestationBundle, error)
	// UpdateAttestState 更新公证状态；乐观锁保护。
	UpdateAttestState(ctx context.Context,
		attestID string, from, to AttestState, reason string, now time.Time) error
	// ListAttestsByState 按状态分页列举公证。
	ListAttestsByState(ctx context.Context, state AttestState, limit int) ([]*AttestationBundle, error)
	// ── DriftProof ────────────────────────────────────────────────────────────
	// InsertDriftProof 追加漂移证明；ProofID 不可重复，相同 SnapGenID+Kind 幂等拒绝。
	InsertDriftProof(ctx context.Context,
		p *PorSnapDriftProof) error
	// GetDriftProof 按 ID 读取漂移证明；不存在返回 persist.ErrNotFound。
	GetDriftProof(ctx context.Context,
		proofID string) (*PorSnapDriftProof, error)
	// ListDriftProofsByGen 按 SnapGenID 列举所有漂移证明。
	ListDriftProofsByGen(ctx context.Context, snapGenID string, limit int) ([]*PorSnapDriftProof, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// SQLStore implementation
// ─────────────────────────────────────────────────────────────────────────────
// SQLStore 基于 *persist.SQLDB 的 porsnapgate 仓储实现。
type SQLStore struct {
	db *persist.SQLDB
}

// NewSQLStore 构造 SQLStore；db 不可为 nil。
   func NewSQLStore(db *persist.SQLDB) *SQLStore {
	  if db == nil {
		   panic("porsnapgate: SQLStore requires non-nil SQLDB")
	  }
	  return &SQLStore{db: db}
   }

// InTx 委托给底层 SQLDB 事务运行器。

   func (s *SQLStore) InTx(ctx context.Context, fn func(context.Context) error) error {
	   return s.db.InTx(ctx, fn)
   }

  // ── SnapGen ──────────────────────────────────────────────────────────────────

  func (s *SQLStore) InsertGen(ctx context.Context, gen *PorSnapshotGen) error {
	  q := `INSERT INTO por_snap_gens
		(snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		 as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		 sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  if s.db.Dialect == persist.Postgres {
		   q = `INSERT INTO por_snap_gens
		(snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		 as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		 sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`
	   }
	  _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		  gen.SnapGenID,
		  gen.SnapshotID, gen.MerkleRoot, gen.LeavesDigest, gen.TotalsFingerprint,
		  gen.AsOf.UTC(), gen.StoreMark, gen.UserCount, gen.LeafCount, string(gen.State), gen.TTLSeconds,
		  gen.SealedAt.UTC(),
		  nilTime(gen.AttestedAt), nilTime(gen.ExpiredAt), nilTime(gen.RevokedAt),
		   gen.CreatedAt.UTC(), gen.UpdatedAt.UTC(),
	   )
	  if err != nil {
		   if isDuplicateKey(err) {
			   return ErrGenSealed
		  }
		   return WrapStore("InsertGen", err)
	   }
	   return nil
   }

  func (s *SQLStore) GetGen(ctx context.Context, snapGenID string) (*PorSnapshotGen, error) {
	   q := `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens WHERE snap_gen_id = ?`
	   if s.db.Dialect == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	   }
	  row := s.db.Conn(ctx).QueryRowContext(ctx, q, snapGenID)
	  return scanGen(row)
  }

  func (s *SQLStore) GetGenBySnapshotID(ctx context.Context, snapshotID string) (*PorSnapshotGen, error) {
	  q := `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens WHERE snapshot_id = ? ORDER BY created_at DESC LIMIT 1`
	  if s.db.Dialect == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	  }
	   row := s.db.Conn(ctx).QueryRowContext(ctx, q, snapshotID)
	   return scanGen(row)
  }

  func (s *SQLStore) UpdateGenState(ctx context.Context, snapGenID string, from, to SnapGenState, now time.Time) error {
	  nt := now.UTC()
	  var q string
	  var args []interface{}
	  if s.db.Dialect == persist.Postgres {
		  q = `UPDATE por_snap_gens SET state=$1, updated_at=$2,
			attested_at=CASE WHEN $3='attested' THEN $4 ELSE attested_at END,
			expired_at=CASE WHEN $5='expired' THEN $6 ELSE expired_at END,
			revoked_at=CASE WHEN $7='revoked' THEN $8 ELSE revoked_at END
			WHERE snap_gen_id=$9 AND state=$10`
	  } else {
		   q = `UPDATE por_snap_gens SET state=?, updated_at=?,
			attested_at=CASE WHEN ?='attested' THEN ? ELSE attested_at END,
			expired_at=CASE WHEN ?='expired' THEN ? ELSE expired_at END,
			revoked_at=CASE WHEN ?='revoked' THEN ? ELSE revoked_at END
			WHERE snap_gen_id=? AND state=?`
	   }
	  args = []interface{}{
		   string(to), nt,
		  string(to), nt,
		  string(to), nt,
		   string(to), nt,
		   snapGenID, string(from),
	   }
	  res, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		  return WrapStore("UpdateGenState", err)
	  }
	  n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	  }
	   return nil
  }

  func (s *SQLStore) ListGensByState(ctx context.Context, state SnapGenState, limit int) ([]*PorSnapshotGen, error) {
	   q := `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens WHERE state=? ORDER BY created_at ASC LIMIT ?`
	   if s.db.Dialect == persist.Postgres {
		  q = `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens WHERE state=$1 ORDER BY created_at ASC LIMIT $2`
	   }
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, string(state), limit)
	   if err != nil {
		   return nil, WrapStore("ListGensByState", err)
	  }
	   defer rows.Close()
	  return scanGens(rows)
   }

   func (s *SQLStore) ListExpiredGens(ctx context.Context, now time.Time, limit int) ([]*PorSnapshotGen, error) {
	  var q string
	   if s.db.Dialect == persist.Postgres {
		   q = `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint,
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens
		WHERE state='sealed'
		  AND sealed_at + (ttl_seconds || ' seconds')::interval < $1
		ORDER BY sealed_at ASC LIMIT $2`
	   } else {
		   q = `SELECT snap_gen_id, snapshot_id, merkle_root, leaves_digest, totals_fingerprint, 
		as_of, store_mark, user_count, leaf_count, state, ttl_seconds,
		sealed_at, attested_at, expired_at, revoked_at, created_at, updated_at
		FROM por_snap_gens
		WHERE state='sealed'
		  AND DATE_ADD(sealed_at, INTERVAL ttl_seconds SECOND) < ?
		ORDER BY sealed_at ASC LIMIT ?`
	   }
	   rows, err := s.db.Conn(ctx).QueryContext(ctx, q, now.UTC(), limit)
	  if err != nil {
		  return nil, WrapStore("ListExpiredGens", err)
	  }
	  defer rows.Close()
	   return scanGens(rows)
  }

// ── Attestation ──────────────────────────────────────────────────────────────

  func (s *SQLStore) InsertAttest(ctx context.Context, a *AttestationBundle) error {
	  q := `INSERT INTO por_attestations
		(attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		 export_payload, export_hash, state, requester,
		 published_at, revoked_at, revoke_reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	  if s.db.Dialect == persist.Postgres {
		  q = `INSERT INTO por_attestations
		(attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		 export_payload, export_hash, state, requester,
		 published_at, revoked_at, revoke_reason, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	   }
	  _,
		   err := s.db.Conn(ctx).ExecContext(ctx, q,
		   a.AttestID, a.SnapGenID, a.SnapshotID, a.MerkleRoot, a.TotalsFingerprint,
		  a.ExportPayload, a.ExportHash, string(a.State), a.Requester,
		   nilTime(a.PublishedAt), nilTime(a.RevokedAt), a.RevokeReason,
		   a.CreatedAt.UTC(), a.UpdatedAt.UTC(),
	  )
	   if err != nil {
		  if isDuplicateKey(err) {
			   return ErrAttestBound
		   }
		   return WrapStore("InsertAttest", err)
	   }
	   return nil
  }

  func (s *SQLStore) GetAttest(ctx context.Context, attestID string) (*AttestationBundle, error) {
	  q := `SELECT attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		export_payload, export_hash, state, requester,
		published_at, revoked_at, revoke_reason, created_at, updated_at
		FROM por_attestations WHERE attest_id=?`
	   if s.db.Dialect == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	   }
	  row := s.db.Conn(ctx).QueryRowContext(ctx, q, attestID)
	   return scanAttest(row)
   }

   func (s *SQLStore) GetAttestByGen(ctx context.Context, snapGenID string) (*AttestationBundle, error) {
	  q := `SELECT attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		export_payload, export_hash, state, requester,
		published_at, revoked_at, revoke_reason, created_at, updated_at
		FROM por_attestations WHERE snap_gen_id=? AND state='published' ORDER BY created_at DESC LIMIT 1`
	  if s.db.Dialect == persist.Postgres {
		  q = strings.ReplaceAll(q, "?", "$1")
	  }
	   row := s.db.Conn(ctx).QueryRowContext(ctx,
		  q, snapGenID)
	  return scanAttest(row)
   }

   func (s *SQLStore) UpdateAttestState(ctx context.Context, attestID string, from, to AttestState, reason string, now time.Time) error {
	   nt := now.UTC()
	   var q string
	   var args []interface{}
	  if s.db.Dialect == persist.Postgres {
		   q = `UPDATE por_attestations SET state=$1, revoke_reason=$2, updated_at=$3,
			published_at=CASE WHEN $4='published' THEN $5 ELSE published_at END,
			revoked_at=CASE WHEN $6='revoked' THEN $7 ELSE revoked_at END
			WHERE attest_id=$8 AND state=$9`
	  } else {
		  q = `UPDATE por_attestations SET state=?, revoke_reason=?, updated_at=?, 
			published_at=CASE WHEN ?='published' THEN ? ELSE published_at END,
			revoked_at=CASE WHEN ?='revoked' THEN ? ELSE revoked_at END
			WHERE attest_id=? AND state=?`
	  }
	  args = []interface{}{
		  string(to), reason, nt,
		  string(to), nt,
		  string(to), nt,
		   attestID, string(from),
	   }
	   res, err := s.db.Conn(ctx).ExecContext(ctx, q, args...)
	   if err != nil {
		   return WrapStore("UpdateAttestState", err)
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		  return ErrInvalidTransition
	   }
	   return nil
  }

   func (s *SQLStore) ListAttestsByState(ctx context.Context, state AttestState, limit int) ([]*AttestationBundle, error) {
	   q := `SELECT attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		export_payload, export_hash, state, requester,
		published_at, revoked_at, revoke_reason, created_at, updated_at
		FROM por_attestations WHERE state=? ORDER BY created_at ASC LIMIT ?`
	   if s.db.Dialect == persist.Postgres {
		  q = `SELECT attest_id, snap_gen_id, snapshot_id, merkle_root, totals_fingerprint,
		export_payload, export_hash, state, requester,
		published_at, revoked_at, revoke_reason, created_at, updated_at
		FROM por_attestations WHERE state=$1 ORDER BY created_at ASC LIMIT $2`
	  }
	   rows, err := s.db.Conn(ctx).QueryContext(ctx, q, string(state), limit)
	  if err != nil {
		   return nil, WrapStore("ListAttestsByState", err)
	   }
	  defer rows.Close()
	  return scanAttests(rows)
   }

// ── DriftProof ────────────────────────────────────────────────────────────────

  func (s *SQLStore) InsertDriftProof(ctx context.Context, p *PorSnapDriftProof) error {
	   q := `INSERT INTO por_snap_proofs
		(proof_id, snap_gen_id, snapshot_id, kind,
		 sealed_root, live_root, sealed_totals, live_totals, drift_assets,
		 sealed_store_mark, live_store_mark, detected_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`
	   if s.db.Dialect == persist.Postgres {
		   q = `INSERT INTO por_snap_proofs
		(proof_id, snap_gen_id, snapshot_id, kind,
		 sealed_root, live_root, sealed_totals, live_totals, drift_assets,
		 sealed_store_mark, live_store_mark, detected_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	  }
	   _, err := s.db.Conn(ctx).ExecContext(ctx, q,
		   p.ProofID, p.SnapGenID, p.SnapshotID, string(p.Kind),
		   p.SealedRoot,
		   p.LiveRoot, p.SealedTotals, p.LiveTotals, p.DriftAssets,
		  p.SealedStoreMark, p.LiveStoreMark, p.DetectedAt.UTC(), p.CreatedAt.UTC(),
	  )
	   if err != nil {
		  if isDuplicateKey(err) {
			  return ErrDriftProofConflict
		   }
		  return WrapStore("InsertDriftProof", err)
	  }
	   return nil
   }

   func (s *SQLStore) GetDriftProof(ctx context.Context, proofID string) (*PorSnapDriftProof, error) {
	   q := `SELECT proof_id, snap_gen_id, snapshot_id, kind,
		sealed_root, live_root, sealed_totals, live_totals, drift_assets,
		sealed_store_mark, live_store_mark, detected_at, created_at
		FROM por_snap_proofs WHERE proof_id=?`
	   if s.db.Dialect == persist.Postgres {
		   q = strings.ReplaceAll(q, "?", "$1")
	   }
	   row := s.db.Conn(ctx).QueryRowContext(ctx,
		   q, proofID)
	   return scanDriftProof(row)
   }

   func (s *SQLStore) ListDriftProofsByGen(ctx context.Context, snapGenID string, limit int) ([]*PorSnapDriftProof, error) {
	   q := `SELECT proof_id, snap_gen_id, snapshot_id, kind,
		sealed_root, live_root, sealed_totals, live_totals, drift_assets,
		sealed_store_mark, live_store_mark, detected_at, created_at
		FROM por_snap_proofs WHERE snap_gen_id=? ORDER BY detected_at DESC LIMIT ?`
	   if s.db.Dialect == persist.Postgres {
		   q = `SELECT proof_id, snap_gen_id, snapshot_id, kind,
		sealed_root, live_root, sealed_totals, live_totals, drift_assets,
		sealed_store_mark, live_store_mark, detected_at, created_at
		FROM por_snap_proofs WHERE snap_gen_id=$1 ORDER BY detected_at DESC LIMIT $2`
	  }
	  rows, err := s.db.Conn(ctx).QueryContext(ctx, q, snapGenID, limit)
	   if err != nil {
		  return nil, WrapStore("ListDriftProofsByGen", err)
	   }
	   defer rows.Close()
	   return scanDriftProofs(rows)
  }

// ─────────────────────────────────────────────────────────────────────────────
// scan helpers
// ─────────────────────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...interface{}) error
}

  func scanGen(row rowScanner) (*PorSnapshotGen, error) {
	   var g PorSnapshotGen
	   var state string
	  var attestedAt, expiredAt, revokedAt sql.NullTime
	   err := row.Scan(
		  &g.SnapGenID, &g.SnapshotID, &g.MerkleRoot, &g.LeavesDigest, &g.TotalsFingerprint,
		   &g.AsOf, &g.StoreMark, &g.UserCount, &g.LeafCount, &state, &g.TTLSeconds,
		  &g.SealedAt, &attestedAt, &expiredAt, &revokedAt,
		   &g.CreatedAt, &g.UpdatedAt,
	   )
	  if err != nil {
		   if err == sql.ErrNoRows {
			  return nil, ErrGenNotFound
		   }
		   return nil, WrapStore("scanGen", err)
	  }
	   g.State = SnapGenState(state)
	  if attestedAt.Valid {
		   t := attestedAt.Time
		  g.AttestedAt = &t
	  }
	   if expiredAt.Valid {
		  t := expiredAt.Time
		  g.ExpiredAt = &t
	  }
	   if revokedAt.Valid {
		  t := revokedAt.Time
		   g.RevokedAt = &t
	  }
	  return &g, nil
  }

   func scanGens(rows *sql.Rows) ([]*PorSnapshotGen, error) {
	   var out []*PorSnapshotGen
	  for rows.Next() {
		  g, err := scanGen(rows)
		   if err != nil {
			   return nil, err
		  }
		  out = append(out, g)
	   }
	  return out, rows.Err()
  }

  func scanAttest(row rowScanner) (*AttestationBundle, error) {
	  var a AttestationBundle
	  var state string
	  var publishedAt, revokedAt sql.NullTime
	   err := row.Scan(
		   &a.AttestID,
		   &a.SnapGenID, &a.SnapshotID, &a.MerkleRoot, &a.TotalsFingerprint,
		   &a.ExportPayload, &a.ExportHash, &state, &a.Requester,
		   &publishedAt, &revokedAt, &a.RevokeReason,
		   &a.CreatedAt, &a.UpdatedAt,
	   )
	   if err != nil {
		  if err == sql.ErrNoRows {
			  return nil, ErrAttestNotFound
		   }
		   return nil, WrapStore("scanAttest", err)
	  }
	   a.State = AttestState(state)
	   if publishedAt.Valid {
		   t := publishedAt.Time
		  a.PublishedAt = &t
	  }
	  if revokedAt.Valid {
		  t := revokedAt.Time
		  a.RevokedAt = &t
	   }
	   return &a, nil
   }

  func scanAttests(rows *sql.Rows) ([]*AttestationBundle, error) {
	   var out []*AttestationBundle
	  for rows.Next() {
		   a, err := scanAttest(rows)
		   if err != nil {
			   return nil, err
		   }
		   out = append(out, a)
	   }
	   return out, rows.Err()
  }

   func scanDriftProof(row rowScanner) (*PorSnapDriftProof, error) {
	   var p PorSnapDriftProof
	   var kind string
	   err := row.Scan(
		   &p.ProofID, &p.SnapGenID, &p.SnapshotID, &kind,
		   &p.SealedRoot, &p.LiveRoot, &p.SealedTotals, &p.LiveTotals, &p.DriftAssets,
		   &p.SealedStoreMark, &p.LiveStoreMark, &p.DetectedAt, &p.CreatedAt,
	  )
	  if err != nil {
		   if err == sql.ErrNoRows {
			   return nil, persist.ErrNotFound
		  }
		   return nil, WrapStore("scanDriftProof", err)
	  }
	   p.Kind = ProofKind(kind)
	   return &p, nil
   }

   func scanDriftProofs(rows *sql.Rows) ([]*PorSnapDriftProof, error) {
	  var out []*PorSnapDriftProof
	  for rows.Next() {
		  p, err := scanDriftProof(rows)
		  if err != nil {
			  return nil, err
		  }
		   out = append(out, p)
	   }
	   return out, rows.Err()
   }

// ─────────────────────────────────────────────────────────────────────────────
// utility helpers
// ─────────────────────────────────────────────────────────────────────────────

   func nilTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC()
}

   func isDuplicateKey(err error) bool {
	   if err == nil {
		   return false
	  }
	  msg := strings.ToLower(err.Error())
	   return strings.Contains(msg, "duplicate") ||
		   strings.Contains(msg, "unique constraint") ||
		   strings.Contains(msg, "duplicate key")
  }

// marshalTotals JSON 序列化 totals 供 DriftProof 使用。

   func marshalTotals(totals interface{}) []byte {
	   b, _ := json.Marshal(totals)
	  return b
   }

// marshalDriftAssets JSON 序列化漂移资产列表。
   func marshalDriftAssets(assets []DriftAsset) []byte {
	   if len(assets) == 0 {
		   return []byte("[]")
	   }
	   b, _ := json.Marshal(assets)
	  return b
  }

// ─────────────────────────────────────────────────────────────────────────────
// MemStore — 内存实现（专用）
// ─────────────────────────────────────────────────────────────────────────────
// MemStore 是 Store 的纯内存实现，专供单元使用。不支持并发安全。
type MemStore struct {
	gens map[string]*PorSnapshotGen
	gensBySnap map[string]*PorSnapshotGen
	attests    map[string]*AttestationBundle
	proofs     map[string]*PorSnapDriftProof
}

// NewMemStore 构造空的内存仓储。
  func NewMemStore() *MemStore {
	  return &MemStore{
		   gens: make(map[string]*PorSnapshotGen),
		  gensBySnap: make(map[string]*PorSnapshotGen),
		   attests: make(map[string]*AttestationBundle),
		   proofs: make(map[string]*PorSnapDriftProof),
	   }
   }

  func (m *MemStore) InTx(_ context.Context,
	   fn func(context.Context) error) error {
	   return fn(context.Background())
  }

  func (m *MemStore) InsertGen(_ context.Context, gen *PorSnapshotGen) error {
	  if _, ok := m.gens[gen.SnapGenID]; ok {
		  return ErrGenSealed
	  }
	  cp := *gen
	  m.gens[gen.SnapGenID] = &cp
	  m.gensBySnap[gen.SnapshotID] = &cp
	  return nil
   }

   func (m *MemStore) GetGen(_ context.Context, id string) (*PorSnapshotGen, error) {
	  g, ok := m.gens[id]
	   if !ok {
		  return nil, ErrGenNotFound
	   }
	   cp := *g
	  return &cp, nil
   }

   func (m *MemStore) GetGenBySnapshotID(_ context.Context, snapshotID string) (*PorSnapshotGen, error) {
	  g, ok := m.gensBySnap[snapshotID]
	   if !ok {
		   return nil, ErrGenNotFound
	  }
	   cp := *g
	  return &cp, nil
  }

   func (m *MemStore) UpdateGenState(_ context.Context, id string, from, to SnapGenState, now time.Time) error {
	  g, ok := m.gens[id]
	  if !ok {
		  return ErrGenNotFound
	   }
	  if g.State != from {
		   return ErrInvalidTransition
	   }
	  g.State = to
	  g.UpdatedAt = now
	  switch to {
	  case SnapAttested:
		  t := now.UTC()
		   g.AttestedAt = &t
	  case SnapExpired:
		   t := now.UTC()
		   g.ExpiredAt = &t
	   case SnapRevoked:
		   t := now.UTC()
		   g.RevokedAt = &t
	   }
	  // sync gensBySnap
	  if gs, ok2 := m.gensBySnap[g.SnapshotID]; ok2 {
		   *gs = *g
	  }
	  return nil
   }

   func (m *MemStore) ListGensByState(_ context.Context, state SnapGenState, limit int) ([]*PorSnapshotGen, error) {
	   var out []*PorSnapshotGen
	   for _, g := range m.gens {
		  if g.State == state {
			  cp := *g
			   out = append(out, &cp)
		  }
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out, nil
   }

   func (m *MemStore) ListExpiredGens(_ context.Context, now time.Time, limit int) ([]*PorSnapshotGen, error) {
	   var out []*PorSnapshotGen
	  for _, g := range m.gens {
		  if g.State == SnapSealed && now.After(g.ExpiresAt()) {
			  cp := *g
			  out = append(out, &cp)
		  }
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
   }

   func (m *MemStore) InsertAttest(_ context.Context, a *AttestationBundle) error {
	  if _, ok := m.attests[a.AttestID]; ok {
		   return ErrAttestBound
	  }
	   cp := *a
	   m.attests[a.AttestID] = &cp
	   return nil
   }

  func (m *MemStore) GetAttest(_ context.Context, id string) (*AttestationBundle, error) {
	  a, ok := m.attests[id]
	  if !ok {
		  return nil, ErrAttestNotFound
	  }
	   cp := *a
	   return &cp, nil
   }

   func (m *MemStore) GetAttestByGen(_ context.Context, snapGenID string) (*AttestationBundle, error) {
	  for _, a := range m.attests {
		  if a.SnapGenID == snapGenID && a.State == AttestPublished {
			   cp := *a
			   return &cp, nil
		  }
	  }
	  return nil, ErrAttestNotFound
  }

   func (m *MemStore) UpdateAttestState(_ context.Context, id string, from, to AttestState, reason string, now time.Time) error {
	  a, ok := m.attests[id]
	  if !ok {
		  return ErrAttestNotFound
	   }
	   if a.State != from {
		   return ErrInvalidTransition
	  }
	  a.State = to
	  a.RevokeReason = reason
	  a.UpdatedAt = now
	  switch to {
	  case AttestPublished:
		   t := now.UTC()
		   a.PublishedAt = &t
	  case AttestRevoked:
		  t := now.UTC()
		   a.RevokedAt = &t
	   }
	   return nil
   }

   func (m *MemStore) ListAttestsByState(_ context.Context, state AttestState, limit int) ([]*AttestationBundle, error) {
	   var out []*AttestationBundle
	  for _, a := range m.attests {
		  if a.State == state {
			  cp := *a
			  out = append(out, &cp)
		  }
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
  }

   func (m *MemStore) InsertDriftProof(_ context.Context, p *PorSnapDriftProof) error {
	  if _, ok := m.proofs[p.ProofID]; ok {
		  return ErrDriftProofConflict
	  }
	   cp := *p
	  m.proofs[p.ProofID] = &cp
	   return nil
  }

  func (m *MemStore) GetDriftProof(_ context.Context, id string) (*PorSnapDriftProof, error) {
	   p, ok := m.proofs[id]
	   if !ok {
		  return nil, persist.ErrNotFound
	   }
	   cp := *p
	   return &cp, nil
  }

  func (m *MemStore) ListDriftProofsByGen(_ context.Context, snapGenID string, limit int) ([]*PorSnapDriftProof, error) {
	  var out []*PorSnapDriftProof
	  for _, p := range m.proofs {
		   if p.SnapGenID == snapGenID {
			   cp := *p
			   out = append(out, &cp)
		   }
		   if len(out) >= limit {
			   break
		  }
	   }
	   return out, nil
  }

// Compile-time interface assertions.
var _ Store = (*SQLStore)(nil)
var _ Store = (*MemStore)(nil)
