// @FileName: gate.go
// @Summary: 在途世代闸核心引擎（HTTP 层适配器）。
// 封堵缺口：
// 1. AcquireRef 存续期 BeginRevoking/Remove 并发 → 广播路径拒旧地址世代，产出 InflightDriftCase。
// 2. ReleaseRef 后持旧 inflight_gen 广播 → VerifyBroadcast 检测并拒绝。
// 3. 同一 movement_id 重复 Acquire → 幂等；跨 entry 冲突 → 显式失败。
// 4. 条目已 revoked 仍广播成功 → DriftCase 产出，后续广播路径被阻断。
// 5. 证明缺失不得放行广播路径。
// 本包基于 destlife.InflightEngine，不重写 destallow 冷却主链或白名单 CRUD 内核。
// @Author: Cason
// @Created: 2021.07.26

package inflightgengate

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// Gate 在途世代闸主引擎（HTTP 层）。
// 调用方（出金广播路径）须按以下顺序调用：
// 1. Gate.AcquireRef(ctx, req) — 签发在途世代租约
// 2. Gate.BindRef(ctx, leaseID) — 绑定广播世代（记录 BoundRevokeGen）
// 3. Gate.VerifyBroadcast(ctx, req) — 校验世代一致性，写入证明
// 4. 调用方执行链上提交
// 5. Gate.ReleaseRef(ctx, leaseID, actor) — 释放引用
// 吊销路径（由运营/安全系统调用）：
// 1. Gate.BeginRevoking(ctx, req) — 发起延迟吊销（存在活跃引用时进入 revoking）
// 2. Gate.CloseDriftCase(ctx, ...) — 四眼收口漂移案例（解除广播阻断）
type Gate struct {
	eng *destlife.InflightEngine
	cfg GateConfig
}

// NewGate 构造在途世代闸。
  func NewGate(
	  leases destlife.GenLeaseStore,
	   revokes destlife.RevokeGenStore,
	  drifts destlife.DriftCaseStore,
	   proofs destlife.ProofStore,
	  cfg GateConfig,
  ) *Gate {
	   eng := destlife.NewInflightEngine(
		  leases, revokes, drifts, proofs,
		   destlife.EngineConfig{LeaseTTL: cfg.leaseTTL()},
	   )
	   return &Gate{eng: eng, cfg: cfg}
  }

// WithClock 注入时钟。
   func (g *Gate) WithClock(c func() interface{}) *Gate { return g }

// Engine 返回底层引擎（供 worker 直接访问）。

  func (g *Gate) Engine() *destlife.InflightEngine { return g.eng }

// ─────────────────────────────────────────────────────────────────────────────
// AcquireRef
// ─────────────────────────────────────────────────────────────────────────────
// AcquireRefInput AcquireRef 输入。
type AcquireRefInput struct {
	EntryID string `json:"entry_id" binding:"required"`
	UserID string `json:"user_id" binding:"required"`
	Chain      string `json:"chain" binding:"required"`
	Address    string `json:"address" binding:"required"`
	MovementID string `json:"movement_id" binding:"required"`
}

