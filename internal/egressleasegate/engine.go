// egressleasegate.Engine：出金门控租约签发、广播绑定复验与 Halt 吊销。
// 职责：
// 1. CheckEgressWithLease：调用 floatdesk 瞬时门控，Allow（或 Warn 且策略允许）签发租约。
// 2. BroadcastAssert：广播/批次提交携带未过期租约，复验 Flags+Breach+Tank+世代。
// 3. RevokeOnHalt / RevokeOnLevelHalt：翻转或水位跌破后批量吊销。
// 4. ExpireLease：TTL 过期。
// 5. 证明固化 / 导出。
// 只读挂钩：Flags、Tanks、CheckEgress、RefillObserve.IsActive。
// 不改写 PlanRefill、不改写 hotbound cap、不写分值/清算/闪兑。

// Author: Rook
// Date: 2021.04.21

package egressleasegate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// Config 引擎配置。
type Config struct {
	LeaseTTL time.Duration
	// AllowWarnLease Warn 结论是否签发租约。建议按台评估；默认 false 更严。
	AllowWarnLease bool
	// RefuseNewLeaseWhenRefill 补仓进行中拒绝新 lease。
	RefuseNewLeaseWhenRefill bool
	// AllowBroadcastWithoutLease 必须 false。
	AllowBroadcastWithoutLease bool
	RevokeBatchSize            int
	ProofBatchSize int
	// leftover 策略名（映射到上方字段，不改变默认 fail-closed）。
	DenyWarn              bool
	AdmitDuringRefill     bool
	RejectTankDigestDrift bool
	MaxBatchItems         int
}

  func (c Config) withDefaults() Config {
	   if c.LeaseTTL <= 0 {
		  c.LeaseTTL = 90 * time.Second
	  }
	   if c.RevokeBatchSize <= 0 {
		  c.RevokeBatchSize = 200
	  }
	   if c.ProofBatchSize <= 0 {
		   c.ProofBatchSize = 200
	   }
	   if c.MaxBatchItems <= 0 {
		  c.MaxBatchItems = 32
	  }
	   if c.DenyWarn {
		  c.AllowWarnLease = false
	   }
	  if c.AdmitDuringRefill {
		   c.RefuseNewLeaseWhenRefill = false
	  }
	  return c
  }

// Counters leftover 计数。

type Counters struct {
	Issued atomic.Int64
	Consumed atomic.Int64
	Revoked atomic.Int64
	Expired  atomic.Int64
	Rejected atomic.Int64
	Proofs   atomic.Int64
}

// RefillActiveFunc 只读：补仓是否进行中。不得回写代次锁。

type RefillActiveFunc func(chain, asset string) bool

// Engine 出金门控租约引擎。
type Engine struct {
	store Store
	desk   *floatdesk.Service
	cfg    Config
	clock  func() time.Time
	refill RefillActiveFunc
	mu       sync.Mutex
	metrics  Counters
	lastSnap map[string]bool
}

// Metrics leftover 计数快照。

  func (e *Engine) Metrics() map[string]int64 {
	   if e == nil {
		  return map[string]int64{}
	  }
	   return map[string]int64{
		   "issued": e.metrics.Issued.Load(),
		  "consumed": e.metrics.Consumed.Load(),
		   "revoked":  e.metrics.Revoked.Load(),
		  "expired": e.metrics.Expired.Load(),
		   "rejected": e.metrics.Rejected.Load(),
		  "proofs":   e.metrics.Proofs.Load(),
	   }
  }

// NewEngine 初始化。desk 只读消费门控与 Flags/Tanks。
  func NewEngine(store Store, desk *floatdesk.Service, cfg Config) *Engine {
	   e := &Engine{
		   store: store,
		  desk: desk,
		  cfg: cfg.withDefaults(),
		   clock: time.Now,
	  }
	  if desk != nil && desk.RefillObserve() != nil {
		  obs := desk.RefillObserve()
		   e.refill = func(chain, asset string) bool {
			   if obs.IsActive(chain, asset) || obs.IsActive(chain, chain) {
				  return true
			  }
			  return obs.AnyActive() && chain == "" && asset == ""
		   }
	   }
	  return e
  }

