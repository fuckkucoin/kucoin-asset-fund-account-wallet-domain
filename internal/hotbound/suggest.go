package hotbound

import (
	"context"
	"sort"
	"account-assets.kucoin.net/funding/pkg/money"
)

// PlanHint 归集建议。Execute 恒为 false，调用方不得据此推进多腿。
type PlanHint struct {
	PlanID string `json:"plan_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
	Execute bool `json:"execute"`
}

// SuggestPlans 读热库存，对超过 HotMax 的资产给出计划编号。
// 做什么：把超额翻译成可引用的计划号。
// 不写归集单，不改库存，不调用多腿执行器。
// 不变量：返回前后 List 的归集单数量不变，Execute 全为 false。
  func (s *Service) SuggestPlans(ctx context.Context) ([]PlanHint, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  rows, err := s.Observe(ctx)
	   if err != nil {
		  return nil, err
	  }
	  out := make([]PlanHint, 0)
	  for _, snap := range rows {
		  if !snap.Over {
			   continue
		  }
		  amt, err := money.Parse(snap.Excess)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, PlanHint{
			   PlanID: "hb-suggest-" + snap.Asset + "-" + amt.String(),
			   Asset:  snap.Asset,
			  Amount: amt.String(),
			   Reason:  "hot available above cap",
			  Execute: false,
		  })
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].Asset < out[j].Asset })
	  return out, nil
   }
