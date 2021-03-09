// 审查案件围栏引擎 — 核心业务逻辑。
// DecideFencedOnly: 强制围栏结案，两个 reviewer 同时 Approve 仅一人成功
// SubmitCheck: 出金 Submit 前校验 ticket + policyVer
// BareDecideBlock: 裸 Decide 调用拦截/告警
// ExpireStaleTickets: 过期扫描 + 围栏 bump

// Author: Talon
// Date: 2021.03.08

package casefencegate

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

// Engine 围栏结案引擎。
type Engine struct {
	cfg Config
	store TicketStore
	cq CaseQuerier
	trail *AuditTrail
	mu    sync.RWMutex
	clock func() time.Time
	log zerolog.Logger
}

// New 创建围栏结案引擎。
  func New(cfg Config) *Engine {
	  cfg = cfg.withDefaults()
	  e := &Engine{
		  cfg:   cfg,
		   store: cfg.Store,
		  cq:    cfg.CaseQuerier,
		   trail: NewAuditTrail(),
		   clock: func() time.Time { return time.Now().UTC() },
		   log:   zerolog.Nop(),
	   }
	  return e
   }

// SetClock 注入时钟。
  func (e *Engine) SetClock(fn func() time.Time) {
	  if e != nil && fn != nil {
		   e.mu.Lock()
		  defer e.mu.Unlock()
		  e.clock = fn
	   }
   }

   // SetLogger 注入日志。

   func (e *Engine) SetLogger(log zerolog.Logger) {
	   if e != nil {
		  e.mu.Lock()
		   defer e.mu.Unlock()
		   e.log = log
	  }
   }

  func (e *Engine) now() time.Time {
	  e.mu.RLock()
	  defer e.mu.RUnlock()
	   return e.clock()
  }

