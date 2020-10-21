// 按链上观测入账；重组过深要回滚已入账充值。
// 作者: Likeo
// 日期: 2020.10.06
package ledger

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// applyObservation 入账前把链观察写到单据。
// 金额/资产/收款地址对不上 hold：禁止按申请额 credit，也禁止把别人地址上的 hash 记到本用户。
// 观察发生在事务外，这里只落已看到的字段。
  func (s *Service) applyObservation(ctx context.Context, cur *Transfer, obs wallet.Observation) error {
	  if cur == nil || cur.Direction != WalletDeposit {
		  return nil
	  }
	   cur.ObservedAmount = obs.Amount
	   cur.ObservedAsset = obs.Asset
	   if err := s.putFenced(ctx, cur); err != nil {
		   return err
	  }
	   if obs.Amount.IsZero() {
		  if obs.Confirmations >= s.needConf(cur.Chain) && obs.Status == wallet.TxConfirmed && !obs.Reorged {
			   metrics.ObservationHolds.WithLabelValues("empty").Inc()
			   return s.holdLocked(ctx, cur, "empty chain observation amount")
		   }
		  return nil
	   }
	  if obs.Asset != "" && obs.Asset != cur.Asset {
		   metrics.ObservationHolds.WithLabelValues("asset").Inc()
		   return s.holdLocked(ctx, cur, "observed asset "+obs.Asset)
	   }
	  if !obs.Amount.Equal(cur.Amount) {
		   metrics.ObservationHolds.WithLabelValues("amount").Inc()
		  return s.holdLocked(ctx, cur, "observed amount "+obs.Amount.String())
	  }
	  if err := s.matchObservedDestination(ctx, cur, obs); err != nil {
		  return err
	   }
	  return nil
   }

// matchObservedDestination 充值收款地址必须是该用户托管 inbound。已确认但没给收款方就 hold。

   func (s *Service) matchObservedDestination(ctx context.Context, cur *Transfer, obs wallet.Observation) error {
	   got := strings.TrimSpace(obs.ToAddress)
	  want := strings.TrimSpace(cur.ToAddress)
	   if want == "" && s.depositAddr != nil {
		  want = strings.TrimSpace(s.depositAddr(ctx, cur.UserID, cur.Chain))
	   }
	   if got == "" {
		   // 已确认但索引没给收款方：不能 fail-open 按申请额入账。
		   if want != "" && obs.Confirmations >= s.needConf(cur.Chain) && obs.Status == wallet.TxConfirmed && !obs.Reorged {
			  metrics.ObservationHolds.WithLabelValues("destination_missing").Inc()
			  return s.holdLocked(ctx, cur, "missing observed destination")
		   }
		   return nil
	  }
	  if want == "" {
		   return nil
	  }
	   if sameChainAddress(cur.Chain, got, want) {
		   return nil
	  }
	   metrics.ObservationHolds.WithLabelValues("destination").Inc()
	  return s.holdLocked(ctx, cur, "observed destination "+got)
  }

// ConfirmNeed 按链确认深度。yaml 只是下限，不能把 BTC/ETH/L2 压到 1。

   func ConfirmNeed(chain string, floor int) int {
	  if floor < 1 {
		   floor = 1
	   }
	   need := floor
	   switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "BTC":
		  if need < 3 {
			  need = 3
		  }
	  case "ETH":
		  if need < 12 {
			  need = 12
		   }
	   case "POLYGON", "BSC", "ARBITRUM", "OPTIMISM", "BASE", "AVAX":
		  if need < 20 {
			   need = 20
		  }
	   case "TRON":
		   if need < 20 {
			   need = 20
		   }
	  case "SOL":
		  if need < 32 {
			   need = 32
		  }
	   case "TON":
		   if need < 16 {
			  need = 16
		  }
	   }
	   return need
  }

   func (s *Service) needConf(chain string) int {
	  floor := 1
	   if s != nil && s.confirmDepth > 0 {
		   floor = s.confirmDepth
	   }
	   return ConfirmNeed(chain, floor)
  }

// holdLocked 观测金额/资产/地址对不上时 hold，禁止按申请额 credit。
   func (s *Service) holdLocked(ctx context.Context, cur *Transfer, reason string) error {
	  err := s.store.InTx(ctx, func(ctx context.Context) error {
		  got, err := s.getFenced(ctx, cur.ID)
		  if err != nil {
			  return err
		  }
		  if got.State == StateHold {
			   return nil
		  }
		  if !CanApply(got.State, EventHold) {
			   got.FailReason = reason
			  got.RiskFlags = appendFlag(got.RiskFlags, "obs_mismatch")
			   return s.putFenced(ctx, got)
		   }
		  next,
			   err := Apply(got.State, EventHold)
		  if err != nil {
			   return err
		  }
		   got.State = next
		   got.FailReason = reason
		  got.RiskFlags = appendFlag(got.RiskFlags, "obs_mismatch")
		  if err := s.putFenced(ctx, got); err != nil {
			  return err
		  }
		  _ = s.audit(ctx, got, "hold", map[string]any{"reason": reason})
		   metrics.TransferByState.WithLabelValues(string(next), string(got.Direction)).Inc()
		   return nil
	  })
	  if err != nil {
		   return err
	   }
	   return fmt.Errorf("%w: %s", ErrObservationMismatch, reason)
  }