// WithClock 时钟。
  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	   if e.desk != nil {
		   e.desk.SetClock(fn)
	   }
	   return e
  }

   // WithRefillHook 覆盖补仓只读挂钩。
   func (e *Engine) WithRefillHook(fn RefillActiveFunc) *Engine {
	   e.refill = fn
	  return e
  }

   func (e *Engine) now() time.Time {
	  if e.clock != nil {
		  return e.clock().UTC()
	  }
	  return time.Now().UTC()
  }

   func (e *Engine) audit(ctx context.Context, leaseID, from, to, actor, note string) {
	  _ = e.store.InsertAudit(ctx, &LeaseAudit{
		  ID: uuid.NewString(), LeaseID: leaseID, FromState: from, ToState: to,
		  Actor: actor, Note: note, CreatedAt: e.now(),
	   })
  }

   func (e *Engine) transit(l *EgressLease, to LeaseState) error {
	  if !CanTransit(l.State, to) {
		   return ErrInvalidTransit
	   }
	   l.State = to
	  l.UpdatedAt = e.now()
	   return nil
   }

// ─── CheckEgressWithLease ─────────────────────────────────────────────────
// IssueRequest 签发请求。

type IssueRequest struct {
	Chain string
	Asset string
	Native string
	Amount   string
	HotAvail string
	FromTank bool
	IdemKey  string
	Actor string
}

// IssueResult 签发结果。
type IssueResult struct {
	Lease *EgressLease
	Event        floatdesk.InstantEgressEvent
	Gate         floatdesk.InstantEgressEvent
	Allowed      bool
	Issued       bool
	RejectReason string
	Proof        *EgressDriftProof
}

// CheckEgressWithLease 瞬时门控通过后签发 egress_lease_id。
// 不变量：
// - Halt 不签发。
// - Warn 仅当 AllowWarnLease。
// - 补仓进行中且 RefuseNewLeaseWhenRefill 时拒绝。

   func (e *Engine) CheckEgressWithLease(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	   if e.desk == nil {
		   return nil, ErrDeskUnavailable
	   }
	  if strings.TrimSpace(req.Chain) == "" {
		   return nil, fmt.Errorf("egressleasegate: chain required")
	  }
	   if req.IdemKey != "" {
		   if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			  return &IssueResult{Lease: existing, Allowed: true}, nil
		   } else if !errors.Is(err, ErrNotFound) {
			  return nil, err
		   }
	   }
	  if e.cfg.RefuseNewLeaseWhenRefill && e.refill != nil && e.refill(req.Chain, req.Asset) {
		  e.metrics.Rejected.Add(1)
		  out := &IssueResult{Allowed: false, Issued: false, RejectReason: "refill_active"}
		   if req.FromTank {
			   if ev, e2 := e.desk.CheckEgressFromTanks(req.Chain, req.Asset); e2 == nil {
				  out.Event, out.Gate = ev, ev
			  }
		   } else if ev, e2 := e.desk.CheckEgress(req.Chain, req.Asset, req.HotAvail); e2 == nil {
			  out.Event, out.Gate = ev, ev
		  }
		   return out, ErrRefillActive
	  }
	   var ev floatdesk.InstantEgressEvent
	   var err error
	  if req.FromTank {
		   ev,
			   err = e.desk.CheckEgressFromTanks(req.Chain, req.Asset)
	  } else {
		   ev, err = e.desk.CheckEgress(req.Chain, req.Asset, req.HotAvail)
	  }
	   if err != nil {
		  return nil, err
	  }
	  switch ev.Decision {
	   case floatdesk.InstantHalt:
		  return &IssueResult{Event: ev, Allowed: false, RejectReason: ev.Reason}, ErrCheckDenied
	  case floatdesk.InstantWarn:
		   if !e.cfg.AllowWarnLease {
			  return &IssueResult{Event: ev, Allowed: false, RejectReason: "warn_not_allowed"}, ErrWarnNotAllowed
		  }
	   case floatdesk.InstantAllow:
	   default:
		   return &IssueResult{Event: ev, Allowed: false, RejectReason: "unknown_decision"}, ErrCheckDenied
	   }
	  now := e.now()
	   src := "catalog"
	   if req.FromTank {
		   src = "tank"
	   }
	   lease := &EgressLease{
		   ID: uuid.NewString(),
		   Chain: ev.Chain,
		   Asset:  ev.Asset,
		  Native: req.Native,
		  Amount:           strings.TrimSpace(req.Amount),
		   Decision:         string(ev.Decision),
		   DecisionAtIssue:  ev.Decision,
		  BreachLevel:      ev.TankLevel,
		  FlagsFP:          ev.FlagsFingerprint,
		  FlagsFingerprint: ev.FlagsFingerprint,
		   TankDigest:      ev.TankDigest,
		  Generation:      ev.HaltGeneration,
		   HaltGeneration:  ev.HaltGeneration,
		   }
		   return nil, fmt.Errorf("egressleasegate: insert lease: %w", err)
	  }
	  e.audit(ctx, lease.ID, "", string(LeaseStateIssued), "engine", "issue")
	   e.metrics.Issued.Add(1)
	  return &IssueResult{Lease: lease, Event: ev, Gate: ev, Allowed: true, Issued: true}, nil
   }

