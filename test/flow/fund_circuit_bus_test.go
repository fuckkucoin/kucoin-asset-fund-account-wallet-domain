package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

// fcClock 可推进的夹具时钟。

type fcClock struct{ at time.Time }

  func (c *fcClock) now() time.Time { return c.at }

  func (c *fcClock) add(d time.Duration) { c.at = c.at.Add(d) }

// newFCBus 建一条带固定时钟与独立占用表的总线。

   func newFCBus(t *testing.T) (*fundcircuit.Bus, *fcClock, *usage.Store) {
	   t.Helper()
	  clk := &fcClock{at: time.Date(2022, 8, 3, 9, 0, 0, 0, time.UTC)}
	   use := usage.New().WithClock(clk.now)
	  bus := fundcircuit.New().WithClock(clk.now).BindUsage(use)
	  return bus, clk, use
  }

// floodCrossPath 用入金与出金两条入口的信号把某用户打到跳闸档。
   func floodCrossPath(t *testing.T, bus *fundcircuit.Bus, user, chain, asset string) {
	  t.Helper()
	  ctx := context.Background()
	  depKey, err := usage.DepVelocityKey(user, chain, asset)
	   if err != nil {
		  t.Fatalf("dep key: %v", err)
	  }
	   for i := 0; i < 3; i++ {
		   if _, _, err := bus.ObserveDepKey(ctx, depKey, "flood observe"); err != nil {
			  t.Fatalf("observe dep signal: %v", err)
		   }
	   }
	   for i := 0; i < 2; i++ {
		   if _, _, err := bus.ObserveWithdrawKey(ctx, "wd:"+user+":"+asset, "flood request"); err != nil {
			   t.Fatalf("observe wd signal: %v", err)
		  }
	   }
  }

// TestFundCircuitCrossPathFloodBlocksBothWrites 跨路径刷量跳闸后入金观察与出金申请同时被挡。

  func TestFundCircuitCrossPathFloodBlocksBothWrites(t *testing.T) {
	   ctx := context.Background()
	  bus, _, _ := newFCBus(t)
	   const user, chain, asset = "u-flood", "ETH", "USDT"
	  if err := bus.AdmitDepositObserve(ctx, user, chain, asset); err != nil {
		  t.Fatalf("first observe must pass: %v", err)
	   }
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); err != nil {
		  t.Fatalf("first request must pass: %v", err)
	  }
	  floodCrossPath(t, bus, user, chain, asset)
	  key, err := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	   if err != nil {
		  t.Fatalf("scope key: %v", err)
	  }
	  row := bus.Circuits().Snapshot(key)
	   if row.State != fundcircuit.StateOpen {
		   t.Fatalf("circuit state %s score %d", row.State, row.Score)
	  }
	   if !row.CrossPath {
		  t.Fatal("cross path evidence missing")
	  }
	   if row.Severity.Rank() < fundcircuit.SevTrip.Rank() {
		  t.Fatalf("severity %s", row.Severity)
	  }
	   if err := bus.AdmitDepositObserve(ctx,
		  user, chain, asset); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		   t.Fatalf("deposit observe must be blocked: %v", err)
	  }
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		  t.Fatalf("withdraw request must be blocked: %v", err)
	  }
	  if err := bus.AdmitTransferHop(ctx, user, asset, "main", "trade"); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		  t.Fatalf("transfer hop must be blocked: %v", err)
	  }
	   // 其它用户不受本作用域影响。
	   if err := bus.AdmitDepositObserve(ctx, "u-other", chain, asset); err != nil {
		   t.Fatalf("other user blocked: %v", err)
	  }
	   cs, ok := bus.Cases().OpenFor(key)
	   if !ok {
		   t.Fatal("case not opened on trip")
	   }
	  if cs.State != fundcircuit.CaseOpen || cs.CircuitKey != key {
		  t.Fatalf("case %+v", cs)
	  }
	   board := bus.Board(ctx, user, chain, asset)
	   if board.AutoUnfreeze {
		   t.Fatal("board must never advertise auto unfreeze")
	   }
	  blocked := 0
	  for _, row := range board.Rows {
		   if row.Blocked {
			  blocked++
		   }
	  }
	   if blocked != len(fundcircuit.Paths()) {
		   t.Fatalf("board blocked rows %d: %+v", blocked, board.Rows)
	  }
   }

