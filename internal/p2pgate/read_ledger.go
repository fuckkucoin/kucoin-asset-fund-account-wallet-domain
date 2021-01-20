package p2pgate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type transferIndex struct {
	byKey map[string]*ledger.Transfer
	flows []FlowEntry
	all   []*ledger.Transfer
}

   func (f *Funds) loadTransfers(ctx context.Context, userID string) ([]*ledger.Transfer, error) {
	  if f == nil || f.led == nil {
		  return nil, nil
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		   return nil, nil
	  }
	  list,
		   err := f.led.List(ctx, userID, 0)
	   if err != nil {
		  return nil, err
	  }
	   if len(list) > MaxFlowScan {
		  list = list[:MaxFlowScan]
	  }
	   return list, nil
  }

   func indexTransfers(list []*ledger.Transfer) transferIndex {
	  idx := transferIndex{
		   byKey: make(map[string]*ledger.Transfer, len(list)),
		  flows: make([]FlowEntry, 0, len(list)),
		  all: list,
	  }
	  for _, tr := range list {
		  if tr == nil {
			   continue
		  }
		  key := strings.TrimSpace(tr.IdempotencyKey)
		   if key != "" {
			   if prev, ok := idx.byKey[key]; !ok || preferTransfer(tr, prev) {
				   idx.byKey[key] = tr
			   }
		   }
		   if !IsP2PDirection(tr.Direction) {
			   continue
		   }
		   idx.flows = append(idx.flows,
			   ClassifyTransfer(tr))
	  }
	  return idx
   }

// preferTransfer 同一幂等键多条时先已入账，其次较新 UpdatedAt。
  func preferTransfer(a, b *ledger.Transfer) bool {
	  if a == nil {
		  return false
	   }
	   if b == nil {
		   return true
	  }
	   if a.State == ledger.StateCredited && b.State != ledger.StateCredited {
		  return true
	  }
	   if a.State != ledger.StateCredited && b.State == ledger.StateCredited {
		   return false
	   }
	  return a.UpdatedAt.After(b.UpdatedAt)
   }

   func (idx transferIndex) lookup(key string) *ledger.Transfer {
	   if key == "" || idx.byKey == nil {
		   return nil
	   }
	  return idx.byKey[key]
  }

   func (idx transferIndex) flowOf(key string) *FlowEntry {
	   tr := idx.lookup(key)
	   if tr == nil {
		  return nil
	  }
	  e := ClassifyTransfer(tr)
	   if e.TransferID == "" {
		  return nil
	  }
	   return &e
  }

  func (f *Funds) occupancyIndex(ctx context.Context, userID string) (transferIndex, error) {
	  list, err := f.loadTransfers(ctx, userID)
	   if err != nil {
		   return transferIndex{}, err
	   }
	   return indexTransfers(list), nil
   }

   func flowPtr(e FlowEntry) *FlowEntry {
	  if e.TransferID == "" {
		  return nil
	  }
	   cp := e
	   return &cp
  }
