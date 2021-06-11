package fundintgate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	DefaultTTLSeconds = 120
	MaxTTLSeconds     = 900
	ExpireBatchSize   = 200
	RevokeBatchSize   = 200
	PatrolBatchSize  = 100
	ProofExportBatch = 100
	DefaultGateImpl  = "fundint.GateFunc.v1"
)

// Service 闸租约核心服务。
type Service struct {
	store    Store
	resolver Resolver
	gate GateEval
	sender Sender
	clock  func() time.Time
	minAmt string
	minSet bool
	metrics *Metrics
}

// New 使用内存仓储构造服务。

  func New(resolver Resolver, gate GateEval, sender Sender) *Service {
	  return &Service{
		   store:    NewMemStore(),
		   resolver: resolver,
		  gate:   gate,
		  sender: sender,
		   clock:  time.Now,
		   metrics: NewMetrics(nil),
	   }
   }

// NewWithStore 注入持久化。
   func NewWithStore(store Store, resolver Resolver, gate GateEval, sender Sender) *Service {
	  return &Service{
		  store:    store,
		   resolver: resolver,
		   gate:     gate,
		   sender:   sender,
		  clock: time.Now,
		   metrics: NewMetrics(nil),
	   }
  }

// WithClock 注入时钟。

  func (s *Service) WithClock(fn func() time.Time) *Service {
	   if s != nil && fn != nil {
		  s.clock = fn
	   }
	   return s
   }

   // WithMetrics 注入指标。
  func (s *Service) WithMetrics(m *Metrics) *Service {
	  if s != nil && m != nil {
		   s.metrics = m
	   }
	  return s
   }

// BindMinAmount 一致 fundint 最小额拒绝语义。

   func (s *Service) BindMinAmount(raw string) *Service {
	   if s == nil {
		  return s
	  }
	  raw = strings.TrimSpace(raw)
	   if raw == "" {
		   s.minSet = false
		   return s
	  }
	   if _, err := CanonicalAmount(raw); err != nil {
		  return s
	  }
	  s.minAmt = raw
	   s.minSet = true
	  return s
   }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	   return s.clock().UTC()
   }

// EnsureGen 确保全局闸世代存在；首次初始化 gen=1。
  func (s *Service) EnsureGen(ctx context.Context, implID string) (GateGeneration, error) {
	  if s == nil || s.store == nil {
		  return GateGeneration{}, ErrUnavailable
	  }
	  g, err := s.store.GetGen(ctx, ScopeGlobal)
	  if err == nil {
		   return g, nil
	  }
	   if !errors.Is(err, ErrGenNotFound) {
		   return GateGeneration{}, err
	   }
	   if strings.TrimSpace(implID) == "" {
		  implID = DefaultGateImpl
	  }
	  now := s.now()
	   g = GateGeneration{
		   ID: uuid.New().String(),
		  Scope:      ScopeGlobal,
		  Gen:        1,
		  GateImplID: implID,
		   Halted:         false,
		   ForbidWithdraw: false,
		  BumpReason: "init",
		   BumpedAt:   now,
		  BumpedBy:   "system",
	  }
	  g.Fingerprint = RecomputeSurfaceFP(g.GateImplID, g.Halted, g.ForbidWithdraw, g.Gen)
	  if err := s.store.SaveGen(ctx, g); err != nil {
		   return GateGeneration{}, err
	  }
	   _ = s.audit(ctx, "", AuditGenBump, "system", "init gen=1", "", "")
	   return g, nil
   }