// TestFundCircuitKeepsInflightFrozen 跳闸与解除都不自动解冻筛查中 / 已广播的在途单。

   func TestFundCircuitKeepsInflightFrozen(t *testing.T) {
	  ctx := context.Background()
	  bus, _, _ := newFCBus(t)
	   const user, chain, asset = "u-inflight", "ETH", "USDT"
	  // 出金域先有一笔筛查中与一笔已广播的在途单。
	   wd := withdraw.New(nil)
	  wd.Runs().Put(withdraw.Run{
		  ID: "run-screen", TransferID: "tr-screen", UserID: user, Asset: asset, Amount: "5",
		   Freeze: withdraw.FreezeScreening, Step: withdraw.StepRequested,
	  })
	   wd.Runs().Put(withdraw.Run{
		   ID: "run-bc", TransferID: "tr-bc", UserID: user, Asset: asset, Amount: "7",
		  Freeze: withdraw.FreezeBroadcast,
		   Step:   withdraw.StepSubmitted,
	   })
	   floodCrossPath(t, bus, user, chain, asset)
	  key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		  t.Fatalf("circuit state %s", got.State)
	   }
	  // 熔断期间任何解冻核验都必须被拒。
	   for _, ref := range []fundcircuit.InflightRef{
		   {TransferID: "tr-screen", UserID: user, Asset: asset, FreezeState: string(withdraw.FreezeScreening)},
		  {TransferID: "tr-bc", UserID: user, Asset: asset, FreezeState: string(withdraw.FreezeBroadcast)},
		  {TransferID: "tr-bc", UserID: user, Asset: asset, Broadcast: string(withdraw.BcConfirming)},
		  {TransferID: "tr-bc", UserID: user, Asset: asset, TxHash: "0xabc"},
	   } {
		   if err := bus.AdmitUnfreeze(ctx, ref); !errors.Is(err, fundcircuit.ErrUnfreezeForbidden) {
			  t.Fatalf("unfreeze must be refused for %+v: %v", ref, err)
		  }
	  }
	   // 在途单相位未被总线改写。
	   if row, ok := wd.Runs().Get("run-screen"); !ok || row.Freeze != withdraw.FreezeScreening {
		  t.Fatalf("screening run mutated: %+v", row)
	   }
	  if row, ok := wd.Runs().Get("run-bc"); !ok || row.Freeze != withdraw.FreezeBroadcast {
		   t.Fatalf("broadcast run mutated: %+v", row)
	  }
	   // 出金域自身的解冻禁令仍然生效，熔断没有放宽它。
	   if err := withdraw.RefuseAutoUnfreeze(withdraw.Broadcast{Status: withdraw.BcBroadcast}); err == nil {
		  t.Fatal("withdraw live-hash guard relaxed")
	   }
	   // 双人解除熔断后，在途单依旧不得自动解冻。
	  req, err := bus.Desk().OpenClear(ctx, "desk-a", key, "已核实为压测流量")
	  if err != nil {
		  t.Fatalf("open clear: %v", err)
	  }
	   voted, err := bus.Desk().VoteClear(ctx, req.ID, "desk-a", true, req.Fence.Epoch, req.Fence.Token)
	   if err != nil {
		  t.Fatalf("vote a: %v", err)
	  }
	   ready, err := bus.Desk().VoteClear(ctx, voted.ID, "desk-b", true, voted.Fence.Epoch, voted.Fence.Token)
	  if err != nil {
		  t.Fatalf("vote b: %v", err)
	   }
	  if _, _, err := bus.Desk().ExecuteClear(ctx, ready.ID, "desk-b", ready.Fence.Epoch, ready.Fence.Token); err != nil {
		   t.Fatalf("execute clear: %v", err)
	  }
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateClosed {
		   t.Fatalf("circuit after clear %s", got.State)
	   }
	  if err := bus.AdmitUnfreeze(ctx, fundcircuit.InflightRef{
		  TransferID: "tr-bc", FreezeState: string(withdraw.FreezeBroadcast),
	   }); !errors.Is(err, fundcircuit.ErrUnfreezeForbidden) {
		   t.Fatalf("clear must not unlock in-flight order: %v", err)
	   }
	  if fundcircuit.UnfreezeAllowed() {
		   t.Fatal("bus must never allow auto unfreeze")
	   }
	  for _, row := range bus.Policy().Current().MatrixView() {
		  if row.AutoUnfreeze {
			  t.Fatalf("matrix row advertises unfreeze: %+v", row)
		  }
	  }
  }

