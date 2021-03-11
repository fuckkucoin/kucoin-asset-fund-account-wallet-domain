// 功能：做什么：签发 decide_lease、Exec 前 AssertLease、消费、翻转吊销。
// 不改写 cerbind 胜出排序与出厂底档；不改写仪式批次绑定、HSM、signpipe。
// 不变量：Required=false 不得签发强制租约；无租约 Exec fail-closed；漂移必落 CerDriftProof。

// 作者：Cason
// 日期：2021.03.10

package cerdecidegate

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// Engine Decide→Exec 租约引擎。
type Engine struct {
	store  Store
	src    LiveSource
	cfg    LeaseConfig
	clock  func() time.Time
	metric *Metrics
}

// EngineOption 注入时钟/配置/指标。

type EngineOption func(*Engine)

// WithClock 时钟。
  func WithClock(fn func() time.Time) EngineOption {
	   return func(e *Engine) {
		  if fn != nil {
			   e.clock = fn
		  }
	  }
}

// WithConfig 租约 TTL。

   func WithConfig(cfg LeaseConfig) EngineOption {
	   return func(e *Engine) {
		  cfg.Fill()
		   e.cfg = cfg
	  }
  }

// WithMetrics 挂指标。
  func WithMetrics(m *Metrics) EngineOption {
	   return func(e *Engine) { e.metric = m }
  }

// NewEngine 构造。

   func NewEngine(store Store, src LiveSource, opts ...EngineOption) *Engine {
	   e := &Engine{
		  store: store,
		  src:   src,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
	   e.cfg.Fill()
	   for _, o := range opts {
		  if o != nil {
			  o(e)
		  }
	  }
	  return e
   }

// Store 暴露仓（进程装配）。

   func (e *Engine) Store() Store {
	   if e == nil {
		  return nil
	  }
	  return e.store
  }

   func (e *Engine) now() time.Time {
	  if e == nil || e.clock == nil {
		   return time.Now().UTC()
	   }
	  return e.clock().UTC()
   }

   func (e *Engine) ok() error {
	   if e == nil || e.store == nil {
		   return ErrUnavailable
	  }
	  return nil
  }

// DecideWithLease Required=true 时签发 decide_lease_id。
// 豁免决策返回空租约且不写强制凭证。

   func (e *Engine) DecideWithLease(ctx context.Context, req DecideReq) (DecideLease, error) {
	   if err := e.ok(); err != nil {
		  return DecideLease{}, err
	   }
	   actor, err := validate.Actor(req.Actor)
	   if err != nil {
		  return DecideLease{}, ErrActor
	  }
	   idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return DecideLease{}, ErrIdem
	  }
	  ln, err := normalizeLine(req.Line)
	   if err != nil {
		   return DecideLease{}, err
	  }
	   if cur, err := e.store.GetLeaseByIdem(ctx, idem); err == nil {
		   want := DecideLease{
			   BatchID: ln.BatchID, Asset: ln.Asset, Chain: ln.Chain, Amount: ln.Amount,
			   Dest:     ln.Dest,
			  PolicyID: req.Pin.PolicyID, Threshold: req.Pin.Threshold, Shares: req.Pin.Shares,
		   }
		   if !SameIdemPayload(cur, want) {
			  return DecideLease{}, ErrIdem
		  }
		  return cur, nil
	  }
	   if !req.Pin.Required {
		   return DecideLease{}, ErrPinExempt
	   }
	  if req.Pin.Threshold < 2 {
		  return DecideLease{}, ErrPinRequired
	  }
	   if req.Pin.Shares < req.Pin.Threshold {
		  req.Pin.Shares = req.Pin.Threshold
	   }
	  ttl := req.TTLSec
	  if ttl <= 0 {
		   ttl = int64(e.cfg.DefaultTTL / time.Second)
	  }
	   if time.Duration(ttl)*time.Second > e.cfg.MaxTTL {
		  return DecideLease{}, ErrTTL
	   }
	   scope := ScopeKey(ln)
	   gen, err := e.store.CurrentGeneration(ctx, scope)
	   if err != nil {
		   return DecideLease{}, err
	   }
	  fp := ComposeFingerprint(req.Pin, ln, req.PackDigest, req.SnapDigest, gen)
	  now := e.now()
	  row := DecideLease{
		  LeaseID: uuid.NewString(), BatchID: ln.BatchID, Asset: ln.Asset, Chain: ln.Chain,
		   Amount: ln.Amount,
		   Dest: ln.Dest, DestRisk: firstRisk(req.Pin.DestRisk, ln.DestRisk),
		   Side: ln.Side, Purpose: ln.Purpose, PolicyID: req.Pin.PolicyID,
		  Threshold: req.Pin.Threshold, Shares: req.Pin.Shares, BindMode: req.Pin.BindMode,
		  Required: true, State: LeaseIssued, TTLSec: ttl, Idem: idem, Actor: actor,
		  Generation: gen, FlipEpoch: gen, IssuedAt: now, Fingerprint: fp,
		  CreatedAt: now, UpdatedAt: now,
	   }
	   if err := e.store.SaveLease(ctx, row); err != nil {
		   return DecideLease{}, err
	   }
	  _ = e.store.AppendAudit(ctx, DecideAudit{
		   AuditID: uuid.NewString(), LeaseID: row.LeaseID, BatchID: row.BatchID,
		   Action: "issue", From: "", To: string(LeaseIssued), Actor: actor,
		  Detail: "decide_with_lease", At: now,
	  })
	   if e.metric != nil {
		   e.metric.IncIssued()
	  }
	  return row.Copy(), nil
  }