// ─── BroadcastAssert ──────────────────────────────────────────────────────
// BroadcastRequest 广播绑定复验。
type BroadcastRequest struct {
	LeaseID string
	Chain  string
	Asset  string
	Amount string
	BroadcastRef string
	BatchID string
	Actor    string
	FromTank bool
	HotAvail string
}

// AssertRequest leftover 广播请求别名。
type AssertRequest = BroadcastRequest

// BroadcastResult 复验结果。
type BroadcastResult struct {
	Lease *EgressLease
	Allowed  bool
	Accepted bool
	RejectReason string
	DriftProof *EgressDriftProof
	Proof *EgressDriftProof
}

// AssertResult leftover 结果别名。
type AssertResult = BroadcastResult

// BroadcastAssert 出金广播/批次提交闸。缺租约 fail-closed。

   func (e *Engine) BroadcastAssert(ctx context.Context, req BroadcastRequest) (*BroadcastResult, error) {
	  if strings.TrimSpace(req.LeaseID) == "" {
		   if !e.cfg.AllowBroadcastWithoutLease {
			  proof, _ := e.recordMissingLease(ctx, req)
			   e.metrics.Rejected.Add(1)
			   return &BroadcastResult{Allowed: false, Accepted: false, RejectReason: "missing_lease", DriftProof: proof, Proof: proof}, ErrNoLease
		  }
		   return &BroadcastResult{Allowed: false, RejectReason: "missing_lease"}, ErrNoLease
	   }
	  lease, err := e.store.GetLeaseByID(ctx, req.LeaseID)
	  if err != nil {
		  if errors.Is(err, ErrNotFound) {
			  proof, _ := e.recordMissingLease(ctx, req)
			  e.metrics.Rejected.Add(1)
			  return &BroadcastResult{Allowed: false, Accepted: false, RejectReason: "lease_not_found", DriftProof: proof, Proof: proof}, ErrNoLease
		  }
		   return nil, err
	   }
	   if req.Chain != "" && lease.Chain != strings.ToUpper(strings.TrimSpace(req.Chain)) {
		   return nil, ErrScopeMismatch
	   }
	  if req.Asset != "" && lease.Asset != strings.ToUpper(strings.TrimSpace(req.Asset)) {
		   return nil, ErrScopeMismatch
	   }
	   if req.Amount != "" && lease.Amount != "" && strings.TrimSpace(req.Amount) != lease.Amount {
		   return nil, ErrScopeMismatch
	  }
	  now := e.now()
	  switch lease.State {
	   case LeaseStateConsumed:
		   if req.BroadcastRef != "" && lease.BroadcastRef == req.BroadcastRef {
			  return &BroadcastResult{Lease: lease, Allowed: true, Accepted: true}, nil
		  }
		  return nil, ErrLeaseConsumed
	  case LeaseStateExpired:
		   return &BroadcastResult{Lease: lease, Allowed: false, Accepted: false, RejectReason: "expired"}, ErrLeaseExpired
	   case LeaseStateRevoked:
		  reason := lease.RevokeReason
		   if reason == "" {
			   reason = "revoked"
		  }
		  return &BroadcastResult{Lease: lease,
			   Allowed: false, Accepted: false, RejectReason: reason}, ErrLeaseRevoked
	   }
	   if lease.Expired(now) {
		  _ = e.ExpireLease(ctx, lease.ID)
		   if fresh, gerr := e.store.GetLeaseByID(ctx, lease.ID); gerr == nil {
			   lease = fresh
		  }
		   proof, _ := e.openProof(ctx, lease, DriftExpired, LiveView{})
		   return &BroadcastResult{Lease: lease, Allowed: false, Accepted: false, RejectReason: "expired", DriftProof: proof, Proof: proof}, ErrLeaseExpired
	   }
	  if _, perr := e.store.GetOpenProofByLease(ctx, lease.ID); perr == nil {
		   return nil, ErrProofBlocked
	   }
	   live, liveErr := e.liveView(lease, req.FromTank || lease.Source == "tank", req.HotAvail)
	   if liveErr != nil {
		   return nil, liveErr
	  }
	   if drift, dt, derr := e.detectDrift(lease, live); drift {
		  proof, _ := e.openProof(ctx, lease, dt, live)
		  _ = e.revokeLease(ctx, lease, reasonOf(dt), proof)
		  e.metrics.Rejected.Add(1)
		  return &BroadcastResult{
			  Lease: lease, Allowed: false, Accepted: false, RejectReason: string(dt), DriftProof: proof, Proof: proof,
		   }, derr
	  }
	   if lease.State == LeaseStateIssued {
		   if err := e.transit(lease, LeaseStateBound); err != nil {
			   return nil, err
		   }
		  t := now
		   lease.BoundAt = &t
		  lease.BroadcastRef = req.BroadcastRef
		   if err := e.store.UpdateLease(ctx, lease); err != nil {
			   return nil, err
		   }
		  e.audit(ctx, lease.ID, string(LeaseStateIssued), string(LeaseStateBound), "engine", "bind")
	  } else if lease.State == LeaseStateBound && req.BroadcastRef != "" && lease.BroadcastRef != "" && lease.BroadcastRef != req.BroadcastRef {
		  return nil, ErrBroadcastMismatch
	   }
	   if err := e.transit(lease, LeaseStateConsumed); err != nil {
		  return nil, err
	  }
	  t := now
	   lease.ConsumedAt = &t
	   if req.BroadcastRef != "" {
		  lease.BroadcastRef = req.BroadcastRef
	   }
	  if req.BatchID != "" {
		   lease.BatchID = req.BatchID
	   }
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return nil, err
	  }
	  e.audit(ctx, lease.ID, string(LeaseStateBound), string(LeaseStateConsumed), "engine", "consume")
	  e.metrics.Consumed.Add(1)
	  return &BroadcastResult{Lease: lease, Allowed: true, Accepted: true}, nil
  }

   func reasonOf(dt DriftType) RevokeReason {
	   switch dt {
	  case DriftHaltFlip, DriftGen:
		   return RevokeHaltFlip
	  case DriftLevelHalt:
		   return RevokeLevelHalt
	   case DriftFlags:
		  return RevokeFlagsDrift
	   case DriftTank:
		   return RevokeTankDrift
	   case DriftExpired:
		   return RevokeExpiry
	   default:
		  return RevokeManual
	  }
   }

  func (e *Engine) liveView(lease *EgressLease, fromTank bool, hotAvail string) (LiveView, error) {
	   if e.desk == nil {
		   return LiveView{}, ErrDeskUnavailable
	   }
	   var ev floatdesk.InstantEgressEvent
	   var err error
	   if fromTank {
		   ev,
			   err = e.desk.CheckEgressFromTanks(lease.Chain, lease.Asset)
	  } else {
		   avail := hotAvail
		   if avail == "" {
			   avail = lease.HotAvailAtIssue
		  }
		  ev, err = e.desk.CheckEgress(lease.Chain, lease.Asset, avail)
	  }
	   if err != nil {
		   return LiveView{}, err
	   }
	   return LiveView{
		  Decision: ev.Decision, BreachLevel: ev.TankLevel,
		   FlagsFingerprint: ev.FlagsFingerprint, TankDigest: ev.TankDigest,
		   HaltGeneration: ev.HaltGeneration,
		   HotAvail:       ev.HotAvail, Reason: ev.Reason,
	   }, nil
  }

   func (e *Engine) detectDrift(lease *EgressLease, live LiveView) (bool, DriftType, error) {
	   if live.HaltGeneration > lease.HaltGeneration &&
		   (live.Decision == floatdesk.InstantHalt || live.Reason == "halt_flag") {
		  return true, DriftGen, ErrGenerationDrift
	   }
	  if live.Decision == floatdesk.InstantHalt || live.BreachLevel == floatdesk.TankHalt {
		  if live.Reason == "halt_flag" {
			   return true, DriftHaltFlip, ErrHaltBlocked
		   }
		   return true, DriftLevelHalt, ErrLevelHalt
	  }
	  if live.FlagsFingerprint != lease.FlagsFingerprint &&
		   (live.Decision == floatdesk.InstantHalt || live.Reason == "halt_flag") {
		   return true, DriftFlags, ErrFlagsDrift
	  }
	  if live.TankDigest != lease.TankDigest && (e.cfg.RejectTankDigestDrift || live.BreachLevel != lease.BreachLevel) {
		   return true, DriftTank, ErrTankDrift
	   }
	  return false, "", nil
  }

   func (e *Engine) openProof(ctx context.Context, lease *EgressLease, dt DriftType, live LiveView) (*EgressDriftProof, error) {
	   idem := proofIdemKey(lease.ID, dt)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		   return existing, nil
	   }
	   now := e.now()
	  p := &EgressDriftProof{
		   ID:      uuid.NewString(),
		   LeaseID: lease.ID,
		   Chain:      lease.Chain,
		  Asset:      lease.Asset,
		  DriftType:  dt,
		  OldFlagsFP: lease.FlagsFingerprint,
		  NewFlagsFP:          live.FlagsFingerprint,
		   OldFlagsFingerprint: lease.FlagsFingerprint,
		   NewFlagsFingerprint: live.FlagsFingerprint,
		   OldBreach:           string(lease.BreachLevel),
		   NewBreach:     string(live.BreachLevel),
		  OldTankDigest: lease.TankDigest,
		  NewTankDigest: live.TankDigest,
		  OldGeneration: lease.HaltGeneration,
		  NewGeneration: live.HaltGeneration,
		  OldHaltGeneration: lease.HaltGeneration,
		   NewHaltGeneration: live.HaltGeneration,
		  State:             ProofStateOpen,
		  IdemKey:           idem,
		  OpenedAt: now,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  p.ProofHash = computeProofHash(p.LeaseID, p.DriftType, p.OldFlagsFingerprint, p.NewFlagsFingerprint, p.OldBreach, p.NewBreach, p.OldHaltGeneration, p.NewHaltGeneration)
	   if err := e.store.InsertProof(ctx, p); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			   return e.store.GetProofByIdem(ctx, idem)
		   }
		   return nil, err
	   }
	   return p, nil
  }

   func (e *Engine) recordMissingLease(ctx context.Context, req BroadcastRequest) (*EgressDriftProof, error) {
	   idem := missingLeaseIdem(req.Chain, req.Asset, req.BroadcastRef)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	  }
	   now := e.now()
	   p := &EgressDriftProof{
		  ID:    uuid.NewString(),
		   Chain: strings.ToUpper(strings.TrimSpace(req.Chain)),
		  Asset: strings.ToUpper(strings.TrimSpace(req.Asset)),
		   DriftType: DriftMissingLease,
		  State: ProofStateOpen,
		  IdemKey: idem,
		  OpenedAt:  now,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   p.ProofHash = computeProofHash("", DriftMissingLease, "", "", "", "", 0, 0) + p.IdemKey[:16]
	   if len(p.ProofHash) > 64 {
		   p.ProofHash = p.ProofHash[:64]
	  }
	   if err := e.store.InsertProof(ctx, p); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx, idem)
		  }
		   return nil, err
	  }
	  return p, nil
  }

   func (e *Engine) revokeLease(ctx context.Context, lease *EgressLease, reason RevokeReason, proof *EgressDriftProof) error {
	   from := lease.State
	   if lease.State.Closed() {
		  return nil
	   }
	   if err := e.transit(lease, LeaseStateRevoked); err != nil {
		  return err
	  }
	  t := e.now()
	   lease.RevokedAt = &t
	   lease.RevokeReason = string(reason)
	   if proof != nil {
		   lease.DriftProofID = proof.ID
	   }
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return err
	  }
	   e.audit(ctx, lease.ID, string(from), string(LeaseStateRevoked), "engine", string(reason))
	  e.metrics.Revoked.Add(1)
	   return nil
   }