// TestFundCircuitPolicyRejectsUnfreeze 带解冻语义的策略不得落版。
  func TestFundCircuitPolicyRejectsUnfreeze(t *testing.T) {
	  bus, _, _ := newFCBus(t)
	   doc := bus.Policy().Current()
	  doc.AutoUnfreeze = true
	   if _, err := bus.Policy().Put("desk-a", doc); !errors.Is(err, fundcircuit.ErrUnfreezeForbidden) {
		   t.Fatalf("policy with auto unfreeze must be rejected: %v", err)
	   }
	  if bus.Policy().Current().AutoUnfreeze {
		  t.Fatal("current policy mutated")
	  }
	   // 阈值不递增同样拒绝落版。
	   bad := bus.Policy().Current()
	  bad.TripScore = 10
	   bad.WarnScore = 50
	   if _, err := bus.Policy().Put("desk-a", bad); err == nil {
		  t.Fatal("non monotonic thresholds accepted")
	  }
   }

// TestFundCircuitHalfOpenProbe 半开只放一枚探针：成功闭合，失败重开。
  func TestFundCircuitHalfOpenProbe(t *testing.T) {
	  ctx := context.Background()
	  bus, clk, _ := newFCBus(t)
	   const user, chain, asset = "u-probe", "ETH", "USDT"
	   floodCrossPath(t,
		   bus, user, chain, asset)
	  key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		   t.Fatalf("state %s", got.State)
	  }
	   pol := bus.Policy().Current()
	  // 冷却未到不得进半开。
	   if trs := bus.AdvanceOnce(ctx); len(trs) != 0 {
		   t.Fatalf("cooldown not elapsed but advanced: %+v", trs)
	   }
	   // 冷却到期后推进半开；同时让窗口内的旧信号退场。
	  clk.add(pol.Cooldown + pol.Window + time.Second)
	  trs := bus.AdvanceOnce(ctx)
	   if len(trs) == 0 {
		  t.Fatal("cooldown elapsed but no half_open transition")
	   }
	   row := bus.Circuits().Snapshot(key)
	   if row.State != fundcircuit.StateHalfOpen {
		  t.Fatalf("state %s", row.State)
	  }
	   // 半开期没有探针时仍然挡新写。
	   if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		  t.Fatalf("half_open without probe must block: %v", err)
	   }
	  probe, err := bus.Probe(ctx, key, fundcircuit.PathWithdrawRequest, "desk-a")
	  if err != nil {
		  t.Fatalf("issue probe: %v", err)
	   }
	  if _, err := bus.Probe(ctx, key, fundcircuit.PathWithdrawRequest, "desk-a"); err == nil {
		  t.Fatal("second probe accepted in half_open")
	  }
	  // 探针在手时放行一笔试写。
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); err != nil {
		   t.Fatalf("probe write blocked: %v", err)
	  }
	   if _, _, err := bus.SettleProbe(ctx, probe.ID, probe.Token, true, "desk-a", "试写成功"); err != nil {
		   t.Fatalf("settle probe: %v", err)
	  }
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateClosed {
		   t.Fatalf("probe pass must close circuit, got %s", got.State)
	  }
	  if err := bus.AdmitDepositObserve(ctx, user, chain, asset); err != nil {
		   t.Fatalf("closed circuit still blocking: %v", err)
	  }
	   // 第二轮：探针失败必须重开，且冷却按次退避加长。
	  floodCrossPath(t, bus, user, chain, asset)
	   if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		   t.Fatalf("second trip state %s", got.State)
	   }
	  trips := bus.Circuits().Snapshot(key).Trips
	   clk.add(pol.CooldownFor(trips) + pol.Window + time.Second)
	   if trs := bus.AdvanceOnce(ctx); len(trs) == 0 {
		   t.Fatal("second cooldown did not advance")
	  }
	   p2, err := bus.Probe(ctx, key, fundcircuit.PathDepositObserve, "desk-a")
	  if err != nil {
		   t.Fatalf("issue second probe: %v", err)
	   }
	  if _, _, err := bus.SettleProbe(ctx, p2.ID, p2.Token, false, "desk-a", "试写仍被刷量"); err != nil {
		   t.Fatalf("settle second probe: %v", err)
	  }
	  reopened := bus.Circuits().Snapshot(key)
	  if reopened.State != fundcircuit.StateOpen {
		  t.Fatalf("probe fail must reopen, got %s", reopened.State)
	  }
	   if reopened.Trips <= trips {
		  t.Fatalf("trips not advanced: %d -> %d", trips, reopened.Trips)
	  }
	  if !reopened.NextAdvanceAt.After(clk.now().Add(pol.Cooldown)) {
		  t.Fatalf("cooldown backoff missing: next=%s now=%s", reopened.NextAdvanceAt, clk.now())
	  }
	  if err := bus.AdmitDepositObserve(ctx, user, chain, asset); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		   t.Fatalf("reopened circuit must block: %v", err)
	  }
	  // 过期探针等同失败：签发后不回报，扫描时熔断重开。
	  clk.add(pol.CooldownFor(reopened.Trips) + pol.Window + time.Second)
	  bus.AdvanceOnce(ctx)
	  p3, err := bus.Probe(ctx, key, fundcircuit.PathDepositObserve, "desk-a")
	  if err != nil {
		  t.Fatalf("issue third probe: %v", err)
	   }
	  clk.add(pol.ProbeTTL + time.Second)
	  expired, back := bus.ProbeSweepOnce(ctx)
	  if len(expired) != 1 || expired[0].ID != p3.ID {
		  t.Fatalf("probe not expired: %+v", expired)
	  }
	  if len(back) != 1 || back[0].To != fundcircuit.StateOpen {
		   t.Fatalf("expired probe must reopen: %+v", back)
	   }
   }

