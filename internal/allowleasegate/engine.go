// engine.go
// 核验放行租约收口引擎 — 核心业务逻辑。
// Issue: GateWithdraw Allow 后签发租约
// SubmitCheck: 出金 Submit 前重新校验租约和案件
// Revoke: ExpireStale / Decide(deny) 撤销关联租约
// Consume: 广播成功后消费租约
// CreateDate: 2024.11.15

// Author: Dax
// Date: 2021.02.01

package allowleasegate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/trace"
)

// Engine 放行租约引擎。

type Engine struct {
	cfg   Config
	store LeaseStore
	cq CaseQuerier
	sm *leaseSM
	trail *AuditTrail
	mu    sync.RWMutex
	clock func() time.Time
	log   zerolog.Logger
}

type leaseSM struct {
	transitions map[LeaseState]map[string]LeaseState
}

   func newLeaseSM() *leaseSM {
	   sm := &leaseSM{transitions: make(map[LeaseState]map[string]LeaseState)}
	   edges := []struct {
		  from LeaseState
		   event string
		  to    LeaseState
	  }{
		   {StateAllowed, EventBind, StateBound},
		  {StateAllowed, EventExpire, StateExpired},
		   {StateAllowed,
			  EventRevoke, StateRevoked},
		   {StateBound, EventConsume, StateConsumed},
		   {StateBound, EventExpire, StateExpired},
		   {StateBound, EventRevoke, StateRevoked},
	  }
	   for _, e := range edges {
		  if sm.transitions[e.from] == nil {
			   sm.transitions[e.from] = make(map[string]LeaseState)
		   }
		   sm.transitions[e.from][e.event] = e.to
	  }
	   return sm
  }

   func (sm *leaseSM) apply(cur LeaseState, event string) (LeaseState, error) {
	   if sm == nil {
		  return cur, ErrLeaseIllegal
	   }
	  m, ok := sm.transitions[cur]
	   if !ok {
		  return cur, ErrLeaseIllegal
	  }
	  next, ok := m[event]
	  if !ok {
		  return cur, ErrLeaseIllegal
	  }
	   return next, nil
   }

   func New(cfg Config) *Engine {
	  cfg = cfg.withDefaults()
	  if cfg.Store == nil {
		   cfg.Store = NewMemStore()
	  }
	   return &Engine{
		   cfg: cfg,
		   store: cfg.Store,
		   cq:    cfg.CaseQuerier,
		   sm:    newLeaseSM(),
		   trail: NewAuditTrail(),
		  clock: func() time.Time { return time.Now().UTC() },
		   log:   zerolog.Nop(),
	  }
  }

  func (e *Engine) SetLogger(log zerolog.Logger) {
	   if e != nil {
		   e.log = log
	  }
   }

   func (e *Engine) SetClock(fn func() time.Time) {
	   if e != nil && fn != nil {
		  e.clock = fn
	  }
   }

   func (e *Engine) Store() LeaseStore { return e.store }

   func (e *Engine) Trail() *AuditTrail { return e.trail }

  func (e *Engine) now() time.Time {
	  if e.clock != nil {
		  return e.clock()
	  }
	  return time.Now().UTC()
  }