// ExpireLease TTL 过期。
   func (e *Engine) ExpireLease(ctx context.Context, leaseID string) error {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	   if err != nil {
		   return err
	  }
	  if lease.State.Closed() {
		  return nil
	   }
	   from := lease.State
	  if err := e.transit(lease, LeaseStateExpired); err != nil {
		  return err
	   }
	   lease.RevokeReason = string(RevokeExpiry)
	  if err := e.store.UpdateLease(ctx, lease); err != nil {
		   return err
	   }
	   e.audit(ctx, lease.ID, string(from), string(LeaseStateExpired), "expiry-worker", "ttl")
	   e.metrics.Expired.Add(1)
	   return nil
  }

// RevokeOnHalt 按范围吊销活跃租约并落世代。

   func (e *Engine) RevokeOnHalt(ctx context.Context, scope, chain, asset, reason string) (int, error) {
	   if e.desk == nil {
		  return 0, ErrDeskUnavailable
	  }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   gen := &EgressGeneration{
		   ID: uuid.NewString(), Scope: scope, Chain: chain, Asset: asset,
		  Generation: e.desk.Halt().Generation(), FlagsFingerprint: e.desk.Halt().Fingerprint(),
		   Reason: reason, CreatedAt: e.now(),
	  }
	   _ = e.store.InsertGeneration(ctx, gen)
	  var rows []*EgressLease
	  var err error
	   switch scope {
	  case "all":
		   rows, err = e.store.ListActiveLeases(ctx, e.cfg.RevokeBatchSize)
	   default:
		   rows, err = e.store.ListActiveByScope(ctx, chain, asset, e.cfg.RevokeBatchSize)
	   }
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	   for _, l := range rows {
		  if scope == "chain" && l.Chain != chain {
			   continue
		   }
		   if scope == "asset" && l.Asset != asset {
			  continue
		  }
		   live := LiveView{
			   Decision: floatdesk.InstantHalt,
			  BreachLevel:      floatdesk.TankHalt,
			   FlagsFingerprint: gen.FlagsFingerprint, HaltGeneration: gen.Generation,
			  Reason: "halt_flag",
		  }
		   proof, _ := e.openProof(ctx, l, DriftHaltFlip, live)
		  if e.revokeLease(ctx, l, RevokeHaltFlip, proof) == nil {
			   n++
		  }
	   }
	  return n, nil
   }

