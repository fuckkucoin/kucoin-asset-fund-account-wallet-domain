package agreeleasegate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

// Config 引擎配置。
type Config struct {
	LeaseTTL time.Duration
	// AllowPlanWithoutLease 必须 false。
	AllowPlanWithoutLease bool
	// AllowExecWithoutLease 必须 false。
	AllowExecWithoutLease bool
	// ConsumeOnExec 首次 Exec 复验通过后直接消费；false 则停留 executing。
	ConsumeOnExec   bool
	RevokeBatchSize int
	ProofBatchSize int
	StalePlanAfter time.Duration
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
	  if c.StalePlanAfter <= 0 {
		   c.StalePlanAfter = 2 * time.Minute
	  }
	   return c
  }

// Engine 同意租约引擎。

type Engine struct {
	store Store
	gate  *sweeprun.AgreeBook
	cfg   Config
	clock func() time.Time
}

// NewEngine 初始化。gate 只读消费 AgreeGate。

  func NewEngine(store Store, gate *sweeprun.AgreeBook, cfg Config) *Engine {
	   return &Engine{
		   store: store,
		  gate:  gate,
		  cfg:   cfg.withDefaults(),
		   clock: time.Now,
	   }
   }

// WithClock 时钟。
  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
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
		  Actor: actor,
		  Note:  note, CreatedAt: e.now(),
	  })
  }

  func (e *Engine) transit(l *AgreeLease, to LeaseState) error {
	  if !CanTransit(l.State, to) {
		  return ErrInvalidTransit
	   }
	  l.State = to
	  l.UpdatedAt = e.now()
	   return nil
   }

// ─── AgreeWithLease ───────────────────────────────────────────────────────
// IssueRequest 签发请求。
type IssueRequest struct {
	Chain string
	Asset string
	IdemKey string
}

// IssueResult 签发结果。

type IssueResult struct {
	Lease        *AgreeLease
	Event        sweeprun.AgreeEvent
	Allowed      bool
	RejectReason string
}

