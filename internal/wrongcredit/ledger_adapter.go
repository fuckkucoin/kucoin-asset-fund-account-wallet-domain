package wrongcredit

import (
	"context"
	"fmt"
	"sync"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// ─── 实现：StoreAdapter ───────────────────────────────────────────────────
// StoreAdapter 将 ledger.Store 适配为 LedgerApplier 接口。
type StoreAdapter struct {
	mu   sync.Mutex
	idem map[string]string // 幂等键 → transferID
	s ledger.Store
}

// NewStoreAdapter 构造适配器。
   func NewStoreAdapter(s ledger.Store) *StoreAdapter {
	  return &StoreAdapter{
		  idem: map[string]string{},
		  s: s,
	  }
   }

// ApplyJournal 提交分录；幂等键防止重复提交。
// 提交前调用 ledger.ValidateJournal 校验；不平则返回 ErrJournal。
   func (a *StoreAdapter) ApplyJournal(ctx context.Context, idem string, entries []ledger.JournalEntry) (string, error) {
	   if a == nil || a.s == nil {
		   return "", ErrUnavailable
	  }
	   if err := ledger.ValidateJournal(entries); err != nil {
		   return "", fmt.Errorf("%w: %v", ErrJournal, err)
	   }
	  if len(entries) == 0 {
		  return "", fmt.Errorf("%w: empty entries", ErrJournal)
	   }
	  transferID := entries[0].TransferID
	  if transferID == "" {
		   return "", fmt.Errorf("%w: missing transfer_id in first entry", ErrJournal)
	  }
	   a.mu.Lock()
	   if tid, ok := a.idem[idem]; ok {
		   a.mu.Unlock()
		   return tid, nil
	  }
	   a.mu.Unlock()
	  if err := a.s.AppendJournal(ctx, entries); err != nil {
		   return "", fmt.Errorf("wrongcredit: append journal: %w", err)
	   }
	   a.mu.Lock()
	   a.idem[idem] = transferID
	   a.mu.Unlock()
	  return transferID, nil
  }

// AvailableBalance 查询用户 spot 账户的可用余额（Available 字段）。

  func (a *StoreAdapter) AvailableBalance(ctx context.Context, userID, asset string) (decimal.Decimal, error) {
	  if a == nil || a.s == nil {
		  return decimal.Zero,
			  ErrUnavailable
	   }
	   acc,
		   err := a.s.GetAccount(ctx, userID, ledger.AccountSpot, asset)
	   if err != nil {
		  return decimal.Zero, fmt.Errorf("wrongcredit: get account: %w", err)
	   }
	   if acc == nil {
		   return decimal.Zero, nil
	   }
	  return acc.Available, nil
   }

// ─── 实现：FakeLedger ─────────────────────────────────────────────────────
// FakeLedger 内存实现 LedgerApplier，供单元和集成沙箱使用。
// ApplyJournal 记录所有已提交分录但不真正改变账本余额（用 SetBalance 手动控制）。

type FakeLedger struct {
	mu sync.Mutex
	balances map[string]decimal.Decimal // key = "userID:asset"
	journals map[string][]ledger.JournalEntry
	idem     map[string]string
	Errors map[string]error // key = idem → 注入错误（用）
}

// NewFakeLedger 构造无副作用的假账本。
  func NewFakeLedger() *FakeLedger {
	  return &FakeLedger{
		   balances: map[string]decimal.Decimal{},
		   journals: map[string][]ledger.JournalEntry{},
		  idem:     map[string]string{},
		   Errors: map[string]error{},
	   }
  }

// SetBalance 注入指定用户/资产的可用余额（供场景预设）。
   func (f *FakeLedger) SetBalance(userID, asset string, avail decimal.Decimal) {
	  if f == nil {
		   return
	  }
	   f.mu.Lock()
	  f.balances[userID+":"+asset] = avail
	  f.mu.Unlock()
  }

// ApplyJournal 记录分录（不改余额）；若 Errors[idem] 非 nil 则返回注入错误。
  func (f *FakeLedger) ApplyJournal(_ context.Context, idem string, entries []ledger.JournalEntry) (string, error) {
	   if f == nil {
		  return "", ErrUnavailable
	  }
	   if err := ledger.ValidateJournal(entries); err != nil {
		   return "", fmt.Errorf("%w: %v", ErrJournal, err)
	   }
	   f.mu.Lock()
	  defer f.mu.Unlock()
	   if injected, ok := f.Errors[idem]; ok && injected != nil {
		  return "", injected
	  }
	   if tid, ok := f.idem[idem]; ok {
		  return tid, nil
	  }
	   tid := ""
	  if len(entries) > 0 {
		   tid = entries[0].TransferID
	   }
	  f.idem[idem] = tid
	  f.journals[idem] = append(f.journals[idem], entries...)
	  return tid, nil
  }

// AvailableBalance 返回注入的余额；未设置时返回 Zero。
  func (f *FakeLedger) AvailableBalance(_ context.Context, userID, asset string) (decimal.Decimal, error) {
	  if f == nil {
		  return decimal.Zero, ErrUnavailable
	   }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	   v, ok := f.balances[userID+":"+asset]
	  if !ok {
		   return decimal.Zero, nil
	   }
	  return v, nil
  }

// JournalsFor 返回指定幂等键的所有分录（断言用）。

   func (f *FakeLedger) JournalsFor(idem string) []ledger.JournalEntry {
	  if f == nil {
		   return nil
	   }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  cp := make([]ledger.JournalEntry, len(f.journals[idem]))
	   copy(cp, f.journals[idem])
	   return cp
   }

// IdemResolved 返回幂等键是否已提交。

  func (f *FakeLedger) IdemResolved(idem string) bool {
	   if f == nil {
		   return false
	  }
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   _, ok := f.idem[idem]
	  return ok
   }