// ScanLevelHalt 水位跌破巡检：复验活跃租约，跌入 Halt 则吊销。

   func (e *Engine) ScanLevelHalt(ctx context.Context) (int, error) {
	   rows, err := e.store.ListActiveLeases(ctx, e.cfg.RevokeBatchSize)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	   for _, l := range rows {
		   live, lerr := e.liveView(l, l.Source == "tank", "")
		  if lerr != nil {
			  continue
		  }
		   if live.BreachLevel != floatdesk.TankHalt && live.Decision != floatdesk.InstantHalt {
			  continue
		   }
		  proof, _ := e.openProof(ctx, l, DriftLevelHalt, live)
		   if e.revokeLease(ctx, l, RevokeLevelHalt, proof) == nil {
			   n++
		   }
	  }
	  return n, nil
  }

// ExpireDue 扫描过期。
   func (e *Engine) ExpireDue(ctx context.Context) (int, error) {
	  rows, err := e.store.ListExpiredLeases(ctx, e.now(), e.cfg.RevokeBatchSize)
	  if err != nil {
		  return 0, err
	   }
	   n := 0
	   for _, l := range rows {
		  if e.ExpireLease(ctx, l.ID) == nil {
			  n++
		  }
	  }
	  return n, nil
  }

// ProveProof open→proven。
   func (e *Engine) ProveProof(ctx context.Context, proofID string) error {
	  p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return err
	  }
	   if p.State != ProofStateOpen {
		   return nil
	   }
	   t := e.now()
	  p.State = ProofStateProven
	  p.ProvenAt = &t
	  p.UpdatedAt = t
	   return e.store.UpdateProof(ctx, p)
  }

