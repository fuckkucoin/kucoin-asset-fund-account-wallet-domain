package ledshard

import (
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Kill 把意图停在当前相位。preparing 且尚未冻结时，worker 补偿后应无冻无贷。
  func (e *Engine) Kill(id string) (Intent, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   it, ok := e.mem.intent(id)
	   if !ok {
		   return Intent{}, ErrNotFound
	  }
	   if it.State.Terminal() {
		   return it, ErrTerminal
	  }
	   it.Halted = true
	   it.Reason = "killed"
	  e.stamp(&it)
	   if err := e.syncLocked(); err != nil {
		   return it, err
	   }
	   return it, nil
  }

// Compensate 按已落分录收口。已贷记且不允许冲正则进台，避免再贷一次。
   func (e *Engine) Compensate(id, reason string) (Intent, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   it, ok := e.mem.intent(id)
	  if !ok {
		   return Intent{}, ErrNotFound
	  }
	  out, err := e.compensateLocked(&it, reason, false)
	   if syncErr := e.syncLocked(); syncErr != nil && err == nil {
		   err = syncErr
	  }
	   return out, err
   }

  func (e *Engine) compensateLocked(it *Intent, reason string, allowReverse bool) (Intent, error) {
	   if it.State == StateCommitted {
		   return *it, ErrTerminal
	  }
	  if it.State == StateCompensated || it.State == StateAborted {
		  return *it, nil
	  }
	  if it.CreditPosted && !allowReverse {
		   if it.State != StateDesk {
			  it.Manual = true
			   _ = e.parkDesk(it, "credit_hold", reason)
		   }
		   return *it, ErrDesk
	  }
	  it.State = StateCompensating
	  if reason != "" {
		  it.Reason = reason
	  }
	  e.stamp(it)
	   metricsState(StateCompensating)
	  if it.CreditPosted && allowReverse {
		   if err := e.reverseCredit(it); err != nil {
			  return *it, err
		   }
	   }
	   if err := e.voidMinted(it); err != nil {
		  return *it, err
	  }
	   if err := e.releaseFreeze(it); err != nil {
		   return *it, err
	   }
	  if !e.mem.hasPosting(it.ID, StepFreeze) && !e.mem.hasPosting(it.ID, StepMint) && !it.CreditPosted {
		   if _, err := e.pushStep(it, StepAbort, it.SrcShard, it.Reason, false, false); err != nil {
			  return *it, err
		  }
		   it.State = StateAborted
		   metricsState(StateAborted)
	  } else {
		  if _, err := e.pushStep(it, StepCompensate, it.SrcShard, it.Reason, it.FreezePosted, it.CreditPosted); err != nil {
			  return *it, err
		  }
		  it.State = StateCompensated
		   metricCompensate.Inc()
		  metricsState(StateCompensated)
	  }
	  it.Halted = false
	  it.Deadline = zeroTime()
	   e.stamp(it)
	   e.hint("xshard.compensate", *it, it.VoucherID)
	  return *it, nil
   }

  func (e *Engine) reverseCredit(it *Intent) error {
	   if it.ReverseCount > 0 || e.mem.hasPosting(it.ID, StepReverse) {
		   return nil
	  }
	   if !it.CreditPosted || it.CreditCount == 0 {
		   return ErrDoubleCredit
	  }
	   if _, _, err := admitStep(StepReverse, true, true); err != nil {
		   return err
	  }
	   if err := e.applyDeltas(*it,
		   delta{shard: it.DstShard, subject: it.DstSubject, asset: it.Asset, dAvail: it.Amount, kind: StepReverse, voucher: it.VoucherID, memo: "reverse_target"},
		  delta{shard: it.SrcShard, subject: it.SrcSubject, asset: it.Asset, cAvail: it.Amount, kind: StepReverse, voucher: it.VoucherID, memo: "reverse_source"},
	  ); err != nil {
		   return err
	  }
	  it.ReverseCount = 1
	   if _, err := e.pushStep(it, StepReverse, it.DstShard, "reverse", true, true); err != nil {
		  return err
	   }
	  e.stamp(it)
	  return nil
   }

   func (e *Engine) voidMinted(it *Intent) error {
	  if it.VoucherID == "" {
		  return nil
	  }
	  v, ok := e.mem.voucher(it.VoucherID)
	   if !ok {
		  return ErrVoucher
	  }
	   if v.State == VoucherVoided || v.State == VoucherConsumed {
		   return nil
	  }
	   if v.State != VoucherMinted {
		   return ErrVoucher
	   }
	   if v.CreditCount > 0 {
		   return ErrDoubleCredit
	  }
	   if _, _, err := admitStep(StepVoid, true, false); err != nil {
		  return err
	   }
	  if err := e.applyDeltas(*it,
		   delta{shard: v.BridgeShard, subject: SubBridge, asset: it.Asset, dAvail: it.Amount, kind: StepVoid, voucher: v.ID, memo: "void_bridge"},
		  delta{shard: it.SrcShard, subject: it.SrcSubject, asset: it.Asset, cFrozen: it.Amount, kind: StepVoid, voucher: v.ID, memo: "void_frozen"},
	  ); err != nil {
		  return err
	   }
	   v.State = VoucherVoided
	  v.VoidedAt = e.now()
	   e.mem.putVoucher(v)
	   metricsVoucher(VoucherVoided)
	  _, err := e.pushStep(it, StepVoid, v.BridgeShard, v.ID, true, false)
	  return err
  }

  func (e *Engine) releaseFreeze(it *Intent) error {
	   if e.mem.hasPosting(it.ID, StepUnfreeze) {
		  return nil
	  }
	   b := e.mem.balance(it.SrcShard, it.SrcSubject, it.Asset)
	   held, err := money.ParseBook(nzZero(b.Frozen))
	  if err != nil {
		   return err
	  }
	  if held.IsZero() {
		   return nil
	   }
	   want, err := money.Parse(it.Amount)
	  if err != nil {
		  return err
	   }
	  if !held.Equal(want) {
		   it.Manual = true
		  return e.parkDesk(it, "freeze_hold", "frozen residual "+held.String())
	   }
	   if _, _, err := admitStep(StepUnfreeze, true, it.CreditPosted); err != nil {
		   return err
	   }
	  if err := e.applyDeltas(*it, delta{
		   shard: it.SrcShard, subject: it.SrcSubject, asset: it.Asset,
		   dFrozen: it.Amount, cAvail: it.Amount, kind: StepUnfreeze, memo: "unfreeze_source",
	  }); err != nil {
		  return err
	  }
	   _, err = e.pushStep(it, StepUnfreeze, it.SrcShard, "unfreeze", true, it.CreditPosted)
	   return err
   }

  func (e *Engine) parkDesk(it *Intent, kind, reason string) error {
	   if kind == "route_drift" {
		   metricDrift.Inc()
	   }
	   e.mem.addAlert(Alert{
		  ID: e.id("alt"), Kind: kind, IntentID: it.ID, Detail: reason, At: e.now(),
	   })
	   it.State = StateDesk
	   it.Manual = true
	  it.Reason = reason
	  if it.DeskID == "" {
		   d := DeskCase{
			   ID: e.id("dsk"), IntentID: it.ID, State: DeskOpen,
			   OpenedBy: "ledshard-worker", Reason: reason,
			   CreatedAt: e.now(), UpdatedAt: e.now(),
		  }
		  e.mem.putDesk(d)
		   it.DeskID = d.ID
	  }
	   e.stamp(it)
	   _, _ = e.pushStep(it, StepDesk, "", reason, it.FreezePosted, it.CreditPosted)
	  metricsState(StateDesk)
	  if kind == "route_drift" {
		   return ErrDrift
	   }
	  return ErrDesk
  }

// Conserve 用分录轧差核对源出、目标入和桥净额。

  func (e *Engine) Conserve(id string) (ConserveReport, error) {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  it, ok := e.mem.intent(id)
	  if !ok {
		  return ConserveReport{},
			   ErrNotFound
	  }
	  return e.conserveLocked(it)
  }

   func (e *Engine) conserveLocked(it Intent) (ConserveReport, error) {
	  nets := map[string]*signed{}
	   touch := func(shard, subject string) *signed {
		  k := shard + "|" + subject
		   if nets[k] == nil {
			  nets[k] = &signed{}
		  }
		  return nets[k]
	  }
	   for _, p := range e.mem.listPostings(it.ID) {
		   n := touch(p.ShardID, p.Subject)
		  n.avail = n.avail.Add(decOf(p.CreditAvail)).Sub(decOf(p.DebitAvail))
		   n.frozen = n.frozen.Add(decOf(p.CreditFrozen)).Sub(decOf(p.DebitFrozen))
	   }
	  src := touch(it.SrcShard, it.SrcSubject)
	  dst := touch(it.DstShard, it.DstSubject)
	  br := touch(e.bridgeShard(), SubBridge)
	   frozenSum := decimal.Zero
	   for _, n := range nets {
		   frozenSum = frozenSum.Add(n.frozen)
	   }
	   rep := ConserveReport{
		  SrcOut:    src.avail.Neg().String(),
		   DstIn:     dst.avail.String(),
		  BridgeNet: br.avail.String(),
		   FrozenLeft: frozenSum.String(),
		  CreditN:    it.CreditCount,
	  }
	  if it.VoucherID != "" {
		   if v, ok := e.mem.voucher(it.VoucherID); ok {
			   rep.VoucherID = v.ID
			   rep.Voucher = string(v.State)
		   }
	  }
	   want := decOf(it.Amount)
	   switch it.State {
	   case StateCommitted:
		  if !src.avail.Equal(want.Neg()) || !dst.avail.Equal(want) || !br.avail.IsZero() || !frozenSum.IsZero() {
			  return rep, nil
		   }
		   if it.CreditCount != 1 || rep.Voucher != string(VoucherConsumed) {
			   return rep, nil
		  }
		  out, err := money.Parse(want.String())
		   if err != nil {
			  return rep, err
		  }
		   in, err := money.Parse(dst.avail.String())
		   if err != nil {
			  return rep, err
		   }
		  if err := money.Conserve(out, in); err != nil {
			  return rep, nil
		   }
		   rep.SrcOut = want.String()
		  rep.OK = true
		   return rep, nil
	   case StateAborted, StateCompensated:
		   if src.avail.IsZero() && dst.avail.IsZero() && br.avail.IsZero() && frozenSum.IsZero() && it.CreditCount == it.ReverseCount {
			  rep.OK = true
			  rep.SrcOut = "0"
		   }
		  return rep, nil
	   default:
		   rep.OK = it.CreditCount <= 1
		   return rep, nil
	  }
   }

type signed struct{ avail, frozen decimal.Decimal }

   func decOf(s string) decimal.Decimal {
	  if s == "" || s == "0" {
		  return decimal.Zero
	   }
	   d, err := decimal.NewFromString(s)
	  if err != nil {
		   return decimal.Zero
	  }
	  return d
  }