// TestFundCircuitDualControlClear 单人 Clear 被拒；双人 Clear 记票并留审计。
   func TestFundCircuitDualControlClear(t *testing.T) {
	   ctx := context.Background()
	   bus, _, _ := newFCBus(t)
	  const user, chain, asset = "u-dual", "ETH", "USDT"
	   floodCrossPath(t, bus, user, chain, asset)
	  key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	   // 单人直解：拒绝，并在案件上留 clear_denied。
	  if err := bus.Desk().ForceClear(ctx, "desk-a", key, "单人解除"); !errors.Is(err, fundcircuit.ErrDualControl) {
		  t.Fatalf("single desk clear must be rejected: %v", err)
	   }
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		   t.Fatalf("circuit opened state changed by rejected clear: %s", got.State)
	   }
	   cs, ok := bus.Cases().OpenFor(key)
	  if !ok {
		  t.Fatal("case missing")
	   }
	  denied := false
	   for _, ev := range cs.Audit {
		  if ev.Action == "clear_denied" {
			   denied = true
		   }
	  }
	  if !denied {
		   t.Fatalf("clear_denied not audited: %+v", cs.Audit)
	   }
	   // 同一人投两票也不成局。
	  req, err := bus.Desk().OpenClear(ctx, "desk-a", key, "复核后解除")
	   if err != nil {
		  t.Fatalf("open clear: %v", err)
	   }
	  voted, err := bus.Desk().VoteClear(ctx, req.ID, "desk-a", true, req.Fence.Epoch, req.Fence.Token)
	   if err != nil {
		   t.Fatalf("vote a: %v", err)
	  }
	   if _, err := bus.Desk().VoteClear(ctx, voted.ID, "desk-a", true, voted.Fence.Epoch, voted.Fence.Token); err == nil {
		   t.Fatal("same actor voted twice")
	  }
	   if _, _, err := bus.Desk().ExecuteClear(ctx, voted.ID, "desk-a", voted.Fence.Epoch, voted.Fence.Token); err == nil {
		  t.Fatal("execute without quorum accepted")
	   }
	   if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		  t.Fatalf("circuit closed without quorum: %s", got.State)
	  }
	  // 第二名值班投票后执行，熔断闭合、案件结案、审计留票。
	   ready, err := bus.Desk().VoteClear(ctx, voted.ID, "desk-b", true, voted.Fence.Epoch, voted.Fence.Token)
	  if err != nil {
		  t.Fatalf("vote b: %v", err)
	   }
	   row, cleared, err := bus.Desk().ExecuteClear(ctx, ready.ID, "desk-b", ready.Fence.Epoch, ready.Fence.Token)
	   if err != nil {
		   t.Fatalf("execute clear: %v", err)
	   }
	   if row.State != fundcircuit.StateClosed {
		  t.Fatalf("circuit state %s", row.State)
	  }
	  if cleared.State != fundcircuit.CaseCleared {
		   t.Fatalf("case state %s", cleared.State)
	   }
	   if len(cleared.ClearVotes) < 2 {
		  t.Fatalf("clear votes %+v", cleared.ClearVotes)
	  }
	   executed := false
	   for _, ev := range bus.Desk().Audit(50) {
		  if ev.Action == "clear_executed" && ev.Key == key && len(ev.Votes) >= 2 {
			   executed = true
		  }
	  }
	  if !executed {
		  t.Fatalf("clear_executed not audited: %+v", bus.Desk().Audit(50))
	  }
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); err != nil {
		   t.Fatalf("writes still blocked after dual clear: %v", err)
	   }
  }

