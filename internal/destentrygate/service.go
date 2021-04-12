// 功能：destentrygate 包内 service 实现。
// 白名单条目生命周期与出金提交租约收口引擎 — 领域服务。
// 1. PrecheckWithLease — destallow 预检通过后签发 dest_entry_lease_id。
// 2. AssertLease — 出金提交必须携带未过期租约并复验 entry_gen / mode / 在册。
// 3. Temp 过期 / Breakglass 收回 / Revoke / Restore 未满 — 抬升 entry_gen 并立即吊销旧租约。
// 4. Fail-closed — WhitelistOnly 开启且缺租约不得出金。

// 作者：Draven
// 日期：2021.04.11

package destentrygate

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
	DefaultTTLSeconds = 180
	MaxTTLSeconds     = 900
	ExpireBatchSize = 200
	RevokeBatchSize = 200
	PatrolBatchSize = 100
	ProofExportBatch = 100
)

// Service 条目租约核心服务。
type Service struct {
	store   Store
	surface EntrySurface
	sink    WithdrawSink
	clock   func() time.Time
	metrics *Metrics
}

// New 内存仓储构造。

   func New(surface EntrySurface, sink WithdrawSink) *Service {
	  return &Service{
		   store:   NewMemStore(),
		   surface: surface,
		  sink:    sink,
		   clock:   time.Now,
		   metrics: NewMetrics(nil),
	  }
  }

// NewWithStore 注入持久化。

   func NewWithStore(store Store, surface EntrySurface, sink WithdrawSink) *Service {
	   if store == nil {
		   store = NewMemStore()
	   }
	   return &Service{
		  store: store,
		  surface: surface,
		  sink:    sink,
		   clock:   time.Now,
		  metrics: NewMetrics(nil),
	  }
   }

  func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil && fn != nil {
		  s.clock = fn
	   }
	   return s
   }

  func (s *Service) WithMetrics(m *Metrics) *Service {
	   if s != nil && m != nil {
		  s.metrics = m
	  }
	   return s
  }

  func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	  return s.clock().UTC()
   }

  func (s *Service) Store() Store {
	  if s == nil {
		  return nil
	   }
	   return s.store
  }

// PrecheckWithLeaseReq 预检并签发租约。

type PrecheckWithLeaseReq struct {
	UserID  string
	Chain   string
	Address string
	TTLSeconds int64
	IdempotencyKey string
	Actor          string
}

// AssertLeaseReq 出金前提交租约复验。

type AssertLeaseReq struct {
	LeaseID string
	UserID string
	Chain          string
	Address        string
	IdempotencyKey string
	Consume bool
	Actor   string
}

// RenewLeaseReq 续租。冷却未满必须失败。

type RenewLeaseReq struct {
	LeaseID string
	TTLSeconds     int64
	IdempotencyKey string
	Actor string
}

// EnsureGen 确保条目世代存在；首次 gen=1。

   func (s *Service) EnsureGen(ctx context.Context, live LiveEntry, actor, reason string) (EntryGeneration, error) {
	   if s == nil || s.store == nil {
		   return EntryGeneration{}, ErrUnavailable
	   }
	  if strings.TrimSpace(live.EntryID) == "" {
		   return EntryGeneration{}, ErrInvalidReq
	  }
	  g, err := s.store.GetGen(ctx, live.EntryID)
	   if err == nil {
		  return g, nil
	  }
	   if !errors.Is(err, ErrGenNotFound) {
		   return EntryGeneration{}, err
	   }
	  now := s.now()
	   g = EntryGeneration{
		  EntryID: live.EntryID,
		  UserID: live.UserID,
		  Chain:  live.Chain,
		  Address:    live.Address,
		   Gen:        1,
		  Mode:       live.Mode,
		   BumpReason: firstNonEmpty(reason, "init"),
		   BumpedAt: now,
		  BumpedBy: firstNonEmpty(actor, "system"),
	  }
	  if err := s.store.SaveGen(ctx, g); err != nil {
		   return EntryGeneration{}, err
	   }
	   _ = s.audit(ctx, "", live.EntryID, AuditGenBump, g.BumpedBy, "init gen=1")
	  return g, nil
  }

