// @FileName: service.go
// @Summary: holdopsgate 顶层服务——签发租约、Place/Release/Ack/Close、世代吊销、四眼清障。
// 核心调用流（Issue）：
// 1. 规范化用户/资产/金额/actor/nonce/kind
// 2. 冻结当前 hold_ops_gen，落 open 租约
// 核心调用流（Exec）：
// 1. 无 lease_id → 落 missing 证明并拒
// 2. 进程互斥 + 仓级状态推进
// 3. 复验用户/金额/资产/token/gap/世代
// 4. Release 与 Gap Close 互斥；Close 时 open token 必拒
// 5. Ack 只标 Acked，不释放资金
// 产出：hold_ops_gen / ops_lease_id。不写 holdcat 矩阵、holdpatrol advice、amlhold。
// @Author: KianFord
// @Created: 2021.07.15

package holdopsgate

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	secret string
	store Store
	gen   *GenerationEngine
	fsm   *LeaseFSM
	proof *ProofEngine
	hold    *TokenMutex
	metrics *GateMetrics
	audit   *AuditLog
	persist *PersistAuditor
	tokens *ShadowTokenBook
	gaps   *ShadowGapBook
	clock  func() time.Time
}

   func NewService(secret string, store Store) *Service {
	   if store == nil {
		   store = NewMemStore()
	  }
	   if secret == "" {
		  secret = "holdopsgate-local-hmac"
	   }
	   clock := func() time.Time { return time.Now().UTC() }
	   tok := NewShadowTokenBook(store)
	   return &Service{
		   secret: strings.TrimSpace(secret),
		   store: store,
		  gen:   newGenerationEngine(store, secret, clock),
		  fsm:   newLeaseFSM(store, secret, clock),
		  proof: newProofEngine(store, secret, clock),
		   hold: NewTokenMutex(),
		  metrics: newGateMetrics(),
		  audit: NewAuditLog(4096),
		   persist: newPersistAuditor(store, secret, clock),
		  tokens: tok,
		  gaps: NewShadowGapBook(store,
			   tok),
		  clock: clock,
	   }
   }

  func (s *Service) WithClock(clock func() time.Time) *Service {
	  if s == nil || clock == nil {
		   return s
	   }
	   s.clock = clock
	  s.gen.clock = clock
	  s.fsm.clock = clock
	   s.proof.clock = clock
	   s.persist.clock = clock
	  return s
   }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	  return s.clock()
  }

  func (s *Service) Store() Store {
	  if s == nil {
		  return nil
	   }
	  return s.store
   }

// Issue 签发 ops_lease_id，冻结 hold_ops_gen。

   func (s *Service) Issue(ctx context.Context, req IssueReq) (*HoldOpsLease, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  if err := rejectIsolation(req.Note, req.Nonce, req.Reason); err != nil {
		  s.metrics.denied.Add(1)
		   return nil, err
	   }
	   gen, err := s.gen.Current(ctx)
	   if err != nil {
		  return nil, err
	   }
	   lease, err := s.fsm.Issue(ctx, req, gen)
	   if err != nil {
		   s.metrics.denied.Add(1)
		  s.audit.Append(AuditEvent{Kind: "issue_deny", UserID: req.UserID, Note: err.Error(), At: s.now()})
		  return nil, err
	  }
	  s.metrics.issued.Add(1)
	  s.audit.Append(AuditEvent{Kind: "issue", LeaseID: lease.ID, OpsID: lease.OpsID, UserID: lease.UserID, At: s.now()})
	  _, _ = s.persist.Write(ctx, lease, "issued", "", "lease open")
	  return &lease, nil
  }