// Issue 签发放行租约。仅 decision=allow 可签发；review/deny/error 一律拒绝。
  func (e *Engine) Issue(ctx context.Context, req IssueReq) (*AllowLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrEngineNil
	   }
	  _, sp := trace.WithSpan(ctx, "allowleasegate.issue")
	   var err error
	   defer func() { sp.End(e.log, err) }()
	   decision := strings.ToLower(strings.TrimSpace(req.Decision))
	  if decision != "allow" {
		   err = fmt.Errorf("%w: decision=%s", ErrCaseNotAllowed, decision)
		  metrics.AllowLeaseDecisions.WithLabelValues("reject_issue").Inc()
		   return nil, err
	   }
	   if req.PolicyVer <= 0 {
		   err = fmt.Errorf("%w: policy_version=%d", ErrPolicyMismatch, req.PolicyVer)
		   return nil, err
	  }
	   ttl := req.TTL
	   if ttl <= 0 {
		  ttl = e.cfg.DefaultTTL
	  }
	   if ttl > e.cfg.MaxTTL {
		  ttl = e.cfg.MaxTTL
	   }
	   if ttl <= 0 {
		  err = ErrInvalidTTL
		  return nil, err
	  }
	  if e.cq != nil && req.CaseID != "" {
		   cs, cerr := e.cq.GetCase(req.CaseID)
		  if cerr == nil && (cs.IsDenied() || cs.IsExpired()) {
			   err = fmt.Errorf("%w: case %s state=%s", ErrCaseNotAllowed, req.CaseID, cs.State)
			   metrics.AllowLeaseDecisions.WithLabelValues("reject_case_state").Inc()
			  return nil, err
		  }
	  }
	  now := e.now()
	  lease := &AllowLease{
		  ID:         uuid.NewString(),
		   MovementID: req.MovementID,
		   UserID:     req.UserID,
		   CaseID: req.CaseID,
		   Asset:     req.Asset,
		   Chain:     req.Chain,
		  ToAddress: req.ToAddress,
		   Amount:    req.Amount,
		  Decision:  decision,
		  PolicyVer: req.PolicyVer,
		   State:     StateAllowed,
		  TTL: Duration(ttl),
		   IssuedAt:  now,
		   ExpiresAt: now.Add(ttl),
		  FenceEpoch: now.UnixMilli(),
		   FenceToken: uuid.NewString(),
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if serr := e.store.SaveLease(lease); serr != nil {
		  err = serr
		  return nil, err
	  }
	   bind := &PolicyBind{
		  LeaseID: lease.ID,
		   PolicyVer: req.PolicyVer,
		  Decision: decision,
		   CaseID: req.CaseID,
		   Hook: req.Hook,
		   BoundAt: now.Format(time.RFC3339),
	  }
	  if serr := e.store.SavePolicyBind(bind); serr != nil {
		   err = serr
		   return nil, err
	   }
	   e.trail.Append(AuditRecord{
		  LeaseID:    lease.ID,
		   MovementID: req.MovementID,
		   UserID: req.UserID,
		   Actor:  "system",
		   Action: "issue",
		   FromState: "",
		  ToState: string(StateAllowed),
		  Reason: fmt.Sprintf("allow; policy_ver=%d ttl=%s", req.PolicyVer, ttl),
		  PolicyVer: req.PolicyVer,
	  })
	  metrics.AllowLeaseDecisions.WithLabelValues("issued").Inc()
	  metrics.AllowLeaseActive.Inc()
	  cp := *lease
	  return &cp, nil
  }

