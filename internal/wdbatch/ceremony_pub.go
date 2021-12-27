package wdbatch

import (
	"context"
	"account-assets.kucoin.net/funding/internal/cerbind"
)

  func (s *Service) publishCeremony(b Batch) error {
	   if s == nil {
		   return nil
	  }
	   eng := cerbind.Current()
	  if eng == nil {
		   return nil
	   }
	   lines := make([]cerbind.Line, 0, len(b.Items))
	   for _, it := range b.Items {
		  lines = append(lines, cerbind.Line{
			   Asset: it.Asset, Chain: it.Chain, Amount: it.Amount, Dest: it.ToAddress,
			  Purpose: cerbind.PurposeWithdraw, Side: cerbind.SideCold,
			  ItemRefs: []string{it.IdempotencyKey},
		   })
	   }
	   if _, err := eng.Ingest(context.Background(), cerbind.Snapshot{BatchID: b.ID, Lines: lines}); err != nil {
		  return err
	  }
	  return nil
   }

// IndexSigned 把账本单据号登记到批次上，供后续重签按单据号过闸。
// 登记失败不回滚已提交条目：开批时的幂等键别名仍然能挡住签播。

   func (s *Service) IndexSigned(batchID, movementID string) error {
	  eng := cerbind.Current()
	  if eng == nil || movementID == "" {
		  return nil
	  }
	  return eng.IndexRef(context.Background(), batchID, movementID, "move")
   }