// Exec 在租约上执行 Place / Release / GapAck / GapClose。

   func (s *Service) Exec(ctx context.Context, req ExecReq) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	  if s == nil {
		   return nil, nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.LeaseID) == "" {
		   p, _ := s.proof.OpenMissing(ctx, req.UserID, "exec without ops_lease_id")
		  s.metrics.denied.Add(1)
		   s.metrics.proofs.Add(1)
		   s.audit.Append(AuditEvent{Kind: "exec_missing", UserID: req.UserID, ProofID: p.ID, At: s.now()})
		  return nil, &p, ErrLeaseMissing
	   }
	   kind, err := normKind(req.Kind)
	   if err != nil {
		  return nil, nil, err
	   }
	  if err := s.proof.UserBlocked(ctx, req.UserID, kind); err != nil {
		  s.metrics.denied.Add(1)
		  return nil, nil, err
	   }
	   lease, err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	  if err != nil {
		  return nil, nil, err
	   }
	  if err := s.matchLease(lease, req, kind); err != nil {
		  p, _ := s.proof.OpenLeaseFail(ctx, lease, errToProofKind(err), err.Error())
		   s.metrics.denied.Add(1)
		  s.metrics.proofs.Add(1)
		  return &lease, &p, err
	  }
	   if err := s.gen.Match(ctx, lease); err != nil {
		  p, _ := s.proof.OpenLeaseFail(ctx, lease, ProofStaleGen, err.Error())
		  s.metrics.denied.Add(1)
		   s.metrics.proofs.Add(1)
		   _, _ = s.persist.Write(ctx, lease, "deny_stale_gen", p.ID, err.Error())
		   return &lease, &p, err
	  }
	   ticket := req.Ticket
	   if ticket == "" {
		  ticket = lease.ID
	   }
	   if err := s.hold.TryHold(kind, lease.TokenID, lease.GapID, ticket); err != nil {
		   return s.loseRace(ctx,
			   lease, kind, err)
	  }
	   defer s.hold.Release(kind, lease.TokenID, lease.GapID, ticket)
	   bound, err := s.fsm.Bind(ctx, lease, ticket)
	  if err != nil {
		   p,
			  _ := s.proof.OpenLeaseFail(ctx, bound, errToProofKind(err), err.Error())
		  s.metrics.denied.Add(1)
		  s.metrics.proofs.Add(1)
		   _, _ = s.persist.Write(ctx, bound, "deny_bind", p.ID, err.Error())
		   return &bound, &p, err
	   }
	  s.metrics.bound.Add(1)
	   var proof *HoldOpsRaceProof
	  switch kind {
	  case OpsPlace:
		  err = s.execPlace(ctx, &bound)
	   case OpsRelease:
		   err = s.execRelease(ctx, &bound)
	  case OpsGapAck:
		   err = s.execAck(ctx, &bound)
	   case OpsGapClose:
		   proof, err = s.execClose(ctx, &bound)
	  default:
		  err = ErrKindInvalid
	   }
	  if err != nil {
		   if proof == nil {
			   pp, _ := s.proof.OpenLeaseFail(ctx, bound, errToProofKind(err), err.Error())
			   proof = &pp
			   s.metrics.proofs.Add(1)
		   }
		  s.metrics.denied.Add(1)
		  _, _ = s.persist.Write(ctx, bound, "deny_exec", proof.ID, err.Error())
		  s.audit.Append(AuditEvent{Kind: "exec_deny", LeaseID: bound.ID, OpsID: bound.OpsID, UserID: bound.UserID, ProofID: proof.ID, Note: err.Error(), At: s.now()})
		  return &bound, proof, err
	   }
	   if req.Consume || kind != OpsGapAck {
		   bound, err = s.fsm.Consume(ctx, bound)
		   if err != nil {
			  return &bound, proof, err
		   }
		   s.metrics.consumed.Add(1)
	   }
	   _, _ = s.persist.Write(ctx, bound, "ok", "", string(kind))
	  s.audit.Append(AuditEvent{Kind: "exec_ok", LeaseID: bound.ID, OpsID: bound.OpsID, UserID: bound.UserID, At: s.now()})
	  return &bound, proof, nil
  }

   func (s *Service) matchLease(lease HoldOpsLease, req ExecReq, kind OpsKind) error {
	   if lease.Kind != kind {
		  return ErrLeaseKind
	   }
	   if req.UserID != "" && lease.UserID != strings.TrimSpace(req.UserID) {
		  return ErrLeaseUser
	   }
	   if req.Asset != "" && lease.Asset != strings.ToUpper(strings.TrimSpace(req.Asset)) {
		   return ErrLeaseAsset
	  }
	   if req.Amount != "" {
		   amt, err := normAmount(req.Amount)
		   if err != nil || amt != lease.Amount {
			   return ErrLeaseAmount
		  }
	  }
	  if req.TokenID != "" && lease.TokenID != strings.TrimSpace(req.TokenID) {
		  return ErrLeaseToken
	  }
	   if req.GapID != "" && lease.GapID != strings.TrimSpace(req.GapID) {
		  return ErrLeaseGap
	   }
	   return nil
  }

   func (s *Service) loseRace(ctx context.Context, lease HoldOpsLease, kind OpsKind, race error) (*HoldOpsLease, *HoldOpsRaceProof, error) {
	   s.metrics.denied.Add(1)
	   s.metrics.proofs.Add(1)
	   var p HoldOpsRaceProof
	   switch {
	   case race == ErrDualRelease:
		  s.metrics.dualRelease.Add(1)
		   winner := HoldOpsLease{ID: "winner-inflight", OpsGen: lease.OpsGen, TokenID: lease.TokenID}
		   p, _ = s.proof.OpenDualRelease(ctx, winner, lease)
	   case race == ErrReleaseClose:
		  winner := HoldOpsLease{ID: "winner-inflight", OpsGen: lease.OpsGen, TokenID: lease.TokenID, GapID: lease.GapID}
		  p, _ = s.proof.OpenReleaseClose(ctx, winner, lease)
	  default:
		  p, _ = s.proof.OpenLeaseFail(ctx, lease, ProofConcurrent, race.Error())
	  }
	  _, _ = s.persist.Write(ctx, lease, "deny_race", p.ID, race.Error())
	  s.audit.Append(AuditEvent{Kind: "race", LeaseID: lease.ID, ProofID: p.ID, Note: race.Error(), At: s.now()})
	   return &lease, &p, race
   }

   func (s *Service) execPlace(ctx context.Context, lease *HoldOpsLease) error {
	   t := ShadowToken{
		  ID:     lease.TokenID,
		  UserID: lease.UserID,
		  Asset: lease.Asset,
		   Amount: lease.Amount,
		  Reason: firstNonEmpty(lease.Reason, "ops_place"),
		  Ref:    lease.Ref,
		  GapID: lease.GapID,
		  Open: true,
	  }
	   _, err := s.tokens.Place(ctx, t)
	  if err != nil {
		  return err
	   }
	   if lease.GapID != "" {
		   _, _ = s.gaps.Ensure(ctx, ShadowGap{
			  ID: lease.GapID,
			   UserID: lease.UserID,
			  Asset:  lease.Asset,
			   Amount: lease.Amount,
		  })
	  }
	   return nil
   }

  func (s *Service) execRelease(ctx context.Context, lease *HoldOpsLease) error {
	   _,
		  err := s.tokens.Release(ctx, lease.TokenID)
	   return err
   }

  func (s *Service) execAck(ctx context.Context, lease *HoldOpsLease) error {
	  if _, err := s.gaps.Ensure(ctx, ShadowGap{
		   ID: lease.GapID,
		   UserID: lease.UserID,
		   Asset: lease.Asset,
		   Amount: lease.Amount,
	   }); err != nil {
		  return err
	   }
	   g, err := s.gaps.Ack(ctx, lease.GapID)
	   if err != nil {
		  return err
	  }
	  if g.Closed {
		  return ErrAckNotRelease
	  }
	   // Ack 不得释放 token。若存在 open token，仅落观测证明，不挡 Ack 本身。
	  open,
		  _ := s.tokens.OpenByGap(ctx, lease.GapID)
	   if len(open) > 0 {
		  p, _ := s.proof.Open(ctx, HoldOpsRaceProof{
			  LeaseID: lease.ID,
			  OpsID: lease.OpsID,
			   TokenID: lease.TokenID,
			   GapID:  lease.GapID,
			   UserID: lease.UserID,
			   Kind:   ProofAckNotRelease,
			   Status: ProofProven,
			  OpenTokenCount: len(open),
			   Note:           "ack recorded; tokens remain open; not a payout",
		   })
		   s.metrics.proofs.Add(1)
		   s.metrics.ackSafe.Add(1)
		   _, _ = s.persist.Write(ctx, *lease, "ack_not_release", p.ID, "ack is not release")
	   } else {
		   s.metrics.ackSafe.Add(1)
	  }
	   return nil
   }

  func (s *Service) execClose(ctx context.Context, lease *HoldOpsLease) (*HoldOpsRaceProof, error) {
	  if _, err := s.gaps.Ensure(ctx, ShadowGap{
		   ID:     lease.GapID,
		   UserID: lease.UserID,
		  Asset:  lease.Asset,
		  Amount: lease.Amount,
	  }); err != nil {
		   return nil, err
	  }
	  g, err := s.gaps.Close(ctx, lease.GapID)
	   if err == ErrGapOpenToken {
		  s.metrics.gapBlocked.Add(1)
		  s.metrics.proofs.Add(1)
		   p, _ := s.proof.OpenGapOpenToken(ctx, *lease, g.OpenCount)
		  return &p, ErrGapOpenToken
	   }
	   return nil, err
  }