// SubmitCheck 出金 Submit 前校验租约。无租约或不合法一律封堵并生成 AllowDriftProof。

  func (e *Engine) SubmitCheck(ctx context.Context, req SubmitReq) (*SubmitResult, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrEngineNil
	   }
	  _, sp := trace.WithSpan(ctx, "allowleasegate.submit_check")
	   var err error
	  defer func() { sp.End(e.log, err) }()
	   now := e.now()
	   if req.LeaseID == "" {
		   proof := e.sealProof(req.MovementID, req.UserID, "", "no_lease", "", "", 0,
			   "no allow_lease provided; broadcast blocked")
		  metrics.AllowLeaseDecisions.WithLabelValues("reject_no_lease").Inc()
		  err = ErrNoLease
		   return &SubmitResult{Allowed: false, Proof: proof, Reason: err.Error()}, err
	   }
	  lease, lerr := e.store.GetLease(req.LeaseID)
	  if lerr != nil {
		  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "lease_not_found", "", "", 0,
			   "lease not found")
		  metrics.AllowLeaseDecisions.WithLabelValues("reject_not_found").Inc()
		   err = ErrLeaseNotFound
		   return &SubmitResult{Allowed: false, Proof: proof, Reason: err.Error()}, err
	  }
	   if lease.MovementID != req.MovementID {
		  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "movement_mismatch",
			   string(lease.State), "", lease.PolicyVer,
			  fmt.Sprintf("lease.movement=%s req.movement=%s", lease.MovementID, req.MovementID))
		   metrics.AllowLeaseDecisions.WithLabelValues("reject_movement").Inc()
		   err = ErrMovementMismatch
		  return &SubmitResult{Allowed: false, Proof: proof, Reason: err.Error()}, err
	   }
	  if lease.State == StateExpired || lease.Expired(now) {
		  if lease.State != StateExpired {
			  e.doExpire(lease, now)
		  }
		  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "lease_expired",
			   string(StateExpired), "", lease.PolicyVer, "lease expired before submit")
		  metrics.AllowLeaseDecisions.WithLabelValues("reject_expired").Inc()
		  err = ErrLeaseExpired
		   return &SubmitResult{Allowed: false,
			   Lease: lease, Proof: proof, Reason: err.Error()}, err
	  }
	  if lease.State == StateRevoked {
		  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "lease_revoked",
			  string(StateRevoked), "", lease.PolicyVer,
			   fmt.Sprintf("lease revoked: %s", lease.RevokeReason))
		   metrics.AllowLeaseDecisions.WithLabelValues("reject_revoked").Inc()
		   err = ErrLeaseRevoked
		   return &SubmitResult{Allowed: false, Lease: lease, Proof: proof, Reason: err.Error()}, err
	  }
	   if lease.State == StateConsumed {
		  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "lease_consumed",
			   string(StateConsumed), "", lease.PolicyVer, "lease already consumed")
		   metrics.AllowLeaseDecisions.WithLabelValues("reject_consumed").Inc()
		   err = ErrLeaseConsumed
		   return &SubmitResult{Allowed: false, Lease: lease, Proof: proof, Reason: err.Error()}, err
	   }
	   if e.cq != nil && lease.CaseID != "" {
		  cs, cerr := e.cq.GetCase(lease.CaseID)
		   if cerr == nil {
			  if cs.IsDenied() {
				  e.doRevoke(lease, now, "case denied at submit time")
				   proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "case_denied",
					  string(StateRevoked), cs.State, lease.PolicyVer,
					   fmt.Sprintf("case %s denied by %s", lease.CaseID, cs.DecidedBy))
				  metrics.AllowLeaseDecisions.WithLabelValues("reject_case_denied").Inc()
				  err = ErrCaseDenied
				   return &SubmitResult{Allowed: false, Lease: lease, Proof: proof, Reason: err.Error()}, err
			  }
			  if cs.IsExpired() {
				  e.doRevoke(lease, now, "case expired at submit time")
				  proof := e.sealProof(req.MovementID, req.UserID, req.LeaseID, "case_expired",
					   string(StateRevoked), cs.State, lease.PolicyVer,
					  fmt.Sprintf("case %s expired", lease.CaseID))
				   metrics.AllowLeaseDecisions.WithLabelValues("reject_case_expired").Inc()
				  err = ErrCaseExpired
				  return &SubmitResult{Allowed: false, Lease: lease, Proof: proof, Reason: err.Error()}, err
			  }
		  }
	   }
	  if lease.State == StateAllowed {
		  e.doBind(lease,
			   now, req.MovementID)
	  }
	   metrics.AllowLeaseDecisions.WithLabelValues("submit_allowed").Inc()
	   cp := *lease
	   return &SubmitResult{Allowed: true, Lease: &cp, Reason: "lease valid; submit permitted"}, nil
  }

// Consume 广播成功后消费租约，标记终态。

  func (e *Engine) Consume(ctx context.Context, leaseID string) (*AllowLease, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrEngineNil
	  }
	  _, sp := trace.WithSpan(ctx, "allowleasegate.consume")
	   var err error
	   defer func() { sp.End(e.log, err) }()
	   lease, lerr := e.store.GetLease(leaseID)
	   if lerr != nil {
		  err = lerr
		  return nil, err
	   }
	   if !CanConsume(lease.State) {
		   err = fmt.Errorf("%w: state=%s", ErrLeaseIllegal, lease.State)
		   return lease, err
	  }
	  now := e.now()
	  prev := lease.State
	  lease.State = StateConsumed
	   lease.ConsumedAt = &now
	   lease.UpdatedAt = now
	  if serr := e.store.SaveLease(lease); serr != nil {
		  err = serr
		   return nil, err
	   }
	  e.trail.Append(AuditRecord{
		  LeaseID: lease.ID,
		   MovementID: lease.MovementID,
		  UserID:     lease.UserID,
		   Actor: "system",
		   Action: "consume",
		  FromState: string(prev),
		  ToState:   string(StateConsumed),
		   Reason:    "broadcast success",
		  PolicyVer: lease.PolicyVer,
	   })
	  metrics.AllowLeaseDecisions.WithLabelValues("consumed").Inc()
	  metrics.AllowLeaseActive.Dec()
	  cp := *lease
	   return &cp, nil
  }

