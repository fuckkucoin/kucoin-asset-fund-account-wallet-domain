package ledshard

import (
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Seed 按当前生效路由把可用额记到该科目所在片。系统科目记到钉扎片。
   func (e *Engine) Seed(user, book, asset, amount string) (Balance, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   route, err := e.activeLocked()
	   if err != nil {
		  return Balance{}, err
	  }
	   amt, err := normAmount(amount)
	  if err != nil {
		  return Balance{}, ErrAmount
	   }
	  a, err := e.assignFresh(route, user, book, asset, false)
	   if err != nil {
		   return Balance{}, err
	   }
	  b := e.mem.balance(a.ShardID, a.Subject, strings.ToUpper(strings.TrimSpace(asset)))
	   cur, err := money.ParseBook(nzZero(b.Available))
	   if err != nil {
		  return Balance{}, err
	   }
	  add, err := money.Parse(amt)
	   if err != nil {
		  return Balance{}, err
	  }
	   sum, err := cur.Add(add)
	   if err != nil {
		   return Balance{}, err
	  }
	   b.Available = sum.String()
	  if b.Frozen == "" {
		   b.Frozen = "0"
	   }
	  e.mem.saveBalance(b)
	   if err := e.syncLocked(); err != nil {
		   return Balance{}, err
	  }
	   return b, nil
  }

// BalanceOf 读取该科目在生效路由下的投影。

  func (e *Engine) BalanceOf(user, book, asset string) (Balance, error) {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   route, err := e.activeLocked()
	  if err != nil {
		   return Balance{}, err
	  }
	   a, err := Resolve(route, user, book, asset)
	   if err != nil {
		   return Balance{}, err
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  return e.mem.balance(a.ShardID, a.Subject, asset), nil
  }

// BalanceOn 按意图钉死的片读取，避免代次切换后读到另一片。
   func (e *Engine) BalanceOn(shard, subject, asset string) Balance {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  return e.mem.balance(shard, subject, strings.ToUpper(strings.TrimSpace(asset)))
  }

// Open 创建跨分片意图。同分片返回 ErrSameShard，调用方走原 hop。
// 同一幂等键重放返回原意图，业务指纹不同则冲突，不会改写分片。
   func (e *Engine) Open(req OpenReq) (Intent, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   it, err := e.openLocked(req)
	  if err != nil {
		  return it, err
	  }
	  if err := e.syncLocked(); err != nil {
		   return it, err
	   }
	   return it, nil
   }

  func (e *Engine) openLocked(req OpenReq) (Intent, error) {
	  req.Idem = strings.TrimSpace(req.Idem)
	   req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	  if req.Idem == "" {
		   return Intent{}, ErrIdemConflict
	  }
	   amt, err := normAmount(req.Amount)
	  if err != nil {
		   return Intent{}, ErrAmount
	  }
	   req.Amount = amt
	   hash, err := bizHash(req.SrcUser, req.SrcBook, req.DstUser, req.DstBook, req.Asset, req.Amount)
	   if err != nil {
		  return Intent{}, ErrAmount
	  }
	   if prev, ok := e.mem.intentByIdem(req.Idem); ok {
		  if prev.BizHash != hash {
			  return prev, ErrIdemConflict
		  }
		   return prev, nil
	  }
	   route, err := e.pickRoute(req.RouteVersion)
	  if err != nil {
		   return Intent{}, err
	   }
	  src, err := e.assignFresh(route, req.SrcUser, req.SrcBook, req.Asset, true)
	  if err != nil {
		   return Intent{}, err
	  }
	   dst, err := e.assignFresh(route, req.DstUser, req.DstBook, req.Asset, true)
	   if err != nil {
		   return Intent{}, err
	  }
	   if SamePlacement(src, dst) {
		  return Intent{}, ErrSameShard
	   }
	   if req.ExpectSrc != "" && req.ExpectSrc != src.ShardID {
		   return Intent{}, ErrShardMismatch
	  }
	   if req.ExpectDst != "" && req.ExpectDst != dst.ShardID {
		   return Intent{}, ErrShardMismatch
	   }
	  now := e.now()
	   it := Intent{
		  ID: e.id("int"), Idem: req.Idem, BizHash: hash,
		  RouteVersion: route.Version, RouteFP: route.Fingerprint,
		   SrcUser: req.SrcUser, SrcBook: bookOrPin(req.SrcBook, src), SrcSubject: src.Subject, SrcShard: src.ShardID,
		   DstUser: req.DstUser, DstBook: bookOrPin(req.DstBook, dst), DstSubject: dst.Subject, DstShard: dst.ShardID,
		  Asset: req.Asset, Amount: req.Amount, State: StateOpen,
		   CreatedAt: now, UpdatedAt: now,
	   }
	  e.mem.saveIntent(it)
	   e.mem.idems[req.Idem] = IdemRec{Key: req.Idem, BizHash: hash, Intent: it.ID}
	   metricsState(StateOpen)
	  return it, nil
   }

  func bookOrPin(book string, a Assignment) string {
	   if a.Pin != "" {
		  return a.Pin
	  }
	  return strings.ToLower(strings.TrimSpace(book))
  }

// Advance 向前推进一步。钉扎指纹或重解析对不上时进台，不把分录写到新片。

   func (e *Engine) Advance(id string) (Intent, error) {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  it, err := e.advanceLocked(id)
	   if syncErr := e.syncLocked(); syncErr != nil && err == nil {
		   err = syncErr
	  }
	   return it, err
   }

   func (e *Engine) advanceLocked(id string) (Intent, error) {
	   it, ok := e.mem.intent(id)
	   if !ok {
		  return Intent{}, ErrNotFound
	   }
	  if it.State.Terminal() {
		  return it, ErrTerminal
	  }
	   if it.State == StateDesk {
		  return it, ErrDesk
	  }
	   if err := e.guardRoute(&it); err != nil {
		   return it, err
	  }
	  if it.Halted && it.State == StatePreparing && !it.FreezePosted {
		  return it, ErrHalted
	   }
	  var err error
	  switch it.State {
	   case StateOpen:
		   err = e.stepPrepare(&it)
	   case StatePreparing:
		   err = e.stepFreeze(&it)
	  case StateFrozen:
		   err = e.stepMint(&it)
	   case StateVouchered:
		   err = e.stepCredit(&it)
	   case StateCredited:
		  err = e.stepCommit(&it)
	   case StateCompensating:
		   _, err = e.compensateLocked(&it, it.Reason, false)
		  return it, err
	  default:
		  err = fmt.Errorf("%w: %s", ErrTransition, it.State)
	  }
	  return it, err
  }

   func (e *Engine) guardRoute(it *Intent) error {
	   route, ok := e.mem.route(it.RouteVersion)
	   if !ok {
		   return e.park(it, "pinned route missing")
	  }
	   if route.Fingerprint != it.RouteFP {
		   return e.park(it, "pinned fingerprint drifted")
	  }
	  src, err := Resolve(route, it.SrcUser, it.SrcBook, it.Asset)
	  if err != nil {
		  return e.park(it, "re-resolve source: "+err.Error())
	   }
	  dst, err := Resolve(route, it.DstUser, it.DstBook, it.Asset)
	   if err != nil {
		  return e.park(it, "re-resolve target: "+err.Error())
	  }
	  if src.ShardID != it.SrcShard || dst.ShardID != it.DstShard {
		   return e.park(it, "re-resolve shard mismatch")
	   }
	  return nil
   }

  func (e *Engine) park(it *Intent, reason string) error {
	   metricDrift.Inc()
	   e.mem.addAlert(Alert{
		  ID: e.id("alt"), Kind: "route_drift", IntentID: it.ID, Detail: reason, At: e.now(),
	  })
	  it.State = StateDesk
	   it.Reason = reason
	  it.Manual = true
	   if it.DeskID == "" {
		  d := DeskCase{
			   ID: e.id("dsk"), IntentID: it.ID, State: DeskOpen,
			   OpenedBy: "ledshard-worker", Reason: reason, CreatedAt: e.now(), UpdatedAt: e.now(),
		   }
		  e.mem.putDesk(d)
		   it.DeskID = d.ID
	  }
	  e.stamp(it)
	  _, _ = e.pushStep(it, StepDesk, "", reason, it.FreezePosted, it.CreditPosted)
	  metricsState(StateDesk)
	  return ErrDrift
  }

  func (e *Engine) stepPrepare(it *Intent) error {
	   if _, err := e.pushStep(it, StepPrepare, it.SrcShard, "prepare", false, false); err != nil {
		  return err
	   }
	  it.State = StatePreparing
	   it.Deadline = e.now().Add(e.prepTO)
	   e.stamp(it)
	  e.hint("xshard.prepare", *it, "")
	   metricsState(StatePreparing)
	   return nil
   }

  func (e *Engine) stepFreeze(it *Intent) error {
	   if it.FreezePosted || e.mem.hasPosting(it.ID, StepFreeze) {
		  it.FreezePosted = true
		   it.State = StateFrozen
		   it.Deadline = e.now().Add(e.freezeTO)
		  e.stamp(it)
		   return nil
	  }
	   if _, _, err := admitStep(StepFreeze, true, false); err != nil {
		   return err
	  }
	   if err := e.applyDeltas(*it, delta{
		   shard: it.SrcShard, subject: it.SrcSubject, asset: it.Asset,
		   dAvail: it.Amount, cFrozen: it.Amount, kind: StepFreeze, memo: "freeze_source",
	  }); err != nil {
		   return err
	   }
	   if _, err := e.pushStep(it, StepFreeze, it.SrcShard, "freeze", true, false); err != nil {
		  return err
	  }
	  it.FreezePosted = true
	   it.State = StateFrozen
	   it.Deadline = e.now().Add(e.freezeTO)
	  e.stamp(it)
	   e.hint("xshard.freeze", *it, "")
	   metricsState(StateFrozen)
	  return nil
   }

   func (e *Engine) stepMint(it *Intent) error {
	   if it.VoucherID != "" {
		  it.State = StateVouchered
		  it.Deadline = e.now().Add(e.voucherTO)
		   e.stamp(it)
		  return nil
	   }
	  bridgeShard := e.bridgeShard()
	  if bridgeShard == "" {
		   return ErrUnpinned
	   }
	  v := BridgeVoucher{
		  ID: e.id("vch"), IntentID: it.ID, Idem: it.Idem, Asset: it.Asset, Amount: it.Amount,
		  SrcShard: it.SrcShard, DstShard: it.DstShard, BridgeShard: bridgeShard,
		   State: VoucherMinted, RouteVersion: it.RouteVersion, CreatedAt: e.now(),
	  }
	   if _, _, err := admitStep(StepMint, true, false); err != nil {
		   return err
	  }
	  if err := e.applyDeltas(*it,
		   delta{shard: it.SrcShard, subject: it.SrcSubject, asset: it.Asset, dFrozen: it.Amount, kind: StepMint, voucher: v.ID, memo: "voucher_out"},
		  delta{shard: bridgeShard, subject: SubBridge, asset: it.Asset, cAvail: it.Amount, kind: StepMint, voucher: v.ID, memo: "voucher_in"},
	  ); err != nil {
		  return err
	   }
	   e.mem.putVoucher(v)
	  metricsVoucher(VoucherMinted)
	  if _, err := e.pushStep(it, StepMint, bridgeShard, v.ID, true, false); err != nil {
		   return err
	   }
	   it.VoucherID = v.ID
	   it.State = StateVouchered
	   it.Deadline = e.now().Add(e.voucherTO)
	  e.stamp(it)
	   e.hint("xshard.voucher", *it, v.ID)
	  metricsState(StateVouchered)
	   return nil
   }

  func (e *Engine) bridgeShard() string {
	  if r, ok := e.mem.activeRoute(); ok {
		   if id := r.Pins[SubBridge]; id != "" {
			   return id
		   }
	  }
	   for _, r := range e.mem.routes {
		  if id := r.Pins[SubBridge]; id != "" {
			  return id
		   }
	   }
	   return ShardBridge
   }

   func (e *Engine) stepCredit(it *Intent) error {
	   if it.CreditPosted {
		  it.State = StateCredited
		   e.stamp(it)
		  return nil
	   }
	  if it.CreditCount > 0 {
		  metricDouble.Inc()
		   return ErrDoubleCredit
	   }
	   v, ok := e.mem.voucher(it.VoucherID)
	   if !ok {
		   return ErrVoucher
	   }
	  if v.State == VoucherConsumed || v.CreditCount > 0 {
		  metricDouble.Inc()
		  return ErrDoubleCredit
	   }
	   if v.State != VoucherMinted {
		  return ErrVoucher
	   }
	  if !decOf(v.Amount).Equal(decOf(it.Amount)) || v.DstShard != it.DstShard || v.Asset != it.Asset {
		  return ErrVoucher
	  }
	   if _, _, err := admitStep(StepCredit, true, false); err != nil {
		  return err
	  }
	  if err := e.applyDeltas(*it,
		  delta{shard: v.BridgeShard, subject: SubBridge, asset: it.Asset, dAvail: it.Amount, kind: StepCredit, voucher: v.ID, memo: "bridge_out"},
		  delta{shard: it.DstShard, subject: it.DstSubject, asset: it.Asset, cAvail: it.Amount, kind: StepCredit, voucher: v.ID, memo: "target_in"},
	  ); err != nil {
		  return err
	   }
	  v.State = VoucherConsumed
	  v.CreditCount = 1
	   v.ConsumedAt = e.now()
	   e.mem.putVoucher(v)
	   metricsVoucher(VoucherConsumed)
	   if _, err := e.pushStep(it, StepCredit, it.DstShard, v.ID, true, false); err != nil {
		  return err
	   }
	  it.CreditPosted = true
	  it.CreditCount = 1
	   it.State = StateCredited
	  it.Deadline = e.now().Add(e.voucherTO)
	   e.stamp(it)
	  e.hint("xshard.credit", *it, v.ID)
	   metricsState(StateCredited)
	  return nil
   }

  func (e *Engine) stepCommit(it *Intent) error {
	  rep, err := e.conserveLocked(*it)
	  if err != nil {
		  return err
	   }
	  if !rep.OK {
		  return e.park(it, "commit conservation failed")
	  }
	  if _, err := e.pushStep(it, StepCommit, it.DstShard, "commit", it.FreezePosted, it.CreditPosted); err != nil {
		   return err
	   }
	  it.State = StateCommitted
	  it.Deadline = zeroTime()
	   it.Halted = false
	  e.stamp(it)
	   e.hint("xshard.commit", *it, it.VoucherID)
	  metricsState(StateCommitted)
	   return nil
  }

// Run 连续推进到终态。已经终态的重放直接返回，不会再次贷记。
  func (e *Engine) Run(id string) (Intent, error) {
	  var last Intent
	  for i := 0; i < 8; i++ {
		  e.mu.Lock()
		   cur, ok := e.mem.intent(id)
		  e.mu.Unlock()
		  if !ok {
			   return Intent{}, ErrNotFound
		  }
		   if cur.State.Terminal() {
			  return cur, nil
		   }
		  it, err := e.Advance(id)
		   last = it
		  if err != nil {
			   if it.State.Terminal() && errors.Is(err, ErrTerminal) {
				   return it, nil
			   }
			  return it, err
		  }
		   if it.State.Terminal() {
			  return it, nil
		  }
	  }
	  if last.ID == "" {
		   return Intent{}, ErrStuck
	  }
	  return last, ErrStuck
  }

// Get 返回意图。

   func (e *Engine) Get(id string) (Intent, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  it, ok := e.mem.intent(id)
	   if !ok {
		   return Intent{}, ErrNotFound
	  }
	  return it, nil
  }

// Steps 返回协议步骤。

   func (e *Engine) Steps(id string) []Step {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  return e.mem.listSteps(id)
   }

// Postings 返回该意图的分录。

  func (e *Engine) Postings(id string) []Posting {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.mem.listPostings(id)
   }

// Voucher 按凭证号读取。
  func (e *Engine) Voucher(id string) (BridgeVoucher, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  v, ok := e.mem.voucher(id)
	  if !ok {
		   return BridgeVoucher{}, ErrNotFound
	  }
	  return v, nil
   }

// VoucherOf 按意图读取桥接凭证。
   func (e *Engine) VoucherOf(intentID string) (BridgeVoucher, error) {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  v, ok := e.mem.voucherByIntent(intentID)
	  if !ok {
		   return BridgeVoucher{}, ErrNotFound
	   }
	  return v, nil
  }

// FindPair 找出落在不同用户片上的两个用户，供夹具使用。

  func (e *Engine) FindPair(asset string) (src, dst, srcShard, dstShard string, err error) {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  route, err := e.activeLocked()
	   if err != nil {
		  return "", "", "", "", err
	   }
	   type hit struct{ user, shard string }
	  var hits []hit
	  seen := map[string]struct{}{}
	  for i := 0; i < 4000 && len(hits) < 2; i++ {
		  u := fmt.Sprintf("u-%d", i)
		   a, rerr := Resolve(route, u, "funding", asset)
		   if rerr != nil {
			  return "", "", "", "", rerr
		  }
		  if _, ok := seen[a.ShardID]; ok {
			  continue
		   }
		   seen[a.ShardID] = struct{}{}
		  hits = append(hits, hit{u, a.ShardID})
	   }
	   if len(hits) < 2 {
		   return "", "", "", "", ErrNoRing
	   }
	   return hits[0].user, hits[1].user, hits[0].shard, hits[1].shard, nil
  }