// AssertLease 冷签 Exec / 放行提交前复验。

  func (e *Engine) AssertLease(ctx context.Context, req BindReq) (DecideLease, error) {
	  if err := e.ok(); err != nil {
		   return DecideLease{}, err
	  }
	  if strings.TrimSpace(req.LeaseID) == "" {
		   _ = e.dropProof(ctx, CerDriftProof{
			   ProofID: uuid.NewString(), Kind: ProofNoLease, BatchID: req.Line.BatchID,
			   Detail: "exec requires decide_lease_id", Actor: req.Actor, At: e.now(), CreatedAt: e.now(),
		   })
		   if e.metric != nil {
			  e.metric.IncFailClosed()
		  }
		  return DecideLease{}, ErrNoLease
	  }
	  cur, err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return DecideLease{}, err
	   }
	  if err := e.guardOpen(ctx, cur, req); err != nil {
		  return DecideLease{}, err
	   }
	   if e.src == nil {
		   return DecideLease{}, ErrLiveUnavailable
	  }
	  live, err := e.src.LiveDecide(lineFromLease(cur, req.Line))
	   if err != nil {
		   return DecideLease{}, ErrLiveUnavailable
	   }
	   if err := CompareLive(cur.Fingerprint, live.Fingerprint); err != nil {
		  _ = e.dropProof(ctx, e.proofFrom(cur, live, err, req.Actor))
		  if e.metric != nil {
			  e.metric.IncDrift()
		   }
		  return DecideLease{}, err
	  }
	   now := e.now()
	   patch := DecideLease{BoundAt: now, UpdatedAt: now, ExecRef: strings.TrimSpace(req.ExecRef)}
	   if err := e.store.CASState(ctx, cur.LeaseID, LeaseIssued, LeaseBound, patch); err != nil {
		   if cur.State == LeaseBound {
			   return DecideLease{}, ErrDupBind
		  }
		  return DecideLease{}, err
	  }
	  _ = e.store.AppendAudit(ctx, DecideAudit{
		  AuditID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		  Action: "bind", From: string(LeaseIssued), To: string(LeaseBound),
		   Actor: req.Actor, Detail: "assert_lease", At: now,
	  })
	   _ = e.dropProof(ctx, CerDriftProof{
		  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		   Kind: ProofBindOK, QuotedPolicy: cur.PolicyID, LivePolicy: live.Pin.PolicyID,
		  QuotedThresh: cur.Threshold, LiveThresh: live.Pin.Threshold,
		   QuotedShares: cur.Shares, LiveShares: live.Pin.Shares,
		   QuotedRisk: cur.DestRisk, LiveRisk: live.DestRisk,
		  QuotedForce: cur.Required, LiveForce: live.ForceCold,
		  QuotedGen:    cur.Generation,
		   LiveGen:      live.Generation,
		  QuotedDigest: cur.Fingerprint.Digest, LiveDigest: live.Fingerprint.Digest,
		  Detail: "assert_ok", Actor: req.Actor, At: now, CreatedAt: now,
	   })
	   if e.metric != nil {
		  e.metric.IncBound()
	   }
	  cur.State = LeaseBound
	   cur.BoundAt = now
	  cur.UpdatedAt = now
	   cur.ExecRef = patch.ExecRef
	   return cur.Copy(), nil
   }