// BumpGen 抬升条目世代（生命周期事件）。
   func (s *Service) BumpGen(ctx context.Context, live LiveEntry, actor, reason string) (EntryGeneration, error) {
	   g, err := s.EnsureGen(ctx, live, actor, reason)
	  if err != nil {
		   return EntryGeneration{}, err
	   }
	  g.Gen++
	  g.Mode = live.Mode
	   g.BumpReason = reason
	   g.BumpedAt = s.now()
	  g.BumpedBy = firstNonEmpty(actor, "system")
	   if err := s.store.SaveGen(ctx, g); err != nil {
		   return EntryGeneration{}, err
	  }
	   if s.metrics != nil {
		   s.metrics.IncGenBump(reason)
	  }
	  _ = s.audit(ctx, "", live.EntryID, AuditGenBump, g.BumpedBy, fmt.Sprintf("gen=%d reason=%s", g.Gen, reason))
	   return g, nil
  }

// PrecheckWithLease 预检通过后签发 dest_entry_lease_id。
  func (s *Service) PrecheckWithLease(ctx context.Context, req PrecheckWithLeaseReq) (DestEntryLease, error) {
	  if s == nil || s.store == nil || s.surface == nil {
		  return DestEntryLease{}, ErrUnavailable
	   }
	   userID := strings.TrimSpace(req.UserID)
	  chain := strings.ToUpper(strings.TrimSpace(req.Chain))
	  addr := strings.TrimSpace(req.Address)
	   if userID == "" || chain == "" || addr == "" {
		   return DestEntryLease{}, ErrInvalidReq
	  }
	  if strings.TrimSpace(req.IdempotencyKey) == "" {
		  return DestEntryLease{}, ErrIdem
	  }
	  if prev, ok, err := s.store.GetLeaseByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
		   return DestEntryLease{}, err
	   } else if ok {
		   return prev, nil
	   }
	   only := s.surface.WhitelistOnly(ctx,
		   userID)
	   live, err := s.surface.Snapshot(ctx, userID, chain, addr)
	  if err != nil {
		   return DestEntryLease{}, err
	   }
	   if live.EntryID == "" {
		   live.EntryID = DeriveEntryID(userID, chain, addr)
		  live.UserID,
			  live.Chain, live.Address = userID, chain, addr
	   }
	   if only && !live.Live {
		   _ = s.audit(ctx, "", live.EntryID, AuditPrecheckDeny, req.Actor, firstNonEmpty(live.Reason, "not_live"))
		  if live.Reason == "cooldown" || !live.CooldownFull && live.Reason != "missing" {
			   return DestEntryLease{}, ErrCooldownNotFull
		   }
		   if live.Reason == "temp_expired" {
			   return DestEntryLease{},
				  ErrEntryExpired
		  }
		   return DestEntryLease{}, ErrPrecheckDenied
	   }
	   if !live.CooldownFull && live.Mode == ModeStrict && only {
		  return DestEntryLease{},
			  ErrCooldownNotFull
	   }
	  if live.Mode == "" {
		  live.Mode = ModeStrict
	   }
	   g, err := s.EnsureGen(ctx, live, req.Actor, "precheck")
	   if err != nil {
		  return DestEntryLease{}, err
	  }
	  ttl := req.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	  }
	  if ttl > MaxTTLSeconds {
		  ttl = MaxTTLSeconds
	  }
	  now := s.now()
	  lease := DestEntryLease{
		  LeaseID: uuid.New().String(),
		   EntryID: live.EntryID,
		  EntryGen: g.Gen,
		  UserID: userID,
		   Chain:  chain,
		  Address: addr,
		  Mode: live.Mode,
		   WhitelistOnly: only,
		  CooldownFull: live.CooldownFull || live.Mode == ModeBreakglass,
		  State:        LeaseIssued,
		  TTLSeconds:   ttl,
		  IssuedAt: now,
		  IdempotencyKey: req.IdempotencyKey,
		   CreatedAt:      now,
		  UpdatedAt: now,
	   }
	  if err := s.store.SaveLease(ctx, lease); err != nil {
		  return DestEntryLease{}, err
	   }
	  if s.metrics != nil {
		  s.metrics.IncIssued(string(live.Mode))
	  }
	  _ = s.audit(ctx, lease.LeaseID, live.EntryID, AuditIssue, firstNonEmpty(req.Actor, userID),
		  fmt.Sprintf("mode=%s gen=%d", live.Mode, g.Gen))
	  return lease, nil
  }