// LiveSurface 读取当前闸面。

  func (s *Service) LiveSurface(ctx context.Context) (GateSurface, error) {
	  g,
		   err := s.EnsureGen(ctx, DefaultGateImpl)
	   if err != nil {
		   return GateSurface{}, err
	  }
	  return SurfaceFromGen(g), nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// CheckWithLease
// ─────────────────────────────────────────────────────────────────────────────
// CheckWithLease 闸通过后签发租约。闸拒绝、最小额、解析失败保持既有拒绝语义，不签发。

   func (s *Service) CheckWithLease(ctx context.Context, req CheckWithLeaseReq) (IntGateLease, error) {
	  if s == nil || s.store == nil {
		  return IntGateLease{}, ErrUnavailable
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   from := strings.TrimSpace(req.UserID)
	   if from == "" {
		   return IntGateLease{}, fmt.Errorf("%w: user_id", ErrInvalidReq)
	  }
	  asset := CanonicalAsset(req.Asset)
	  if asset == "" {
		   return IntGateLease{}, fmt.Errorf("%w: asset", ErrInvalidReq)
	  }
	   amt, err := CanonicalAmount(req.Amount)
	   if err != nil {
		   return IntGateLease{}, err
	   }
	  if s.minSet {
		   minC, err := CanonicalAmount(s.minAmt)
		  if err == nil {
			  // 字符串固定 18 位后可直接字典比较（同精度）。
			   if amt < minC {
				  _ = s.audit(ctx, "", AuditRejectMinAmt, actorOf(req.Actor), "below min amount", "", "")
				  return IntGateLease{},
					  ErrMinAmount
			  }
		   }
	  }
	  if req.IdempotencyKey != "" {
		  if existing, ok, err := s.store.GetLeaseByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			   return IntGateLease{}, err
		   } else if ok {
			   return existing, nil
		  }
	  }
	  surf, err := s.LiveSurface(ctx)
	  if err != nil {
		  return IntGateLease{}, err
	  }
	   if surf.Halted {
		  return IntGateLease{}, ErrGateHalted
	   }
	   if surf.ForbidWithdraw {
		  return IntGateLease{},
			  ErrForbidWithdrawRaised
	   }
	   if s.gate != nil {
		   if err := s.gate.Allow(ctx, from); err != nil {
			  return IntGateLease{}, fmt.Errorf("%w: %v", ErrGateDenied, err)
		  }
	  }
	   if s.resolver == nil {
		   return IntGateLease{}, ErrResolveFail
	   }
	   res, err := s.resolver.Resolve(ctx, req.ToUID, req.ToEmail, req.ToPhone)
	   if err != nil {
		   _ = s.audit(ctx, "", AuditRejectResolve, actorOf(req.Actor), err.Error(), "", "")
		  return IntGateLease{}, fmt.Errorf("%w: %v", ErrResolveFail, err)
	  }
	   if EqualFoldID(res.ToUserID, from) {
		  _ = s.audit(ctx, "", AuditRejectSelfXfer, actorOf(req.Actor), "self transfer", "", "")
		   return IntGateLease{}, ErrSelfTransfer
	   }
	  ttl := req.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	   }
	   if ttl > MaxTTLSeconds {
		  ttl = MaxTTLSeconds
	  }
	   now := s.now()
	  lease := IntGateLease{
		   LeaseID:    uuid.New().String(),
		  FromUserID: from,
		  ToUserID:   strings.TrimSpace(res.ToUserID),
		  ResolveChannel: res.Channel,
		   ToHint:         firstNonEmpty(res.Hint, ComposeToHint(req.ToUID, req.ToEmail, req.ToPhone)),
		   Asset:          asset,
		   Amount: amt,
		  GateFingerprint: surf.Fingerprint,
		  GateImplID:      surf.ImplID,
		  GateGen:         surf.Gen,
		   Halted:          surf.Halted,
		   ForbidWithdraw:  surf.ForbidWithdraw,
		   State:           LeaseGated,
		   TTLSeconds: ttl,
		   IssuedAt: now,
		  IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := s.store.SaveLease(ctx, lease); err != nil {
		   return IntGateLease{}, err
	  }
	  _ = s.audit(ctx, lease.LeaseID, AuditIssue, actorOf(req.Actor), "issue gated lease", "", string(LeaseGated))
	   if s.metrics != nil {
		  s.metrics.LeaseIssued.Inc()
	  }
	   return lease, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// SendAssert
// ─────────────────────────────────────────────────────────────────────────────
// SendAssert 复验租约与闸面后调用 SpotSend。fail-closed：无租约立即拒绝并落证明。
  func (s *Service) SendAssert(ctx context.Context, req SendAssertReq) (SendAssertResult, error) {
	  if s == nil || s.store == nil {
			  LeaseID: req.LeaseID,
			  FromUserID: req.UserID, Asset: CanonicalAsset(req.Asset), Amount: req.Amount,
		   }, GateSurface{}, "lease not found")
		  _ = s.store.SaveProof(ctx, p)
		   return SendAssertResult{}, ErrNoLease
	   }
	   // 已消费幂等：相同 send idem 返回成功语义，不二次 SpotSend。
	  if lease.State == LeaseSent && req.IdempotencyKey != "" && lease.SendIdempotencyKey == req.IdempotencyKey {
		  return SendAssertResult{Lease: lease, TransferID: lease.TransferID, ToUserID: lease.ToUserID, Channel: lease.ResolveChannel}, nil
	   }
	   now := s.now()
	   if err := AssertGatedForBind(&lease, now); err != nil {
		   kind, mapped := mapLeaseErr(err)
		   p := s.buildProof(kind, lease, GateSurface{}, err.Error())
		   _ = s.store.SaveProof(ctx, p)
		  if s.metrics != nil {
			  s.metrics.DriftProofs.WithLabelValues(string(kind)).Inc()
		   }
		  return SendAssertResult{Lease: lease}, mapped
	   }
	   surf, err := s.LiveSurface(ctx)
	  if err != nil {
		  return SendAssertResult{}, err
	  }
	   if driftErr, kind := s.reverifySurface(lease, surf); driftErr != nil {
		   p := s.buildProof(kind, lease, surf, driftErr.Error())
		  _ = s.store.SaveProof(ctx, p)
		  if s.metrics != nil {
			  s.metrics.DriftProofs.WithLabelValues(string(kind)).Inc()
		   }
		  return SendAssertResult{Lease: lease},
			   driftErr
	  }
	   // 复验对手解析与金额资产。
	   if err := s.reverifyCounterpart(ctx, req, lease); err != nil {
		   kind := DriftResolve
		   if errors.Is(err, ErrAmountDrift) {
			  kind = DriftAmount
		  }
		   p := s.buildProof(kind, lease, surf, err.Error())
		   _ = s.store.SaveProof(ctx, p)
		  if s.metrics != nil {
			  s.metrics.DriftProofs.WithLabelValues(string(kind)).Inc()
		  }
		   return SendAssertResult{Lease: lease}, err
	  }
	   fromState := string(lease.State)
	  if lease.State == LeaseGated {
		   if err := ApplyLeaseTransition(&lease, LeaseBound, "", now); err != nil {
			   return SendAssertResult{}, err
		   }
	   }
	  lease.SendIdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	  if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return SendAssertResult{}, err
	  }
	   _ = s.audit(ctx, lease.LeaseID, AuditBind, actorOf(""), "bind send", fromState, string(lease.State))
	   if s.sender == nil {
		  return SendAssertResult{}, ErrSpotSendBlocked
	  }
	   idem := strings.TrimSpace(req.IdempotencyKey)
	   if idem == "" {
		   return SendAssertResult{}, ErrIdem
	  }
	  xferID, err := s.sender.SpotSend(ctx, lease.FromUserID, lease.ToUserID, lease.Asset, lease.Amount, idem)
	  if err != nil {
		  return SendAssertResult{Lease: lease}, err
	   }
	  if err := ApplyLeaseTransition(&lease, LeaseSent, "", s.now()); err != nil {
		  return SendAssertResult{}, err
	   }
	  lease.TransferID = xferID
	  if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return SendAssertResult{}, err
	   }
	  _ = s.audit(ctx, lease.LeaseID, AuditSend, actorOf(""), "spot send ok", string(LeaseBound), string(LeaseSent))
	  if s.metrics != nil {
		   s.metrics.SendBoundOK.Inc()
	  }
	  return SendAssertResult{
		   Lease:      lease,
		   TransferID: xferID,
		   ToUserID: lease.ToUserID,
		   Channel:  lease.ResolveChannel,
	   }, nil
   }

   func (s *Service) reverifySurface(lease IntGateLease, live GateSurface) (error, ProofKind) {
	  if live.Halted {
		   return ErrGateHalted, DriftHalt
	   }
	   if live.ForbidWithdraw && !lease.ForbidWithdraw {
		   return ErrForbidWithdrawRaised, DriftForbidRaised
	  }
	   if live.ForbidWithdraw && lease.ForbidWithdraw {
		   return ErrForbidWithdrawRaised, DriftForbidRaised
	   }
	   if !EqualFoldID(live.ImplID, lease.GateImplID) {
		  return ErrGateImplSwap, DriftImplSwap
	  }
	  if live.Gen != lease.GateGen {
		  return ErrGenerationDrift, DriftGeneration
	  }
	  if live.Fingerprint != lease.GateFingerprint {
		   return ErrFingerprintDrift, DriftFingerprint
	  }
	  return nil, ""
  }

  func (s *Service) reverifyCounterpart(ctx context.Context, req SendAssertReq, lease IntGateLease) error {
	   if strings.TrimSpace(req.UserID) != "" && !EqualFoldID(req.UserID, lease.FromUserID) {
		   return ErrAmountDrift
	  }
	   if asset := CanonicalAsset(req.Asset); asset != "" && asset != lease.Asset {
		   return ErrAmountDrift
	  }
	   if strings.TrimSpace(req.Amount) != "" {
		   amt, err := CanonicalAmount(req.Amount)
		  if err != nil {
			   return ErrAmountDrift
		   }
		   if amt != lease.Amount {
			  return ErrAmountDrift
		  }
	   }
	   if s.resolver == nil {
		   return ErrResolveFail
	  }
	  // 若请求未带对手标识，仅信任租约钉扎（绑定路径已解析）。
	  if strings.TrimSpace(req.ToUID) == "" && strings.TrimSpace(req.ToEmail) == "" && strings.TrimSpace(req.ToPhone) == "" {
		   return nil
	  }
	  res,
		  err := s.resolver.Resolve(ctx, req.ToUID, req.ToEmail, req.ToPhone)
	   if err != nil {
		   return fmt.Errorf("%w: %v", ErrResolveFail, err)
	   }
	   if !EqualFoldID(res.ToUserID, lease.ToUserID) {
		   return ErrResolveDrift
	   }
	  return nil
  }

   func mapLeaseErr(err error) (ProofKind, error) {
	  switch {
	   case errors.Is(err, ErrLeaseExpired):
		   return DriftExpired, ErrLeaseExpired
	  case errors.Is(err,
		   ErrLeaseRevoked):
		   return DriftRevoked, ErrLeaseRevoked
	   case errors.Is(err, ErrLeaseConsumed):
		  return DriftConsumed, ErrLeaseConsumed
	   default:
		  return DriftFingerprint, err
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// Halt / Policy flip
// ─────────────────────────────────────────────────────────────────────────────
// HaltGate 置 Halt 并抬升世代，立即吊销在途租约。

  func (s *Service) HaltGate(ctx context.Context, actor, reason string) (GateGeneration, int, error) {
	  return s.bump(ctx, actor, firstNonEmpty(reason, "halt"), func(g *GateGeneration) {
		   g.Halted = true
	  })
   }

// ResumeGate 解除 Halt 并抬升世代（旧 Halt 期间租约已吊销，不可复活）。
   func (s *Service) ResumeGate(ctx context.Context, actor string) (GateGeneration, int, error) {
	  return s.bump(ctx, actor, "resume", func(g *GateGeneration) {
		  g.Halted = false
	   })
  }

// SwapGateImpl 替换闸实现标识并抬升世代。
  func (s *Service) SwapGateImpl(ctx context.Context, newImpl, actor string) (GateGeneration, int, error) {
	  newImpl = strings.TrimSpace(newImpl)
	  if newImpl == "" {
		  return GateGeneration{}, 0, fmt.Errorf("%w: impl", ErrInvalidReq)
	  }
	  return s.bump(ctx, actor, "impl_swap:"+newImpl, func(g *GateGeneration) {
		   g.GateImplID = newImpl
	  })
  }

// RaiseForbidWithdraw 抬升禁提标志。

  func (s *Service) RaiseForbidWithdraw(ctx context.Context, actor string) (GateGeneration, int, error) {
	   return s.bump(ctx, actor, "forbid_withdraw_raised", func(g *GateGeneration) {
		  g.ForbidWithdraw = true
	   })
  }

// ClearForbidWithdraw 放下禁提并抬升世代。
   func (s *Service) ClearForbidWithdraw(ctx context.Context, actor string) (GateGeneration, int, error) {
	  return s.bump(ctx, actor, "forbid_withdraw_cleared", func(g *GateGeneration) {
		   g.ForbidWithdraw = false
	   })
  }

  func (s *Service) bump(ctx context.Context, actor, reason string, mut func(*GateGeneration)) (GateGeneration, int, error) {
	  if s == nil || s.store == nil {
		   return GateGeneration{}, 0, ErrUnavailable
	  }
	   g, err := s.EnsureGen(ctx, DefaultGateImpl)
	   if err != nil {
		   return GateGeneration{}, 0, err
	  }
	   mut(&g)
	  g.Gen++
	  g.Fingerprint = RecomputeSurfaceFP(g.GateImplID, g.Halted, g.ForbidWithdraw, g.Gen)
	  g.BumpReason = reason
	  g.BumpedAt = s.now()
	   g.BumpedBy = actorOf(actor)
	   if err := s.store.SaveGen(ctx, g); err != nil {
		   return GateGeneration{}, 0, err
	   }
	   _ = s.audit(ctx, "", AuditGenBump, actorOf(actor), reason, "", "")
	  n, err := s.RevokeStaleByGen(ctx, g.Gen, reason)
	   if s.metrics != nil {
		   s.metrics.GenBump.Inc()
		  s.metrics.LeaseRevoked.Add(float64(n))
	  }
	   return g, n, err
   }

  // RevokeStaleByGen 吊销 gate_gen < current 的 gated/bound 租约。

  func (s *Service) RevokeStaleByGen(ctx context.Context, current int64, reason string) (int, error) {
	   list, err := s.store.ListRevocableByGen(ctx, current, RevokeBatchSize)
	  if err != nil {
		  return 0, err
	  }
	   n := 0
	   now := s.now()
	  for i := range list {
		   from := list[i].State
		  if err := ApplyLeaseTransition(&list[i], LeaseRevoked, reason, now); err != nil {
			   continue
		  }
		  if err := s.store.UpdateLease(ctx, list[i]); err != nil {
			   continue
		   }
		  act := AuditRevokeHalt
		  if strings.Contains(reason, "impl_swap") {
			   act = AuditRevokeSwap
		  } else if strings.Contains(reason, "forbid") {
			  act = AuditRevokeForbid
		   }
		   _ = s.audit(ctx, list[i].LeaseID, act, "system", reason, string(from), string(LeaseRevoked))
		  n++
	  }
	  return n, nil
  }

// OpsRevoke 运维单笔吊销。
   func (s *Service) OpsRevoke(ctx context.Context, req OpsRevokeReq) (IntGateLease, error) {
	  lease, err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	   if err != nil {
		  return IntGateLease{}, err
	  }
	   from := lease.State
	   if err := ApplyLeaseTransition(&lease, LeaseRevoked, firstNonEmpty(req.Reason, "ops"), s.now()); err != nil {
		   return lease, err
	  }
	   if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return lease, err
	  }
	  _ = s.audit(ctx,
		  lease.LeaseID, AuditRevokeOps, actorOf(req.Actor), lease.RevokeReason, string(from), string(LeaseRevoked))
	  if s.metrics != nil {
		   s.metrics.LeaseRevoked.Inc()
	  }
	   return lease, nil
   }

// ExpireLeases 过期扫描。

   func (s *Service) ExpireLeases(ctx context.Context) (int, error) {
	   now := s.now()
	   list, err := s.store.ListExpiredGated(ctx, now, ExpireBatchSize)
	   if err != nil {
		   return 0, err
	  }
	  n := 0
	  for i := range list {
		   from := list[i].State
		   if err := ApplyLeaseTransition(&list[i], LeaseExpired, "ttl", now); err != nil {
			  continue
		  }
		   if err := s.store.UpdateLease(ctx, list[i]); err != nil {
			  continue
		   }
		   _ = s.audit(ctx, list[i].LeaseID, AuditExpire, "system", "ttl", string(from), string(LeaseExpired))
		  n++
	  }
	   if s.metrics != nil {
		  s.metrics.LeaseExpired.Add(float64(n))
	   }
	   return n, nil
   }

// PatrolUnconsumed 巡检未消费租约：闸面已变则吊销并落证明。
   func (s *Service) PatrolUnconsumed(ctx context.Context) (int, error) {
	   surf, err := s.LiveSurface(ctx)
	  if err != nil {
		  return 0, err
	   }
	  list, err := s.store.ListUnconsumed(ctx, PatrolBatchSize)
	   if err != nil {
		  return 0, err
	  }
	  n := 0
	   now := s.now()
	   for i := range list {
		  if driftErr, kind := s.reverifySurface(list[i], surf); driftErr != nil {
			   p := s.buildProof(DriftPatrol, list[i], surf, driftErr.Error()+"/"+string(kind))
			  _ = s.store.SaveProof(ctx, p)
			   from := list[i].State
			   if err := ApplyLeaseTransition(&list[i], LeaseRevoked, "patrol:"+string(kind), now); err == nil {
				   _ = s.store.UpdateLease(ctx, list[i])
				  _ = s.audit(ctx, list[i].LeaseID, AuditPatrol, "system", string(kind), string(from), string(LeaseRevoked))
				   n++
			  }
		   }
	  }
	  if s.metrics != nil {
		  s.metrics.PatrolRevoked.Add(float64(n))
	  }
	   return n, nil
   }

// ExportProofs 导出漂移证明。
  func (s *Service) ExportProofs(ctx context.Context, filter ExportProofFilter) ([]IntGateDriftProof, error) {
	   if filter.Limit <= 0 {
		  filter.Limit = ProofExportBatch
	  }
	   list, err := s.store.ListProofs(ctx, filter)
	  if err != nil {
		   return nil, err
	   }
	  ids := make([]string, 0, len(list))
	  for _, p := range list {
		  if p.ExportedAt == nil {
			   ids = append(ids, p.ProofID)
		   }
	   }
	  if len(ids) > 0 {
		  _ = s.store.MarkProofsExported(ctx, ids, s.now())
	  }
	  return list, nil
   }

// RebindOnly 仅复验闸指纹与对手并迁入 bound，不调用 SpotSend。
// 供 fundint.SendResolved 主链接着 Create，避免双记。
  func (s *Service) RebindOnly(ctx context.Context, req SendAssertReq) error {
	   if s == nil || s.store == nil {
		   return ErrUnavailable
	   }
	   if strings.TrimSpace(req.LeaseID) == "" {
		   p := s.buildProof(DriftNoLease, IntGateLease{FromUserID: req.UserID, Asset: CanonicalAsset(req.Asset), Amount: req.Amount}, GateSurface{}, "missing lease on rebind")
		   _ = s.store.SaveProof(ctx, p)
		  return ErrNoLease
	  }
	   lease,
		  err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	  if err != nil {
		  p := s.buildProof(DriftNoLease, IntGateLease{LeaseID: req.LeaseID, FromUserID: req.UserID}, GateSurface{}, "lease not found on rebind")
		  _ = s.store.SaveProof(ctx, p)
		  return ErrNoLease
	  }
	   now := s.now()
	   if err := AssertGatedForBind(&lease, now); err != nil {
		   kind, mapped := mapLeaseErr(err)
		  p := s.buildProof(kind, lease, GateSurface{}, err.Error())
		  _ = s.store.SaveProof(ctx, p)
		  return mapped
	  }
	  surf, err := s.LiveSurface(ctx)
	  if err != nil {
		  return err
	   }
	  if driftErr, kind := s.reverifySurface(lease, surf); driftErr != nil {
		   p := s.buildProof(kind, lease, surf, driftErr.Error())
		  _ = s.store.SaveProof(ctx, p)
		   return driftErr
	  }
	   if err := s.reverifyCounterpart(ctx, req, lease); err != nil {
		   kind := DriftResolve
		   if errors.Is(err, ErrAmountDrift) {
			   kind = DriftAmount
		   }
		  p := s.buildProof(kind, lease, surf, err.Error())
		   _ = s.store.SaveProof(ctx, p)
		  return err
	   }
	  from := string(lease.State)
	  if lease.State == LeaseGated {
		  if err := ApplyLeaseTransition(&lease, LeaseBound, "", now); err != nil {
			   return err
		  }
	  }
	  lease.SendIdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	  if err := s.store.UpdateLease(ctx, lease); err != nil {
		  return err
	   }
	   _ = s.audit(ctx, lease.LeaseID, AuditBind, "fundint", "rebind-only", from, string(lease.State))
	  return nil
  }

// MarkSent 在 fundint 主链 SpotSend 成功后将租约迁入 sent。

   func (s *Service) MarkSent(ctx context.Context, leaseID, transferID string) error {
	   lease,
		   err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return err
	   }
	   if lease.State == LeaseSent {
		  return nil
	  }
	  if err := ApplyLeaseTransition(&lease, LeaseSent, "", s.now()); err != nil {
		  return err
	   }
	  lease.TransferID = transferID
	   if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return err
	  }
	   _ = s.audit(ctx, lease.LeaseID, AuditSend, "fundint", "mark sent", string(LeaseBound), string(LeaseSent))
	  return nil
   }

