package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// 本层 Row 类型（不依赖 dualrelease 包）
// ──────────────────────────────────────────────────
// DualReleaseCaseRow 放行案例 SQL 行。
type DualReleaseCaseRow struct {
	ID string
	RequestID string
	TicketID     string
	Gen          int64
	State        string
	Actor        string
	FundActionID string
	EffectProofID string
	AbortReason string
	FenceEpoch int64
	FenceToken string
	CreatedAt  time.Time
	UpdatedAt time.Time
}

// DualReleaseLeaseRow 放行互斥租约 SQL 行。
type DualReleaseLeaseRow struct {
	ID string
	RequestID string
	Holder string
	Gen        int64
	State      string
	LeaseToken string
	AcquiredAt time.Time
	ExpiresAt time.Time
	ReleasedAt sql.NullTime
	FenceEpoch int64
	FenceToken string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DualTicketExpiryRow 票证老化审计 SQL 行。

type DualTicketExpiryRow struct {
	ID        string
	TicketID  string
	RequestID string
	Reason    string
	EpochAt   int64
	TTLSecs   int
	ExpiredAt time.Time
	Actor     string
	AuditHash string
	CreatedAt time.Time
}

// DualEffectProofRow 效果证明 SQL 行。

type DualEffectProofRow struct {
	ID        string
	RequestID string
	TicketID     string
	Gen          int64
	FundActionID string
	Actor        string
	ProofHash string
	SignedAt time.Time
	FenceEpoch int64
	FenceToken string
	CreatedAt time.Time
}

// ──────────────────────────────────────────────────
// DualRelease SQL 仓储
// ──────────────────────────────────────────────────
// DualRelease SQL 仓储。

type DualReleaseRepo struct{ db *persist.SQLDB }

// NewDualRelease 初始化 SQL 仓储。

  func NewDualReleaseRepo(db *persist.SQLDB) *DualReleaseRepo { return &DualReleaseRepo{db: db} }

  func (r *DualReleaseRepo) on() bool { return sqlOn(r.db) }

// ──────────────────────────────────────────────────
// ReleaseCase CRUD
// ──────────────────────────────────────────────────
// PutCase 幂等写入或更新 ReleaseCase。
  func (r *DualReleaseRepo) PutCase(rc DualReleaseCaseRow) error {
	   if !r.on() {
		  return ErrUnavailable
	  }
	  ctx := context.Background()
	   q := `INSERT INTO dual_release_cases
(id, request_id, ticket_id, gen, state, actor, fund_action_id, effect_proof_id, abort_reason,
 fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 ticket_id=VALUES(ticket_id), gen=VALUES(gen), state=VALUES(state), actor=VALUES(actor),
 fund_action_id=VALUES(fund_action_id), effect_proof_id=VALUES(effect_proof_id),
 abort_reason=VALUES(abort_reason), fence_epoch=VALUES(fence_epoch),
 fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO dual_release_cases
(id, request_id, ticket_id, gen, state, actor, fund_action_id, effect_proof_id, abort_reason,
 fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (request_id) DO UPDATE SET
 ticket_id=EXCLUDED.ticket_id, gen=EXCLUDED.gen, state=EXCLUDED.state, actor=EXCLUDED.actor,
 fund_action_id=EXCLUDED.fund_action_id, effect_proof_id=EXCLUDED.effect_proof_id,
 abort_reason=EXCLUDED.abort_reason, fence_epoch=EXCLUDED.fence_epoch,
 fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q,
		   rc.ID, rc.RequestID, rc.TicketID, rc.Gen, rc.State, rc.Actor,
		   rc.FundActionID, rc.EffectProofID, rc.AbortReason,
		  rc.FenceEpoch, rc.FenceToken, rc.CreatedAt, rc.UpdatedAt,
	   )
	   return err
  }

// GetCase 按 requestID 读取 ReleaseCase。

  func (r *DualReleaseRepo) GetCase(requestID string) (DualReleaseCaseRow, error) {
	   if !r.on() {
		  return DualReleaseCaseRow{}, ErrUnavailable
	  }
	  ctx := context.Background()
	  var rc DualReleaseCaseRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, request_id, ticket_id, gen, state, actor, fund_action_id, effect_proof_id, 
		        abort_reason, fence_epoch, fence_token, created_at, updated_at
		 FROM dual_release_cases WHERE request_id=?`, requestID,
	   ).Scan(&rc.ID,
		  &rc.RequestID, &rc.TicketID, &rc.Gen, &rc.State,
		  &rc.Actor, &rc.FundActionID, &rc.EffectProofID, &rc.AbortReason,
		  &rc.FenceEpoch, &rc.FenceToken, &rc.CreatedAt, &rc.UpdatedAt,
	   )
	  if errors.Is(err, sql.ErrNoRows) {
		   return DualReleaseCaseRow{}, ErrNotFound
	  }
	  return rc, err
  }

