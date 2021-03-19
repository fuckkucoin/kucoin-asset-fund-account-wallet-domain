package clearing

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/portfolio"
)

// ApplyReleased 释放条目后贷记资金账。同一条目只许贷一次，失败要把占用撤掉。

   func (b *Book) ApplyReleased(led *portfolio.BookLedger, itemID, actor string, epoch int64, token string) (portfolio.Position, error) {
	   if b == nil || led == nil {
		  return portfolio.Position{}, ErrUnavailable
	  }
	   if _, err := actorOK(actor); err != nil {
		   return portfolio.Position{}, err
	   }
	   itemID = strings.TrimSpace(itemID)
	  b.mu.Lock()
	   it, ok := b.items[itemID]
	   if !ok {
		   b.mu.Unlock()
		  return portfolio.Position{}, ErrNotFound
	   }
	  if b.applied[itemID] {
		  b.mu.Unlock()
		   return portfolio.Position{}, ErrApplied
	  }
	  if it.Status != StatusReady && it.Status != StatusReleased {
		   b.mu.Unlock()
		   return portfolio.Position{}, ErrState
	   }
	   needRelease := it.Status == StatusReady
	   cp := *it
	  b.applied[itemID] = true
	  b.mu.Unlock()
	  pos, err := led.Credit(cp.UserID, portfolio.BookFunding, cp.Asset, cp.Amount)
	   if err != nil {
		  b.mu.Lock()
		  delete(b.applied, itemID)
		   b.mu.Unlock()
		   return portfolio.Position{}, err
	  }
	   if needRelease {
		  if _, err := b.Release(itemID, actor, epoch, token); err != nil {
			   b.mu.Lock()
			  delete(b.applied, itemID)
			  b.mu.Unlock()
			   _, _ = led.Debit(cp.UserID, portfolio.BookFunding, cp.Asset, cp.Amount)
			   return portfolio.Position{}, err
		  }
	  }
	   return pos, nil
   }

  func (b *Book) Applied(itemID string) bool {
	   if b == nil {
		  return false
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   return b.applied[strings.TrimSpace(itemID)]
  }

  func (s *Service) Apply(led *portfolio.BookLedger, itemID, actor string, epoch int64, token string) (portfolio.Position, error) {
	  return s.Book().ApplyReleased(led, itemID, actor, epoch, token)
  }