// AssertLease 出金前提复验。WhitelistOnly 开启且缺租约 fail-closed。
  func (s *Service) AssertLease(ctx context.Context, req AssertLeaseReq) (WithdrawBindResult, error) {
	  if s == nil || s.store == nil || s.surface == nil {
		   return WithdrawBindResult{}, ErrUnavailable
	   }
	  userID := strings.TrimSpace(req.UserID)
	   chain := strings.ToUpper(strings.TrimSpace(req.Chain))
	  addr := strings.TrimSpace(req.Address)
	  only := false
	   if userID != "" {
		   only = s.surface.WhitelistOnly(ctx, userID)
	  }
	   if strings.TrimSpace(req.LeaseID) == "" {
		  if only {
			   _ = s.openProof(ctx, DestEntryDriftProof{
				   Kind: DriftNoLease, UserID: userID, Chain: chain, Address: addr,
				   Evidence: "whitelist_only missing dest_entry_lease_id",
			  })
			   if s.metrics != nil {
				   s.metrics.IncProof(string(DriftNoLease))
			  }
			  return WithdrawBindResult{},
				  ErrWhitelistOnly
		  }
		  return WithdrawBindResult{}, ErrNoLease
	   }
	   lease, err := s.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  if errors.Is(err, ErrLeaseNotFound) && only {
			  _ = s.openProof(ctx, DestEntryDriftProof{
				   Kind: DriftNoLease, LeaseID: req.LeaseID, UserID: userID, Chain: chain, Address: addr,
				  Evidence: "lease not found under whitelist_only",
			   })
			  return WithdrawBindResult{}, ErrWhitelistOnly
		   }
		  return WithdrawBindResult{}, err
	  }
	   now := s.now()
	   if err := AssertIssuedForBind(&lease, now); err != nil {
		   kind := proofKindFromAssert(err)
		  _ = s.openProof(ctx, DestEntryDriftProof{
			   Kind: kind, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			  UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			   BindMode: lease.Mode, BindGen: lease.EntryGen,
			  Evidence: err.Error(),
		  })
		  return WithdrawBindResult{}, err
	  }
	  if userID != "" && (lease.UserID != userID || lease.Chain != chain || lease.Address != addr) {
		  _ = s.openProof(ctx, DestEntryDriftProof{
			   Kind: DriftAddr, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			  UserID: userID, Chain: chain, Address: addr,
			  BindMode: lease.Mode,
			  BindGen:  lease.EntryGen,
			  Evidence: "submit tuple differs from lease pin",
		   })
		   return WithdrawBindResult{}, ErrAddrDrift
	  }
	   live, err := s.surface.Snapshot(ctx, lease.UserID, lease.Chain, lease.Address)
	   if err != nil {
		   return WithdrawBindResult{}, err
	  }
	   if live.EntryID == "" {
		  live.EntryID = lease.EntryID
	  }
	   g, gerr := s.store.GetGen(ctx, lease.EntryID)
	   liveGen := lease.EntryGen
	  if gerr == nil {
		   liveGen = g.Gen
	   }
	  if !live.Live {
		  kind := DriftMissing
		  ret := ErrEntryMissing
		  switch live.Reason {
		   case "temp_expired":
			   kind, ret = DriftTempExpire, ErrEntryExpired
		   case "breakglass_reclaimed":
			  kind, ret = DriftBreakglassReclaim, ErrBreakglassReclaimed
		  case "cooldown":
			   kind, ret = DriftCooldown, ErrCooldownNotFull
		  }
		  _ = s.openProof(ctx, DestEntryDriftProof{
			   Kind: kind, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			  UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			  BindMode: lease.Mode, LiveMode: live.Mode, BindGen: lease.EntryGen, LiveGen: liveGen,
			   Evidence: firstNonEmpty(live.Reason, "entry not live"),
		  })
		  return WithdrawBindResult{}, ret
	   }
	  if live.Mode != "" && live.Mode != lease.Mode {
		  _ = s.openProof(ctx, DestEntryDriftProof{
			  Kind: DriftMode, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			   UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			   BindMode: lease.Mode,
			   LiveMode: live.Mode, BindGen: lease.EntryGen, LiveGen: liveGen,
			   Evidence: "mode changed after precheck",
		   })
		   return WithdrawBindResult{}, ErrModeDrift
	   }
	   if gerr == nil && g.Gen != lease.EntryGen {
		  _ = s.openProof(ctx, DestEntryDriftProof{
			  Kind: DriftEntryGen, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			   UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			   BindMode: lease.Mode,
			   LiveMode: live.Mode, BindGen: lease.EntryGen, LiveGen: g.Gen,
			  Evidence: fmt.Sprintf("lease gen=%d live gen=%d", lease.EntryGen, g.Gen),
		  })
		   return WithdrawBindResult{}, ErrEntryGenDrift
	   }
	  if lease.State == LeaseIssued {
		   if err := ApplyLeaseTransition(&lease, LeaseBound, "", now); err != nil {
			   return WithdrawBindResult{}, err
		   }
		   lease.BindIdemKey = req.IdempotencyKey
		   if err := s.store.UpdateLease(ctx, lease); err != nil {
			   return WithdrawBindResult{}, err
		   }
		   _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditBind, firstNonEmpty(req.Actor, userID), "bound")
		   if s.metrics != nil {
			  s.metrics.IncBound()
		   }
	   }
	  out := WithdrawBindResult{LeaseID: lease.LeaseID, State: lease.State, Bound: true}
	   if !req.Consume {
		  return out, nil
	   }
	  if s.sink == nil {
		  return WithdrawBindResult{}, ErrUnavailable
	   }
	   ref, err := s.sink.Submit(ctx, lease.UserID, lease.Chain, lease.Address, lease.LeaseID, req.IdempotencyKey)
	   if err != nil {
		  return WithdrawBindResult{}, err
	   }
	   if err := ApplyLeaseTransition(&lease, LeaseConsumed, "", s.now()); err != nil {
		  return WithdrawBindResult{}, err
	   }
	   lease.WithdrawRef = ref
	   if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return WithdrawBindResult{}, err
	   }
	  _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditConsume, firstNonEmpty(req.Actor, userID), ref)
	  if s.metrics != nil {
		  s.metrics.IncConsumed()
	   }
	   out.WithdrawRef = ref
	   out.State = lease.State
	   out.Consumed = true
	   return out, nil
   }