// TestFundCircuitKeyspaceCoexists fc: 键与 dep:/wd: 键并存，桥接不改写原键。
  func TestFundCircuitKeyspaceCoexists(t *testing.T) {
	  ctx := context.Background()
	   bus, _, use := newFCBus(t)
	  const user, chain, asset = "u-keys", "ETH", "USDT"
	   depKey, err := usage.DepVelocityKey(user, chain, asset)
	   if err != nil {
		   t.Fatalf("dep key: %v", err)
	  }
	  // 原域先记两笔入金占用。
	   for i := 0; i < 2; i++ {
		  if err := use.AddDep(depKey, "1"); err != nil {
			   t.Fatalf("add dep: %v", err)
		  }
	   }
	   before := use.DepVelocity(depKey, time.Minute)
	   if before != 2 {
		   t.Fatalf("dep velocity %d", before)
	  }
	  // 桥接进总线：生成 fc: 键，dep: 原键原文保留。
	   sig, _, err := bus.ObserveDepKey(ctx, depKey, "bridge")
	  if err != nil {
		   t.Fatalf("observe dep key: %v", err)
	   }
	   if sig.SourceKey != depKey {
		  t.Fatalf("source key rewritten: %q", sig.SourceKey)
	   }
	  if !usage.IsDepKey(sig.SourceKey) || fundcircuit.IsKey(sig.SourceKey) {
		   t.Fatalf("source key namespace drift: %q", sig.SourceKey)
	   }
	   if got := use.DepVelocity(depKey, time.Minute); got != before {
		  t.Fatalf("bus mutated dep velocity: %d -> %d", before, got)
	   }
	   for _, key := range fundcircuit.KeysFor(sig) {
		  if !fundcircuit.IsKey(key) {
			   t.Fatalf("circuit key without fc: prefix: %q", key)
		  }
		  if usage.IsDepKey(key) {
			  t.Fatalf("circuit key collides with dep keyspace: %q", key)
		   }
		   if err := fundcircuit.GuardKey(key); err != nil {
			  t.Fatalf("guard key %q: %v", key, err)
		   }
	   }
	   // 原域键不得当作熔断键写入。
	  if err := fundcircuit.GuardKey(depKey); !errors.Is(err, fundcircuit.ErrForeignKey) {
		  t.Fatalf("dep key accepted as circuit key: %v", err)
	   }
	   if err := fundcircuit.GuardKey("wd:" + user + ":" + asset); !errors.Is(err, fundcircuit.ErrForeignKey) {
		   t.Fatalf("wd key accepted as circuit key: %v", err)
	  }
	   // 非 dep: 键不得被桥接成入金信号。
	  if _, err := fundcircuit.BridgeDep("ab:"+user+"|ETH|USDT", 0, ""); !errors.Is(err, fundcircuit.ErrBridgeKey) {
		  t.Fatalf("foreign prefix bridged: %v", err)
	  }
	   if _, err := fundcircuit.BridgeWithdraw("dep:"+user+"|ETH|USDT", 0, ""); !errors.Is(err, fundcircuit.ErrBridgeKey) {
		  t.Fatalf("dep key bridged as withdraw: %v", err)
	  }
	   wdSig, _, err := bus.ObserveWithdrawKey(ctx, "wd:"+user+":"+asset, "bridge")
	  if err != nil {
		   t.Fatalf("observe wd key: %v", err)
	   }
	  if wdSig.SourceKey != "wd:"+user+":"+asset {
		  t.Fatalf("wd source key rewritten: %q", wdSig.SourceKey)
	  }
	  stats := bus.Stats()
	   if stats.Keyspace != fundcircuit.Prefix {
		   t.Fatalf("keyspace %q", stats.Keyspace)
	   }
	   for _, note := range stats.Sources {
		   if note.Mutated {
			  t.Fatalf("bridged source reported as mutated: %+v", note)
		  }
	   }
  }