// ConsumeLease Exec 成功后 issued/bound→consumed。仅 bound 可消费。
  func (e *Engine) ConsumeLease(ctx context.Context, req ConsumeReq) (DecideLease, error) {
	  if err := e.ok(); err != nil {
		  return DecideLease{}, err
	   }
	  cur,
		  err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return DecideLease{}, err
	  }
	  if cur.State != LeaseBound {
		   return DecideLease{}, ErrLeaseNotBound
	  }
	   now := e.now()
	  if err := e.store.CASState(ctx, cur.LeaseID, LeaseBound, LeaseConsumed, DecideLease{
		   ConsumedAt: now, UpdatedAt: now, ExecRef: strings.TrimSpace(req.ExecRef),
	  }); err != nil {
		   return DecideLease{}, err
	   }
	  _ = e.store.AppendAudit(ctx, DecideAudit{
		  AuditID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		  Action: "consume", From: string(LeaseBound), To: string(LeaseConsumed),
		   Actor: req.Actor, Detail: "exec_ok", At: now,
	   })
	   if e.metric != nil {
		  e.metric.IncConsumed()
	  }
	   cur.State = LeaseConsumed
	  cur.ConsumedAt = now
	  cur.UpdatedAt = now
	  return cur.Copy(), nil
   }

// RevokeLease 运维吊销。
  func (e *Engine) RevokeLease(ctx context.Context, req RevokeReq) (DecideLease, error) {
	  if err := e.ok(); err != nil {
		  return DecideLease{}, err
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		   return DecideLease{}, ErrActor
	   }
	   cur, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return DecideLease{}, err
	   }
	  if !cur.State.Open() {
		  return DecideLease{}, NextError(cur.State, LeaseRevoked)
	   }
	  why := strings.TrimSpace(req.Why)
	   if why == "" {
		  why = "ops_revoke"
	  }
	  now := e.now()
	   if err := e.store.CASState(ctx, cur.LeaseID, cur.State, LeaseRevoked, DecideLease{
		  RevokedAt: now, UpdatedAt: now, RevokeWhy: why,
	  }); err != nil {
		  return DecideLease{}, err
	  }
	  _ = e.store.AppendAudit(ctx, DecideAudit{
		  AuditID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		  Action: "revoke", From: string(cur.State), To: string(LeaseRevoked),
		   Actor:  actor,
		   Detail: why, At: now,
	   })
	   _ = e.dropProof(ctx, CerDriftProof{
		  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		   Kind: ProofOpsRevoke, QuotedPolicy: cur.PolicyID, Detail: why,
		   Actor: actor, At: now, CreatedAt: now,
	  })
	  if e.metric != nil {
		  e.metric.IncRevoked()
	  }
	  cur.State = LeaseRevoked
	  cur.RevokedAt = now
	  cur.RevokeWhy = why
	  return cur.Copy(), nil
   }