// Revoke 撤销租约（ExpireStale / Decide deny 调用）。

  func (e *Engine) Revoke(ctx context.Context, req RevokeReq) (*AllowLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrEngineNil
	   }
	  _, sp := trace.WithSpan(ctx, "allowleasegate.revoke")
	  var err error
	   defer func() { sp.End(e.log, err) }()
	   lease, lerr := e.store.GetLease(req.LeaseID)
	  if lerr != nil {
		   err = lerr
		   return nil, err
	   }
	   if !CanRevoke(lease.State) {
		  if lease.State.Terminal() {
			   cp := *lease
			  return &cp, nil
		   }
		   err = fmt.Errorf("%w: state=%s", ErrLeaseIllegal, lease.State)
		   return lease, err
	   }
	  now := e.now()
	  e.doRevoke(lease, now, req.Reason)
	  actor := req.Actor
	   if actor == "" {
		   actor = "system"
	  }
	   e.trail.Append(AuditRecord{
		   LeaseID: lease.ID,
		  MovementID: lease.MovementID,
		  UserID:    lease.UserID,
		   Actor:     actor,
		  Action:    "revoke",
		   FromState: string(StateAllowed),
		  ToState:   string(StateRevoked),
		  Reason:    req.Reason,
		  PolicyVer: lease.PolicyVer,
	   })
	  metrics.AllowLeaseDecisions.WithLabelValues("revoked").Inc()
	   metrics.AllowLeaseActive.Dec()
	  cp := *lease
	  return &cp, nil
  }

// ExpireStale 过期所有超时的活跃租约，返回被过期的租约 ID。
   func (e *Engine) ExpireStale(ctx context.Context) ([]string, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrEngineNil
	   }
	   _, sp := trace.WithSpan(ctx, "allowleasegate.expire_stale")
	   var err error
	   defer func() {
		   sp.End(e.log,
			   err)
	   }()
	   active, aerr := e.store.ListActive()
	  if aerr != nil {
		  err = aerr
		   return nil, err
	  }
	  now := e.now()
	  var expired []string
	   for _, lease := range active {
		  if lease.Expired(now) {
			   e.doExpire(lease, now)
			  expired = append(expired, lease.ID)
		   }
	   }
	   if len(expired) > 0 {
		   metrics.AllowLeaseExpired.Add(float64(len(expired)))
	  }
	  return expired, nil
   }

// ExportProofs 导出封堵凭证。

  func (e *Engine) ExportProofs(_ context.Context, req ProofExportReq) ([]*AllowDriftProof, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrEngineNil
	   }
	  limit := req.Limit
	  if limit <= 0 {
		  limit = e.cfg.MaxProofsExport
	   }
	   if req.LeaseID != "" {
		  return e.store.ListProofsByLease(req.LeaseID, limit)
	  }
	  if req.UserID != "" {
		   return e.store.ListProofsByUser(req.UserID, limit)
	   }
	   return e.store.ListProofs(req.MovementID, limit)
  }

// PatrolCases 巡检案件状态，撤销 denied/expired 案件的活跃租约。
   func (e *Engine) PatrolCases(ctx context.Context) (*PatrolResult, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrEngineNil
	   }
	   _, sp := trace.WithSpan(ctx, "allowleasegate.patrol_cases")
	  var err error
	   defer func() { sp.End(e.log, err) }()
	   if e.cq == nil {
		  return &PatrolResult{}, nil
	   }
	   active, aerr := e.store.ListActive()
	  if aerr != nil {
		  err = aerr
		   return nil, err
	   }
	   now := e.now()
	  result := &PatrolResult{}
	   for _, lease := range active {
		   if lease.CaseID == "" {
			  continue
		   }
		   cs, cerr := e.cq.GetCase(lease.CaseID)
		   if cerr != nil {
			   continue
		   }
		   if cs.IsDenied() {
			  e.doRevoke(lease, now, fmt.Sprintf("patrol: case %s denied by %s", cs.ID, cs.DecidedBy))
			  result.Revoked = append(result.Revoked, lease.ID)
			  proof := e.sealProof(lease.MovementID, lease.UserID, lease.ID, "patrol_case_denied",
				  string(StateRevoked), cs.State, lease.PolicyVer,
				   fmt.Sprintf("patrol: case %s denied", cs.ID))
			   if proof != nil {
				   result.Proofs = append(result.Proofs, proof.ID)
			  }
		   } else if cs.IsExpired() {
			   e.doRevoke(lease, now, fmt.Sprintf("patrol: case %s expired", cs.ID))
			  result.Revoked = append(result.Revoked, lease.ID)
			   proof := e.sealProof(lease.MovementID, lease.UserID, lease.ID, "patrol_case_expired",
				   string(StateRevoked),
				  cs.State, lease.PolicyVer,
				  fmt.Sprintf("patrol: case %s expired", cs.ID))
			   if proof != nil {
				   result.Proofs = append(result.Proofs, proof.ID)
			   }
		  }
	  }
	   if len(result.Revoked) > 0 {
		  metrics.AllowLeasePatrolRevoked.Add(float64(len(result.Revoked)))
	   }
	  return result, nil
  }