// GetLease 只读查询。
  func (s *Service) GetLease(ctx context.Context, id string) (IntGateLease, error) {
	  return s.store.GetLease(ctx, id)
  }

// GetProof 只读查询。

  func (s *Service) GetProof(ctx context.Context, id string) (IntGateDriftProof, error) {
	  return s.store.GetProof(ctx, id)
  }

// ListAudit 只读审计。

  func (s *Service) ListAudit(ctx context.Context, leaseID string, limit int) ([]IntGateAudit, error) {
	   return s.store.ListAuditByLease(ctx, leaseID, limit)
   }

  func (s *Service) buildProof(kind ProofKind, lease IntGateLease, live GateSurface, evidence string) IntGateDriftProof {
	   now := s.now()
	   ev := evidence
	   if ev == "" {
		  b, _ := json.Marshal(map[string]any{
			   "kind": kind, "lease": lease.LeaseID, "live_gen": live.Gen,
		  })
		  ev = string(b)
	  }
	  return IntGateDriftProof{
		   ProofID: uuid.New().String(),
		   Kind:    kind,
		  LeaseID: lease.LeaseID,
		   FromUserID: lease.FromUserID,
		  ToUserID: lease.ToUserID,
		   Asset:    lease.Asset,
		  Amount:   lease.Amount,
		  BindFingerprint: lease.GateFingerprint,
		  LiveFingerprint: live.Fingerprint,
		  BindGen:         lease.GateGen,
		  LiveGen:    live.Gen,
		  BindToUser: lease.ToUserID,
		  LiveToUser: "",
		  Evidence:   ev,
		  DetectedAt: now,
		  CreatedAt: now,
	   }
   }

  func (s *Service) audit(ctx context.Context, leaseID string, action AuditAction, actor, detail, from, to string) error {
	  row := IntGateAudit{
		  AuditID: uuid.New().String(),
		  LeaseID: leaseID,
		  Action: action,
		  Actor:     actor,
		   Detail:    detail,
		   FromState: from,
		   ToState: to,
		  CreatedAt: s.now(),
	  }
	  return s.store.SaveAudit(ctx, row)
  }

  func actorOf(a string) string {
	   a = strings.TrimSpace(a)
	  if a == "" {
		  return "system"
	  }
	  return a
   }

  func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	   }
	   return b
  }
