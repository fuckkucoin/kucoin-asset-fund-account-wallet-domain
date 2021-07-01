package georebal

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/regioinv"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Planner 是计划编排器。Firewall 是区级防火墙检查接口（regioinv.Service）。
type Planner struct {
	st Store
	firewall Firewall
	clock func() time.Time
}

// Firewall 只读查询区状态（不写 custodyhalt，不改区 halt 状态）。
type Firewall interface {
	GetRegion(ctx context.Context, code string) (regioinv.Region, error)
	CheckRebalance(ctx context.Context, t regioinv.Transfer, now time.Time) error
	MoveQuoteRegions(ctx context.Context, t regioinv.Transfer, now time.Time) (regioinv.MoveQuote, error)
}

   func NewPlanner(st Store, fw Firewall) *Planner {
	   return &Planner{st: st, firewall: fw, clock: func() time.Time { return time.Now().UTC() }}
  }

   func (p *Planner) SetClock(fn func() time.Time) {
	  if fn != nil {
		  p.clock = fn
	  }
  }

   func (p *Planner) now() time.Time { return p.clock() }

// CreatePlan 校验防火墙后新建计划。区 halt 时返回 ErrRegionHalted。
   func (p *Planner) CreatePlan(ctx context.Context, req CreatePlanReq) (GeoRebalPlan, error) {
	   if strings.TrimSpace(req.Actor) == "" {
		   return GeoRebalPlan{}, ErrActor
	   }
	  idem := strings.TrimSpace(req.Idem)
	   if idem == "" {
		  return GeoRebalPlan{}, ErrInvalid
	  }
	   // 幂等检查
	  if existing, err := p.st.FindPlanByIdem(ctx, idem); err == nil {
		  if existing.SrcRegion != req.SrcRegion || existing.DstRegion != req.DstRegion ||
			  existing.Asset != strings.ToUpper(req.Asset) {
			   return GeoRebalPlan{}, ErrIdem
		   }
		   return existing.Clone(), nil
	  } else if !errors.Is(err, ErrNotFound) {
		  return GeoRebalPlan{}, err
	   }
	  // 区防火墙校验（不写 custodyhalt）
	   srcR,
		   err := p.firewall.GetRegion(ctx, req.SrcRegion)
	  if err != nil {
		   return GeoRebalPlan{}, regioinv.ErrNotFound
	   }
	   dstR, err := p.firewall.GetRegion(ctx, req.DstRegion)
	  if err != nil {
		   return GeoRebalPlan{},
			   regioinv.ErrNotFound
	   }
	   // 区 halt 时拒绝新建计划
	   if srcR.Halted || dstR.Halted {
		  return GeoRebalPlan{}, ErrRegionHalted
	   }
	   if srcR.Firewall == regioinv.FWBlocked || dstR.Firewall == regioinv.FWBlocked {
		   return GeoRebalPlan{}, ErrFirewallBlocked
	   }
	   amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return GeoRebalPlan{}, ErrInvalid
	   }
	  tier := strings.ToLower(strings.TrimSpace(req.Tier))
	   if tier == "" {
		   tier = regioinv.TierHot
	   }
	  // 防火墙校验再平衡合法性
	  tr := regioinv.Transfer{
		   SrcRegionID: srcR.ID,
		  DstRegionID: dstR.ID,
		  Tier: tier,
		  Asset:  strings.ToUpper(req.Asset),
		  Chain:  strings.ToLower(req.Chain),
		  Amount: amt.String(),
	  }
	  if err := p.firewall.CheckRebalance(ctx, tr, p.now()); err != nil {
		  return GeoRebalPlan{}, errors.Join(ErrFirewall, err)
	   }
	  now := p.now()
	   plan := GeoRebalPlan{
		  ID:    newPlanID(),
		   Idem:  idem,
		  Asset: strings.ToUpper(req.Asset),
		  Chain: strings.ToLower(req.Chain),
		  Tier:      tier,
		  SrcRegion: req.SrcRegion,
		  DstRegion: req.DstRegion,
		   Amount:    amt.String(),
		  State:     PlanDraft,
		  Approval: ApprovalPending,
		   Actor: req.Actor,
		   Reason: strings.TrimSpace(req.Reason),
		  Hooks:  req.Hooks,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	  // 生成单腿（当前代次每个计划一条腿；多腿扩展见 MoveQuote）
	  leg := GeoRebalLeg{
		  ID: newLegID(),
		  PlanID:    plan.ID,
		   Seq:       1,
		  SrcRegion: req.SrcRegion,
		   DstRegion: req.DstRegion,
		  Tier:      tier,
		  Asset: plan.Asset,
		  Chain: plan.Chain,
		   Amount: amt.String(),
		   State: LegQueued,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := p.st.SavePlan(ctx, plan); err != nil {
		   return GeoRebalPlan{}, err
	  }
	  if err := p.st.SaveLeg(ctx, leg); err != nil {
		   return GeoRebalPlan{}, err
	   }
	   plan.Legs = []GeoRebalLeg{leg}
	   return plan, nil
   }

// ApprovePlan 审批挂钩。通过后可调 ArmPlan；拒绝则中止。
  func (p *Planner) ApprovePlan(ctx context.Context, req ApprovePlanReq) (GeoRebalPlan, error) {
	  if strings.TrimSpace(req.Actor) == "" {
		   return GeoRebalPlan{}, ErrActor
	  }
	  plan, err := p.st.GetPlan(ctx, req.PlanID)
	   if err != nil {
		   return GeoRebalPlan{}, err
	  }
	   if plan.State != PlanDraft {
		  return GeoRebalPlan{}, ErrBadTransition
	  }
	  now := p.now()
	  if req.Approved {
		   plan.Approval = ApprovalApproved
	  } else {
		  plan.Approval = ApprovalRejected
		  plan.State = PlanAborted
		  plan.Code = CodeApproval
		   plan.Reason = strings.TrimSpace(req.Reason)
	   }
	   plan.UpdatedAt = now
	   if err := p.st.SavePlan(ctx, plan); err != nil {
		  return GeoRebalPlan{}, err
	   }
	  return plan.Clone(), nil
   }

// ArmPlan 将 draft→armed；审批未通过时拒绝。

   func (p *Planner) ArmPlan(ctx context.Context, req ArmPlanReq) (GeoRebalPlan, error) {
	   if strings.TrimSpace(req.Actor) == "" {
		  return GeoRebalPlan{}, ErrActor
	   }
	  plan, err := p.st.GetPlan(ctx, req.PlanID)
	   if err != nil {
		  return GeoRebalPlan{}, err
	  }
	  if !AllowPlanTransition(plan.State, PlanArmed) {
		  return GeoRebalPlan{}, ErrBadTransition
	  }
	  if plan.Approval == ApprovalRejected {
		  return GeoRebalPlan{}, ErrApproval
	   }
	   // 再次校验防火墙（区可能在审批期间改变状态）
	   srcR, err := p.firewall.GetRegion(ctx, plan.SrcRegion)
	  if err == nil && srcR.Halted {
		  return GeoRebalPlan{},
			  ErrRegionHalted
	  }
	  dstR, err := p.firewall.GetRegion(ctx, plan.DstRegion)
	  if err == nil && dstR.Halted {
		  return GeoRebalPlan{}, ErrRegionHalted
	   }
	   plan.State = PlanArmed
	   plan.UpdatedAt = p.now()
	  if err := p.st.SavePlan(ctx, plan); err != nil {
		   return GeoRebalPlan{}, err
	   }
	   return plan.Clone(), nil
   }

// AbortPlan 中止计划（任意非终态均可）。

  func (p *Planner) AbortPlan(ctx context.Context, req AbortPlanReq) (GeoRebalPlan, error) {
	  if strings.TrimSpace(req.Actor) == "" {
		   return GeoRebalPlan{}, ErrActor
	   }
	   plan, err := p.st.GetPlan(ctx, req.PlanID)
	  if err != nil {
		  return GeoRebalPlan{}, err
	  }
	   if plan.Terminal() {
		   return GeoRebalPlan{}, ErrBadTransition
	   }
	  plan.State = PlanAborted
	   plan.Code = "aborted"
	   plan.Reason = strings.TrimSpace(req.Reason)
	   plan.UpdatedAt = p.now()
	   if err := p.st.SavePlan(ctx, plan); err != nil {
		  return GeoRebalPlan{}, err
	   }
	   return plan.Clone(), nil
   }

// GetPlan 返回计划及全部腿。

   func (p *Planner) GetPlan(ctx context.Context, id string) (GeoRebalPlan, error) {
	  plan, err := p.st.GetPlan(ctx, id)
	   if err != nil {
		  return GeoRebalPlan{}, err
	   }
	  legs, err := p.st.ListLegs(ctx, id)
	  if err != nil {
		  return GeoRebalPlan{}, err
	  }
	  plan.Legs = legs
	  return plan.Clone(), nil
  }

// ListPlans 按状态过滤；为空时返回全部。

  func (p *Planner) ListPlans(ctx context.Context, states []string) ([]GeoRebalPlan, error) {
	   return p.st.ListPlans(ctx, states)
   }

// ConserveCheck 验证计划各腿之和 = 计划总量。
  func ConserveCheck(plan GeoRebalPlan, legs []GeoRebalLeg) error {
	   total, err := money.ParseBook(plan.Amount)
	  if err != nil {
		  return ErrConserve
	   }
	   sum := money.Zero()
	   for _, l := range legs {
		  a,
			  err := money.ParseBook(l.Amount)
		  if err != nil {
			   return ErrConserve
		  }
		  sum, err = sum.Add(a)
		   if err != nil {
			   return ErrConserve
		  }
	  }
	  if !sum.Equal(total) {
		  return ErrConserve
	  }
	   return nil
  }