// AgreeWithLease 瞬时读闸通过后签发 agree_lease_id。
// 不变量：
// - 同意码非空不签发。
// - 闸不可用 fail-closed。
  func (e *Engine) AgreeWithLease(ctx context.Context, req IssueRequest) (*IssueResult, error) {
	   if e.gate == nil {
		  return nil, ErrGateUnavailable
	  }
	  if strings.TrimSpace(req.Asset) == "" {
		   return nil, fmt.Errorf("agreeleasegate: asset required")
	   }
	  if req.IdemKey != "" {
		  if existing, err := e.store.GetLeaseByIdem(ctx, req.IdemKey); err == nil {
			  return &IssueResult{Lease: existing, Allowed: existing.State.Active()}, nil
		  } else if !errors.Is(err, ErrNotFound) {
			  return nil, err
		   }
	  }
	  ev,
		  err := e.gate.CheckAgree(req.Chain, req.Asset)
	   if err != nil {
		  return nil, err
	   }
	   if ev.Code.Failed() {
		  return &IssueResult{Event: ev, Allowed: false, RejectReason: ev.Code.String()}, ErrAgreeDenied
	   }
	   now := e.now()
	  lease := &AgreeLease{
		  ID: uuid.NewString(),
		   Chain: ev.Chain,
		  Asset: ev.Asset,
		   AgreeCode: ev.Code.String(),
		  BalFingerprint: ev.BalFingerprint,
		  RuleFingerprint: ev.RuleFingerprint,
		   Kinds:       joinKinds(ev.Kinds),
		   RuleVersion: ev.RuleVersion,
		  ShadowBalAtIssue: ev.ShadowBal,
		  WaterlineAtIssue: ev.Waterline,
		   MinSweepAtIssue:  ev.MinSweep,
		  State:            LeaseStateAgreed,
		  IdemKey:          req.IdemKey,
		  IssuedAt:  now,
		   ExpiresAt: now.Add(e.cfg.LeaseTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := e.store.InsertLease(ctx, lease); err != nil {
		   if errors.Is(err, ErrDuplicate) && req.IdemKey != "" {
			   existing, getErr := e.store.GetLeaseByIdem(ctx, req.IdemKey)
			  if getErr == nil {
				  return &IssueResult{Lease: existing, Event: ev, Allowed: true}, nil
			   }
		   }
		  return nil, fmt.Errorf("agreeleasegate: insert lease: %w", err)
	   }
	   e.audit(ctx, lease.ID, "", string(LeaseStateAgreed), "engine", "issue")
	  incLeaseIssued()
	   return &IssueResult{Lease: lease,
		   Event: ev, Allowed: true}, nil
  }

// ─── 活快照与漂移探测 ─────────────────────────────────────────────────────

  func (e *Engine) liveView(chain, asset string) (LiveView, error) {
	  if e.gate == nil {
		  return LiveView{}, ErrGateUnavailable
	   }
	  ev,
		  err := e.gate.CheckAgree(chain, asset)
	   if err != nil {
		  return LiveView{}, err
	  }
	  return LiveView{
		  AgreeCode: ev.Code.String(), BalFingerprint: ev.BalFingerprint,
		  RuleFingerprint: ev.RuleFingerprint, RuleVersion: ev.RuleVersion,
		   ShadowBal: ev.ShadowBal, Waterline: ev.Waterline,
		   Kinds: joinKinds(ev.Kinds), Reason: ev.Reason,
	   }, nil
  }

  func (e *Engine) detectDrift(lease *AgreeLease, live LiveView) (bool, DriftType, error) {
	   if live.RuleVersion > lease.RuleVersion {
		  return true, DriftVersion, ErrVersionDrift
	  }
	   if live.RuleFingerprint != "" && live.RuleFingerprint != lease.RuleFingerprint {
		   return true, DriftRule, ErrRuleDrift
	   }
	   if live.AgreeCode != "" {
		   if live.Reason == "below_waterline" || live.AgreeCode == string(sweeprun.AgreeDeniedBelow) {
			   return true, DriftWaterline, ErrWaterlineDrift
		  }
		   if live.AgreeCode == string(sweeprun.AgreeDeniedHalt) {
			   return true, DriftAgreeFailed, ErrAgreeDenied
		   }
		  return true, DriftAgreeFailed, ErrAgreeDenied
	   }
	   if live.BalFingerprint != "" && live.BalFingerprint != lease.BalFingerprint {
		  return true, DriftBal, ErrBalDrift
	  }
	  if live.Kinds != "" && live.Kinds != lease.Kinds {
		  return true, DriftKinds, ErrKindsDrift
	  }
	  if live.Waterline != "" && live.Waterline != lease.WaterlineAtIssue {
		   return true, DriftWaterline, ErrWaterlineDrift
	   }
	   return false, "", nil
   }

   func reasonOf(dt DriftType) RevokeReason {
	  switch dt {
	   case DriftBal:
		   return RevokeBalDrift
	   case DriftRule, DriftVersion:
		   return RevokeRuleFlip
	  case DriftWaterline:
		  return RevokeWaterline
	  case DriftExpired:
		  return RevokeExpiry
	   case DriftHalfPlan:
		   return RevokeHalfPlan
	  case DriftKinds:
		   return RevokeKindsDrift
	   case DriftAgreeFailed:
		   return RevokeAgreeFailed
	  default:
		   return RevokeManual
	  }
  }

  func (e *Engine) openProof(ctx context.Context, lease *AgreeLease, dt DriftType, live LiveView) (*AgreeDriftProof, error) {
	  idem := proofIdemKey(lease.ID, dt)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	  }
	  now := e.now()
	   p := &AgreeDriftProof{
		   ID:      uuid.NewString(),
		   LeaseID: lease.ID,
		  Chain:             lease.Chain,
		   Asset:             lease.Asset,
		   DriftType:         dt,
		   OldBalFingerprint: lease.BalFingerprint,
		  NewBalFingerprint: live.BalFingerprint,
		   OldRuleFingerprint: lease.RuleFingerprint,
		   NewRuleFingerprint: live.RuleFingerprint,
		  OldRuleVersion:     lease.RuleVersion,
		   NewRuleVersion: live.RuleVersion,
		  OldShadowBal: lease.ShadowBalAtIssue,
		  NewShadowBal: live.ShadowBal,
		   OldWaterline: lease.WaterlineAtIssue,
		  NewWaterline: live.Waterline,
		  State:        ProofStateOpen,
		  IdemKey:  idem,
		   OpenedAt: now,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  p.ProofHash = computeProofHash(p.LeaseID, p.DriftType, p.OldBalFingerprint, p.NewBalFingerprint, p.OldRuleFingerprint, p.NewRuleFingerprint, p.OldRuleVersion, p.NewRuleVersion)
	  if err := e.store.InsertProof(ctx, p); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx, idem)
		  }
		  return nil, err
	   }
	   incProofOpened()
	   return p, nil
  }

   func (e *Engine) recordMissingLease(ctx context.Context, chain, asset, planRef, execRef string) (*AgreeDriftProof, error) {
	   idem := missingLeaseIdem(chain,
		   asset, planRef, execRef)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	  }
	   now := e.now()
	   p := &AgreeDriftProof{
		   ID: uuid.NewString(),
		  Chain:     upperTrim(chain),
		   Asset:     upperTrim(asset),
		  DriftType: DriftMissingLease,
		   State:     ProofStateOpen,
		  IdemKey:   idem,
		  OpenedAt:  now,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   h := computeProofHash("", DriftMissingLease, "", "", "", "", 0, 0) + p.IdemKey[:16]
	   if len(h) > 64 {
		   h = h[:64]
	   }
	  p.ProofHash = h
	   if err := e.store.InsertProof(ctx, p); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx, idem)
		  }
		  return nil, err
	  }
	  incProofOpened()
	  return p, nil
  }

   func (e *Engine) revokeLease(ctx context.Context, lease *AgreeLease, reason RevokeReason, proof *AgreeDriftProof) error {
	  from := lease.State
	   if lease.State.Closed() {
		  return nil
	   }
	   if err := e.transit(lease,
		  LeaseStateRevoked); err != nil {
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
	  incLeaseRevoked()
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
	   incLeaseExpired()
	  return nil
  }

// loadAndGuard 取租约并做公共闸：缺租约、终态、TTL、open proof。

   func (e *Engine) loadAndGuard(ctx context.Context, leaseID, chain, asset, planRef, execRef string, allowMissing bool) (*AgreeLease, *AgreeDriftProof, error) {
	   if strings.TrimSpace(leaseID) == "" {
		  if allowMissing {
			   return nil, nil, ErrNoLease
		  }
		  proof,
			   _ := e.recordMissingLease(ctx, chain, asset, planRef, execRef)
		   return nil, proof, ErrNoLease
	   }
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		   if errors.Is(err, ErrNotFound) {
			  proof, _ := e.recordMissingLease(ctx, chain, asset, planRef, execRef)
			   return nil, proof, ErrNoLease
		  }
		  return nil, nil, err
	   }
	   if chain != "" && lease.Chain != "" && lease.Chain != upperTrim(chain) {
		   return lease, nil, ErrScopeMismatch
	  }
	  if asset != "" && lease.Asset != upperTrim(asset) {
		  return lease, nil, ErrScopeMismatch
	  }
	   switch lease.State {
	  case LeaseStateConsumed:
		   return lease, nil, ErrLeaseConsumed
	   case LeaseStateExpired:
		   return lease, nil, ErrLeaseExpired
	   case LeaseStateRevoked:
		   return lease, nil, ErrLeaseRevoked
	   }
	  if lease.Expired(e.now()) {
		  _ = e.ExpireLease(ctx, lease.ID)
		  proof,
			  _ := e.openProof(ctx, lease, DriftExpired, LiveView{})
		  return lease, proof, ErrLeaseExpired
	  }
	  if open, perr := e.store.GetOpenProofByLease(ctx, lease.ID); perr == nil {
		   return lease, open, ErrProofBlocked
	   }
	  if _, herr := e.store.GetHalfPlanByLease(ctx, lease.ID); herr == nil {
		   return lease, nil, ErrHalfPlanBlocked
	   }
	   return lease, nil, nil
  }

  func (e *Engine) reverifyOrRevoke(ctx context.Context, lease *AgreeLease) (*AgreeDriftProof, error) {
	   live, err := e.liveView(lease.Chain, lease.Asset)
	  if err != nil {
		   return nil, err
	  }
	  if drift, dt, derr := e.detectDrift(lease, live); drift {
		   proof, _ := e.openProof(ctx, lease, dt, live)
		  _ = e.revokeLease(ctx,
			  lease, reasonOf(dt), proof)
		  return proof, derr
	  }
	  return nil, nil
   }
