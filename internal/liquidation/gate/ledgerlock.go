package gate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LockPoster 既有锁仓过账口。

type LockPoster interface {
	PostLock(ctx context.Context, userID, asset, amount, idemKey string) error
	PostingCount(userID string) int
	LastLines(userID string) []ledger.JournalEntry
}

// LedgerLockAdapter 经 Store 写入 freeze_available / freeze_frozen。

type LedgerLockAdapter struct {
	mu    sync.Mutex
	store ledger.Store
	byIdem map[string]string
	byUser map[string][]ledger.JournalEntry
	counts map[string]int
}

   func NewLedgerLockAdapter(store ledger.Store) *LedgerLockAdapter {
	  return &LedgerLockAdapter{
		  store:  store,
		  byIdem: map[string]string{},
		   byUser: map[string][]ledger.JournalEntry{},
		   counts: map[string]int{},
	  }
  }

  func (p *LedgerLockAdapter) PostLock(ctx context.Context, userID, asset, amount, idemKey string) error {
	  userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  idemKey = strings.TrimSpace(idemKey)
	  if userID == "" || asset == "" || idemKey == "" {
		  return fmt.Errorf("lock posting subject incomplete")
	  }
	  amt,
		   err := ledger.ParseAmount(amount)
	   if err != nil {
		  return err
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  if _, ok := p.byIdem[idemKey]; ok {
		  return nil
	  }
	  tr := &ledger.Transfer{
		  ID:             uuid.NewString(),
		  IdempotencyKey: idemKey,
		   UserID: userID,
		  Direction: ledger.P2PDebit,
		   Asset:     asset,
		  Amount: amt,
		   State:  ledger.StateSpotFrozen,
	  }
	   book := ledger.P2PDebit.SourceBook()
	   lines := ledger.BuildFreezeLockLines(tr, book)
	   if err := ledger.ValidateJournal(lines); err != nil {
		  return err
	  }
	  if p.store != nil {
		   if err := p.applyStore(ctx, userID, asset, book, lines); err != nil {
			  return err
		   }
	   }
	   p.byIdem[idemKey] = tr.ID
	   p.byUser[userID] = append(p.byUser[userID], lines...)
	  p.counts[userID]++
	  return nil
   }

  func (p *LedgerLockAdapter) applyStore(ctx context.Context, userID, asset string, book ledger.AccountType, lines []ledger.JournalEntry) error {
	   acc, err := p.store.GetAccount(ctx, userID, book, asset)
	   if err != nil {
		  return err
	   }
	   for _, e := range lines {
		   if err := ledger.ApplyJournalToAccount(acc, e); err != nil {
			   return err
		   }
	  }
	   if err := p.store.AppendJournal(ctx,
		   lines); err != nil {
		   return err
	   }
	  return p.store.UpsertAccount(ctx, acc)
   }

  func (p *LedgerLockAdapter) PostingCount(userID string) int {
	  p.mu.Lock()
	  defer p.mu.Unlock()
	   return p.counts[strings.TrimSpace(userID)]
  }

   func (p *LedgerLockAdapter) LastLines(userID string) []ledger.JournalEntry {
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   src := p.byUser[strings.TrimSpace(userID)]
	  out := make([]ledger.JournalEntry, len(src))
	  copy(out, src)
	  return out
  }

// LockIdemKey 同一账户维持违约只允许一笔锁仓幂等键。
  func LockIdemKey(accountID string) string {
	   return "maint-lock:" + strings.TrimSpace(accountID)
  }

  func lockAmount(required string) (string, error) {
	  d,
		  err := decimal.NewFromString(strings.TrimSpace(required))
	  if err != nil || !d.IsPositive() {
		   return "", fmt.Errorf("lock amount")
	  }
	  return d.String(), nil
  }
