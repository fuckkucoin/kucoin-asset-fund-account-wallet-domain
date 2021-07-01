package georebal

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/regioinv"
	"account-assets.kucoin.net/funding/internal/sweeprun"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func itoa(n int) string { return strconv.Itoa(n) }

// IntentEnqueuer 调用现有归集意图接口（sweeprun.Service.Enqueue）。
// 本包只传递意图，不负责证明链落仓。

type IntentEnqueuer interface {
	Enqueue(req sweeprun.EnqueueReq) (sweeprun.Job, error)
}

// InvTransfer 执行库存域的划转（regioinv.Store.Transfer）。

type InvTransfer interface {
	// TransferInv 在两个区之间划转可用。守恒失败返回 ErrConserve。
	TransferInv(ctx context.Context, t regioinv.Transfer, now time.Time) (regioinv.RegionInv, regioinv.RegionInv, error)
}

// Executor 推进 armed 计划的执行。
type Executor struct {
	st Store
	inv      InvTransfer
	enqueuer IntentEnqueuer
	fw    Firewall
	clock func() time.Time
}

  func NewExecutor(st Store, inv InvTransfer, enqueuer IntentEnqueuer, fw Firewall) *Executor {
	  return &Executor{st: st, inv: inv, enqueuer: enqueuer, fw: fw, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (e *Executor) SetClock(fn func() time.Time) {
	   if fn != nil {
		   e.clock = fn
	  }
   }

   func (e *Executor) now() time.Time { return e.clock() }

// RunPlan 把 armed 计划推进一个 tick：armed→running，执行最早 queued 腿。
// 调用方负责在 done 之后的轮次做守恒检查并归档。
   func (e *Executor) RunPlan(ctx context.Context, planID string) (ExecResult, error) {
	  plan,
		  err := e.st.GetPlan(ctx, planID)
	   if err != nil {
		   return ExecResult{}, err
	  }
	  if plan.Terminal() {
		   return ExecResult{Plan: plan}, nil
	   }
	   if plan.State == PlanDraft {
		  return ExecResult{}, ErrBadTransition
	  }
	   // armed → running
	  now := e.now()
	  if plan.State == PlanArmed {
		   plan.State = PlanRunning
		   plan.UpdatedAt = now
		  if err := e.st.SavePlan(ctx, plan); err != nil {
			  return ExecResult{}, err
		   }
	   }
	   legs, err := e.st.ListLegs(ctx, planID)
	  if err != nil {
		  return ExecResult{}, err
	   }
	   // 找最早 queued 腿
	  var cur *GeoRebalLeg
	  for i := range legs {
		   if legs[i].State == LegQueued {
			  cur = &legs[i]
			   break
		   }
	   }
	  if cur == nil {
		   // 全部腿已完成或失败，检查守恒
		   if err := e.finalise(ctx, plan, legs, now); err != nil {
			  return ExecResult{Plan: plan}, err
		   }
		   updated, _ := e.st.GetPlan(ctx, planID)
		  return ExecResult{Plan: updated, Advance: true}, nil
	   }
	  res, err := e.execLeg(ctx, plan, *cur, now)
	   if err != nil {
		   return ExecResult{Plan: plan}, err
	   }
	   return res, nil
   }

  func (e *Executor) execLeg(ctx context.Context, plan GeoRebalPlan, leg GeoRebalLeg, now time.Time) (ExecResult, error) {
	  // 防火墙再次校验（区状态可能已变）
	  srcR, err := e.fw.GetRegion(ctx, leg.SrcRegion)
	   if err != nil {
		   return ExecResult{}, err
	   }
	  dstR, err := e.fw.GetRegion(ctx, leg.DstRegion)
	   if err != nil {
		   return ExecResult{}, err
	  }
	   if srcR.Halted || dstR.Halted {
		  // 暂停计划；不中止（人工 Clear 后可接着）
		   plan.State = PlanAborted
		  plan.Code = CodeRegionHalted
		  plan.Reason = "halted during execution"
		  plan.UpdatedAt = now
		  _ = e.st.SavePlan(ctx, plan)
		   return ExecResult{Plan: plan}, ErrRegionHalted
	   }
	   // 腿：queued → running
	  if err := e.st.UpdateLegState(ctx, leg.ID, LegRunning, "", ""); err != nil {
		   return ExecResult{}, err
	  }
	   leg.State = LegRunning
	   // 调用归集意图接口（只传递意图，不写证明链）
	  idem := plan.Idem + ":leg:" + itoa(leg.Seq)
	   var intentID string
	  if e.enqueuer != nil {
		   amt,
			  err := money.ParseBook(leg.Amount)
		   if err != nil {
			  return ExecResult{}, ErrInvalid
		   }
		   _ = amt // 已通过 ParseBook 验证
		   job, err := e.enqueuer.Enqueue(sweeprun.EnqueueReq{
			  Chain:  leg.Chain,
			   Asset:  leg.Asset,
			   Amount: leg.Amount,
			  From: leg.Tier,
			   To: leg.Tier,
			  IdempotencyKey: idem,
		   })
		   if err != nil {
			   // 意图入队失败：腿标 failed，计划不立即中止（可重试）
			   _ = e.st.UpdateLegState(ctx, leg.ID, LegFailed, "", err.Error())
			  return ExecResult{Plan: plan,
				  Leg: &leg}, ErrLegFailed
		   }
		  intentID = job.ID
	   }
	   // 执行库存划转
	  tr := regioinv.Transfer{
		  SrcRegionID: srcR.ID,
		   DstRegionID: dstR.ID,
		  Tier:        leg.Tier,
		   Asset:  leg.Asset,
		  Chain:  leg.Chain,
		   Amount: leg.Amount,
	  }
	   if e.inv != nil {
		   if _, _, err := e.inv.TransferInv(ctx, tr, now); err != nil {
			   if !errors.Is(err, regioinv.ErrNotFound) {
				   // 守恒失败 = 严重错误，中止计划
				   plan.State = PlanAborted
				   plan.Code = CodeConserve
				   plan.Reason = err.Error()
				   plan.UpdatedAt = now
				  _ = e.st.SavePlan(ctx, plan)
				   _ = e.st.UpdateLegState(ctx, leg.ID, LegFailed, intentID, err.Error())
				   return ExecResult{Plan: plan}, errors.Join(ErrConserve, err)
			   }
			  // 快照未找到：只标意图ID，库存将在下次汇聚后一致
		   }
	   }
	  // 腿标 done
	  if err := e.st.UpdateLegState(ctx, leg.ID, LegDone, intentID, ""); err != nil {
		  return ExecResult{}, err
	   }
	   leg.State = LegDone
	   leg.SweepIntentID = intentID
	   return ExecResult{Plan: plan,
		   Leg: &leg, Advance: true}, nil
  }

  func (e *Executor) finalise(ctx context.Context, plan GeoRebalPlan, legs []GeoRebalLeg, now time.Time) error {
	  // 所有腿 done 才算计划 done；有 failed 则中止
	   allDone := true
	   for _, l := range legs {
		   if l.State != LegDone {
			  allDone = false
			  break
		   }
	  }
	  if allDone {
		  if err := ConserveCheck(plan, legs); err != nil {
			  plan.State = PlanAborted
			  plan.Code = CodeConserve
			   plan.Reason = "conserve check failed on finalise"
		  } else {
			  plan.State = PlanDone
		  }
	   } else {
		  plan.State = PlanAborted
		  plan.Code = CodeLegFailed
		   plan.Reason = "one or more legs failed"
	  }
	   plan.UpdatedAt = now
	  return e.st.SavePlan(ctx, plan)
  }

// LegDone 手动标记腿完成（ops 调用：调用方已验证意图执行结果）。

   func (e *Executor) LegDone(ctx context.Context, req LegDoneReq) (ExecResult, error) {
	  if strings.TrimSpace(req.Actor) == "" {
		   return ExecResult{}, ErrActor
	  }
	   plan, err := e.st.GetPlan(ctx, req.PlanID)
	   if err != nil {
		   return ExecResult{}, err
	   }
	   legs, err := e.st.ListLegs(ctx, req.PlanID)
	  if err != nil {
		  return ExecResult{}, err
	   }
	   var target *GeoRebalLeg
	   for i := range legs {
		  if legs[i].Seq == req.LegSeq {
			  target = &legs[i]
			  break
		   }
	   }
	  if target == nil {
		  return ExecResult{}, ErrNotFound
	   }
	   if err := e.st.UpdateLegState(ctx, target.ID, LegDone, req.SweepIntentID, ""); err != nil {
		   return ExecResult{}, err
	  }
	  target.State = LegDone
	  target.SweepIntentID = req.SweepIntentID
	  return ExecResult{Plan: plan, Leg: target, Advance: true}, nil
  }
