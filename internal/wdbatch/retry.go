package wdbatch

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Retry 只把失败项重开一批。已提交/跳过的不再动，避免二次冻账。

   func (s *Service) Retry(id, actor, idem string) (Batch, error) {
	  if s == nil {
		  return Batch{}, ErrUnavailable
	   }
	   act, err := requireActor(actor)
	  if err != nil {
		  return Batch{}, err
	   }
	   key, err := validate.Idem(idem)
	   if err != nil {
		  return Batch{}, ErrIdem
	  }
	   cur, err := s.Get(id)
	  if err != nil {
		   return Batch{}, err
	  }
	   if cur.Status != StatusSubmitted {
		   return Batch{}, ErrState
	  }
	   items := make([]Item, 0)
	  for _, it := range cur.Items {
		   if it.Status != ItemFailed {
			   continue
		  }
		  row := it
		   row.Status = ItemPending
		   row.Reason = ""
		  row.LedgerID = ""
		  row.IdempotencyKey = strings.TrimSpace(row.IdempotencyKey) + "-retry"
		  items = append(items, row)
	   }
	  if len(items) == 0 {
		   return Batch{}, ErrEmpty
	  }
	  return s.Open(OpenReq{Actor: act, Items: items, IdempotencyKey: key})
  }
