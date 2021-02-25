package batchhopgate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// BatchResidualFSM 管理 BatchResidualCase 的完整生命周期。

type BatchResidualFSM struct {
	store Store
	clock func() time.Time
}

// NewResidualFSM 构造残留案状态机。store 为 nil 时使用内存存储（仅用）。

  func NewResidualFSM(store Store) *BatchResidualFSM {
	  if store == nil {
		   store = NewMemStore()
	   }
	  return &BatchResidualFSM{store: store, clock: func() time.Time { return time.Now().UTC() }}
   }

   func (f *BatchResidualFSM) WithClock(c func() time.Time) *BatchResidualFSM {
	   if f != nil && c != nil {
		   f.clock = c
	  }
	   return f
   }

   // ─────────────────────────────────────────────────────────────────────────────
  // OpenResidual — 落库新残留案（BatchHop item i 失败时调用）
   // ─────────────────────────────────────────────────────────────────────────────
   // OpenResidualReq 描述落库参数。

  type OpenResidualReq struct {
	   UserID string
	   Asset string
	  Generation int64
	   TotalItems int
	  FailIndex     int
	  SucceededHops []HopSummary
	   FailedReason  string
	  BeforeRoot string
	   PartialRoot string
   }

  // OpenResidual 原子落库 BatchResidualCase（status=open）。
  // 若同用户同资产已存在非终态残留案（open/proven/resuming），返回 ErrConcurrentBatch——
   // 调用方须先等前一残留收口再重试。
  func (f *BatchResidualFSM) OpenResidual(ctx context.Context, req OpenResidualReq) (*BatchResidualCase, error) {
	  if f == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, ErrUserInvalid
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		   return nil, ErrAssetInvalid
	   }
	  existing, err := f.store.FindOpenResidual(ctx, req.UserID, req.Asset)
	   if err != nil {
		  return nil, err
	  }
	  if existing != nil {
		   return nil, ErrConcurrentBatch
	   }
	  now := f.clock()
	   c := &BatchResidualCase{
		  ID: uuid.New().String(),
		  UserID: req.UserID,
		   Asset:      req.Asset,
		   Generation: req.Generation,
		  TotalItems:    req.TotalItems,
		   FailIndex:     req.FailIndex,
		  SucceededHops: req.SucceededHops,
		  FailedReason: req.FailedReason,
		  BeforeRoot:   req.BeforeRoot,
		   PartialRoot: req.PartialRoot,
		  Status:     ResidualOpen,
		  DetectedAt: now,
		  UpdatedAt:  now,
	   }
	  if err := f.store.SaveResidual(ctx, c); err != nil {
		  return nil, err
	   }
	   return c, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// CheckConcurrent — 并行 BatchHop 互斥检测