// UnwindReorg 已 credited 的充提，确认数回退或块 hash 变了，把入账流水反向扣回。
// 链观察必须在调用方事务外做完再进来。持用户锁 + fence。不发明第二笔 credit。

  func (s *Service) UnwindReorg(ctx context.Context, id string) error {
	  if s == nil {
		  return fmt.Errorf("ledger unavailable")
	  }
	   cur, err := s.store.GetTransfer(ctx, id)
	  if err != nil {
		  return err
	   }
	   lease, err := s.lock.Lock(ctx, UserLockKey(cur.UserID), s.lockTTL)
	  if err != nil {
		  return err
	   }
	   defer lease.Unlock()
	  ctx,
		   err = s.beginFence(ctx, id, lease)
	   if err != nil {
		  return err
	   }
	  if err := s.unwindReorg(ctx, cur); err != nil {
		  return err
	   }
	  if s.OnBooks != nil {
		   s.OnBooks(ctx, cur.UserID)
	  }
	   return nil
   }

// unwindReorg 确认数回退后，把已 credited 的充值从钱包扣回，状态进 reorged。

  func (s *Service) unwindReorg(ctx context.Context, cur *Transfer) error {
	   return s.store.InTx(ctx, func(ctx context.Context) error {
		  got, err := s.getFenced(ctx, cur.ID)
		  if err != nil {
			   return err
		   }
		  if got.State != StateCredited {
			  return nil
		   }
		  prev,
			  err := s.store.ListJournal(ctx, got.ID)
		  if err != nil {
			   return err
		   }
		   if settlementSide(prev) != "credit" {
			   next, err := Apply(got.State, EventReorgUnwind)
			   if err != nil {
				   return err
			   }
			   got.State = next
			  return s.putFenced(ctx, got)
		  }
		  lines := reorgUnwindLines(got, prev, NextJournalSeq(prev))
		  if len(lines) == 0 {
			  return fmt.Errorf("no reorg unwind lines for %s", got.Direction)
		   }
		   if err := s.store.AppendJournal(ctx, lines); err != nil {
			  return err
		  }
		   touched := map[string]*Account{}
		  for _, e := range lines {
			   k := e.UserID + "|" + string(e.AccountType) + "|" + e.Asset
			   acc, ok := touched[k]
			   if !ok {
				   acc, err = s.store.GetAccount(ctx, e.UserID, e.AccountType, e.Asset)
				  if err != nil {
					   return err
				   }
				  touched[k] = acc
			   }
			  if err := ApplyJournalToAccount(acc, e); err != nil {
				   return err
			  }
		   }
		   for _, a := range touched {
			  if err := requireFreshBook(a); err != nil {
				   return err
			  }
			  if err := s.store.UpsertAccount(ctx, a); err != nil {
				  return err
			   }
		   }
		   next,
			   err := Apply(got.State, EventReorgUnwind)
		  if err != nil {
			  return err
		  }
		   got.State = next
		   got.RiskFlags = appendFlag(got.RiskFlags, "reorg")
		   if err := s.putFenced(ctx, got); err != nil {
			  return err
		   }
		   metrics.ReorgUnwinds.WithLabelValues(string(got.Direction)).Inc()
		  metrics.TransferByState.WithLabelValues(string(next), string(got.Direction)).Inc()
		  return s.audit(ctx, got, "reorg_unwound", map[string]any{"tx_hash": got.TxHash})
	  })
   }

  func appendFlag(flags []string, f string) []string {
	  for _, x := range flags {
		  if x == f {
			   return flags
		   }
	  }
	  return append(flags, f)
   }

   func dropFlag(flags []string, f string) []string {
	   if len(flags) == 0 {
		   return flags
	  }
	  out := flags[:0]
	   for _, x := range flags {
		  if x != f {
			  out = append(out, x)
		   }
	  }
	   return out
  }

   func hasFlag(flags []string, f string) bool {
	  for _, x := range flags {
		  if x == f {
			   return true
		   }
	  }
	  return false
   }

  func settlementSide(j []JournalEntry) string {
	  last := ""
	  for _, e := range j {
		   switch e.Memo {
		   case "credit_available", "consume_frozen":
			   last = "credit"
		   case "reorg_debit_available", "reorg_restore_frozen":
			   last = "unwound"
		  case "restore_available":
			   // FeeDebit 冲正恢复可用；出金 rollback 解冻亦同 memo，仅 credited 路径用本函数净侧。
			  last = "unwound"
		   }
	  }
	   return last
  }

// ReverseFee 运营冲正已入账 FeeDebit（罚金/手续费贷记 system.fee）。
// 复用 UnwindReorg 分录机；状态进 reorged；幂等。
// 非 FeeDebit / 未 credited 拒；不得二次冲正。

   func (s *Service) ReverseFee(ctx context.Context, id string) error {
	   if s == nil {
		   return fmt.Errorf("ledger unavailable")
	  }
	   cur, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		   return err
	   }
	  if cur.Direction != FeeDebit {
		  return fmt.Errorf("reverse fee requires fee_debit direction")
	  }
	   if cur.State != StateCredited {
		   if cur.State == StateReorged {
			  return nil
		   }
		   return fmt.Errorf("reverse fee requires credited fee_debit")
	   }
	  return s.UnwindReorg(ctx, id)
  }