// RenewLease 续租。冷却未满或条目非 live 拒绝（Restore not-full）。
   func (s *Service) RenewLease(ctx context.Context, req RenewLeaseReq) (DestEntryLease, error) {
	  if s == nil || s.store == nil || s.surface == nil {
		   return DestEntryLease{}, ErrUnavailable
	   }
	   lease, err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	  if err != nil {
		   return DestEntryLease{}, err
	   }
	  live,
		  err := s.surface.Snapshot(ctx, lease.UserID, lease.Chain, lease.Address)
	   if err != nil {
		  return DestEntryLease{}, err
	   }
	  if !live.Live || !live.CooldownFull {
		  _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditRenewReject, req.Actor, firstNonEmpty(live.Reason, "cooldown_not_full"))
		  _ = s.openProof(ctx, DestEntryDriftProof{
			   Kind: DriftCooldown, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			  UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			  BindMode: lease.Mode, BindGen: lease.EntryGen,
			  Evidence: "renew path requires live entry and full cooldown",
		  })
		  return DestEntryLease{}, ErrRenewDenied
	   }
	  if err := AssertIssuedForBind(&lease, s.now()); err != nil {
		  return DestEntryLease{}, err
	   }
	  ttl := req.TTLSeconds
	  if ttl <= 0 {
		  ttl = DefaultTTLSeconds
	  }
	  if ttl > MaxTTLSeconds {
		  ttl = MaxTTLSeconds
	  }
	   lease.TTLSeconds = ttl
	   lease.IssuedAt = s.now()
	   lease.UpdatedAt = s.now()
	  if err := s.store.UpdateLease(ctx, lease); err != nil {
		   return DestEntryLease{}, err
	   }
	   return lease, nil
   }

