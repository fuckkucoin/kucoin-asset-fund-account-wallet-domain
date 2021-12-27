package wdbatch

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

  func (s *Service) BindResidueFence(eng *batchresiduefence.Engine) *Service {
	  if s != nil {
		   s.fence = eng
	  }
	   return s
   }

   func (s *Service) ResidueFence() *batchresiduefence.Engine {
	  if s == nil {
		  return nil
	   }
	   return s.fence
  }

  func (s *Service) ViewItem(_ context.Context, batchID, itemIdem string) (batchresiduefence.BatchItemView, error) {
	   if s == nil {
		  return batchresiduefence.BatchItemView{}, ErrUnavailable
	  }
	  cur, err := s.Get(batchID)
	   if err != nil {
		  return batchresiduefence.BatchItemView{}, err
	   }
	  want := strings.TrimSpace(itemIdem)
	  for _, it := range cur.Items {
		  if strings.TrimSpace(it.IdempotencyKey) == want {
			  return batchresiduefence.BatchItemView{
				   BatchID: cur.ID, ItemIdem: it.IdempotencyKey,
				  Phase: mapItemPhase(it.Status), UserID: it.UserID,
				  Asset: it.Asset,
				  Amount: it.Amount, LedgerID: it.LedgerID, Reason: it.Reason,
			  }, nil
		  }
	   }
	   return batchresiduefence.BatchItemView{}, ErrItem
   }

   func mapItemPhase(st ItemStatus) batchresiduefence.ItemPhase {
	   switch st {
	   case ItemSubmitted:
		   return batchresiduefence.PhaseSubmitted
	  case ItemSkipped:
		  return batchresiduefence.PhaseSkipped
	  case ItemFailed:
		  return batchresiduefence.PhaseFailed
	   default:
		   return batchresiduefence.PhasePending
	  }
   }

   func (s *Service) ensureFence() *batchresiduefence.Engine {
	   if s == nil {
		  return nil
	  }
	   if s.fence != nil {
		  return s.fence
	  }
	  s.fence = batchresiduefence.New(batchresiduefence.Config{Viewer: s})
	  return s.fence
   }
