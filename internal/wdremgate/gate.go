package wdremgate

import (
	"context"
	"errors"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// RemReleaseGate 出金分片剩余释放收口门控。
type RemReleaseGate struct {
	svc   *wdpartial.Service
	clock func() time.Time
}

// NewRemReleaseGate 构造 RemReleaseGate。

   func NewRemReleaseGate(svc *wdpartial.Service) *RemReleaseGate {
	   return &RemReleaseGate{
		  svc:   svc,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (g *RemReleaseGate) now() time.Time {
	   if g.clock != nil {
		   return g.clock()
	   }
	   return time.Now().UTC()
   }

// OpenHold 开立 ShardHold，SkipLeg=true 时同步创建 BroadcastGuardGen。
  func (g *RemReleaseGate) OpenHold(req GateOpenHoldReq) (*wdpartial.ShardHold, error) {
	   h, err := g.svc.OpenHold(wdpartial.OpenHoldReq{
		   PlanID: req.PlanID,
		   LegID: req.LegID,
		   UserID: req.UserID,
		   Asset:  req.Asset,
		   Amount: req.Amount,
		  ExpiryTTL: req.ExpiryTTL,
		  SkipLeg:   req.SkipLeg,
		  LiveHash: req.LiveHash,
	  })
	   if err != nil {
		   return nil, fmt.Errorf("wdremgate: OpenHold: %w", err)
	  }
	  return &h, nil
  }

// IssueLease 颁发剩余释放租约。

  func (g *RemReleaseGate) IssueLease(req GateIssueLeaseReq) (*GateIssueLeaseResp, error) {
	   ttl := time.Duration(req.TTLSeconds) * time.Second
	  if ttl <= 0 {
		   ttl = 5 * time.Minute
	   }
	  lease, err := g.svc.IssueLease(wdpartial.IssueLeaseReq{
		   PlanID: req.PlanID,
		   UserID: req.UserID,
		   TTL:    ttl,
	   })
	  if err != nil {
		   return nil, fmt.Errorf("wdremgate: IssueLease: %w", err)
	  }
	   return &GateIssueLeaseResp{
		   LeaseID: lease.ID,
		   Generation: lease.Generation,
		   ExpiresAt:  lease.ExpiresAt,
	  }, nil
  }

// Release 带世代剩余释放。
  func (g *RemReleaseGate) Release(req GateReleaseReq) (*GateReleaseResp, error) {
	   resp, err := g.svc.ReleasePlanRemainder(wdpartial.ReleasePlanRemainderReq{
		   PlanID:     req.PlanID,
		   LeaseID:    req.LeaseID,
		  Generation: req.Generation,
		  Actor: req.Actor,
	   })
	   if err != nil {
		   return nil, fmt.Errorf("wdremgate: Release: %w", err)
	   }
	  return &GateReleaseResp{
		   PlanID:        resp.PlanID,
		   ReleasedCount: len(resp.ReleasedHolds),
		  Remainder:     resp.Remainder,
	   }, nil
  }

// IssueFillMutex 颁发 FillMutexToken（ConfirmFill leftover 路径）。

   func (g *RemReleaseGate) IssueFillMutex(req wdpartial.IssueFillMutexReq) (*wdpartial.FillMutexToken, error) {
	  tok,
		   err := g.svc.IssueFillMutex(req)
	   if err != nil {
		   return nil, fmt.Errorf("wdremgate: IssueFillMutex: %w", err)
	  }
	   return tok, nil
   }

// ConfirmFill 链上确认成交分录（ShardHold leftover）。

   func (g *RemReleaseGate) ConfirmFill(req wdpartial.ConfirmFillReq) (wdpartial.PartialFill, error) {
	   f, err := g.svc.ConfirmFill(req)
	   if err != nil {
		  return wdpartial.PartialFill{}, fmt.Errorf("wdremgate: ConfirmFill: %w", err)
	  }
	   return f, nil
  }

// ConsumeHold 扣减 Hold（ShardHold leftover）。
   func (g *RemReleaseGate) ConsumeHold(req wdpartial.ConsumeHoldReq) (wdpartial.ShardHold, error) {
	  h, err := g.svc.ConsumeHold(req)
	  if err != nil {
		  return wdpartial.ShardHold{}, fmt.Errorf("wdremgate: ConsumeHold: %w", err)
	  }
	  return h, nil
  }

// ConsumeFillMutex 消费 FillMutexToken。

   func (g *RemReleaseGate) ConsumeFillMutex(tokenID string) error {
	   if err := g.svc.ConsumeFillMutex(tokenID); err != nil {
		  return fmt.Errorf("wdremgate: ConsumeFillMutex: %w", err)
	  }
	   return nil
  }

// Conserve 守恒校验（ShardHold leftover）。
  func (g *RemReleaseGate) Conserve(req GateConserveReq) (*GateConserveResp, error) {
	   summary, err := g.svc.HoldSummary(req.PlanID)
	   if err != nil {
		   return nil, fmt.Errorf("wdremgate: Conserve summary: %w", err)
	  }
	   resp := &GateConserveResp{
		  PlanID: req.PlanID,
		  Summary: summary,
	  }
	  conserveErr := g.svc.PlanConservation(req.PlanID)
	  if conserveErr != nil {
		   if errors.Is(conserveErr, wdpartial.ErrConservationFail) {
			   cases := g.svc.ListOpenConserveCases(1)
			  if len(cases) > 0 {
				   resp.CaseID = cases[len(cases)-1].ID
			  }
			   resp.Conserved = false
			   return resp, conserveErr
		   }
		  return nil, fmt.Errorf("wdremgate: Conserve: %w", conserveErr)
	  }
	   resp.Conserved = true
	  return resp, nil
   }

// ExportConserveCase 守恒证明导出。
   func (g *RemReleaseGate) ExportConserveCase(caseID string) (*GateConserveCaseExport, error) {
	   c, err := g.svc.GetConserveCase(caseID)
	   if err != nil {
		  return nil, fmt.Errorf("wdremgate: ExportConserveCase: %w", err)
	   }
	  summary, err := g.svc.HoldSummary(c.PlanID)
	  if err != nil {
		  return nil, fmt.Errorf("wdremgate: ExportConserveCase summary: %w", err)
	  }
	  return &GateConserveCaseExport{Case: c,
		  Summary: summary}, nil
  }

// ProveConserve 守恒案件证明固化。
  func (g *RemReleaseGate) ProveConserve(req GateProveConserveReq) (*wdpartial.ConserveCase, error) {
	   c, err := g.svc.ProveConserveCase(wdpartial.ProveConserveCaseReq{
		   CaseID:       req.CaseID,
		  ProofPayload: req.ProofPayload,
		  Approver:     req.Approver,
		  Approver2:    req.Approver2,
	   })
	  if err != nil {
		   return nil, fmt.Errorf("wdremgate: ProveConserve: %w", err)
	   }
	   return c, nil
  }

// CloseConserve 守恒案件收口。
   func (g *RemReleaseGate) CloseConserve(caseID, approver string) (*wdpartial.ConserveCase, error) {
	   c, err := g.svc.CloseConserveCase(wdpartial.CloseConserveCaseReq{
		   CaseID:   caseID,
		   Approver: approver,
	  })
	  if err != nil {
		   return nil, fmt.Errorf("wdremgate: CloseConserve: %w", err)
	  }
	   return c, nil
  }

// ForceRelease 双人确认强制释放。
  func (g *RemReleaseGate) ForceRelease(req GateForceReleaseReq) (*GateReleaseResp, error) {
	   if req.Approver == "" || req.Approver2 == "" {
		   return nil, wdpartial.ErrForceReleaseDenied
	  }
	   if req.Approver == req.Approver2 {
		  return nil, fmt.Errorf("%w: approver and approver2 must be different", wdpartial.ErrForceReleaseDenied)
	   }
	  if req.ProofPayload == "" {
		   return nil, wdpartial.ErrConserveCaseProofRequired
	   }
	  g.svc.ClearFillMutexForPlan(req.PlanID)
	  _,
		   err := g.svc.ForceCloseConserveCase(req.PlanID, req.CaseID, req.ProofPayload, req.Approver, req.Approver2)
	  if err != nil {
		  return nil, fmt.Errorf("wdremgate: ForceRelease close case: %w", err)
	  }
	  lease, err := g.svc.IssueLease(wdpartial.IssueLeaseReq{
		  PlanID: req.PlanID,
		  UserID: req.Approver,
		  TTL:    5 * time.Minute,
	   })
	   if err != nil {
		  return nil, fmt.Errorf("wdremgate: ForceRelease issue lease: %w", err)
	  }
	   resp, err := g.svc.ReleasePlanRemainder(wdpartial.ReleasePlanRemainderReq{
		  PlanID:     req.PlanID,
		  LeaseID:    lease.ID,
		  Generation: lease.Generation,
		   Actor:      req.Approver,
	  })
	   if err != nil {
		   return nil, fmt.Errorf("wdremgate: ForceRelease release: %w", err)
	   }
	   return &GateReleaseResp{
		  PlanID:        resp.PlanID,
		   ReleasedCount: len(resp.ReleasedHolds),
		  Remainder: resp.Remainder,
	   }, nil
  }

// InvalidateGuard 使守卫世代失效。

  func (g *RemReleaseGate) InvalidateGuard(req GateInvalidateGuardReq) (*wdpartial.BroadcastGuardGen, error) {
	  result, err := g.svc.InvalidateGuardGen(wdpartial.InvalidateGuardGenReq{
		   GuardGenID: req.GuardGenID,
		   Reason:     req.Reason,
	  })
	   if err != nil {
		  return nil, fmt.Errorf("wdremgate: InvalidateGuard: %w", err)
	  }
	  return result, nil
  }

// ExportRemReleaseGen 导出当前 rem_release_gen。

  func (g *RemReleaseGate) ExportRemReleaseGen(planID string) (int64, error) {
	   gen, err := g.svc.ExportRemReleaseGen(planID)
	   if err != nil {
		  return 0, fmt.Errorf("wdremgate: ExportRemReleaseGen: %w", err)
	  }
	  return gen, nil
   }

// RegisterHold 代理 store remgate RegisterHold。
   func (g *RemReleaseGate) RegisterHold(ctx context.Context, in wdpartial.HoldInput, liveHash string) (*wdpartial.HoldRecord, error) {
	   return g.svc.RegisterHold(ctx, in, liveHash)
  }

// RegisterFill 代理 store remgate RegisterFill。

   func (g *RemReleaseGate) RegisterFill(ctx context.Context, in wdpartial.FillInput) (*wdpartial.FillRecord, error) {
	   return g.svc.RegisterFill(ctx, in)
  }

// IssueFillMutexStore 代理 store remgate IssueFillMutex。

   func (g *RemReleaseGate) IssueFillMutexStore(ctx context.Context, req wdpartial.IssueFillMutexReq) (*wdpartial.FillMutexToken, string, error) {
	  return g.svc.IssueFillMutexStore(ctx, req)
   }

// ConfirmFillRecord 代理 store remgate ConfirmFill。

   func (g *RemReleaseGate) ConfirmFillRecord(ctx context.Context, req wdpartial.RemConfirmFillReq) (*wdpartial.FillRecord, error) {
	  return g.svc.ConfirmFillRecord(ctx, req)
  }

// ConsumeHoldRecord 代理 store remgate ConsumeHold。
   func (g *RemReleaseGate) ConsumeHoldRecord(ctx context.Context, req wdpartial.RemConsumeHoldReq) (*wdpartial.HoldRecord, error) {
	   return g.svc.ConsumeHoldRecord(ctx, req)
   }

// GetSummary 代理 store remgate GetSummary。
   func (g *RemReleaseGate) GetSummary(ctx context.Context, planID string) (*wdpartial.RemPlanFillSummary, error) {
	   return g.svc.GetSummary(ctx, planID)
   }
