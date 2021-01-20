package partial

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/google/uuid"
)

// ReleasePoster 既有释冻过账口。
type ReleasePoster interface {
	PostRelease(ctx context.Context, userID, asset, amount, idemKey string) error
	PostingCount(userID string) int
	LastLines(userID string) []ledger.JournalEntry
	IdemSeen(idemKey string) bool
}

// LedgerReleaseAdapter 经 Store 写入 unfreeze_frozen / restore_available。
type LedgerReleaseAdapter struct {
	mu sync.Mutex
	store ledger.Store
	byIdem map[string]string
	byUser map[string][]ledger.JournalEntry
	counts map[string]int
}

   func NewLedgerReleaseAdapter(store ledger.Store) *LedgerReleaseAdapter {
	   return &LedgerReleaseAdapter{
		   store: store,
		  byIdem: map[string]string{},
		   byUser: map[string][]ledger.JournalEntry{},
		  counts: map[string]int{},
	   }
  }

  func (p *LedgerReleaseAdapter) PostRelease(ctx context.Context, userID, asset, amount, idemKey string) error {
	   userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   idemKey = strings.TrimSpace(idemKey)
	  if userID == "" || asset == "" || idemKey == "" {
		  return fmt.Errorf("release posting subject incomplete")
	   }
	   amt, err := ledger.ParseAmount(amount)
	   if err != nil {
		   return err
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   if _, ok := p.byIdem[idemKey]; ok {
		   return nil
	  }
	   if amt.IsZero() {
		   p.byIdem[idemKey] = "zero"
		  return nil
	  }
	  tr := &ledger.Transfer{
		  ID: uuid.NewString(),
		   IdempotencyKey: idemKey,
		  UserID: userID,
		  Direction: ledger.P2PCredit,
		   Asset:     asset,
		  Amount: amt,
		  State: ledger.StateRolledBack,
	  }
	   book := ledger.P2PDebit.SourceBook()
	   lines := ledger.BuildUnfreezeReleaseLines(tr, book)
	   if err := ledger.ValidateJournal(lines); err != nil {
		  return err
	  }
	   if p.store != nil {
		  if err := p.applyStore(ctx, userID, asset, book, lines); err != nil {
			  return err
		  }
	  }
	   p.byIdem[idemKey] = tr.ID
	  p.byUser[userID] = append(p.byUser[userID],
		  lines...)
	  p.counts[userID]++
	  return nil
  }

   func (p *LedgerReleaseAdapter) applyStore(ctx context.Context, userID, asset string, book ledger.AccountType, lines []ledger.JournalEntry) error {
	  acc, err := p.store.GetAccount(ctx, userID, book, asset)
	   if err != nil {
		   return err
	  }
	   for _, e := range lines {
		  if err := ledger.ApplyJournalToAccount(acc, e); err != nil {
			  return err
		  }
	   }
	   if err := p.store.AppendJournal(ctx, lines); err != nil {
		   return err
	  }
	  return p.store.UpsertAccount(ctx,
		  acc)
  }

   func (p *LedgerReleaseAdapter) PostingCount(userID string) int {
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   return p.counts[strings.TrimSpace(userID)]
   }

   func (p *LedgerReleaseAdapter) LastLines(userID string) []ledger.JournalEntry {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   src := p.byUser[strings.TrimSpace(userID)]
	   out := make([]ledger.JournalEntry, len(src))
	  copy(out, src)
	   return out
   }

  func (p *LedgerReleaseAdapter) IdemSeen(idemKey string) bool {
	   p.mu.Lock()
	   defer p.mu.Unlock()
	   _, ok := p.byIdem[strings.TrimSpace(idemKey)]
	   return ok
  }
