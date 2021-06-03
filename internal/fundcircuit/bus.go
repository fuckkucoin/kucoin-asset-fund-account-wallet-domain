package fundcircuit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/usage"
)

// HaltEngager 全局停充提升级口。实现由 custodyhalt.Gate 提供。

type HaltEngager interface {
	Engage(ctx context.Context, actor, reason string) error
}

// Bus 熔断总线。各子台账自带锁，总线只保护自己的装配字段与计数。

type Bus struct {
	mu  sync.Mutex
	pol *PolicyRegistry
	sig *Ledger
	cir *Table
	probes *ProbeDesk
	cases *CaseBook
	desk *Desk
	use []*usage.Store
	halt HaltEngager
	haltActor string
	haltErr string
	repo    Repo
	repoErr string
	clock func() time.Time
	admitted  int64
	blocked   int64
	escalated int64
}

// New 建总线。默认策略为 DefaultPolicy，占用表默认独立进程内表。

  func New() *Bus {
	   clock := func() time.Time { return time.Now().UTC() }
	  b := &Bus{
		  pol: NewPolicyRegistry(), sig: NewLedger(DefaultSignalTTL), cir: NewTable(),
		  probes: NewProbeDesk(), cases: NewCaseBook(), desk: NewDesk(2),
		   use: []*usage.Store{usage.New()}, haltActor: "fund-circuit", clock: clock,
	   }
	   b.desk.Bind(b.cir, b.cases)
	  return b
   }

// WithClock 注入时钟，夹具据此推进窗口、冷却与探针有效期。
   func (b *Bus) WithClock(c func() time.Time) *Bus {
	  if b == nil || c == nil {
		   return b
	   }
	  b.mu.Lock()
	  b.clock = c
	  b.mu.Unlock()
	  b.pol.WithClock(c)
	   b.sig.WithClock(c)
	  b.cir.WithClock(c)
	   b.probes.WithClock(c)
	   b.cases.WithClock(c)
	  b.desk.WithClock(c)
	   for _, use := range b.usageSources() {
		  use.WithClock(c)
	   }
	  return b
  }

   func (b *Bus) now() time.Time {
	  if b == nil {
		   return time.Now().UTC()
	   }
	   b.mu.Lock()
	  fn := b.clock
	  b.mu.Unlock()
	  if fn == nil {
		  return time.Now().UTC()
	  }
	  return fn()
  }

// BindUsage 指定主占用表（替换默认表）。只读，不回写占用。
  func (b *Bus) BindUsage(use *usage.Store) *Bus {
	  if b != nil && use != nil {
		   b.mu.Lock()
		   if len(b.use) == 0 {
			   b.use = []*usage.Store{use}
		   } else {
			  b.use[0] = use
		   }
		   b.mu.Unlock()
	  }
	   return b
   }