// ──────────────────────────────────────────
// DecideFencedOnly 强制围栏结案
// ──────────────────────────────────────────

  func (e *Engine) DecideFencedOnly(_ context.Context, req DecideFencedReq) (*DecideFencedResult, error) {
	  if e == nil {
		  return nil, ErrEngineNil
	  }
	   actor := strings.TrimSpace(req.Actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return nil, ErrMakerCheck
	  }
	   decision := strings.ToLower(strings.TrimSpace(req.Decision))
	   if decision != "approve" && decision != "allow" && decision != "deny" {
		  return nil, ErrInvalidDecision
	   }
	   cs, err := e.cq.GetCase(req.CaseID)
	   if err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrCaseNotFound, err)
	   }
	  if strings.EqualFold(actor, cs.UserID) {
		   return nil, ErrActorSelf
	   }
	  if cs.FenceEpoch != req.FenceEpoch || cs.FenceToken != req.FenceToken {
		  proof := e.sealDualDecideProof(cs, actor, decision, req.FenceEpoch, cs.FenceEpoch)
		   return &DecideFencedResult{Proof: proof}, ErrStaleFence
	  }
	   if !cs.IsOpen() {
		  return nil, ErrCaseNotOpen
	  }
	  caseDecision := decision
	  if caseDecision == "allow" {
		   caseDecision = "approve"
	   }
	   updated, err := e.cq.DecideFenced(req.CaseID, actor, caseDecision, req.FenceEpoch, req.FenceToken)
	   if err != nil {
		   if strings.Contains(err.Error(), "stale") || strings.Contains(err.Error(), "fence") {
			  proof := e.sealDualDecideProof(cs, actor, decision, req.FenceEpoch, cs.FenceEpoch)
			  return &DecideFencedResult{Proof: proof}, ErrStaleFence
		  }
		   return nil, err
	  }
	  now := e.now()
	  var ticket *CaseReleaseTicket
	  if caseDecision == "approve" {
		  ttl := e.cfg.DefaultTTL
		   ticket = &CaseReleaseTicket{
			  ID:     uuid.NewString(),
			   CaseID: cs.ID,
			   MovementID: cs.MovementID,
			  UserID:   cs.UserID,
			  Decision: "approve",
			  DecidedBy: actor,
			  PolicyVer:  cs.PolicyVer,
			   FenceEpoch: updated.FenceEpoch,
			  FenceToken: updated.FenceToken,
			  State:    TicketApproved,
			   TTL:      Duration(ttl),
			  IssuedAt: now,
			   ExpiresAt: now.Add(ttl),
			  CreatedAt: now,
			   UpdatedAt: now,
		   }
		  if err := e.store.SaveTicket(ticket); err != nil {
			  return nil, err
		  }
		  lease := &CaseFenceLease{
			  ID:         uuid.NewString(),
			   CaseID:     cs.ID,
			   MovementID: cs.MovementID,
			  UserID: cs.UserID,
			   FenceEpoch: updated.FenceEpoch,
			   FenceToken: updated.FenceToken,
			   PolicyVer:  cs.PolicyVer,
			   HeldBy:     actor,
			  HeldAt: now,
			  CreatedAt: now,
			   UpdatedAt: now,
		  }
		  if err := e.store.SaveFenceLease(lease); err != nil {
			   e.log.Warn().Err(err).Str("case_id", cs.ID).Msg("save fence lease failed")
		   }
	  } else {
		  ticket = &CaseReleaseTicket{
			  ID: uuid.NewString(),
			  CaseID:     cs.ID,
			  MovementID: cs.MovementID,
			   UserID:     cs.UserID,
			   Decision:   "deny",
			   DecidedBy: actor,
			   PolicyVer: cs.PolicyVer,
			   FenceEpoch: updated.FenceEpoch,
			  FenceToken: updated.FenceToken,
			  State: TicketDenied,
			   IssuedAt:  now,
			   ExpiresAt: now,
			   CreatedAt: now,
			  UpdatedAt: now,
		   }
		  if err := e.store.SaveTicket(ticket); err != nil {
			   return nil, err
		   }
	  }
	   e.trail.Append(AuditRecord{
		  CaseID: cs.ID, TicketID: ticket.ID, MovementID: cs.MovementID,
		  UserID: cs.UserID, Actor: actor, Action: "decide_fenced_" + caseDecision,
		  ToState: string(ticket.State), PolicyVer: cs.PolicyVer,
		  FenceEpoch: updated.FenceEpoch,
	   })
	  metrics.CaseFenceDecideTotal.WithLabelValues(caseDecision, "fenced").Inc()
	  return &DecideFencedResult{Ticket: ticket, Case: &updated}, nil
  }

   func (e *Engine) sealDualDecideProof(cs CaseSnapshot, loserActor, loserDecision string, loserEpoch, winnerEpoch int64) *CaseDualDecideProof {
	   now := e.now()
	   proof := &CaseDualDecideProof{
		   ID:         uuid.NewString(),
		   CaseID:     cs.ID,
		  MovementID: cs.MovementID,
		   WinnerActor: cs.DecidedBy,
		   WinnerDecision: cs.State,
		  LoserActor:    loserActor,
		  LoserDecision: loserDecision,
		  FenceEpoch:    cs.FenceEpoch,
		   WinnerEpoch: winnerEpoch,
		   LoserEpoch:  loserEpoch,
		  Sealed:      true,
		   SealedAt: now,
		  CreatedAt: now,
	   }
	   if err := e.store.SaveProof(proof); err != nil {
		   e.log.Error().Err(err).Str("case_id", cs.ID).Msg("save dual decide proof failed")
	   }
	   e.trail.Append(AuditRecord{
		  CaseID: cs.ID, MovementID: cs.MovementID, UserID: cs.UserID,
		   Actor: loserActor, Action: "dual_decide_proof_sealed",
		  Reason: fmt.Sprintf("loser=%s decision=%s epoch=%d; winner=%s epoch=%d",
			   loserActor, loserDecision, loserEpoch, cs.DecidedBy, winnerEpoch),
		   FenceEpoch: cs.FenceEpoch,
	  })
	  metrics.CaseFenceDualDecideTotal.Inc()
	  return proof
  }

// ──────────────────────────────────────────
// BareDecideBlock 裸 Decide 拦截
// ──────────────────────────────────────────

  func (e *Engine) BareDecideBlock(_ context.Context, req BareDecideReq) (*BareFenceAlert, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	  }
	   alert := &BareFenceAlert{
		  ID: uuid.NewString(),
		   CaseID:     req.CaseID,
		  MovementID: req.MovementID,
		   Actor:      req.Actor,
		  Blocked:    e.cfg.BlockBareDecide,
		   Reason:    "bare Decide call on fund path without fence epoch/token",
		   CreatedAt: e.now(),
	  }
	   if err := e.store.SaveBareAlert(alert); err != nil {
		   e.log.Warn().Err(err).Str("case_id", req.CaseID).Msg("save bare alert failed")
	  }
	   e.trail.Append(AuditRecord{
		   CaseID: req.CaseID, MovementID: req.MovementID,
		  Actor: req.Actor, Action: "bare_decide_blocked",
		   Reason: alert.Reason,
	  })
	  metrics.CaseFenceBareDecideTotal.Inc()
	  if e.cfg.BlockBareDecide {
		  return alert, ErrBareForbidden
	   }
	   return alert, nil
  }

