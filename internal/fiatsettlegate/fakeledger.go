package fiatsettlegate

import (
	"context"
	"sync"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// FakeLedger 进程内入账桩。

type FakeLedger struct {
	mu    sync.Mutex
	failN int
	calls  int
	byIdem map[string]*ledger.Transfer
}

// NewFakeLedger 创建桩。
   func NewFakeLedger() *FakeLedger {
	   return &FakeLedger{byIdem: map[string]*ledger.Transfer{}}
   }

// FailNext 随后 N 次 Create 失败。
  func (f *FakeLedger) FailNext(n int) { f.failN = n }

// Calls Create 调用次数（含失败）。
  func (f *FakeLedger) Calls() int {
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  return f.calls
  }

// Create 实现 LedgerHook。
  func (f *FakeLedger) Create(_ context.Context, req ledger.CreateReq) (*ledger.Transfer, error) {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	  f.calls++
	   if existing, ok := f.byIdem[req.IdempotencyKey]; ok {
		  return existing, nil
	  }
	   if f.failN > 0 {
		   f.failN--
		   return nil, ErrLedger
	   }
	  tr := &ledger.Transfer{
		  ID:             uuid.NewString(),
		  IdempotencyKey: req.IdempotencyKey,
		   UserID:         req.UserID,
		   Direction: req.Direction,
		   Asset:  req.Asset,
		   Amount: req.Amount,
	  }
	   f.byIdem[req.IdempotencyKey] = tr
	   return tr, nil
   }
