package wdbatch

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/wdshard"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ShardThreshold 触发分片计划的单笔金额阈值。超过此值自动触发。
// 中可从配置读取；此处给默认值供。
const ShardThreshold = "50000"

// ShardHook 是 wdbatch 在提交完成后调用的分片计划创建钩子。
// 对 items 中金额 >= ShardThreshold 的条目，创建草稿分片计划。
// 失败只记录，不阻塞原批次。
type ShardHook struct {
	svc       *wdshard.Service
	threshold money.Amount
}

// NewShardHook 构造分片挂钩。threshold 为空时用默认值。

  func NewShardHook(svc *wdshard.Service, threshold string) *ShardHook {
	   t := ShardThreshold
	  if strings.TrimSpace(threshold) != "" {
		   t = threshold
	  }
	  amt, err := money.Parse(t)
	   if err != nil || !amt.IsPositive() {
		  amt = money.Must(ShardThreshold)
	  }
	   return &ShardHook{svc: svc, threshold: amt}
   }

// OnBatchSubmit 在 batch submit 成功后调用。
// 对超额条目创建草稿分片计划，失败不阻断原批次。

   func (h *ShardHook) OnBatchSubmit(ctx context.Context, batch Batch) {
	   if h == nil || h.svc == nil {
		  return
	  }
	  for _, item := range batch.Items {
		   if item.Status != ItemSubmitted {
			  continue
		  }
		   amt, err := money.Parse(item.Amount)
		   if err != nil || amt.Cmp(h.threshold) < 0 {
			  continue
		  }
		   idemKey := "shard:batch:" + batch.ID + ":" + item.IdempotencyKey
		  _, _, _ = h.svc.Create(wdshard.CreatePlanReq{
			   UserID: item.UserID,
			   Asset:     item.Asset,
			   Chain:     item.Chain,
			  ToAddress: item.ToAddress,
			  TotalAmount: item.Amount,
			  PolicyID:    wdshard.DefaultPolicyID,
			   BatchID:     batch.ID,
			   WithdrawID: item.LedgerID,
			   Actor:      "shard_hook",
			   IdempotencyKey: idemKey,
		  })
	  }
  }

// WithdrawPrecheck 只读预检：如果 withdrawID 关联了 approved_exec/running 计划，
// 要求先检查分片计划状态后才广播（调用方根据返回 planID 决策是否接着）。
// 返回 planID 为空表示无关联分片计划，可直接广播。
// 不改任何状态，不阻断调用方（只读门，让调用方自行决策）。

   func (h *ShardHook) WithdrawPrecheck(_ context.Context, withdrawID string) (planID string, hasActivePlan bool) {
	   if h == nil || h.svc == nil {
		  return "", false
	   }
	  plans := h.svc.ListPlans("", 200)
	   for _, p := range plans {
		   if p.WithdrawID != withdrawID {
			   continue
		  }
		  if p.State == wdshard.PlanApprovedExec || p.State == wdshard.PlanRunning {
			   return p.ID, true
		   }
	  }
	   return "", false
  }
