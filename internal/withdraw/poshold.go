package withdraw

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// watchDrifts 内存冻结簿加上账本投影。retain 决定旧阻断在本次没有新漂移时是否保留。
   func (s *Service) watchDrifts() ([]Drift, func(Block) bool) {
	   if s == nil || s.book == nil {
		  return nil, func(Block) bool { return true }
	}
	base := s.book.List()
	seen := map[string]struct{}{}
	bookPairs := map[string]struct{}{}
	for _, r := range base {
		if r.TransferID != "" {
			seen[r.TransferID] = struct{}{}
		}
		if r.UserID != "" && r.Asset != "" {
			bookPairs[pairKey(r.UserID, r.Asset)] = struct{}{}
		}
	}
	view := s.readLedgerView(seen)
	runs := append([]Run{}, base...)
	runs = append(runs, view.runs...)
	incomplete := s.broadcastLegIncomplete()
	retain := func(b Block) bool {
		   if incomplete {
			  return true
		  }
		  if _, ok := bookPairs[pairKey(b.UserID, b.Asset)]; ok {
			  return false
		  }
		  if s.led == nil || !view.ok {
			  return true
		  }
		  return view.open[pairKey(b.UserID, b.Asset)]
	  }
	return s.collectFrom(runs), retain
}

// readLedgerView 按已知用户读取未结出金。列举失败或被截断时 ok 为假。

  func (s *Service) readLedgerView(have map[string]struct{}) (out struct {
	   ok   bool
	   open map[string]bool
	  runs []Run
   }) {
	  out.open = map[string]bool{}
	   out.ok = true
	  if s == nil || s.led == nil {
		   out.ok = false
		   return out
	  }
	  users := s.ledgerUsers()
	  if len(users) > 200 {
		   out.ok = false
		   users = users[:200]
	  }
	   for _, user := range users {
		   list, err := s.led.List(context.Background(), user, 200)
		   if err != nil {
			  out.ok = false
			   continue
		  }
		   if len(list) >= 200 {
			   out.ok = false
		   }
		   for _, tr := range list {
			   if tr == nil || tr.Direction != ledger.WalletWithdraw || tr.ID == "" {
				   continue
			  }
			   fr, live := freezeFromLedger(tr.State)
			   if !live {
				  continue
			  }
			   out.open[pairKey(tr.UserID, tr.Asset)] = true
			  if _, ok := have[tr.ID]; ok {
				   continue
			   }
			  out.runs = append(out.runs, Run{
				   ID: tr.ID, TransferID: tr.ID, UserID: tr.UserID,
				  Asset: tr.Asset,
				   Amount: tr.Amount.String(), Freeze: fr,
			  })
		   }
	  }
	  return out
  }

  func (s *Service) ledgerUsers() []string {
	  seen := map[string]struct{}{}
	   var out []string
	   add := func(user string) {
		  user = strings.TrimSpace(user)
		  if user == "" {
			  return
		   }
		  if _, ok := seen[user]; ok {
			   return
		   }
		   seen[user] = struct{}{}
		   out = append(out, user)
	   }
	  if s.book != nil {
		  for _, r := range s.book.List() {
			  add(r.UserID)
		  }
	  }
	   if s.conserve != nil {
		   for _, b := range s.conserve.Blocks() {
			  add(b.UserID)
		   }
		   for _, in := range s.conserve.intentCopy() {
			  add(in.UserID)
		   }
	   }
	  return out
  }

// freezeFromLedger 把账本未结出金映射成冻结相位。已入账或已回滚不占冻结位。

   func freezeFromLedger(st ledger.State) (FreezeState, bool) {
	  switch st {
	   case ledger.StateInitiated, ledger.StateSpotFrozen, ledger.StateWalletLocked, ledger.StateHold:
		   return FreezeHeld, true
	  case ledger.StateChainSubmitted,
		   ledger.StateReorged:
		   return FreezeBroadcast, true
	   case ledger.StateConfirmed:
		  return FreezeConfirming, true
	  case ledger.StateFailed:
		  return FreezeFailed, true
	   default:
		  return FreezeNone, false
	   }
   }

// broadcastLegIncomplete 广播表绑了 SQL 却还没回填成功。这时不能用空跟踪证明漂移消失。
  func (s *Service) broadcastLegIncomplete() bool {
	  if s == nil {
		  return true
	   }
	  h := s.Broadcasts().Health()
	   if !h.SQLBound {
		  return false
	   }
	   return h.HydrateErr != "" || h.HydratedAt.IsZero()
   }