// Bump 抬 hold_ops_gen 并吊销未终态租约。

   func (s *Service) Bump(ctx context.Context, req BumpReq) (*HoldOpsGeneration, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if _, err := normActor(req.Actor); err != nil {
		   return nil, err
	   }
	   gen, err := s.gen.Bump(ctx, req.Expect, req.Fence, req.Note)
	   if err != nil {
		   return nil, err
	   }
	  s.metrics.genBumps.Add(1)
	   s.metrics.revoked.Add(1)
	   s.audit.Append(AuditEvent{Kind: "bump", Note: req.Note, At: s.now()})
	   return &gen, nil
   }

// ExpireSweep 过期扫描。
   func (s *Service) ExpireSweep(ctx context.Context, limit int) (int, error) {
	  if s == nil {
		  return 0, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 64
	   }
	   open, err := s.store.ListOpenLeases(ctx)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	   now := s.now()
	   for _, l := range open {
		  if n >= limit {
			   break
		  }
		   if !l.ExpiredAt(now) {
			   continue
		  }
		   got, err := s.fsm.ExpireOne(ctx, l)
		  if err != nil {
			  continue
		  }
		   p, _ := s.proof.OpenLeaseFail(ctx, got, ProofWorkerExpire, "worker expired lease")
		   s.metrics.expired.Add(1)
		  s.metrics.proofs.Add(1)
		  _, _ = s.persist.Write(ctx, got, "expired", p.ID, "ttl")
		   n++
	   }
	   s.metrics.workerTicks.Add(1)
	  return n, nil
  }