// ProveOpen 批量固化。
   func (e *Engine) ProveOpen(ctx context.Context) (int, error) {
	   rows, err := e.store.ListProofsByState(ctx, ProofStateOpen, e.cfg.ProofBatchSize)
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	  for _, p := range rows {
		  if e.ProveProof(ctx, p.ID) == nil {
			   n++
		  }
	   }
	   return n, nil
   }

// ConfirmProof / DismissProof 关闭证明。
  func (e *Engine) ConfirmProof(ctx context.Context, proofID, closedBy, notes string) error {
	   return e.closeProof(ctx, proofID, closedBy, notes, true)
   }

// DismissProof 驳回证明。

   func (e *Engine) DismissProof(ctx context.Context, proofID, closedBy, notes string) error {
	  return e.closeProof(ctx, proofID, closedBy, notes, false)
   }

  func (e *Engine) closeProof(ctx context.Context, proofID, closedBy, notes string, requireProven bool) error {
	   p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		  return err
	  }
	  if p.State.Closed() {
		  return nil
	  }
	  if requireProven && p.State != ProofStateProven {
		   return fmt.Errorf("egressleasegate: proof not proven")
	   }
	  t := e.now()
	   p.State = ProofStateClosed
	  p.ClosedAt = &t
	  p.ClosedBy = closedBy
	   p.Notes = notes
	  p.UpdatedAt = t
	   return e.store.UpdateProof(ctx, p)
  }