// AddUsage 追加一个占用表来源。入金与出金各自维护占用表，取最大速度参与判定。
// 功能：让总线读到各域既有的 dep:/wd:/划转占用，而不新建一套速度键。
// 重点：只读；总线不往占用表写入任何一笔。
// 风险点：漏挂来源会让该入口没有速度证据，只能靠显式信号或运营强开。

   func (b *Bus) AddUsage(use *usage.Store) *Bus {
	   if b == nil || use == nil {
		  return b
	   }
	  b.mu.Lock()
	  for _, cur := range b.use {
		   if cur == use {
			  b.mu.Unlock()
			  return b
		   }
	  }
	  b.use = append(b.use, use)
	  b.mu.Unlock()
	   return b
   }

   func (b *Bus) usageSources() []*usage.Store {
	   if b == nil {
		  return nil
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   return append([]*usage.Store{}, b.use...)
  }

// BindHalt 挂托管全局停充提升级口。actor 用于审计。

   func (b *Bus) BindHalt(h HaltEngager, actor string) *Bus {
	  if b == nil {
		   return b
	  }
	   b.mu.Lock()
	   b.halt = h
	  if a := strings.TrimSpace(actor); a != "" {
		  b.haltActor = a
	   }
	   b.mu.Unlock()
	  return b
   }

// Policy 策略代次表。
   func (b *Bus) Policy() *PolicyRegistry {
	  if b == nil {
		  return nil
	  }
	  return b.pol
  }

// Signals 信号台账。
   func (b *Bus) Signals() *Ledger {
	   if b == nil {
		  return nil
	  }
	  return b.sig
  }

// Circuits 熔断器表。

   func (b *Bus) Circuits() *Table {
	   if b == nil {
		   return nil
	  }
	   return b.cir
  }

// Probes 探针台。
   func (b *Bus) Probes() *ProbeDesk {
	   if b == nil {
		  return nil
	  }
	   return b.probes
   }

// Cases 案件台。
  func (b *Bus) Cases() *CaseBook {
	   if b == nil {
		   return nil
	   }
	   return b.cases
   }

// Desk 运营台。
  func (b *Bus) Desk() *Desk {
	  if b == nil {
		   return nil
	   }
	   return b.desk
   }

// Usage 主占用表，供读面与各域复用同一张表核对 dep:/wd: 与 fc: 并存。

   func (b *Bus) Usage() *usage.Store {
	  if b == nil {
		   return nil
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if len(b.use) == 0 {
		   return nil
	  }
	   return b.use[0]
  }

// Observe 收一条信号并就地重算相关作用域。
// 功能：任何来源（入金、出金、划转、限流桶、回压）都经此进总线。
// 重点：信号只写 fc: 键空间；原域 dep:/wd: 占用不被改写。
// 风险点：信号校验失败按错误返回，调用方不得把失败当成放行。
   func (b *Bus) Observe(ctx context.Context, sig AbuseSignal) (AbuseSignal, []Transition, error) {
	  if b == nil {
		  return AbuseSignal{}, nil, ErrBus
	   }
	   row, err := b.sig.Ingest(sig)
	  if err != nil {
		  return AbuseSignal{}, nil, err
	   }
	   metrics.FundCircuitSignals.WithLabelValues(row.Kind.String()).Inc()
	  b.persistSignal(ctx, row)
	  trs := b.Evaluate(ctx, KeysFor(row))
	   return row, trs, nil
   }

// ObserveDepKey 桥接入金速度键后入总线。键形仍由 usage 约束，本函数不改写。

   func (b *Bus) ObserveDepKey(ctx context.Context, key, note string) (AbuseSignal, []Transition, error) {
	  sig, err := BridgeDep(key, 0, note)
	   if err != nil {
		  return AbuseSignal{}, nil, err
	  }
	   return b.Observe(ctx, sig)
   }

// ObserveWithdrawKey 桥接出金刷提键后入总线。

   func (b *Bus) ObserveWithdrawKey(ctx context.Context, key, note string) (AbuseSignal, []Transition, error) {
	  sig, err := BridgeWithdraw(key, 0, note)
	  if err != nil {
		  return AbuseSignal{}, nil, err
	   }
	   return b.Observe(ctx, sig)
  }

// ObserveHop 记一条划转 hop 信号。
  func (b *Bus) ObserveHop(ctx context.Context, user, asset, note string) (AbuseSignal, []Transition, error) {
	   sig, err := BridgeHop(user, asset, 0, note)
	  if err != nil {
		   return AbuseSignal{}, nil, err
	  }
	   return b.Observe(ctx, sig)
  }

// ObserveHTTPBurst 记一条 HTTP 限流桶溢出信号。桶名原样留在 SourceKey。

  func (b *Bus) ObserveHTTPBurst(ctx context.Context, bucket, user, asset string) (AbuseSignal, []Transition, error) {
	   bucket = strings.TrimSpace(bucket)
	  if bucket == "" {
		   return AbuseSignal{}, nil, ErrSignal
	   }
	   return b.Observe(ctx, AbuseSignal{
		   Kind: SigHTTPBurst, UserID: user, Asset: asset,
		  SourceKey: "rl:" + bucket, Note: "http bucket overflow",
	  })
   }

// ObserveJobHalt 记一条作业回压停写信号。

   func (b *Bus) ObserveJobHalt(ctx context.Context, queue, user, asset string) (AbuseSignal, []Transition, error) {
	  queue = strings.TrimSpace(queue)
	   if queue == "" {
		  return AbuseSignal{}, nil, ErrSignal
	   }
	   return b.Observe(ctx, AbuseSignal{
		   Kind: SigJobHalt, UserID: user, Asset: asset,
		  SourceKey: "bp:" + queue, Note: "backpressure halt",
	   })
  }

// Evaluate 重算给定作用域键：达跳闸档则跳闸开案，否则只更新读面分数。
// 功能：把跨入口信号汇总成熔断状态；halt 档可升级全局停充提。
// 重点：不涉及在途单；跳闸只挡后续新写。
// 风险点：策略窗口调大后旧信号仍在窗口内，闸门会延后恢复，须看 next_advance_at。

  func (b *Bus) Evaluate(ctx context.Context, keys []string) []Transition {
	   if b == nil {
		   return nil
	  }
	  pol := b.pol.Current()
	  now := b.now()
	  out := make([]Transition, 0, len(keys))
	  for _, key := range keys {
		   if err := GuardKey(key); err != nil {
			   continue
		  }
		  sigs := b.sig.Window(key, pol.Window)
		   agg := Score(key, sigs, pol, now)
		   // 广度不足时只记读面：单个用户刷量不得把整条资产 / 链 / 全局入口挡掉。
		  if !pol.Trips(agg.Score) || agg.Users < pol.MinUsers(agg.Scope) {
			  b.cir.Note(agg, pol)
			   continue
		   }
		   cur := b.cir.Snapshot(key)
		   if cur.State == StateHalfOpen {
			  // 半开期内又攒够分：直接重开，不等探针结论。
			   if _, tr, err := b.cir.ProbeFailed(key, "", "half-open re-trip: score "+itoa(agg.Score), pol); err == nil {
				  out = append(out, tr)
			  }
			   continue
		   }
		   c, tr, err := b.cir.Trip(agg, pol, tripReason(agg), "", false)
		  if err != nil {
			   continue
		   }
		   if tr.From != tr.To || tr.From == StateClosed {
			  metrics.FundCircuitTrips.WithLabelValues(c.Scope.String(), c.Severity.String()).Inc()
		  }
		   out = append(out, tr)
		  if cs, cerr := b.cases.Open(c, agg, "fund-circuit", tripReason(agg)); cerr == nil {
			  b.cir.BindCase(c.Key, cs.ID)
			  b.persistCase(ctx, cs)
		  }
		   b.escalate(ctx, c, agg, pol)
	   }
	   b.emitGauges()
	  return out
  }

  func tripReason(agg Aggregate) string {
	  kind := "single-path"
	  if agg.CrossPath {
		   kind = "cross-path"
	   }
	  return fmt.Sprintf("%s abuse score %d severity %s signals %d", kind, agg.Score, agg.Severity, agg.Signals)
  }

   func itoa(n int) string { return fmt.Sprintf("%d", n) }

// escalate halt 档且策略允许时升级托管全局停充提。失败只记读面，不回滚熔断。
  func (b *Bus) escalate(ctx context.Context, c Circuit, agg Aggregate, pol BlockPolicy) {
	   if b == nil || !pol.Escalates(agg.Score) {
		   return
	  }
	   b.mu.Lock()
	  h := b.halt
	   actor := b.haltActor
	  b.mu.Unlock()
	  if h == nil {
		   return
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   reason := "fund circuit halt escalation: " + c.Key + " " + tripReason(agg)
	  if err := h.Engage(ctx, actor, reason); err != nil {
		   b.mu.Lock()
		   b.haltErr = err.Error()
		  b.mu.Unlock()
		  return
	   }
	  b.mu.Lock()
	   b.haltErr = ""
	   b.escalated++
	   b.mu.Unlock()
  }

// Admit 入口闸。先按当前熔断状态判定，再把这次写记成信号并重算。
// 功能：三条入口共用一套判定；先窄后宽，任一作用域拦截即拒绝。
// 重点：只读各域状态，不改冻结、不改在途单；决策 AutoUnfreeze 恒 false。
// 风险点：闸门拒绝时调用方须原样返回错误，不得降级为放行或改写在途单。
   func (b *Bus) Admit(ctx context.Context, req GateReq) (Decision, error) {
	  if b == nil {
		  return Decision{}, ErrBus
	  }
	   path := req.Path
	  if !path.Valid() {
		  return Decision{Path: path, InflightPreserved: true}, ErrPath
	   }
	   pol := b.pol.Current()
	   now := b.now()
	  keys := KeysForReq(req)
	  for _, key := range keys {
		   c := b.cir.Snapshot(key)
		   if !c.Blocking() {
			  continue
		   }
		   var probe *Probe
		   if p, ok := b.probes.InFlight(key); ok {
			  probe = &p
		   }
		  d := Decide(pol, c, path, probe, now)
		  if d.Allow {
			   continue
		   }
		  b.mu.Lock()
		   b.blocked++
		  b.mu.Unlock()
		  metrics.FundCircuitBlocks.WithLabelValues(path.String(),
			   c.Scope.String()).Inc()
		   return d, fmt.Errorf("%w: %s", ErrCircuitOpen, d.Reason)
	   }
	   d := Decision{
		  Path: path, Allow: true, PolicyVer: pol.Version, AutoUnfreeze: false,
		  InflightPreserved: true, At: now,
	   }
	  b.mu.Lock()
	   b.admitted++
	  b.mu.Unlock()
	   b.noteWrite(ctx, req)
	  return d, nil
  }

// noteWrite 入口放行后的信号采集。只读占用表；正常频次不产生信号。
// 功能：把各域既有 dep:/wd:/划转占用折成总线信号，实现跨入口联动。
// 重点：仅当窗口内速度超过策略免评笔数（PathFloor）才产生信号，且不写占用表。
// 风险点：占用表未挂来源时此处无输入，跳闸只能靠显式信号或运营强开。
   func (b *Bus) noteWrite(ctx context.Context, req GateReq) {
	  for _, sig := range b.sampleUsage(req) {
		  _, _, _ = b.Observe(ctx, sig)
	   }
   }

// sampleUsage 读既有占用表，把超出免评笔数的速度折成信号。
  func (b *Bus) sampleUsage(req GateReq) []AbuseSignal {
	   sources := b.usageSources()
	  if len(sources) == 0 {
		   return nil
	   }
	  pol := b.pol.Current()
	   win := pol.Window
	   floor := pol.PathFloor
	   user := strings.TrimSpace(req.UserID)
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   chain := strings.ToUpper(strings.TrimSpace(req.Chain))
	   out := make([]AbuseSignal, 0, 1)
	  switch req.Path {
	  case PathDepositObserve:
		  key, err := usage.DepVelocityKey(user, chain, asset)
		  if err != nil {
			  return nil
		   }
		   n := 0
		  for _, use := range sources {
			   if got := use.DepVelocity(key, win); got > n {
				  n = got
			  }
		  }
		   if n <= floor {
			  return nil
		   }
		  out = append(out, AbuseSignal{
			   Kind: SigDepVelocity, Path: PathDepositObserve, UserID: user, Chain: chain, Asset: asset,
			   Weight: SigDepVelocity.BaseWeight() * minInt(n-floor,
				  4),
			   SourceKey: key, Note: "usage dep velocity " + itoa(n) + " floor " + itoa(floor),
		  })
	   case PathWithdrawRequest:
		  if user == "" || asset == "" {
			  return nil
		   }
		   n := 0
		   for _, use := range sources {
			  if got := use.Velocity(usage.KindWithdraw, user, asset, win); got > n {
				  n = got
			   }
		   }
		  if n <= floor {
			  return nil
		   }
		  out = append(out, AbuseSignal{
			   Kind: SigWdVelocity, Path: PathWithdrawRequest, UserID: user, Chain: chain, Asset: asset,
			   Weight:    SigWdVelocity.BaseWeight() * minInt(n-floor, 4),
			  SourceKey: "wd:" + user + ":" + asset,
			   Note: "usage withdraw velocity " + itoa(n) + " floor " + itoa(floor),
		  })
	   case PathTransferHop:
		  src,
			  err := HopSourceKey(user, asset)
		   if err != nil {
			   return nil
		  }
		  n := 0
		  for _, use := range sources {
			  if got := use.Velocity(usage.KindTransfer, user, asset, win); got > n {
				   n = got
			  }
		   }
		   if n <= floor {
			  return nil
		  }
		  out = append(out, AbuseSignal{
			  Kind: SigHopVelocity, Path: PathTransferHop, UserID: user, Asset: asset,
			  Weight: SigHopVelocity.BaseWeight() * minInt(n-floor, 4),
			   SourceKey: src, Note: "usage transfer velocity " + itoa(n) + " floor " + itoa(floor),
		  })
	   }
	   return out
   }

   func minInt(a, b int) int {
	  if a < b {
		   return a
	   }
	  return b
  }

// AdmitDepositObserve 入金观察入口闸。被挡返回 ErrCircuitOpen 包装错误。

   func (b *Bus) AdmitDepositObserve(ctx context.Context, user, chain, asset string) error {
	   _, err := b.Admit(ctx, GateReq{
		  Path: PathDepositObserve,
		   UserID: user, Chain: chain, Asset: asset,
	  })
	  return err
  }

// AdmitWithdrawRequest 出金申请入口闸。只挡新申请，不触碰在途冻结。

   func (b *Bus) AdmitWithdrawRequest(ctx context.Context, user, chain, asset string) error {
	   _, err := b.Admit(ctx, GateReq{
		   Path: PathWithdrawRequest, UserID: user, Chain: chain, Asset: asset,
	   })
	   return err
  }

// AdmitTransferHop 划转 hop 入口闸。
   func (b *Bus) AdmitTransferHop(ctx context.Context, user, asset, fromBook, toBook string) error {
	   _, err := b.Admit(ctx, GateReq{
		   Path: PathTransferHop, UserID: user, Asset: asset, FromBook: fromBook, ToBook: toBook,
	  })
	  return err
   }

// DepositGate 入金域绑定用的只读闸。

   func (b *Bus) DepositGate() func(context.Context, string, string, string) error {
	   return func(ctx context.Context, user, chain, asset string) error {
		  return b.AdmitDepositObserve(ctx, user, chain, asset)
	  }
   }

// WithdrawGate 出金域绑定用的只读闸。
  func (b *Bus) WithdrawGate() func(context.Context, string, string, string) error {
	   return func(ctx context.Context, user, chain, asset string) error {
		   return b.AdmitWithdrawRequest(ctx, user, chain, asset)
	   }
   }

// HopGate 划转域绑定用的只读闸。

   func (b *Bus) HopGate() func(context.Context, string, string, string, string) error {
	  return func(ctx context.Context, user, asset, from, to string) error {
		  return b.AdmitTransferHop(ctx, user, asset, from, to)
	   }
   }

// AdmitUnfreeze 在途单解冻核验：熔断总线永不解冻，受保护相位一律拒绝。
// 功能：给出金域一个显式断言点——跳闸、冷却、探针、解熔断都不会放开在途单。
// 重点：筛查中 / 待签 / 签名中 / 已广播 / 等确认或已有链上 hash 一律 ErrUnfreezeForbidden。
// 风险点：调用方不得凭本函数返回 nil 就去解冻，解冻仍须走出金域自身的守恒与人工闸。
   func (b *Bus) AdmitUnfreeze(ctx context.Context, ref InflightRef) error {
	  _ = ctx
	  if err := RefuseAutoUnfreeze(ref); err != nil {
		  return err
	  }
	  return nil
  }

// Probe 半开期签发探针。

  func (b *Bus) Probe(ctx context.Context, key string, path Path, actor string) (Probe, error) {
	  if b == nil {
		  return Probe{}, ErrBus
	  }
	  pol := b.pol.Current()
	   c, err := b.cir.Get(key)
	   if err != nil {
		  return Probe{}, err
	   }
	  p, err := b.probes.Issue(c, path, actor, pol.ProbeTTL)
	  if err != nil {
		   return p, err
	  }
	  if _, merr := b.cir.MarkProbe(key, pol); merr != nil {
		   // 配额用尽时回收刚签发的探针，避免半开期多放一笔试写。
		  _, _ = b.probes.Settle(p.ID, p.Token, false, actor, "budget exhausted")
		   return Probe{}, merr
	   }
	   b.persistProbe(ctx, p)
	  return p, nil
   }

// SettleProbe 回报探针结论：成功闭合熔断，失败重开并加长冷却。
  func (b *Bus) SettleProbe(ctx context.Context, id, token string, pass bool, actor, note string) (Probe, Circuit, error) {
	  if b == nil {
		  return Probe{},
			   Circuit{}, ErrBus
	   }
	  pol := b.pol.Current()
	  p, err := b.probes.Settle(id, token, pass, actor, note)
	   if err != nil {
		  return p, Circuit{}, err
	   }
	  b.persistProbe(ctx, p)
	  result := "failed"
	  if pass {
		  result = "passed"
	   }
	   metrics.FundCircuitProbes.WithLabelValues(result).Inc()
	   var c Circuit
	  if pass {
		  c, _, err = b.cir.ProbePassed(p.CircuitKey, actor, "probe passed: "+strings.TrimSpace(note))
	   } else {
		  c, _, err = b.cir.ProbeFailed(p.CircuitKey, actor, "probe failed: "+strings.TrimSpace(note), pol)
	   }
	  if err != nil {
		   return p, c, err
	  }
	  b.persistCircuit(ctx, c)
	   b.emitGauges()
	   return p, c, nil
   }

// Board 阻断读面：给定目标在三条入口上的结论、命中键与矩阵。
  func (b *Bus) Board(ctx context.Context, user, chain, asset string) BlockBoard {
	   _ = ctx
	  if b == nil {
		  return BlockBoard{}
	  }
	   pol := b.pol.Current()
	  now := b.now()
	   keys := KeysFor(AbuseSignal{UserID: user, Chain: chain, Asset: asset})
	   board := BlockBoard{
		  UserID: strings.TrimSpace(user), Chain: strings.ToUpper(strings.TrimSpace(chain)),
		   Asset:     strings.ToUpper(strings.TrimSpace(asset)),
		  PolicyVer: pol.Version,
		  Matrix:    pol.MatrixView(), Inflight: InflightStates(), AutoUnfreeze: UnfreezeAllowed(),
		   Keys: keys, At: now, Scopes: Scopes(),
	  }
	  for _, key := range keys {
		   c := b.cir.Snapshot(key)
		   board.Circuits = append(board.Circuits, c)
	   }
	   for _, path := range Paths() {
		  row := BoardRow{Path: path}
		  for _, key := range keys {
			  c := b.cir.Snapshot(key)
			  if !c.Blocking() {
				  continue
			  }
			  var probe *Probe
			   if p, ok := b.probes.InFlight(key); ok {
				   probe = &p
			  }
			  d := Decide(pol, c, path, probe, now)
			  board.Decisions = append(board.Decisions, d)
			   if d.Allow {
				  continue
			   }
			  row.Blocked = true
			   row.Reason = d.Reason
			   row.CircuitKey = c.Key
			   row.Scope = c.Scope
			   row.State = c.State
			   row.Severity = c.Severity
			   row.Score = c.Score
			  break
		  }
		  board.Rows = append(board.Rows, row)
	  }
	   board.SourceKeys = b.sourceNotes(user,
		  chain, asset)
	   return board
  }

// sourceNotes 原域键共存说明：列出被桥接的原键并声明未被改写。
   func (b *Bus) sourceNotes(user, chain, asset string) []SourceNote {
	   out := make([]SourceNote, 0, 3)
	  if key, err := usage.DepVelocityKey(user, chain, asset); err == nil {
		  out = append(out, SourceNote{Key: key, Kind: "usage_dep", Mutated: false})
	   }
	  u := strings.TrimSpace(user)
	  as := strings.ToUpper(strings.TrimSpace(asset))
	  if u != "" && as != "" {
		  out = append(out, SourceNote{Key: "wd:" + u + ":" + as, Kind: "wdguard_abuse", Mutated: false})
		  if src, err := HopSourceKey(user, asset); err == nil {
			   out = append(out, SourceNote{Key: src, Kind: "transfer_hop", Mutated: false})
		  }
	  }
	   return out
  }

// BusStats 总线读面。
type BusStats struct {
	PolicyVer int   `json:"policy_ver"`
	Admitted  int64 `json:"admitted"`
	Blocked int64 `json:"blocked"`
	Escalated int64       `json:"halt_escalations"`
	HaltErr   string      `json:"halt_err,omitempty"`
	RepoErr   string      `json:"repo_err,omitempty"`
	Signals   LedgerStats `json:"signals"`
	Circuits TableStats `json:"circuits"`
	Probes   ProbeStats `json:"probes"`
	Cases    CaseStats  `json:"cases"`
	Keyspace string     `json:"keyspace"`
	Unfreeze bool       `json:"auto_unfreeze"`
	Sources []SourceNote `json:"bridged_sources,omitempty"`
}

// Stats 总线统计。

   func (b *Bus) Stats() BusStats {
	   if b == nil {
		   return BusStats{}
	  }
	   b.mu.Lock()
	   admitted, blocked, esc := b.admitted, b.blocked, b.escalated
	   haltErr, repoErr := b.haltErr, b.repoErr
	  b.mu.Unlock()
	  return BusStats{
		  PolicyVer: b.pol.Current().Version, Admitted: admitted, Blocked: blocked,
		   Escalated: esc,
		   HaltErr:   haltErr, RepoErr: repoErr,
		   Signals: b.sig.Stats(), Circuits: b.cir.Stats(), Probes: b.probes.Stats(),
		   Cases:    b.cases.Stats(),
		   Keyspace: Prefix, Unfreeze: UnfreezeAllowed(),
		   Sources: []SourceNote{
			   {Key: usage.DepPrefix, Kind: "usage_dep", Mutated: false},
			  {Key: "wd:", Kind: "wdguard_abuse", Mutated: false},
			  {Key: HopKeyPrefix, Kind: "transfer_hop", Mutated: false},
		  },
	  }
  }

// emitGauges 刷新熔断与案件 gauge。

   func (b *Bus) emitGauges() {
	   if b == nil {
		   return
	   }
	  ts := b.cir.Stats()
	   metrics.FundCircuitCircuits.WithLabelValues(StateOpen.String()).Set(float64(ts.Open))
	   metrics.FundCircuitCircuits.WithLabelValues(StateHalfOpen.String()).Set(float64(ts.HalfOpen))
	  metrics.FundCircuitCircuits.WithLabelValues(StateClosed.String()).Set(float64(ts.Closed))
	  cs := b.cases.Stats()
	   metrics.FundCircuitCases.WithLabelValues(CaseOpen.String()).Set(float64(cs.Open))
	   metrics.FundCircuitCases.WithLabelValues(CaseReview.String()).Set(float64(cs.Review))
	  metrics.FundCircuitCases.WithLabelValues(CaseCleared.String()).Set(float64(cs.Cleared))
	   maxByScope := map[Scope]int{}
	   for _, c := range b.cir.List(1000) {
		  if c.Score > maxByScope[c.Scope] {
			  maxByScope[c.Scope] = c.Score
		  }
	  }
	  for _, sc := range Scopes() {
		   metrics.FundCircuitScore.WithLabelValues(sc.String()).Set(float64(maxByScope[sc]))
	  }
  }
