package batchhopgate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ResumeLeaseTTL 续跳令牌默认有效期（30 分钟）。超时未绑定的 open 租约被 worker 标记为 expired。
const ResumeLeaseTTL = 30 * time.Minute

// ResumeLeaseFSM 管理 ResumeLease 生命周期。
type ResumeLeaseFSM struct {
	store Store
	clock func() time.Time
}

// NewResumeLeaseFSM 构造续跳互斥租约状态机。

  func NewResumeLeaseFSM(store Store) *ResumeLeaseFSM {
	   if store == nil {
		   store = NewMemStore()
	  }
	  return &ResumeLeaseFSM{store: store, clock: func() time.Time { return time.Now().UTC() }}
  }

   func (f *ResumeLeaseFSM) WithClock(c func() time.Time) *ResumeLeaseFSM {
	  if f != nil && c != nil {
		   f.clock = c
	  }
	   return f
  }

   // ─────────────────────────────────────────────────────────────────────────────
   // ValidateToken — 世代令牌验证
   // ─────────────────────────────────────────────────────────────────────────────
  // ValidateToken 验证调用方携带的世代令牌合法性。
  // 检查项：
   // 1. token.UserID == residual.UserID；
   // 2. token.Asset == residual.Asset；
  // 3. token.Generation == residual.Generation；
   // 4. token.ExpiresAt > now（未过期）。
  func (f *ResumeLeaseFSM) ValidateToken(token GenToken, residual *BatchResidualCase) error {
	   now := f.clock()
	   if token.UserID == "" || token.Asset == "" {
		   return ErrNoTokenProvided
	   }
	  if !token.Valid(now) {
		   return ErrLeaseExpired
	  }
	  if token.UserID != residual.UserID || token.Asset != residual.Asset {
		   return ErrTokenGenerationMismatch
	   }
	   if token.Generation != residual.Generation {
		   return ErrTokenGenerationMismatch
	   }
	  return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// Acquire — 申请续跳租约
// ─────────────────────────────────────────────────────────────────────────────
// AcquireReq 描述申请续跳租约的参数。
type AcquireReq struct {
	ResidualCaseID string
	UserID      string
	Asset       string
	Generation  int64
	ResumeFrom  int
	ConfirmedBy string
	Note string
}

// Acquire 为一次续跳操作申请互斥租约。
// 同一 residual_case_id 同一时刻只允许一条 open 或 bound 租约；
// 若已存在活跃租约则返回 ErrLeaseConflict（调用方应等前一租约过期或完成后重试）。
// ResumeFrom 须 >= BatchResidualCase.FailIndex，否则返回 ErrResumeIndexInvalid。
  func (f *ResumeLeaseFSM) Acquire(ctx context.Context, req AcquireReq) (*ResumeLease, error) {
	  if f == nil {
		  return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.UserID) == "" {
		  return nil, ErrUserInvalid
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		   return nil, ErrAssetInvalid
	   }
	   if strings.TrimSpace(req.ResidualCaseID) == "" {
		   return nil, ErrResidualNotFound
	   }
	   existing, err := f.store.FindActiveLease(ctx, req.ResidualCaseID)
	  if err != nil {
		  return nil, err
	  }
	   if existing != nil {
		  if !f.clock().After(existing.ExpiresAt) {
			  return nil, ErrLeaseConflict
		   }
		  // 已过期的活跃租约：将其标记 expired 再创建新租约。
		  existing.Status = LeaseExpired
		   existing.UpdatedAt = f.clock()
		  if err := f.store.SaveLease(ctx, existing); err != nil {
			  return nil, err
		   }
	  }
	  now := f.clock()
	   lease := &ResumeLease{
		   ID: uuid.New().String(),
		   ResidualCaseID: req.ResidualCaseID,
		  UserID:         req.UserID,
		  Asset: req.Asset,
		   Generation: req.Generation,
		   ResumeFrom: req.ResumeFrom,
		   Status:      LeaseOpen,
		   ExpiresAt:   now.Add(ResumeLeaseTTL),
		   ConfirmedBy: req.ConfirmedBy,
		   Note:        req.Note,
		  CreatedAt:   now,
		   UpdatedAt:   now,
	  }
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	   }
	  return lease, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Bind — open → bound（绑定续跳 idem 键）
// ─────────────────────────────────────────────────────────────────────────────
// Bind 将租约绑定至具体续跳 idem 键。
// 若 boundIdem 已存在于当前租约（幂等重试），直接返回已有租约；
// 若 boundIdem 与其他租约冲突，返回 ErrResumeConflict；
// 租约须处于 open 状态且未过期，否则返回 ErrLeaseExpired。

   func (f *ResumeLeaseFSM) Bind(ctx context.Context, leaseID, boundIdem string) (*ResumeLease, error) {
	   if f == nil {
		   return nil, ErrUnavailable
	  }
	   lease,
		  err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, ErrLeaseNotFound
	  }
	  now := f.clock()
	   if lease.Status == LeaseExpired || (lease.Status == LeaseOpen && now.After(lease.ExpiresAt)) {
		  if lease.Status != LeaseExpired {
			  lease.Status = LeaseExpired
			  lease.UpdatedAt = now
			   _ = f.store.SaveLease(ctx, lease)
		  }
		   return nil, ErrLeaseExpired
	  }
	  if lease.Status == LeaseDone {
		  return lease, nil
	   }
	   if lease.Status == LeaseBound {
		  if lease.BoundIdem == boundIdem {
			   return lease, nil
		  }
		  return nil, ErrResumeConflict
	  }
	  if lease.Status != LeaseOpen {
		   return nil, fmt.Errorf("%w: status=%s", ErrLeaseExpired, lease.Status)
	   }
	   lease.Status = LeaseBound
	   lease.BoundIdem = boundIdem
	  lease.UpdatedAt = now
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	  }
	  return lease, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// Done — bound → done（续跳完成）