// ExpireOpen 过期扫描。
   func (e *Engine) ExpireOpen(ctx context.Context, limit int) (int, error) {
	  if err := e.ok(); err != nil {
		  return 0, err
	   }
	  now := e.now()
	  rows, err := e.store.ListIssuedOrBound(ctx, now, limit)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, row := range rows {
		   if !row.State.Open() {
			  continue
		  }
		   dead := row.IssuedAt.Add(time.Duration(row.TTLSec) * time.Second)
		   if now.Before(dead) {
			  continue
		   }
		  if err := e.store.CASState(ctx, row.LeaseID, row.State, LeaseExpired, DecideLease{
			   ExpiredAt: now, UpdatedAt: now, RevokeWhy: "ttl",
		  }); err != nil {
			  continue
		  }
		  _ = e.dropProof(ctx, CerDriftProof{
			   ProofID: uuid.NewString(), LeaseID: row.LeaseID, BatchID: row.BatchID,
			  Kind: ProofTTLExpired, QuotedPolicy: row.PolicyID, Detail: "ttl_expired",
			   Actor: "cer-decide-timer", At: now, CreatedAt: now,
		  })
		   n++
	   }
	   if e.metric != nil {
		   e.metric.AddExpired(n)
	  }
	  return n, nil
   }

  func (e *Engine) guardOpen(ctx context.Context, cur DecideLease, req BindReq) error {
	  if cur.State == LeaseConsumed {
		  return ErrLeaseConsumed
	  }
	   if cur.State == LeaseRevoked {
		  _ = e.dropProof(ctx, CerDriftProof{
			   ProofID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
			   Kind: ProofRevoked,
			  Detail: cur.RevokeWhy, Actor: req.Actor, At: e.now(), CreatedAt: e.now(),
		   })
		   return ErrLeaseRevoked
	  }
	  if cur.State == LeaseExpired {
		   return ErrLeaseExpired
	   }
	  if cur.State == LeaseBound {
		   return ErrDupBind
	   }
	   if cur.State != LeaseIssued {
		  return ErrLeaseNotIssued
	  }
	  if !cur.Required {
		  _ = e.dropProof(ctx, CerDriftProof{
			  ProofID: uuid.NewString(),
			   LeaseID: cur.LeaseID, BatchID: cur.BatchID,
			  Kind: ProofExemptMisuse, Detail: "exempt lease cannot enter exec", Actor: req.Actor,
			   At: e.now(), CreatedAt: e.now(),
		   })
		  return ErrExemptMisuse
	  }
	  dead := cur.IssuedAt.Add(time.Duration(cur.TTLSec) * time.Second)
	  if !e.now().Before(dead) {
		  _ = e.store.CASState(ctx, cur.LeaseID, cur.State, LeaseExpired, DecideLease{
			   ExpiredAt: e.now(), UpdatedAt: e.now(), RevokeWhy: "ttl",
		  })
		   _ = e.dropProof(ctx, CerDriftProof{
			  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
			   Kind: ProofTTLExpired, Detail: "ttl_expired", Actor: req.Actor, At: e.now(), CreatedAt: e.now(),
		  })
		  return ErrLeaseExpired
	   }
	   if req.Line.BatchID != "" && cur.BatchID != "" && req.Line.BatchID != cur.BatchID {
		  return ErrFieldDrift
	  }
	   return nil
   }

   func (e *Engine) dropProof(ctx context.Context, p CerDriftProof) error {
	   if e == nil || e.store == nil {
		   return ErrUnavailable
	   }
	  if p.ProofID == "" {
		  p.ProofID = uuid.NewString()
	  }
	   if p.CreatedAt.IsZero() {
		  p.CreatedAt = e.now()
	  }
	   return e.store.SaveProof(ctx, p)
  }

  func (e *Engine) proofFrom(cur DecideLease, live LiveView, cause error, actor string) CerDriftProof {
	  now := e.now()
	  return CerDriftProof{
		  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, BatchID: cur.BatchID,
		   Kind: ProofKindOf(cause), QuotedPolicy: cur.PolicyID, LivePolicy: live.Pin.PolicyID,
		   QuotedThresh: cur.Threshold, LiveThresh: live.Pin.Threshold,
		  QuotedShares: cur.Shares, LiveShares: live.Pin.Shares,
		  QuotedRisk: cur.DestRisk, LiveRisk: live.DestRisk,
		   QuotedForce: cur.Required, LiveForce: live.ForceCold,
		   QuotedGen: cur.Generation,
		  LiveGen: live.Generation,
		  QuotedDigest: cur.Fingerprint.Digest, LiveDigest: live.Fingerprint.Digest,
		  Detail: cause.Error(), Actor: actor, At: now, CreatedAt: now,
	   }
  }

   func normalizeLine(ln LineFact) (LineFact, error) {
	   asset, err := validate.Asset(ln.Asset)
	  if err != nil {
		  return LineFact{}, ErrAsset
	  }
	   chain, err := validate.Chain(ln.Chain)
	  if err != nil {
		   return LineFact{}, ErrBatch
	   }
	  amt, err := money.Parse(strings.TrimSpace(ln.Amount))
	  if err != nil || !amt.IsPositive() {
		  return LineFact{}, ErrAmount
	  }
	  dest := strings.TrimSpace(ln.Dest)
	   if dest == "" || len(dest) > 191 {
		  return LineFact{}, ErrDest
	   }
	   batch := strings.TrimSpace(ln.BatchID)
	  if batch == "" {
		   return LineFact{}, ErrBatch
	  }
	   ln.Asset, ln.Chain, ln.Amount = asset, chain, amt.String()
	  ln.Dest = dest
	  ln.BatchID = batch
	  ln.DestRisk = strings.ToLower(strings.TrimSpace(ln.DestRisk))
	  ln.Side = strings.ToLower(strings.TrimSpace(ln.Side))
	  ln.Purpose = strings.ToLower(strings.TrimSpace(ln.Purpose))
	   return ln, nil
   }

  func firstRisk(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return strings.ToLower(strings.TrimSpace(a))
	   }
	   return strings.ToLower(strings.TrimSpace(b))
  }

   func lineFromLease(cur DecideLease, overlay LineFact) LineFact {
	   ln := LineFact{
		   BatchID: cur.BatchID, Asset: cur.Asset, Chain: cur.Chain, Amount: cur.Amount,
		   Dest: cur.Dest,
		   DestRisk: cur.DestRisk, Side: cur.Side, Purpose: cur.Purpose,
	   }
	   if overlay.DestRisk != "" {
		   ln.DestRisk = overlay.DestRisk
	   }
	  if overlay.BatchID != "" {
		  ln.BatchID = overlay.BatchID
	  }
	   return ln
   }

// GetLease 只读。
   func (e *Engine) GetLease(ctx context.Context, id string) (DecideLease, error) {
	  if err := e.ok(); err != nil {
		   return DecideLease{}, err
	  }
	   return e.store.GetLease(ctx, id)
  }

// ListProofs 按租约导出证明。
   func (e *Engine) ListProofs(ctx context.Context, leaseID string) ([]CerDriftProof, error) {
	  if err := e.ok(); err != nil {
		  return nil, err
	   }
	  return e.store.ListProofsByLease(ctx, leaseID)
  }

// ExportProofs 按批次导出。
  func (e *Engine) ExportProofs(ctx context.Context, batchID string, limit int) ([]CerDriftProof, error) {
	   if err := e.ok(); err != nil {
		  return nil, err
	  }
	  return e.store.ListProofsByBatch(ctx, batchID, limit)
  }