// OnTempExpired Temp 过期：抬升世代并吊销旧租约。
   func (s *Service) OnTempExpired(ctx context.Context, live LiveEntry, actor string) (int, error) {
	  return s.lifecycleRevoke(ctx, live, actor, "temp_expire", AuditRevokeTemp, DriftTempExpire)
  }

// OnBreakglassReclaim 破窗收回。
  func (s *Service) OnBreakglassReclaim(ctx context.Context, live LiveEntry, actor string) (int, error) {
	   return s.lifecycleRevoke(ctx, live, actor, "breakglass_reclaim", AuditRevokeGlass, DriftBreakglassReclaim)
   }

// OnEntryRevoke 条目撤销。
   func (s *Service) OnEntryRevoke(ctx context.Context, live LiveEntry, actor string) (int, error) {
	  return s.lifecycleRevoke(ctx, live, actor, "entry_revoke", AuditRevokeEntry, DriftRevoke)
  }

// OnRestoreCooldownNotFull Restore 冷却未满：旧租约立即失效。
   func (s *Service) OnRestoreCooldownNotFull(ctx context.Context, live LiveEntry, actor string) (int, error) {
	   _ = s.audit(ctx, "", live.EntryID, AuditRestoreNotFull, actor, "cooldown not full")
	   return s.lifecycleRevoke(ctx, live, actor, "cooldown_not_full", AuditRevokeCooldown, DriftCooldown)
   }

   func (s *Service) lifecycleRevoke(ctx context.Context, live LiveEntry, actor, reason string, act AuditAction, kind ProofKind) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	   }
	  if live.EntryID == "" {
		  live.EntryID = DeriveEntryID(live.UserID, live.Chain, live.Address)
	   }
	  g, err := s.BumpGen(ctx, live, actor, reason)
	   if err != nil {
		   return 0, err
	   }
	   n, err := s.revokeLeasesOfEntry(ctx, live.EntryID, g.Gen, reason, act, kind, live)
	   if err != nil {
		   return n, err
	   }
	   if s.metrics != nil {
		   s.metrics.IncRevoked(reason, n)
	   }
	  return n, nil
   }

   func (s *Service) revokeLeasesOfEntry(ctx context.Context, entryID string, newGen int64, reason string, act AuditAction, kind ProofKind, live LiveEntry) (int, error) {
	   rows, err := s.store.ListRevocableByEntryGen(ctx, entryID, newGen, RevokeBatchSize)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	  now := s.now()
	  for _, lease := range rows {
		  if err := ApplyLeaseTransition(&lease, LeaseRevoked, reason, now); err != nil {
			   continue
		   }
		  if err := s.store.UpdateLease(ctx, lease); err != nil {
			   return n, err
		   }
		   _ = s.openProof(ctx, DestEntryDriftProof{
			   Kind: kind, LeaseID: lease.LeaseID, EntryID: entryID,
			   UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			  BindMode: lease.Mode,
			  LiveMode: live.Mode, BindGen: lease.EntryGen, LiveGen: newGen,
			   Evidence: reason,
		   })
		  _ = s.audit(ctx, lease.LeaseID, entryID, act, "system", reason)
		   n++
	   }
	  return n, nil
  }

// OpsRevoke 运维按 lease_id 吊销。
   func (s *Service) OpsRevoke(ctx context.Context, leaseID, actor, reason string) (DestEntryLease, error) {
	   if s == nil || s.store == nil {
		   return DestEntryLease{}, ErrUnavailable
	  }
	   lease, err := s.store.GetLease(ctx, strings.TrimSpace(leaseID))
	  if err != nil {
		  return DestEntryLease{}, err
	  }
	   if err := ApplyLeaseTransition(&lease, LeaseRevoked, firstNonEmpty(reason, "ops"), s.now()); err != nil {
		   return DestEntryLease{}, err
	   }
	   if err := s.store.UpdateLease(ctx, lease); err != nil {
		  return DestEntryLease{}, err
	   }
	   _ = s.openProof(ctx, DestEntryDriftProof{
		   Kind: DriftRevoked, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
		   UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
		   BindMode: lease.Mode, BindGen: lease.EntryGen, Evidence: lease.RevokeReason,
	  })
	   _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditRevokeOps, actor, lease.RevokeReason)
	  if s.metrics != nil {
		  s.metrics.IncRevoked("ops", 1)
	   }
	   return lease, nil
  }