// TestFundCircuitGlobalScopeAndHaltEscalation 全局强开挡住三条入口，并收紧托管停提解除。

   func TestFundCircuitGlobalScopeAndHaltEscalation(t *testing.T) {
	   ctx := context.Background()
	   bus, _, _ := newFCBus(t)
	   if _, err := bus.Desk().ForceOpen(ctx, "desk-a", fundcircuit.ScopeGlobal, "", "", "", "热钱包异常"); err != nil {
		  t.Fatalf("force open global: %v", err)
	  }
	  for _, path := range fundcircuit.Paths() {
		   var err error
		   switch path {
		   case fundcircuit.PathDepositObserve:
			  err = bus.AdmitDepositObserve(ctx, "any-user", "ETH", "USDT")
		   case fundcircuit.PathWithdrawRequest:
			   err = bus.AdmitWithdrawRequest(ctx, "any-user", "ETH", "USDT")
		  case fundcircuit.PathTransferHop:
			   err = bus.AdmitTransferHop(ctx, "any-user", "USDT", "main", "trade")
		  }
		  if !errors.Is(err, fundcircuit.ErrCircuitOpen) {
			   t.Fatalf("global circuit must block %s: %v", path, err)
		   }
	   }
	  // 强开的熔断器不自动进半开，必须双人解除。
	   if trs := bus.AdvanceOnce(ctx); len(trs) != 0 {
		  t.Fatalf("forced circuit auto advanced: %+v", trs)
	   }
	   // 熔断未解除时收紧托管停充提的 Clear。
	   if err := bus.AllowClear(ctx); !errors.Is(err, custodyhalt.ErrClearBlocked) {
		   t.Fatalf("custody clear must be blocked while circuits open: %v", err)
	   }
	  key := fundcircuit.KeyGlobal
	  req, err := bus.Desk().OpenClear(ctx, "desk-a", key, "热钱包已恢复")
	   if err != nil {
		  t.Fatalf("open clear: %v", err)
	   }
	  voted, err := bus.Desk().VoteClear(ctx, req.ID, "desk-a", true, req.Fence.Epoch, req.Fence.Token)
	   if err != nil {
		   t.Fatalf("vote a: %v", err)
	  }
	   ready, err := bus.Desk().VoteClear(ctx, voted.ID, "desk-b", true, voted.Fence.Epoch, voted.Fence.Token)
	  if err != nil {
		   t.Fatalf("vote b: %v", err)
	  }
	   if _, _, err := bus.Desk().ExecuteClear(ctx, ready.ID, "desk-b", ready.Fence.Epoch, ready.Fence.Token); err != nil {
		  t.Fatalf("execute clear: %v", err)
	   }
	  if err := bus.AllowClear(ctx); err != nil {
		   t.Fatalf("custody clear still blocked after dual clear: %v", err)
	  }
	   if err := bus.AdmitWithdrawRequest(ctx, "any-user", "ETH", "USDT"); err != nil {
		  t.Fatalf("global clear did not restore writes: %v", err)
	   }
   }