// ─────────────────────────────────────────────────────────────────────────────
// CheckConcurrent 检查 user+asset 是否存在未闭合残留案。
// 返回 ErrConcurrentBatch 时调用方须拒绝新 BatchHop；返回 nil 表示可以安全发起。
   func (f *BatchResidualFSM) CheckConcurrent(ctx context.Context, userID, asset string) error {
	  if f == nil {
		   return ErrUnavailable
	   }
	   existing, err := f.store.FindOpenResidual(ctx, userID, asset)
	   if err != nil {
		   return err
	   }
	   if existing != nil {
		   return ErrConcurrentBatch
	   }
	   return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Prove — open → proven（附占用证明 ID）
// ─────────────────────────────────────────────────────────────────────────────
// Prove 将残留案状态从 open 推进至 proven，并记录 proof_id。
// 要求残留案当前状态为 open；否则返回 ErrResidualStatusInvalid。
  func (f *BatchResidualFSM) Prove(ctx context.Context, caseID, proofID string) (*BatchResidualCase, error) {
	   if f == nil {
		   return nil, ErrUnavailable
	  }
	   c, err := f.store.GetResidual(ctx, caseID)
	  if err != nil {
		   return nil, ErrResidualNotFound
	  }
	  if c.Status != ResidualOpen {
		   return nil, fmt.Errorf("%w: current=%s, need open", ErrResidualStatusInvalid, c.Status)
	   }
	   c.Status = ResidualProven
	  c.ProofID = proofID
	   c.UpdatedAt = f.clock()
	  if err := f.store.SaveResidual(ctx, c); err != nil {
		  return nil, err
	   }
	   return c, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// BindLease — proven → resuming（绑定续跳租约 ID）
// ─────────────────────────────────────────────────────────────────────────────
// BindLease 将残留案状态从 proven 推进至 resuming，并记录 lease_id。
// 要求残留案当前状态为 proven；否则返回 ErrResidualStatusInvalid。
   func (f *BatchResidualFSM) BindLease(ctx context.Context, caseID, leaseID string) (*BatchResidualCase, error) {
	   if f == nil {
		   return nil, ErrUnavailable
	   }
	  c, err := f.store.GetResidual(ctx, caseID)
	  if err != nil {
		   return nil, ErrResidualNotFound
	  }
	   if c.Status != ResidualProven {
		  return nil, fmt.Errorf("%w: current=%s, need proven", ErrResidualStatusInvalid, c.Status)
	   }
	   c.Status = ResidualResuming
	   c.LeaseID = leaseID
	   c.UpdatedAt = f.clock()
	  if err := f.store.SaveResidual(ctx, c); err != nil {
		  return nil, err
	  }
	  return c, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Close — resuming → closed（续跳完成收口）
// ─────────────────────────────────────────────────────────────────────────────
// Close 将残留案状态推进至 closed。
// 合法前置状态：resuming（续跳完成）或 open/proven（管理员强制收口）。
  func (f *BatchResidualFSM) Close(ctx context.Context, caseID, closedBy, note string) (*BatchResidualCase, error) {
	   if f == nil {
		  return nil, ErrUnavailable
	  }
	  c, err := f.store.GetResidual(ctx, caseID)
	   if err != nil {
		  return nil, ErrResidualNotFound
	   }
	  switch c.Status {
	  case ResidualOpen, ResidualProven, ResidualResuming:
		   // 合法前置
	  default:
		  return nil, fmt.Errorf("%w: current=%s", ErrResidualStatusInvalid, c.Status)
	  }
	  now := f.clock()
	  c.Status = ResidualClosed
	  c.Confirm1By = closedBy
	  t := now
	   c.Confirm1At = &t
	  c.Note = note
	   c.UpdatedAt = now
	  if err := f.store.SaveResidual(ctx, c); err != nil {
		  return nil, err
	  }
	  return c, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// ConfirmClose — 双人四眼确认收口
// ─────────────────────────────────────────────────────────────────────────────
// ConfirmClose 四眼确认机制：首次调用记录 confirm1_by；再次调用（不同人）记录 confirm2_by 并关闭。
// 规则：
// - confirm1_by 与 confirm2_by 不得为同一人（ErrSameConfirmer）。
// - 残留案须处于 proven/resuming 状态。
// - 完成双人确认后状态转为 closed。
  func (f *BatchResidualFSM) ConfirmClose(ctx context.Context, caseID, confirmerID, note string) (*BatchResidualCase, error) {
	   if f == nil {
		  return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(confirmerID) == "" {
		   return nil, ErrUserInvalid
	  }
	   c, err := f.store.GetResidual(ctx, caseID)
	   if err != nil {
		  return nil, ErrResidualNotFound
	   }
	   if c.Status != ResidualProven && c.Status != ResidualResuming {
		  return nil, fmt.Errorf("%w: current=%s, need proven or resuming", ErrResidualStatusInvalid, c.Status)
	   }
	   now := f.clock()
	   if c.Confirm1By == "" {
		  c.Confirm1By = confirmerID
		  t := now
		  c.Confirm1At = &t
		  c.Note = note
		   c.UpdatedAt = now
		   if err := f.store.SaveResidual(ctx, c); err != nil {
			  return nil, err
		   }
		  return c, nil
	   }
	   if c.Confirm1By == confirmerID {
		   return nil, ErrSameConfirmer
	   }
	  if c.Confirm2By != "" {
		  return c, nil
	   }
	  c.Confirm2By = confirmerID
	  t := now
	  c.Confirm2At = &t
	  c.Status = ResidualClosed
	  c.UpdatedAt = now
	   if err := f.store.SaveResidual(ctx, c); err != nil {
		   return nil, err
	  }
	   return c, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Block — * → blocked（审核拒绝）
// ─────────────────────────────────────────────────────────────────────────────
// Block 将残留案推进至 blocked 终态（审核拒绝或世代冲突）。
   func (f *BatchResidualFSM) Block(ctx context.Context, caseID, reason string) (*BatchResidualCase, error) {
	  if f == nil {
		  return nil, ErrUnavailable
	  }
	  c, err := f.store.GetResidual(ctx, caseID)
	   if err != nil {
		  return nil, ErrResidualNotFound
	  }
	   if c.Status == ResidualClosed || c.Status == ResidualBlocked {
		  return c, nil
	   }
	   c.Status = ResidualBlocked
	   c.Note = reason
	   c.UpdatedAt = f.clock()
	   if err := f.store.SaveResidual(ctx, c); err != nil {
		   return nil, err
	   }
	  return c, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// SnapshotRoot — 从 hops 构造快照根（sha256 摘要）
// ─────────────────────────────────────────────────────────────────────────────
// SnapshotRoot 对已成功 hop 列表和资产总计字符串生成 sha256 摘要，作为 before/partial 快照根。
// 用于确认残留案记录的完整性；不依赖外部密钥。
   func SnapshotRoot(hops []HopSummary, assetTotal string) string {
	  body, _ := json.Marshal(hops)
	  h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s", string(body), assetTotal)))
	  return fmt.Sprintf("%x", h)
  }