// ──────────────────────────────────────────
// SubmitCheck 出金 Submit 前校验 ticket + policyVer
// ──────────────────────────────────────────

  func (e *Engine) SubmitCheck(_ context.Context, req SubmitCheckReq) (*SubmitCheckResult, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	   }
	  ticket, err := e.store.GetTicket(req.TicketID)
	   if err != nil {
		  return &SubmitCheckResult{Allowed: false,
			   Reason: ErrTicketNotFound.Error()}, ErrTicketNotFound
	  }
	  if ticket.MovementID != req.MovementID {
		  return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: ErrMovementMismatch.Error()}, ErrMovementMismatch
	  }
	  now := e.now()
	  if ticket.State == TicketDenied {
		  return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: "ticket denied"}, nil
	   }
	   if ticket.State == TicketStale {
		  return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: ErrTicketStale.Error()}, ErrTicketStale
	   }
	  if ticket.State == TicketExpired || ticket.Expired(now) {
		   if ticket.State != TicketExpired {
			  ticket.State = TicketExpired
			  expAt := now
			  ticket.StaleAt = &expAt
			   ticket.StaleReason = "expired at submit check"
			   _ = e.store.SaveTicket(ticket)
		  }
		   return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: ErrTicketExpired.Error()}, ErrTicketExpired
	   }
	   if ticket.State != TicketApproved {
		   return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: "ticket not approved"}, nil
	   }
	  if !ticket.MatchPolicy(req.PolicyVer) {
		   staleAt := now
		   ticket.State = TicketStale
		   ticket.StaleAt = &staleAt
		   ticket.StaleReason = fmt.Sprintf("policy drift: ticket=%d submit=%d", ticket.PolicyVer, req.PolicyVer)
		  _ = e.store.SaveTicket(ticket)
		   e.trail.Append(AuditRecord{
			   CaseID: ticket.CaseID, TicketID: ticket.ID, MovementID: ticket.MovementID,
			  UserID: ticket.UserID, Actor: "system", Action: "policy_drift_reject",
			  Reason: ticket.StaleReason,
			   PolicyVer: req.PolicyVer, FenceEpoch: ticket.FenceEpoch,
		  })
		   metrics.CaseFencePolicyDriftTotal.Inc()
		  return &SubmitCheckResult{Allowed: false, Ticket: ticket, Reason: ErrPolicyDrift.Error()}, ErrPolicyDrift
	  }
	   consumed := now
	   ticket.ConsumedAt = &consumed
	  _ = e.store.SaveTicket(ticket)
	   e.trail.Append(AuditRecord{
		   CaseID: ticket.CaseID, TicketID: ticket.ID, MovementID: ticket.MovementID,
		  UserID: ticket.UserID, Actor: "system", Action: "submit_check_pass",
		   PolicyVer: ticket.PolicyVer, FenceEpoch: ticket.FenceEpoch,
	  })
	   return &SubmitCheckResult{Allowed: true, Ticket: ticket}, nil
  }