// ExpireLeases TTL 扫描。
  func (s *Service) ExpireLeases(ctx context.Context) (int, error) {
	   if s == nil || s.store == nil {
		   return 0, ErrUnavailable
	   }
	  now := s.now()
	   rows,
		  err := s.store.ListExpiredIssued(ctx, now, ExpireBatchSize)
	   if err != nil {
		  return 0, err
	   }
	   n := 0
	  for _, lease := range rows {
		  if err := ApplyLeaseTransition(&lease, LeaseExpired, "ttl", now); err != nil {
			  continue
		   }
		  if err := s.store.UpdateLease(ctx, lease); err != nil {
			  return n, err
		  }
		   _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditExpire, "expire-worker", "ttl")
		  n++
	   }
	  if s.metrics != nil && n > 0 {
		   s.metrics.IncExpired(n)
	   }
	   return n, nil
  }

// ScanTempExpire 扫描 destallow 过期 Temp 并吊销。

   func (s *Service) ScanTempExpire(ctx context.Context) (int, error) {
	  if s == nil || s.surface == nil {
		   return 0, ErrUnavailable
	  }
	   rows, err := s.surface.ListExpiredTemp(ctx, s.now(), ExpireBatchSize)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	   for _, e := range rows {
		  k, err := s.OnTempExpired(ctx, e, "temp-expire-scan")
		   if err != nil {
			   return n, err
		  }
		   n += k
	   }
	   return n, nil
  }

// ScanBreakglassReclaim 扫描失效破窗。
   func (s *Service) ScanBreakglassReclaim(ctx context.Context) (int, error) {
	  if s == nil || s.surface == nil {
		  return 0, ErrUnavailable
	   }
	  rows,
		   err := s.surface.ListExpiredGlass(ctx, s.now(), ExpireBatchSize)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	   for _, e := range rows {
		  k, err := s.OnBreakglassReclaim(ctx, e, "glass-reclaim-scan")
		  if err != nil {
			  return n, err
		   }
		  n += k
	  }
	  return n, nil
   }

// PatrolUnconsumed 巡检未消费租约：活面漂移则吊销并落证明。

  func (s *Service) PatrolUnconsumed(ctx context.Context) (int, error) {
	  if s == nil || s.store == nil || s.surface == nil {
		  return 0, ErrUnavailable
	   }
	   rows,
		  err := s.store.ListUnconsumed(ctx, PatrolBatchSize)
	   if err != nil {
		   return 0, err
	  }
	   n := 0
	   now := s.now()
	  for _, lease := range rows {
		  live, err := s.surface.Snapshot(ctx, lease.UserID, lease.Chain, lease.Address)
		  if err != nil {
			  continue
		  }
		   g, gerr := s.store.GetGen(ctx, lease.EntryID)
		  drift := !live.Live || (gerr == nil && g.Gen != lease.EntryGen)
		  if !drift {
			   continue
		   }
		   if err := ApplyLeaseTransition(&lease, LeaseRevoked, "patrol", now); err != nil {
			  continue
		   }
		  if err := s.store.UpdateLease(ctx, lease); err != nil {
			   return n, err
		   }
		   liveGen := lease.EntryGen
		  if gerr == nil {
			   liveGen = g.Gen
		   }
		   _ = s.openProof(ctx, DestEntryDriftProof{
			  Kind: DriftPatrol, LeaseID: lease.LeaseID, EntryID: lease.EntryID,
			  UserID: lease.UserID, Chain: lease.Chain, Address: lease.Address,
			  BindMode: lease.Mode,
			  LiveMode: live.Mode, BindGen: lease.EntryGen, LiveGen: liveGen,
			  Evidence: firstNonEmpty(live.Reason, "patrol drift"),
		   })
		   _ = s.audit(ctx, lease.LeaseID, lease.EntryID, AuditPatrol, "patrol", live.Reason)
		  n++
	  }
	   if s.metrics != nil && n > 0 {
		   s.metrics.IncPatrol(n)
	  }
	   return n, nil
  }