// GetLease 获取租约。
  func (e *Engine) GetLease(_ context.Context, leaseID string) (*AllowLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrEngineNil
	  }
	  return e.store.GetLease(leaseID)
  }

// GetLeaseByMovement 按 movement 获取租约。

  func (e *Engine) GetLeaseByMovement(_ context.Context, movementID string) (*AllowLease, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrEngineNil
	  }
	   return e.store.GetLeaseByMovement(movementID)
   }

// ──────────────────────────────────────────
// 内部状态转移
// ──────────────────────────────────────────

   func (e *Engine) doBind(lease *AllowLease, now time.Time, movementID string) {
	   prev := lease.State
	  lease.State = StateBound
	   lease.BoundAt = &now
	  lease.BoundMovement = movementID
	  lease.UpdatedAt = now
	  _ = e.store.SaveLease(lease)
	  e.trail.Append(AuditRecord{
		   LeaseID: lease.ID,
		  MovementID: lease.MovementID,
		   UserID: lease.UserID,
		   Actor:     "system",
		  Action:    "bind",
		   FromState: string(prev),
		   ToState: string(StateBound),
		  Reason:  fmt.Sprintf("submit bind movement=%s", movementID),
		   PolicyVer: lease.PolicyVer,
	   })
   }

   func (e *Engine) doRevoke(lease *AllowLease, now time.Time, reason string) {
	  prev := lease.State
	   lease.State = StateRevoked
	   lease.RevokedAt = &now
	  lease.RevokeReason = reason
	   lease.UpdatedAt = now
	   _ = e.store.SaveLease(lease)
	  e.trail.Append(AuditRecord{
		  LeaseID:    lease.ID,
		   MovementID: lease.MovementID,
		   UserID:     lease.UserID,
		  Actor:      "system",
		  Action:    "revoke",
		  FromState: string(prev),
		  ToState:   string(StateRevoked),
		  Reason:    reason,
		  PolicyVer: lease.PolicyVer,
	  })
  }

   func (e *Engine) doExpire(lease *AllowLease, now time.Time) {
	   prev := lease.State
	   lease.State = StateExpired
	   lease.ExpiredAt = &now
	   lease.UpdatedAt = now
	   _ = e.store.SaveLease(lease)
	  e.trail.Append(AuditRecord{
		   LeaseID: lease.ID,
		   MovementID: lease.MovementID,
		   UserID: lease.UserID,
		   Actor:  "system",
		   Action: "expire",
		   FromState: string(prev),
		   ToState:   string(StateExpired),
		   Reason:    "ttl exceeded",
		  PolicyVer: lease.PolicyVer,
	  })
	  metrics.AllowLeaseActive.Dec()
  }

  func (e *Engine) sealProof(movementID, userID, leaseID, driftKind, leaseState, caseState string, policyVer int, reason string) *AllowDriftProof {
	   now := e.now()
	  proof := &AllowDriftProof{
		  ID: uuid.NewString(),
		  LeaseID:    leaseID,
		   MovementID: movementID,
		  UserID:     userID,
		   DriftKind: driftKind,
		  LeaseState: LeaseState(leaseState),
		  CaseState: caseState,
		   PolicyVer: policyVer,
		   Reason:    reason,
		   Blocked:   true,
		   Sealed:    true,
		  SealedAt: &now,
		  CreatedAt: now,
	   }
	   if lease, err := e.store.GetLease(leaseID); err == nil && lease != nil {
		  proof.Asset = lease.Asset
		   proof.Chain = lease.Chain
		   proof.ToAddress = lease.ToAddress
		   proof.Amount = lease.Amount
	   }
	   if serr := e.store.SaveProof(proof); serr != nil {
		   e.log.Error().Err(serr).Str("lease_id", leaseID).Msg("save drift proof failed")
		   return proof
	   }
	  e.trail.Append(AuditRecord{
		  LeaseID: leaseID,
		  MovementID: movementID,
		  UserID:     userID,
		  Actor:      "system",
		   Action:     "seal_proof",
		   FromState: leaseState,
		   ToState:   leaseState,
		   Reason:    fmt.Sprintf("drift_kind=%s: %s", driftKind, reason),
		  PolicyVer: policyVer,
	   })
	   metrics.AllowDriftProofSealed.Inc()
	   return proof
   }