// ──────────────────────────────────────────
// ExpireStaleTickets 过期扫描
// ──────────────────────────────────────────

   func (e *Engine) ExpireStaleTickets(_ context.Context) (*ExpireResult, error) {
	  if e == nil {
		  return nil, ErrEngineNil
	  }
	   result := &ExpireResult{}
	   now := e.now()
	   tickets, err := e.store.ListActiveTickets()
	  if err != nil {
		  return nil, err
	   }
	   for _, t := range tickets {
		   if t.Expired(now) {
			   t.State = TicketExpired
			  staleAt := now
			  t.StaleAt = &staleAt
			  t.StaleReason = "TTL expired by patrol"
			  if err := e.store.SaveTicket(t); err == nil {
				   result.Expired = append(result.Expired, t.ID)
			   }
		  }
	   }
	  if e.cq != nil {
		  expired := e.cq.ExpireOlderThan(e.cfg.ExpireAge)
		   for _, cs := range expired {
			  result.Bumped = append(result.Bumped, cs.ID)
			  ticket, err := e.store.GetTicketByCase(cs.ID)
			  if err == nil && ticket.State.Active() {
				   ticket.State = TicketStale
				  staleAt := now
				   ticket.StaleAt = &staleAt
				  ticket.StaleReason = "case expired; fence epoch bumped"
				   _ = e.store.SaveTicket(ticket)
				  result.Expired = append(result.Expired, ticket.ID)
			  }
		  }
	  }
	   if len(result.Expired) > 0 || len(result.Bumped) > 0 {
		  e.trail.Append(AuditRecord{
			  Actor: "system", Action: "expire_patrol",
			   Reason: fmt.Sprintf("expired=%d bumped=%d", len(result.Expired), len(result.Bumped)),
		  })
	  }
	   return result, nil
  }

// ──────────────────────────────────────────
// ExpireApproveInterleave Expire↔Approve 互斥检查
// ──────────────────────────────────────────

   func (e *Engine) ExpireApproveInterleave(_ context.Context, caseID string) error {
	  if e == nil {
		  return ErrEngineNil
	  }
	  cs, err := e.cq.GetCase(caseID)
	  if err != nil {
		  return fmt.Errorf("%w: %v", ErrCaseNotFound, err)
	   }
	  if cs.IsExpired() {
		   return ErrExpireMutex
	   }
	   if cs.IsApproved() || cs.IsDenied() {
		   return ErrCaseAlreadyClosed
	  }
	  return nil
   }

// ──────────────────────────────────────────
// 查询接口
// ──────────────────────────────────────────

  func (e *Engine) GetTicket(_ context.Context, id string) (*CaseReleaseTicket, error) {
	  if e == nil {
		  return nil, ErrEngineNil
	   }
	   return e.store.GetTicket(id)
   }

  func (e *Engine) GetTicketByMovement(_ context.Context, movementID string) (*CaseReleaseTicket, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	  }
	  return e.store.GetTicketByMovement(movementID)
  }

  func (e *Engine) GetProof(_ context.Context, id string) (*CaseDualDecideProof, error) {
	   if e == nil {
		  return nil, ErrEngineNil
	  }
	  return e.store.GetProof(id)
  }

  func (e *Engine) ExportProofs(_ context.Context, req ProofExportReq) ([]*CaseDualDecideProof, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	   }
	  limit := req.Limit
	  if limit <= 0 {
		  limit = e.cfg.MaxProofsExport
	   }
	  if req.MovementID != "" {
		  return e.store.ListProofsByMovement(req.MovementID, limit)
	   }
	   return e.store.ListProofs(req.CaseID, limit)
   }

   func (e *Engine) ExportBareAlerts(_ context.Context, caseID string, limit int) ([]*BareFenceAlert, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	  }
	   return e.store.ListBareAlerts(caseID, limit)
   }

   func (e *Engine) Audit(caseID string, limit int) []AuditRecord {
	   return e.trail.List(caseID,
		   limit)
  }

  func (e *Engine) AuditByMovement(movementID string, limit int) []AuditRecord {
	   return e.trail.ListByMovement(movementID, limit)
   }

// ──────────────────────────────────────────
// DecideFencedOnlyForTest 围栏结案（内存案件查询器直通版，仅用）
// ──────────────────────────────────────────

   func (e *Engine) Trail() *AuditTrail { return e.trail }
   func (e *Engine) Store() TicketStore { return e.store }
   func (e *Engine) Config() Config     { return e.cfg }

// DecideFencedAndSeal 组合方法：调用 DecideFencedOnly 后立刻返回 result。
// 调用方在上层保证 CaseQuerier 的围栏检查逻辑。这里只暴露给 HTTP 层做一步调用。
   func (e *Engine) DecideFencedAndSeal(ctx context.Context, req DecideFencedReq) (*DecideFencedResult, error) {
	  return e.DecideFencedOnly(ctx, req)
  }

// traceSpan 创建可追踪的 span。

   func traceSpan(ctx context.Context, name string) (context.Context, *trace.Span) {
	   return trace.WithSpan(ctx, name)
   }

var _ = traceSpan