// ─────────────────────────────────────────────────────────────────────────────
// Done 将续跳租约推进至 done 终态，表示续跳已成功执行。
// 须处于 bound 状态；done 幂等（重复调用直接返回已有租约）。

   func (f *ResumeLeaseFSM) Done(ctx context.Context, leaseID string) (*ResumeLease, error) {
	  if f == nil {
		   return nil, ErrUnavailable
	  }
	  lease, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, ErrLeaseNotFound
	  }
	   if lease.Status == LeaseDone {
		  return lease, nil
	  }
	  if lease.Status != LeaseBound {
		   return nil, fmt.Errorf("%w: status=%s, need bound", ErrLeaseExpired, lease.Status)
	  }
	  now := f.clock()
	  lease.Status = LeaseDone
	  t := now
	  lease.ConfirmedAt = &t
	  lease.UpdatedAt = now
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	   }
	   return lease, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// ExpireStale — worker 调用：批量过期超时未绑定的 open 租约
// ─────────────────────────────────────────────────────────────────────────────
// ExpireStale 扫描所有在当前时间点已超过 ExpiresAt 的 open 租约，将其标记为 expired。
// before 一般取 now，limit 限制单次处理批量。返回实际处理的租约数量。

   func (f *ResumeLeaseFSM) ExpireStale(ctx context.Context, before time.Time, limit int) (int, error) {
	   if f == nil {
		   return 0, ErrUnavailable
	  }
	  leases,
		   err := f.store.ListExpiredOpenLeases(ctx, before, limit)
	  if err != nil {
		  return 0, err
	  }
	   count := 0
	  now := f.clock()
	   for _, l := range leases {
		  if l.Status != LeaseOpen {
			   continue
		  }
		   l.Status = LeaseExpired
		  l.UpdatedAt = now
		  if err := f.store.SaveLease(ctx, l); err != nil {
			   continue
		  }
		   count++
	   }
	   return count, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// IssueToken — 颁发世代令牌
// ─────────────────────────────────────────────────────────────────────────────
// IssueToken 基于 BatchResidualCase 颁发 GenToken，供调用方在续跳时携带鉴权。
   func (f *ResumeLeaseFSM) IssueToken(residual *BatchResidualCase) GenToken {
	   now := f.clock()
	   return GenToken{
		   UserID: residual.UserID,
		  Asset:  residual.Asset,
		  Generation: residual.Generation,
		  IssuedAt:   now,
		  ExpiresAt:  now.Add(ResumeLeaseTTL),
	  }
  }