// FourEye 清障。
  func (s *Service) FourEye(ctx context.Context, req FourEyeReq) (*HoldOpsRaceProof, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  p, err := s.proof.FourEye(ctx, req)
	  if err != nil {
		  return nil, err
	   }
	  s.audit.Append(AuditEvent{Kind: "four_eye", ProofID: p.ID, Note: req.Reason, At: s.now()})
	  return &p, nil
   }

   func (s *Service) GetLease(ctx context.Context, id string) (HoldOpsLease, error) {
	  return s.store.GetLease(ctx, id)
   }

   func (s *Service) GetProof(ctx context.Context, id string) (HoldOpsRaceProof, error) {
	   return s.store.GetProof(ctx, id)
   }

   func (s *Service) GetGeneration(ctx context.Context) (HoldOpsGeneration, error) {
	   return s.gen.Current(ctx)
  }

   func (s *Service) GetToken(ctx context.Context, id string) (ShadowToken, error) {
	   return s.store.GetToken(ctx, id)
  }

   func (s *Service) GetGap(ctx context.Context, id string) (ShadowGap, error) {
	   return s.gaps.Lookup(ctx, id)
  }

   func (s *Service) Stats(ctx context.Context) Stats {
	   open,
		  _ := s.store.ListOpenLeases(ctx)
	  proofs, _ := s.store.ListOpenProofs(ctx)
	  gen, _ := s.gen.Current(ctx)
	  return s.metrics.Snapshot(len(open), len(proofs), gen.OpsGen)
  }

  func (s *Service) AuditRing() []AuditEvent {
	   return s.audit.Snapshot()
   }