// ExportProofs 导出未导出证明。
  func (s *Service) ExportProofs(ctx context.Context) ([]DestEntryDriftProof, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	  rows, err := s.store.ListProofs(ctx, ExportProofFilter{UnexportedOnly: true, Limit: ProofExportBatch})
	   if err != nil {
		  return nil, err
	  }
	   ids := make([]string, 0, len(rows))
	   for _, p := range rows {
		   ids = append(ids, p.ProofID)
	  }
	  if len(ids) > 0 {
		  if err := s.store.MarkProofsExported(ctx, ids, s.now()); err != nil {
			   return nil, err
		   }
	   }
	   _ = s.audit(ctx, "", "", AuditExport, "export-worker", fmt.Sprintf("n=%d", len(rows)))
	   return rows, nil
   }

// GetLease 读租约。

   func (s *Service) GetLease(ctx context.Context, id string) (DestEntryLease, error) {
	   if s == nil || s.store == nil {
		   return DestEntryLease{}, ErrUnavailable
	   }
	  return s.store.GetLease(ctx, id)
   }

// GetProof 读证明。

   func (s *Service) GetProof(ctx context.Context, id string) (DestEntryDriftProof, error) {
	  if s == nil || s.store == nil {
		  return DestEntryDriftProof{}, ErrUnavailable
	   }
	   return s.store.GetProof(ctx, id)
  }

// ListAudit 列审计。

   func (s *Service) ListAudit(ctx context.Context, leaseID string, limit int) ([]DestEntryAudit, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	   return s.store.ListAuditByLease(ctx, leaseID, limit)
  }

// ListProofs 过滤证明。

  func (s *Service) ListProofs(ctx context.Context, f ExportProofFilter) ([]DestEntryDriftProof, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.ListProofs(ctx, f)
  }

  func (s *Service) openProof(ctx context.Context, p DestEntryDriftProof) error {
	   if s == nil || s.store == nil {
		   return ErrUnavailable
	   }
	  now := s.now()
	  if p.ProofID == "" {
		  p.ProofID = uuid.New().String()
	  }
	  if p.DetectedAt.IsZero() {
		  p.DetectedAt = now
	  }
	   p.CreatedAt = now
	   if p.Evidence == "" {
		  p.Evidence = string(p.Kind)
	  }
	  if err := s.store.SaveProof(ctx, p); err != nil {
		  return err
	  }
	  if s.metrics != nil {
		   s.metrics.IncProof(string(p.Kind))
	  }
	  _ = s.audit(ctx, p.LeaseID, p.EntryID, AuditProof, "system", string(p.Kind))
	   return nil
  }

  func (s *Service) audit(ctx context.Context, leaseID, entryID string, act AuditAction, actor, detail string) error {
	  if s == nil || s.store == nil {
		  return ErrUnavailable
	   }
	  return s.store.SaveAudit(ctx, DestEntryAudit{
		  AuditID: uuid.New().String(),
		  LeaseID: leaseID,
		  EntryID: entryID,
		  Action:  act,
		   Actor:  firstNonEmpty(actor, "system"),
		   Detail: detail,
		   CreatedAt: s.now(),
	  })
   }

   func proofKindFromAssert(err error) ProofKind {
	   switch {
	   case errors.Is(err, ErrLeaseExpired):
		   return DriftExpired
	   case errors.Is(err, ErrLeaseRevoked):
		   return DriftRevoked
	  case errors.Is(err,
		  ErrLeaseConsumed):
		   return DriftConsumed
	   default:
		  return DriftNoLease
	  }
  }

   func firstNonEmpty(vs ...string) string {
	  for _, v := range vs {
		  if strings.TrimSpace(v) != "" {
			   return v
		  }
	  }
	  return ""
  }

// ProofJSON 证明序列化（导出）。
  func ProofJSON(p DestEntryDriftProof) ([]byte, error) {
	   return json.Marshal(p)
  }