// ExportProof 导出证明。
  func (e *Engine) ExportProof(ctx context.Context, proofID, actor string) (*ProofExport, error) {
	  p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		  return nil, err
	   }
	  out := &ProofExport{Proof: *p, ExportedAt: e.now(), ExportedBy: actor}
	   if p.LeaseID != "" {
		  if l, lerr := e.store.GetLeaseByID(ctx, p.LeaseID); lerr == nil {
			  out.Lease = l
		  }
	  }
	  return out, nil
  }

// GetLease 查询。
  func (e *Engine) GetLease(ctx context.Context, id string) (*EgressLease, error) {
	   return e.store.GetLeaseByID(ctx, id)
  }

// GetProof 查询证明。

  func (e *Engine) GetProof(ctx context.Context, id string) (*EgressDriftProof, error) {
	   return e.store.GetProofByID(ctx, id)
  }

// ListActive 列举活跃租约。
   func (e *Engine) ListActive(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	  if chain != "" || asset != "" {
		  return e.store.ListActiveByScope(ctx, strings.ToUpper(chain), strings.ToUpper(asset), limit)
	   }
	   return e.store.ListActiveLeases(ctx, limit)
  }

// ManualRevoke 运维吊销。
  func (e *Engine) ManualRevoke(ctx context.Context, leaseID, reason string) (*EgressLease, error) {
	  lease, err := e.store.GetLeaseByID(ctx, leaseID)
	   if err != nil {
		  return nil, err
	  }
	   if reason == "" {
		  reason = string(RevokeManual)
	   }
	  if err := e.revokeLease(ctx, lease, RevokeReason(reason), nil); err != nil {
		   return nil, err
	  }
	   return e.store.GetLeaseByID(ctx, leaseID)
  }