// AcquireRef 签发在途世代租约。

   func (g *Gate) AcquireRef(ctx context.Context, in AcquireRefInput) (AcquireRefResp, error) {
	  res, err := g.eng.AcquireRef(ctx, destlife.AcquireRefReq{
		  EntryID: in.EntryID,
		  UserID: in.UserID,
		   Chain: in.Chain,
		  Address: in.Address,
		   MovementID: in.MovementID,
		   Actor: g.cfg.Actor,
	  })
	   if err != nil {
		   return AcquireRefResp{}, err
	  }
	  return AcquireRefResp{
		  LeaseID: res.Lease.LeaseID,
		   EntryID: res.Lease.EntryID,
		  MovementID: res.Lease.MovementID,
		   InflightGen: res.Lease.InflightGen,
		   BoundRevokeGen: res.Lease.BoundRevokeGen,
		   State:          res.Lease.State,
		   IssuedAt:       res.Lease.IssuedAt,
		  ExpiresAt:      res.Lease.ExpiresAt,
		  Idempotent: res.Idempotent,
	   }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// BindRef
// ─────────────────────────────────────────────────────────────────────────────
// BindRef 将 acquired 引用绑定到出金广播，快照当前吊销世代。
  func (g *Gate) BindRef(ctx context.Context, leaseID string) (BindRefResp, error) {
	  lease, err := g.eng.BindRef(ctx, leaseID)
	  if err != nil {
		  return BindRefResp{}, err
	   }
	  return BindRefResp{
		  LeaseID:        lease.LeaseID,
		  EntryID:        lease.EntryID,
		   BoundRevokeGen: lease.BoundRevokeGen,
		  State:   lease.State,
		  BoundAt: lease.BoundAt,
	  }, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef
// ─────────────────────────────────────────────────────────────────────────────
// ReleaseRef 释放在途引用（出金完成或回滚时调用）。
  func (g *Gate) ReleaseRef(ctx context.Context, leaseID, actor string) (ReleaseRefResp, error) {
	   lease, err := g.eng.GetRef(ctx, leaseID)
	   if err != nil {
		  return ReleaseRefResp{}, err
	   }
	  if err := g.eng.ReleaseRef(ctx, leaseID, actor); err != nil {
		   return ReleaseRefResp{}, err
	  }
	  released, err := g.eng.GetRef(ctx, leaseID)
	  if err != nil {
		   return ReleaseRefResp{}, err
	   }
	   return ReleaseRefResp{
		  LeaseID:    lease.LeaseID,
		   EntryID:    lease.EntryID,
		  State:      released.State,
		   ReleasedAt: released.ReleasedAt,
	  }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// VerifyBroadcast
// ─────────────────────────────────────────────────────────────────────────────
// VerifyBroadcastInput VerifyBroadcast 输入。

type VerifyBroadcastInput struct {
	LeaseID       string `json:"lease_id" binding:"required"`
	MovementID    string `json:"movement_id" binding:"required"`
	BroadcastTxID string `json:"broadcast_tx_id,omitempty"`
}

// VerifyBroadcast 校验出金广播世代有效性，写入在途证明。

  func (g *Gate) VerifyBroadcast(ctx context.Context, in VerifyBroadcastInput) (VerifyBroadcastResp, error) {
	   res, err := g.eng.VerifyBroadcast(ctx, destlife.BroadcastVerifyReq{
		   LeaseID: in.LeaseID,
		   MovementID:    in.MovementID,
		  BroadcastTxID: in.BroadcastTxID,
	   })
	   if err != nil && !res.OK {
		  return VerifyBroadcastResp{
			   OK:          false,
			  LeaseID:     res.LeaseID,
			   EntryID:     res.EntryID,
			   InflightGen: res.InflightGen,
			   BoundRevokeGen: res.BoundRevokeGen,
			  CurrentRevokeGen: res.CurrentRevokeGen,
			   EntryState:   res.EntryState,
			   RejectReason: res.RejectReason,
		  }, err
	  }
	  return VerifyBroadcastResp{
		   OK: res.OK,
		   LeaseID:          res.LeaseID,
		   EntryID:          res.EntryID,
		   InflightGen:      res.InflightGen,
		  BoundRevokeGen:   res.BoundRevokeGen,
		   CurrentRevokeGen: res.CurrentRevokeGen,
		   EntryState: res.EntryState,
	   }, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// BeginRevoking
// ─────────────────────────────────────────────────────────────────────────────
// BeginRevokingInput BeginRevoking 输入。
type BeginRevokingInput struct {
	EntryID string `json:"entry_id" binding:"required"`
	UserID  string `json:"user_id,omitempty"`
	Chain string `json:"chain,omitempty"`
	Address string `json:"address,omitempty"`
	Actor   string `json:"actor,omitempty"`
}

// BeginRevoking 发起延迟吊销（存在活跃引用时进入 revoking）。

   func (g *Gate) BeginRevoking(ctx context.Context, in BeginRevokingInput) (BeginRevokingResp, error) {
	  actor := in.Actor
	   if actor == "" {
		   actor = g.cfg.Actor
	  }
	   res, err := g.eng.BeginRevoking(ctx, destlife.RevokeReq{
		  EntryID: in.EntryID,
		   UserID: in.UserID,
		   Chain:  in.Chain,
		  Address: in.Address,
		   Actor:   actor,
	   })
	  if err != nil {
		  return BeginRevokingResp{}, err
	  }
	  return BeginRevokingResp{
		  EntryID: res.EntryID,
		   Gen:     res.Gen,
		   State:   res.State,
		  ActiveRefs: res.ActiveRefs,
		   Deferred:   res.Deferred,
	   }, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// 只读查询
// ─────────────────────────────────────────────────────────────────────────────
// GetRef 按 lease_id 查询在途引用。

  func (g *Gate) GetRef(ctx context.Context, leaseID string) (destlife.InflightGenLease, error) {
	   return g.eng.GetRef(ctx, leaseID)
  }

// GetRefByMovement 按 (entry_id, movement_id) 查询在途引用。

  func (g *Gate) GetRefByMovement(ctx context.Context, entryID, movementID string) (destlife.InflightGenLease, error) {
	  return g.eng.GetRefByMovement(ctx, entryID, movementID)
   }

// ListActiveRefs 列出 entry_id 下所有活跃引用。

  func (g *Gate) ListActiveRefs(ctx context.Context, entryID string) ([]destlife.InflightGenLease, error) {
	   return g.eng.ListActiveRefs(ctx, entryID)
   }

// GetRevokeGen 查询条目吊销世代。

   func (g *Gate) GetRevokeGen(ctx context.Context, entryID string) (RevokeGenResp, error) {
	  rg, err := g.eng.GetRevokeGeneration(ctx, entryID)
	  if err != nil {
		  return RevokeGenResp{}, err
	  }
	   return RevokeGenResp{
		   EntryID: rg.EntryID,
		   UserID:  rg.UserID,
		   Chain:   rg.Chain,
		  Address: rg.Address,
		   Gen:        rg.Gen,
		   State:      rg.State,
		   ActiveRefs: rg.ActiveRefs,
	  }, nil
  }

// GetProof 按 (lease_id, movement_id) 查询在途证明。
   func (g *Gate) GetProof(ctx context.Context, leaseID, movementID string) (ProofResp, error) {
	   p, err := g.eng.GetProof(ctx, leaseID, movementID)
	  if err != nil {
		   return ProofResp{}, err
	   }
	   return toProofResp(p), nil
   }

// GetOpenDriftCase 查询 entry_id 下未关闭漂移案例。

  func (g *Gate) GetOpenDriftCase(ctx context.Context, entryID string) (DriftCaseResp, error) {
	  dc, err := g.eng.GetOpenDriftCase(ctx, entryID)
	  if err != nil {
		  return DriftCaseResp{}, err
	  }
	   return toDriftCaseResp(dc), nil
  }

// CloseDriftCase 四眼收口漂移案例。

   func (g *Gate) CloseDriftCase(ctx context.Context, caseID, closedBy, resolution string) (DriftCaseResp, error) {
	  if closedBy == "" || resolution == "" {
		   return DriftCaseResp{}, fmt.Errorf("inflightgengate: closed_by and resolution required")
	   }
	   dc, err := g.eng.CloseDriftCase(ctx, caseID, closedBy, resolution)
	  if err != nil {
		   return DriftCaseResp{}, err
	  }
	  return toDriftCaseResp(dc), nil
   }

// ListOpenDriftCases 列出所有未关闭漂移案例。
  func (g *Gate) ListOpenDriftCases(ctx context.Context, limit int) ([]DriftCaseResp, error) {
	  cases,
		  err := g.eng.ListOpenDriftCases(ctx, limit)
	   if err != nil {
		  return nil, err
	   }
	  out := make([]DriftCaseResp, len(cases))
	  for i, dc := range cases {
		  out[i] = toDriftCaseResp(dc)
	  }
	  return out, nil
  }
