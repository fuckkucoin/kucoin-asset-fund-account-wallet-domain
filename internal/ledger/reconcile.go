// 现货/钱包和对账单核对，对不上就拦住新单。
// 作者: Likeo
// 日期: 2021.01.06
package ledger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/shopspring/decimal"
)

type ReconcileReport struct {
	OK bool `json:"ok"`
	UserID string `json:"user_id"`
	Mismatches []Mismatch `json:"mismatches"`
	Checked int `json:"checked"`
	Transfers int  `json:"transfers"`
	Blocked   bool `json:"blocked,omitempty"`
}

type Mismatch struct {
	UserID string `json:"user_id"`
	Book   string `json:"book"`
	Asset         string `json:"asset"`
	BookAvailable string `json:"book_available,omitempty"`
	BookFrozen string `json:"book_frozen,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
	State string `json:"state,omitempty"`
	Note string `json:"note"`
}

// DriftGuard 对账对不上就拦住新单。幂等重放仍放行，避免再冻一笔。
type DriftGuard struct {
	mu      sync.RWMutex
	reasons map[string]string
}

  func NewDriftGuard() *DriftGuard {
	   return &DriftGuard{reasons: map[string]string{}}
   }

   func (g *DriftGuard) Block(userID, reason string) {
	   g.mu.Lock()
	  g.reasons[userID] = reason
	  g.mu.Unlock()
  }

  func (g *DriftGuard) Clear(userID string) {
	  g.mu.Lock()
	  delete(g.reasons, userID)
	   g.mu.Unlock()
   }

   func (g *DriftGuard) Check(userID string) error {
	  // 平台级三方差异与单用户差异同时生效。任一未清都挡住新的冻结。
	   if err := g.hit(driftPlatform); err != nil && userID != driftPlatform {
		   return err
	   }
	  return g.hit(userID)
  }

  func (g *DriftGuard) hit(userID string) error {
	   g.mu.RLock()
	   reason, ok := g.reasons[userID]
	   g.mu.RUnlock()
	  if !ok {
		  return nil
	  }
	   if reason == "" {
		  reason = ErrBooksDrift.Error()
	   }
	   return fmt.Errorf("%w: %s", ErrBooksDrift, reason)
  }

// driftPlatform 三方对账未平时的全站挡冻键。不是某个用户号。

const driftPlatform = "*"

// SetPlatformDrift 热桥用户三方不一致时挡住全部新冻结；一致后解除平台挡板。
// 单用户 NoteReconcile 不替代本挡板。
  func (s *Service) SetPlatformDrift(blocked bool, reason string) {
	  if s == nil || s.drift == nil || s.disableDriftBlock {
		  return
	  }
	  if blocked {
		  if strings.TrimSpace(reason) == "" {
			   reason = "triparty drift"
		  }
		   s.drift.Block(driftPlatform, reason)
		   metrics.DriftBlocks.WithLabelValues("platform").Inc()
		  return
	   }
	   s.drift.Clear(driftPlatform)
  }

  func (g *DriftGuard) Blocked(userID string) bool {
	  g.mu.RLock()
	   defer g.mu.RUnlock()
	  _, ok := g.reasons[userID]
	   return ok
   }

// Reconcile 两本账和在途冻结/入账/兑换；对不平拦住新单。

  func Reconcile(ctx context.Context, st Store, userID string) (*ReconcileReport, error) {
	  return ReconcileWith(ctx, st, userID, nil)
  }

// ReconcileWith 核用户各科目。
// 入参：Store、userID、extraFrozen（swap/现货在途，按 asset 加到冻结侧）。
// 出参：ReconcileReport。OK=false 时 NoteReconcile 会挡住新冻。
// 核对：账面 available/frozen ≥ 0；科目借贷轧差 = 账面；
// 在途冻结单的 Amount 计入冻结侧，已确认未入账计入对应科目。
// extra 防止账本和撮合各记一笔冻、对账两边都“平”。

   func ReconcileWith(ctx context.Context, st Store, userID string, extraFrozen map[string]decimal.Decimal) (*ReconcileReport, error) {
	   accs,
		   err := st.ListUserBooks(ctx, userID)
	   if err != nil {
		  return nil, err
	  }
	   rep := &ReconcileReport{OK: true,
		   UserID: userID}
	   books := map[string]Account{}
	   for _, a := range accs {
		   rep.Checked++
		  books[string(a.AccountType)+"|"+a.Asset] = a
		  if a.Available.IsNegative() || a.Frozen.IsNegative() {
			  rep.OK = false
			   rep.Mismatches = append(rep.Mismatches, Mismatch{
				  UserID: a.UserID, Book: string(a.AccountType), Asset: a.Asset,
				  BookAvailable: a.Available.String(), BookFrozen: a.Frozen.String(),
				  Note: "negative balance",
			  })
		  }
		  d, c, err := st.SumDebitCredit(ctx, a.UserID, a.AccountType, a.Asset)
		  if err != nil {
			   return nil, err
		   }
		  if d.IsNegative() || c.IsNegative() {
			   rep.OK = false
			   rep.Mismatches = append(rep.Mismatches, Mismatch{
				   UserID: a.UserID,
				  Book:   string(a.AccountType), Asset: a.Asset,
				   Note: fmt.Sprintf("journal debit/credit negative d=%s c=%s", d, c),
			  })
		  }
		   // 流水轧差（贷−借）不得大于账面总额。
		   bookTotal := a.Available.Add(a.Frozen)
		  net := c.Sub(d)
		   if net.GreaterThan(bookTotal) {
			   rep.OK = false
			   rep.Mismatches = append(rep.Mismatches, Mismatch{
				  UserID: a.UserID, Book: string(a.AccountType), Asset: a.Asset,
				   BookAvailable: a.Available.String(),
				  BookFrozen:    a.Frozen.String(),
				  Note:          fmt.Sprintf("journal net exceeds book total net=%s book=%s", net, bookTotal),
			   })
		  }
		  // 账面不得无流水膨胀。仅 MemStore.Seed 夹具有水位； Store 水位为 0。
		   allow := seedAllowance(st, a.UserID, a.AccountType, a.Asset)
		   if bookTotal.GreaterThan(net.Add(allow)) {
			  rep.OK = false
			  rep.Mismatches = append(rep.Mismatches, Mismatch{
				  UserID: a.UserID,
				  Book:   string(a.AccountType), Asset: a.Asset,
				   BookAvailable: a.Available.String(), BookFrozen: a.Frozen.String(),
				  Note: fmt.Sprintf("unexplained book above journal net=%s book=%s seed=%s", net, bookTotal, allow),
			   })
		   }
	   }
	   trs, err := st.ListUserTransfers(ctx, userID, 0)
	  if err != nil {
		   return nil, err
	  }
	  rep.Transfers = len(trs)
	   // inflight is the freeze that MUST still sit on the source book for any
	   // non-terminal movement. If Frozen < sum(inflight), units vanished or a
	   // freeze journal was skipped.
	   inflight := map[string]decimal.Decimal{}
	  for _, tr := range trs {
		  j, err := st.ListJournal(ctx, tr.ID)
		   if err != nil {
			   return nil, err
		   }
		   if len(j) > 0 {
			  if err := ValidateJournal(j); err != nil {
				   rep.OK = false
				  rep.Mismatches = append(rep.Mismatches, Mismatch{
					  UserID:     userID,
					  TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					   Note: "unbalanced transfer journal: " + err.Error(),
				   })
			   }
		  }
		   hasFreeze, hasUnfreeze, _ := journalFlags(j)
		  switch tr.State {
		  case StateSpotFrozen,
			   StateWalletLocked, StateChainSubmitted, StateConfirmed:
			   if tr.Direction.Freezes() {
				  k := string(sourceBook(tr.Direction)) + "|" + tr.Asset
				   inflight[k] = inflight[k].Add(tr.Amount)
			  }
			  if tr.Direction.Freezes() && !hasFreeze {
				  rep.OK = false
				   rep.Mismatches = append(rep.Mismatches, Mismatch{
					   UserID: userID,
					   TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					  Note: "in-flight transfer missing freeze journal",
				   })
			  }
			  if tr.Direction == WalletDeposit && hasFreeze {
				   rep.OK = false
				   rep.Mismatches = append(rep.Mismatches, Mismatch{
					   UserID:     userID,
					  TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					   Note: "deposit must not freeze user inventory",
				   })
			  }
		   case StateCredited:
			   if settlementSide(j) != "credit" {
				  rep.OK = false
				   rep.Mismatches = append(rep.Mismatches, Mismatch{
					   UserID: userID, TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					  Note: "credited transfer missing net credit journal",
				  })
			   }
			  if tr.Direction == FeeDebit && !hasFeeCreditJournal(j) {
				  rep.OK = false
				  rep.Mismatches = append(rep.Mismatches, Mismatch{
					  UserID: userID, TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					   Note: "fee_debit credited missing system.fee credit journal",
				   })
			  }
		   case StateHold:
			  if tr.Direction.Freezes() {
				   k := string(sourceBook(tr.Direction)) + "|" + tr.Asset
				   inflight[k] = inflight[k].Add(tr.Amount)
			  }
		   case StateReorged:
			  if tr.Direction == FeeDebit {
				  // 罚金冲正恢复可用，不得再计入在途冻结。
				  if settlementSide(j) != "unwound" {
					  rep.OK = false
					   rep.Mismatches = append(rep.Mismatches, Mismatch{
						   UserID: userID, TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
						  Note: "fee_debit reverse missing unwind journal",
					   })
				  }
				  break
			   }
			  if tr.Direction.Freezes() {
				   k := string(sourceBook(tr.Direction)) + "|" + tr.Asset
				  inflight[k] = inflight[k].Add(tr.Amount)
			   }
		  case StateFailed:
			   if hasFreeze && !hasUnfreeze {
				  rep.OK = false
				  rep.Mismatches = append(rep.Mismatches, Mismatch{
					   UserID:     userID,
					  TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					  Note: "failed transfer still frozen; rollback required",
				   })
			  }
		  case StateRolledBack:
			  if hasFreeze && !hasUnfreeze {
				   rep.OK = false
				   rep.Mismatches = append(rep.Mismatches, Mismatch{
					  UserID: userID, TransferID: tr.ID, State: string(tr.State), Asset: tr.Asset,
					   Note: "rolled_back transfer missing unfreeze journal",
				   })
			  }
		  }
	   }
	  for k, v := range extraFrozen {
		   inflight[k] = inflight[k].Add(v)
	   }
	  if extra := ParkJournalMismatches(ctx, st, userID); len(extra) > 0 {
		  rep.Mismatches = append(rep.Mismatches, extra...)
		  rep.OK = false
	  }
	  for k, need := range inflight {
		  a, ok := books[k]
		  frozen := decimal.Zero
		  if ok {
			   frozen = a.Frozen
		  }
		   if frozen.LessThan(need) {
			  rep.OK = false
			   rep.Mismatches = append(rep.Mismatches, Mismatch{
				   UserID: userID, Book: k, Asset: need.String(), BookFrozen: frozen.String(),
				  Note: "in-flight freeze below open transfer sum",
			   })
		  }
	   }
	  // 账上有冻、单据对不上，后面再冻会被卡住。
	  for k, a := range books {
		   if !a.Frozen.IsPositive() {
			  continue
		   }
		  need := inflight[k]
		   if need.IsZero() {
			   rep.OK = false
			  rep.Mismatches = append(rep.Mismatches, Mismatch{
				  UserID: userID, Book: string(a.AccountType), Asset: a.Asset,
				   BookFrozen: a.Frozen.String(),
				  Note:       "orphan frozen with no open transfer",
			  })
			  continue
		  }
		  if a.Frozen.GreaterThan(need) {
			   rep.OK = false
			   rep.Mismatches = append(rep.Mismatches, Mismatch{
				   UserID: userID, Book: string(a.AccountType), Asset: a.Asset,
				  BookFrozen: a.Frozen.String(),
				   Note:       fmt.Sprintf("frozen above open transfer sum need=%s", need),
			   })
		  }
	   }
	  return rep, nil
   }

  func journalFlags(j []JournalEntry) (freeze, unfreeze, credit bool) {
	  for _, e := range j {
		   switch e.Memo {
		   case "freeze_available", "freeze_frozen":
			  freeze = true
		   case "unfreeze_frozen", "restore_available":
			   unfreeze = true
		  case "consume_frozen", "credit_available", "swap_credit":
			   credit = true
		  case "reorg_restore_frozen":
			   freeze = true
		  }
	   }
	  return
  }

   func hasFeeCreditJournal(j []JournalEntry) bool {
	  for _, e := range j {
		  if e.Memo == "fee_credit" && e.AccountType == AccountFee && e.UserID == "system" && e.Credit.IsPositive() {
			   return true
		   }
	  }
	   return false
  }

  func MergeMismatches(rep *ReconcileReport, extra []Mismatch) {
	   if rep == nil || len(extra) == 0 {
		  return
	  }
	  rep.Mismatches = append(rep.Mismatches, extra...)
	  rep.OK = false
   }

// NoteReconcile 对账结果脏了就挡住新划转/兑换，避免账默默漂。
  func (s *Service) NoteReconcile(ctx context.Context, rep *ReconcileReport) {
	  if rep == nil {
		   return
	  }
	   result := "ok"
	  if !rep.OK {
		   result = "mismatch"
	   }
	   metrics.ReconcileRuns.WithLabelValues(result).Inc()
	  metrics.ReconcileMismatches.WithLabelValues(rep.UserID).Set(float64(len(rep.Mismatches)))
	  if s.disableDriftBlock {
		  return
	   }
	  if rep.OK {
		   s.drift.Clear(rep.UserID)
		  rep.Blocked = false
		   return
	  }
	  // 对不上先拦住新单，审计里能看到。
	   s.drift.Block(rep.UserID, "reconcile mismatch")
	  rep.Blocked = true
	  _ = s.store.AppendAudit(ctx, AuditEvent{
		  EntityType: "reconcile",
		   EntityID: rep.UserID,
		   Action: "reconcile_block",
		   Actor:  "system",
		  Detail: map[string]any{"mismatches": len(rep.Mismatches), "ok": false},
	  })
   }

type seedBook interface {
	SeedAllowance(userID string,
		typ AccountType, asset string) decimal.Decimal
}

// BookGap 账面与流水轧差是否互相解释。
// 账面高于贷减借加水位，或流水净额高于账面，都视为解释不了。
// 水位只来自夹具 Seed；实现返回 0。

  func BookGap(ctx context.Context, st Store, userID string, typ AccountType, asset string, bookTotal decimal.Decimal) (net decimal.Decimal, unexplained bool, err error) {
	   if st == nil {
		   return decimal.Zero, false, ErrNotFound
	   }
	  d, c, err := st.SumDebitCredit(ctx, userID, typ, asset)
	  if err != nil {
		  return decimal.Zero, false, err
	  }
	   net = c.Sub(d)
	  allow := seedAllowance(st, userID, typ, asset)
	   if bookTotal.GreaterThan(net.Add(allow)) || net.GreaterThan(bookTotal) {
		  return net, true, nil
	   }
	   return net, false, nil
   }

   func seedAllowance(st Store, userID string, typ AccountType, asset string) decimal.Decimal {
	   if sb, ok := st.(seedBook); ok {
		   return sb.SeedAllowance(userID, typ, asset)
	  }
	  return decimal.Zero
  }

  func (s *Service) CheckDrift(userID string) error {
	  if s.disableDriftBlock {
		  return nil
	  }
	  err := s.drift.Check(userID)
	  if err != nil {
		  metrics.DriftBlocks.WithLabelValues("check").Inc()
	  }
	   return err
  }