// TestFundCircuitScopeGateAndMatrix 阻断矩阵摘掉入口后该入口不再被挡。
   func TestFundCircuitScopeGateAndMatrix(t *testing.T) {
	  ctx := context.Background()
	   bus, _, _ := newFCBus(t)
	  const user, chain, asset = "u-matrix", "ETH", "USDT"
	   doc := bus.Policy().Current()
	  doc.Matrix = map[fundcircuit.Path]fundcircuit.Severity{
		   fundcircuit.PathWithdrawRequest: fundcircuit.SevTrip,
	  }
	   if _, err := bus.Policy().Put("desk-a", doc); err != nil {
		  t.Fatalf("put policy: %v", err)
	  }
	  floodCrossPath(t, bus, user, chain, asset)
	  if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); !errors.Is(err, fundcircuit.ErrCircuitOpen) {
		   t.Fatalf("withdraw must stay blocked: %v", err)
	  }
	  if err := bus.AdmitDepositObserve(ctx, user, chain, asset); err != nil {
		   t.Fatalf("deposit removed from matrix but blocked: %v", err)
	  }
	  // 关掉 user 作用域后连出金也放行（仍留案件与读面）。
	  doc = bus.Policy().Current()
	   doc.ScopeGate = map[fundcircuit.Scope]bool{fundcircuit.ScopeUser: false}
	  if _, err := bus.Policy().Put("desk-a", doc); err != nil {
		  t.Fatalf("put scope gate: %v", err)
	  }
	   if err := bus.AdmitWithdrawRequest(ctx, user, chain, asset); err != nil {
		   t.Fatalf("scope gate off but blocked: %v", err)
	  }
	   key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	   if _, ok := bus.Cases().OpenFor(key); !ok {
		  t.Fatal("case closed by scope gate")
	  }
  }

// TestFundCircuitTickAndSignalDecay 一轮作业推进冷却并摘除窗口外信号。

   func TestFundCircuitTickAndSignalDecay(t *testing.T) {
	  ctx := context.Background()
	   bus, clk, _ := newFCBus(t)
	   const user, chain, asset = "u-decay", "ETH", "USDT"
	  // 窗口比冷却长：冷却结束时旧信号仍在窗口内，聚合必须立刻重开。
	  doc := bus.Policy().Current()
	  doc.Window = 30 * time.Minute
	   doc.SignalTTL = 45 * time.Minute
	   doc.Cooldown = time.Minute
	  doc.CooldownMax = 10 * time.Minute
	  if _, err := bus.Policy().Put("desk-a", doc); err != nil {
		   t.Fatalf("put policy: %v", err)
	  }
	  floodCrossPath(t, bus, user, chain, asset)
	  key, _ := fundcircuit.ScopeKey(fundcircuit.ScopeUser, user, "", "")
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		  t.Fatalf("state %s", got.State)
	  }
	   pol := bus.Policy().Current()
	  clk.add(pol.Cooldown + time.Second)
	  rep := bus.Tick(ctx)
	  if len(rep.Cooled) == 0 {
		  t.Fatalf("tick did not advance cooldown: %+v", rep)
	   }
	  // 冷却推进后窗口内旧信号仍在，重算会立刻重开。
	   if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateOpen {
		  t.Fatalf("stale signals should re-trip, got %s", got.State)
	  }
	  clk.add(pol.SignalTTL + time.Second)
	  rep = bus.Tick(ctx)
	   if rep.SignalsPruned == 0 {
		  t.Fatalf("expired signals not pruned: %+v", rep)
	  }
	   if bus.Signals().Len() != 0 {
		  t.Fatalf("signal ledger not drained: %d", bus.Signals().Len())
	   }
	  clk.add(pol.CooldownFor(bus.Circuits().Snapshot(key).Trips) + time.Second)
	  bus.AdvanceOnce(ctx)
	  if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateHalfOpen {
		   t.Fatalf("state after decay %s", got.State)
	  }
	  probe, err := bus.Probe(ctx, key, fundcircuit.PathDepositObserve, "desk-a")
	  if err != nil {
		  t.Fatalf("issue probe: %v", err)
	   }
	   if _, _, err := bus.SettleProbe(ctx, probe.ID, probe.Token, true, "desk-a", "恢复"); err != nil {
		   t.Fatalf("settle: %v", err)
	  }
	   if got := bus.Circuits().Snapshot(key); got.State != fundcircuit.StateClosed {
		   t.Fatalf("final state %s", got.State)
	   }
  }
