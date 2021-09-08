package occupbook

import (
	"context"
	"fmt"
)

// BreakSummary 守恒击穿摘要。

type BreakSummary struct {
	LotID    string `json:"lot_id"`
	Inflight string `json:"inflight"`
	Available string `json:"available"`
	Void      string `json:"void"`
	Sum       string `json:"sum"`
	Total     string `json:"total"`
	Delta     string `json:"delta"`
	Reason string `json:"reason"`
}

// InspectLot 检查单批次守恒。

  func InspectLot(lot *Lot) (*BreakSummary, error) {
	  if lot == nil {
		   return nil, ErrLotNotFound
	   }
	  sum := lot.Buckets.Sum()
	  if sum.Equal(lot.OccupiedTotal) {
		   return nil, nil
	  }
	  return &BreakSummary{
		   LotID: lot.LotID,
		  Inflight:  lot.Buckets.Inflight.String(),
		   Available: lot.Buckets.Available.String(),
		  Void:  lot.Buckets.Void.String(),
		   Sum:   sum.String(),
		   Total: lot.OccupiedTotal.String(),
		  Delta:  sum.Sub(lot.OccupiedTotal).String(),
		  Reason: "triple_bucket_sum_ne_occupied_total",
	  }, ErrConserveBroken
  }

// Patrol 扫描打开批次，返回全部击穿摘要。
   func (b *Book) Patrol(ctx context.Context, limit int) ([]*BreakSummary, error) {
	  if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	  }
	  lots, err := b.store.ListOpenLots(ctx, limit)
	  if err != nil {
		  return nil, err
	  }
	  out := make([]*BreakSummary, 0)
	   for _, lot := range lots {
		   sum, err := InspectLot(lot)
		  if err == nil {
			  continue
		   }
		   if sum != nil {
			   lot.State = LotBroken
			   _ = b.store.UpdateLot(ctx, lot)
			   b.audit(ctx, lot, "patrol", sum.Reason, false)
			   out = append(out, sum)
		   }
	  }
	  return out, nil
  }

// FormatBreak 人类可读击穿行。

  func FormatBreak(s *BreakSummary) string {
	  if s == nil {
		  return ""
	  }
	  return fmt.Sprintf("lot=%s inflight=%s available=%s void=%s sum=%s total=%s delta=%s reason=%s",
		  s.LotID,
		  s.Inflight, s.Available, s.Void, s.Sum, s.Total, s.Delta, s.Reason)
   }