// ListExpiredCases 返回租约已过期但案例未终结的 cases。

  func (r *DualReleaseRepo) ListExpiredCases(now time.Time) ([]DualReleaseCaseRow, error) {
	  if !r.on() {
		  return nil, ErrUnavailable
	   }
	   ctx := context.Background()
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT c.id, c.request_id, c.ticket_id, c.gen, c.state, c.actor, c.fund_action_id,
		        c.effect_proof_id, c.abort_reason, c.fence_epoch, c.fence_token, c.created_at, c.updated_at
		 FROM dual_release_cases c
		 JOIN dual_release_leases l ON l.request_id = c.request_id
		 WHERE c.state NOT IN ('executed','expired','aborted') AND l.expires_at < ?
		 LIMIT 200`, now,
	  )
	   if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	   return scanCaseRows(rows)
   }

// ListStaleLeaseCases 返回租约超时但 state 仍为 unlocking/executing 的案例。

   func (r *DualReleaseRepo) ListStaleLeaseCases(now time.Time) ([]DualReleaseCaseRow, error) {
	  if !r.on() {
		  return nil, ErrUnavailable
	   }
	  ctx := context.Background()
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT c.id, c.request_id, c.ticket_id, c.gen, c.state, c.actor, c.fund_action_id,
		        c.effect_proof_id, c.abort_reason, c.fence_epoch, c.fence_token, c.created_at, c.updated_at
		 FROM dual_release_cases c
		 JOIN dual_release_leases l ON l.request_id = c.request_id
		 WHERE c.state IN ('unlocking','executing') AND l.expires_at < ?
		 LIMIT 200`, now,
	   )
	  if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   return scanCaseRows(rows)
   }

// ListExecutingCases 返回所有 state==executing 的案例。

   func (r *DualReleaseRepo) ListExecutingCases() ([]DualReleaseCaseRow, error) {
	   if !r.on() {
		  return nil, ErrUnavailable
	  }
	  ctx := context.Background()
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, request_id, ticket_id, gen, state, actor, fund_action_id,
		        effect_proof_id, abort_reason, fence_epoch, fence_token, created_at, updated_at
		 FROM dual_release_cases WHERE state='executing' LIMIT 200`,
	  )
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   return scanCaseRows(rows)
   }

   func scanCaseRows(rows *sql.Rows) ([]DualReleaseCaseRow, error) {
	  out := make([]DualReleaseCaseRow,
		   0)
	   for rows.Next() {
		  var rc DualReleaseCaseRow
		  if err := rows.Scan(&rc.ID, &rc.RequestID, &rc.TicketID, &rc.Gen, &rc.State,
			  &rc.Actor, &rc.FundActionID, &rc.EffectProofID, &rc.AbortReason,
			  &rc.FenceEpoch, &rc.FenceToken, &rc.CreatedAt, &rc.UpdatedAt,
		  ); err != nil {
			   return nil, err
		   }
		   out = append(out, rc)
	  }
	  return out, rows.Err()
   }

// ──────────────────────────────────────────────────
// ReleaseLease CRUD
// ──────────────────────────────────────────────────
// PutLease 幂等写入或更新 ReleaseLease。
  func (r *DualReleaseRepo) PutLease(l DualReleaseLeaseRow) error {
	  if !r.on() {
		   return ErrUnavailable
	  }
	   if l.ID == "" {
		   l.ID = uuid.NewString()
	  }
	   ctx := context.Background()
	  var releasedAt interface{}
	  if l.ReleasedAt.Valid {
		   releasedAt = l.ReleasedAt.Time
	   }
	   q := `INSERT INTO dual_release_leases
(id, request_id, holder, gen, state, lease_token, acquired_at, expires_at, released_at,
 fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 holder=VALUES(holder), gen=VALUES(gen), state=VALUES(state), lease_token=VALUES(lease_token),
 acquired_at=VALUES(acquired_at), expires_at=VALUES(expires_at), released_at=VALUES(released_at),
 fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO dual_release_leases
(id, request_id, holder, gen, state, lease_token, acquired_at, expires_at, released_at,
 fence_epoch, fence_token, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (request_id) DO UPDATE SET
 holder=EXCLUDED.holder, gen=EXCLUDED.gen, state=EXCLUDED.state, lease_token=EXCLUDED.lease_token,
 acquired_at=EXCLUDED.acquired_at, expires_at=EXCLUDED.expires_at, released_at=EXCLUDED.released_at,
 fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, updated_at=EXCLUDED.updated_at`
	  }
	   _,
		   err := r.db.Conn(ctx).ExecContext(ctx, q,
		   l.ID, l.RequestID, l.Holder, l.Gen, l.State, l.LeaseToken,
		  l.AcquiredAt, l.ExpiresAt, releasedAt,
		  l.FenceEpoch,
		   l.FenceToken, l.CreatedAt, l.UpdatedAt,
	   )
	   return err
  }

// GetLease 按 requestID 读取 ReleaseLease。

   func (r *DualReleaseRepo) GetLease(requestID string) (DualReleaseLeaseRow, error) {
	   if !r.on() {
		  return DualReleaseLeaseRow{}, ErrUnavailable
	  }
	  ctx := context.Background()
	  var l DualReleaseLeaseRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, request_id, holder, gen, state, lease_token, acquired_at, expires_at,
		        released_at, fence_epoch, fence_token, created_at, updated_at
		 FROM dual_release_leases WHERE request_id=?`, requestID,
	  ).Scan(&l.ID, &l.RequestID, &l.Holder, &l.Gen, &l.State,
		  &l.LeaseToken, &l.AcquiredAt, &l.ExpiresAt, &l.ReleasedAt,
		   &l.FenceEpoch, &l.FenceToken, &l.CreatedAt, &l.UpdatedAt,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		   return DualReleaseLeaseRow{}, ErrNotFound
	  }
	  return l, err
   }

// ──────────────────────────────────────────────────
// TicketExpiry 追加写
// ──────────────────────────────────────────────────
// AppendExpiry 追加票证老化审计记录（只写不改）。

   func (r *DualReleaseRepo) AppendExpiry(e DualTicketExpiryRow) error {
	   if !r.on() {
		   return ErrUnavailable
	   }
	  if e.ID == "" {
		   e.ID = uuid.NewString()
	  }
	  ctx := context.Background()
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT IGNORE INTO dual_ticket_expiries 
(id, ticket_id, request_id, reason, epoch_at, ttl_secs, expired_at, actor, audit_hash, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`,
		  e.ID, e.TicketID, e.RequestID, e.Reason, e.EpochAt, e.TTLSecs,
		  e.ExpiredAt, e.Actor, e.AuditHash, e.CreatedAt,
	   )
	   return err
   }

// ──────────────────────────────────────────────────
// EffectProof CRUD
// ──────────────────────────────────────────────────
// PutProof 幂等写入效果证明（同 requestID+gen 唯一约束）。

   func (r *DualReleaseRepo) PutProof(p DualEffectProofRow) error {
	   if !r.on() {
		   return ErrUnavailable
	   }
	  if p.ID == "" {
		   p.ID = uuid.NewString()
	  }
	  ctx := context.Background()
	  q := `INSERT INTO dual_effect_proofs
(id, request_id, ticket_id, gen, fund_action_id, actor, proof_hash,
 signed_at, fence_epoch, fence_token, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 fund_action_id=VALUES(fund_action_id), actor=VALUES(actor),
 proof_hash=VALUES(proof_hash), signed_at=VALUES(signed_at),
 fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO dual_effect_proofs
(id, request_id, ticket_id, gen, fund_action_id, actor, proof_hash,
 signed_at, fence_epoch, fence_token, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (request_id, gen) DO UPDATE SET
 fund_action_id=EXCLUDED.fund_action_id, actor=EXCLUDED.actor,
 proof_hash=EXCLUDED.proof_hash, signed_at=EXCLUDED.signed_at,
 fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q,
		   p.ID, p.RequestID, p.TicketID, p.Gen, p.FundActionID, p.Actor, p.ProofHash,
		  p.SignedAt,
		   p.FenceEpoch, p.FenceToken, p.CreatedAt,
	  )
	   return err
  }

// GetProof 按 requestID + gen 读取效果证明。

  func (r *DualReleaseRepo) GetProof(requestID string, gen int64) (DualEffectProofRow, error) {
	   if !r.on() {
		   return DualEffectProofRow{}, ErrUnavailable
	  }
	  ctx := context.Background()
	   var p DualEffectProofRow
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, request_id, ticket_id, gen, fund_action_id, actor, proof_hash,
		        signed_at, fence_epoch, fence_token, created_at
		 FROM dual_effect_proofs WHERE request_id=? AND gen=?`, requestID, gen,
	   ).Scan(&p.ID, &p.RequestID, &p.TicketID, &p.Gen,
		   &p.FundActionID, &p.Actor, &p.ProofHash,
		   &p.SignedAt,
		   &p.FenceEpoch, &p.FenceToken, &p.CreatedAt,
	   )
	   if errors.Is(err, sql.ErrNoRows) {
		   return DualEffectProofRow{}, ErrNotFound
	   }
	  return p, err
  }

// ListProofs 按 requestID 列举所有世代的效果证明（按 gen 升序）。
  func (r *DualReleaseRepo) ListProofs(requestID string) ([]DualEffectProofRow, error) {
	   if !r.on() {
		  return nil, ErrUnavailable
	  }
	  ctx := context.Background()
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, request_id, ticket_id, gen, fund_action_id, actor, proof_hash,
		        signed_at, fence_epoch, fence_token, created_at
		 FROM dual_effect_proofs WHERE request_id=? ORDER BY gen ASC`, requestID,
	   )
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]DualEffectProofRow, 0)
	   for rows.Next() {
		   var p DualEffectProofRow
		   if err := rows.Scan(&p.ID, &p.RequestID, &p.TicketID, &p.Gen,
			   &p.FundActionID, &p.Actor, &p.ProofHash,
			  &p.SignedAt, &p.FenceEpoch, &p.FenceToken, &p.CreatedAt,
		   ); err != nil {
			  return nil, err
		   }
		   out = append(out, p)
	  }
	  return out, rows.Err()
  }
// temporary guard removed after the production check
