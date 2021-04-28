// 不改写 dustleak 超额公式、不改写 cvfund Quote→Execute、不改写 feeeng 档位租约。
// 不变量：无租约不得落账；漂移必落 DustQuoteDriftProof；Conserved 失败不得部分借记。

// Author: Hux
// Date: 2021.04.20

package dustquotegate

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Engine 尘兑牌价租约引擎。
type Engine struct {
	store Store
	src    QuoteSource
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

// WithConfig 租约 TTL/cap。
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
   func NewEngine(store Store, src QuoteSource, opts ...EngineOption) *Engine {
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

// QuoteWithLease Convert 预检签发 dust_quote_lease_id。
   func (e *Engine) QuoteWithLease(ctx context.Context, req QuoteReq) (DustQuoteLease, error) {
	   if err := e.ok(); err != nil {
		   return DustQuoteLease{}, err
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return DustQuoteLease{}, ErrUser
	  }
	   as,
		   err := validate.Asset(req.Asset)
	  if err != nil {
		   return DustQuoteLease{}, ErrAsset
	  }
	   if as == "USDT" {
		   return DustQuoteLease{}, ErrUSDT
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil || !amt.IsPositive() {
		   return DustQuoteLease{}, ErrAmount
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		   return DustQuoteLease{},
			   ErrActor
	   }
	  idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return DustQuoteLease{}, ErrIdem
	   }
	   if cur, err := e.store.GetLeaseByIdem(ctx, idem); err == nil {
		   if !SameIdemPayload(cur, DustQuoteLease{UserID: uid, Asset: as, Amount: amt.String()}) {
			   return DustQuoteLease{}, ErrIdem
		   }
		   return cur, nil
	   }
	  if e.src == nil {
		  return DustQuoteLease{}, ErrQuoteSnapshot
	   }
	   live, err := e.src.LiveQuote(uid, as)
	  if err != nil {
		   return DustQuoteLease{}, ErrQuoteSnapshot
	   }
	  px,
		  err := money.Parse(live.Rate)
	  if err != nil || !px.IsPositive() {
		   return DustQuoteLease{}, ErrQuoteSnapshot
	   }
	   usdt := money.FromDecimal(amt.Dec().Mul(px.Dec()).Round(18))
	   if !usdt.IsPositive() {
		   return DustQuoteLease{},
			  ErrAmount
	   }
	   if err := money.Conserve(usdt, money.FromDecimal(amt.Dec().Mul(px.Dec()).Round(18))); err != nil {
		   return DustQuoteLease{}, ErrConserve
	   }
	  ttl := req.TTLSec
	   if ttl <= 0 {
		  ttl = int64(e.cfg.DefaultTTL / time.Second)
	  }
	  if time.Duration(ttl)*time.Second > e.cfg.MaxTTL {
		  return DustQuoteLease{}, ErrTTL
	   }
	  fp := live.ToFingerprint(uid)
	   now := e.now()
	  epoch, _ := e.store.CurrentFlipEpoch(ctx, as)
	  row := DustQuoteLease{
		   LeaseID: uuid.NewString(), UserID: uid, Asset: as, Amount: amt.String(),
		  USDTWant: usdt.String(), Rate: live.Rate, PolicyGen: live.PolicyGen, CapBPS: live.CapBPS,
		  BookFP: fp.BookFP, Digest: fp.Digest, State: LeaseQuoted, TTLSec: ttl,
		   Idem:  idem,
		  Actor: actor, ScanBatch: strings.TrimSpace(req.ScanBatch), ScanIndex: req.ScanIndex,
		   QuotedAt: now, FlipEpoch: epoch, Fingerprint: fp, CreatedAt: now, UpdatedAt: now,
	  }
	   if err := e.store.SaveLease(ctx, row); err != nil {
		  return DustQuoteLease{}, err
	   }
	  _ = e.store.AppendAudit(ctx, DustQuoteAudit{
		  AuditID: uuid.NewString(), LeaseID: row.LeaseID, Action: "quote",
		   From: "", To: string(LeaseQuoted), Actor: actor, Detail: "quote_with_lease", At: now,
	  })
	  if e.metric != nil {
		  e.metric.IncQuoted()
	  }
	  return row.Copy(), nil
  }

// AssertLease Convert 前复验 Rate+PolicyGen，quoted→bound。

  func (e *Engine) AssertLease(ctx context.Context, req BindReq) (DustQuoteLease, error) {
	  if err := e.ok(); err != nil {
		   return DustQuoteLease{}, err
	   }
	  if strings.TrimSpace(req.LeaseID) == "" {
		   _ = e.dropProof(ctx, DustQuoteDriftProof{
			  ProofID: uuid.NewString(),
			   Kind: ProofNoLease, UserID: req.UserID, Asset: req.Asset,
			  Detail: "convert requires dust_quote_lease_id", Actor: req.Actor, At: e.now(), CreatedAt: e.now(),
		  })
		  return DustQuoteLease{}, ErrNoLease
	   }
	  cur, err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return DustQuoteLease{}, err
	  }
	  if err := e.guardOpen(ctx, cur, req); err != nil {
		   return DustQuoteLease{}, err
	  }
	   if e.src == nil {
		  return DustQuoteLease{}, ErrQuoteSnapshot
	  }
	  live, err := e.src.LiveQuote(cur.UserID, cur.Asset)
	   if err != nil {
		   return DustQuoteLease{}, ErrQuoteSnapshot
	   }
	   liveFP := live.ToFingerprint(cur.UserID)
	   if err := CompareLive(cur.Fingerprint, liveFP); err != nil {
		   _ = e.dropProof(ctx,
			  e.proofFrom(cur, liveFP, err, req.Actor))
		  if e.metric != nil {
			  e.metric.IncDrift()
		   }
		   return DustQuoteLease{}, err
	   }
	   now := e.now()
	   if err := e.store.CASState(ctx, cur.LeaseID, LeaseQuoted, LeaseBound, DustQuoteLease{BoundAt: now, UpdatedAt: now}); err != nil {
		   if cur.State == LeaseBound {
			  return DustQuoteLease{}, ErrDupBind
		   }
		  return DustQuoteLease{}, err
	   }
	   cur.State = LeaseBound
	  cur.BoundAt = now
	  cur.UpdatedAt = now
	  _ = e.store.AppendAudit(ctx, DustQuoteAudit{
		  AuditID: uuid.NewString(), LeaseID: cur.LeaseID, Action: "bind",
		   From: string(LeaseQuoted), To: string(LeaseBound), Actor: req.Actor, Detail: "assert_lease", At: now,
	  })
	   _ = e.dropProof(ctx, DustQuoteDriftProof{
		   ProofID: uuid.NewString(),
		  LeaseID: cur.LeaseID, UserID: cur.UserID, Asset: cur.Asset,
		  Kind: ProofBindOK, QuotedRate: cur.Rate, LiveRate: live.Rate, QuotedGen: cur.PolicyGen, LiveGen: live.PolicyGen,
		   QuotedBook: cur.BookFP, LiveBook: liveFP.BookFP, QuotedCap: cur.CapBPS, LiveCap: live.CapBPS,
		  Detail: "bound", Actor: req.Actor, At: now, CreatedAt: now,
	   })
	   if e.metric != nil {
		  e.metric.IncBound()
	   }
	  return cur.Copy(), nil
  }

  func (e *Engine) guardOpen(ctx context.Context, cur DustQuoteLease, req BindReq) error {
	   now := e.now()
	   if cur.State == LeaseSettled {
		   return ErrLeaseSettled
	   }
	  if cur.State == LeaseRevoked {
		  _ = e.dropProof(ctx, e.proofKind(cur, ProofRevoked, req.Actor, "revoked"))
		  return ErrLeaseRevoked
	   }
	   if cur.State == LeaseExpired || now.After(cur.QuotedAt.Add(time.Duration(cur.TTLSec)*time.Second)) {
		  _ = e.store.CASState(ctx, cur.LeaseID, cur.State, LeaseExpired, DustQuoteLease{ExpiredAt: now, UpdatedAt: now})
		  _ = e.dropProof(ctx, e.proofKind(cur, ProofTTLExpired, req.Actor, "ttl"))
		  return ErrLeaseExpired
	   }
	  if !cur.State.CanBind() && cur.State != LeaseBound {
		  return ErrLeaseNotQuoted
	   }
	   if cur.State == LeaseBound {
		  return ErrDupBind
	   }
	   if req.UserID != "" && cur.UserID != req.UserID {
		  return ErrLeaseMismatch
	   }
	  if req.Asset != "" && cur.Asset != req.Asset {
		  return ErrLeaseMismatch
	   }
	  if req.Amount != "" && cur.Amount != req.Amount {
		  return ErrLeaseMismatch
	   }
	  return nil
   }

// SettleLease Conserved 成功后 quoted/bound→settled。
   func (e *Engine) SettleLease(ctx context.Context, req SettleReq) (DustQuoteLease, error) {
	  if err := e.ok(); err != nil {
		   return DustQuoteLease{}, err
	  }
	  cur, err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		   return DustQuoteLease{}, err
	   }
	  if cur.State == LeaseSettled {
		   return cur, nil
	  }
	  if !cur.State.CanSettle() {
		   return DustQuoteLease{}, ErrLeaseNotBound
	  }
	  want, err := money.Parse(cur.USDTWant)
	   if err != nil {
		  return DustQuoteLease{}, ErrAmount
	  }
	  got, err := money.Parse(req.USDT)
	   if err != nil {
		  return DustQuoteLease{},
			   ErrAmount
	  }
	  if err := money.Conserve(got, want); err != nil {
		  _ = e.dropProof(ctx, e.proofKind(cur, ProofConserveFail, req.Actor, "conserve fail-closed"))
		   if e.metric != nil {
			   e.metric.IncConserveFail()
		   }
		  return DustQuoteLease{}, ErrConserve
	   }
	  now := e.now()
	   if err := e.store.CASState(ctx, cur.LeaseID, LeaseBound, LeaseSettled, DustQuoteLease{
		   SettledAt: now, UpdatedAt: now, ConvertID: req.ConvertID,
	   }); err != nil {
		  return DustQuoteLease{}, err
	  }
	   cur.State = LeaseSettled
	  cur.SettledAt = now
	  cur.ConvertID = req.ConvertID
	   _ = e.store.AppendAudit(ctx, DustQuoteAudit{
		   AuditID: uuid.NewString(), LeaseID: cur.LeaseID, Action: "settle",
		  From: string(LeaseBound), To: string(LeaseSettled), Actor: req.Actor, At: now,
	   })
	  _ = e.dropProof(ctx, DustQuoteDriftProof{
		   ProofID: uuid.NewString(), LeaseID: cur.LeaseID, UserID: cur.UserID, Asset: cur.Asset,
		  Kind: ProofSettleOK, QuotedRate: cur.Rate, LiveRate: cur.Rate, Detail: "settled",
		   Actor: req.Actor, At: now, CreatedAt: now,
	  })
	  if e.metric != nil {
		   e.metric.IncSettled()
	   }
	  return cur.Copy(), nil
   }

// FailClosedConserve Conserved 失败：不结清、不部分落账。

  func (e *Engine) FailClosedConserve(ctx context.Context, leaseID, actor, detail string) error {
	  if err := e.ok(); err != nil {
		  return err
	   }
	   cur, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return err
	   }
	   _ = e.dropProof(ctx,
		  e.proofKind(cur, ProofConserveFail, actor, detail))
	   if e.metric != nil {
		   e.metric.IncConserveFail()
	   }
	   return ErrConserve
  }

// RevokeLease 运维吊销 quoted/bound。

   func (e *Engine) RevokeLease(ctx context.Context, req RevokeReq) (DustQuoteLease, error) {
	  if err := e.ok(); err != nil {
		   return DustQuoteLease{}, err
	  }
	  if strings.TrimSpace(req.Actor) == "" {
		   return DustQuoteLease{}, ErrActor
	  }
	  cur, err := e.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		  return DustQuoteLease{}, err
	  }
	  if cur.State.Terminal() {
		   return cur, nil
	   }
	  now := e.now()
	   if err := e.store.CASState(ctx, cur.LeaseID, cur.State, LeaseRevoked, DustQuoteLease{
		  RevokedAt: now, UpdatedAt: now, RevokeWhy: req.Reason,
	  }); err != nil {
		   return DustQuoteLease{}, err
	   }
	  cur.State = LeaseRevoked
	  cur.RevokedAt = now
	  cur.RevokeWhy = req.Reason
	   _ = e.dropProof(ctx, e.proofKind(cur, ProofOpsRevoke, req.Actor, req.Reason))
	  _ = e.store.AppendAudit(ctx, DustQuoteAudit{
		  AuditID: uuid.NewString(), LeaseID: cur.LeaseID, Action: "revoke",
		  From: string(LeaseQuoted), To: string(LeaseRevoked), Actor: req.Actor, Detail: req.Reason, At: now,
	  })
	  if e.metric != nil {
		   e.metric.IncRevoked()
	  }
	  return cur.Copy(), nil
  }

// OnFlip PutRate/PutPolicyGen 后旧租约立即失效。

   func (e *Engine) OnFlip(ctx context.Context, ev FlipEvent) (int, error) {
	  if err := e.ok(); err != nil {
		  return 0, err
	   }
	   if ev.EventID == "" {
		  ev.EventID = uuid.NewString()
	  }
	  if ev.At.IsZero() {
		  ev.At = e.now()
	   }
	   epoch,
		  err := e.store.NextFlipEpoch(ctx, ev.Asset)
	  if err != nil {
		  return 0, err
	  }
	   ev.Epoch = epoch
	   if err := e.store.SaveFlip(ctx, ev); err != nil {
		  return 0, err
	   }
	   open, err := e.store.ListOpenByAsset(ctx, ev.Asset)
	  if err != nil {
		  return 0, err
	   }
	   n := 0
	   now := e.now()
	  why := string(ev.Kind)
	  for _, l := range open {
		  if err := e.store.CASState(ctx, l.LeaseID, l.State, LeaseRevoked, DustQuoteLease{
			   RevokedAt: now, UpdatedAt: now, RevokeWhy: why,
		  }); err != nil {
			  continue
		   }
		   kind := ProofRevoked
		  if ev.Kind == FlipRate {
			   kind = ProofRateDrift
		  }
		   if ev.Kind == FlipPolicyGen {
			   kind = ProofPolicyDrift
		   }
		   _ = e.dropProof(ctx, e.proofKind(l, kind, e.workerActor(), why))
		   n++
	   }
	  _ = e.store.MarkFlipDone(ctx, ev.EventID)
	   if e.metric != nil {
		   e.metric.AddRevoked(n)
	  }
	  return n, nil
   }

// ExpireOpen 过期扫描：quoted/bound 超 TTL → expired。
   func (e *Engine) ExpireOpen(ctx context.Context, limit int) (int, error) {
	  if err := e.ok(); err != nil {
		   return 0, err
	  }
	  now := e.now()
	   rows, err := e.store.ListQuotedOrBound(ctx, now, limit)
	   if err != nil {
		  return 0, err
	  }
	   n := 0
	  for _, l := range rows {
		   if !now.After(l.QuotedAt.Add(time.Duration(l.TTLSec) * time.Second)) {
			  continue
		  }
		  if err := e.store.CASState(ctx, l.LeaseID, l.State, LeaseExpired, DustQuoteLease{ExpiredAt: now, UpdatedAt: now}); err != nil {
			  continue
		  }
		   _ = e.dropProof(ctx, e.proofKind(l, ProofTTLExpired, e.workerActor(), "expire_scan"))
		  n++
	  }
	   if e.metric != nil {
		  e.metric.AddExpired(n)
	  }
	  return n, nil
  }

// HaltScan ConvertScan 中途翻价：已落笔保留，后续停。
   func (e *Engine) HaltScan(ctx context.Context, batch, user, actor, reason string, next, settled, rejected int) error {
	  if err := e.ok(); err != nil {
		   return err
	   }
	   if strings.TrimSpace(batch) == "" {
		   return ErrScanCursor
	   }
	  now := e.now()
	  cur := ScanCursor{
		   BatchID: batch, UserID: user, NextIndex: next, Halted: true, HaltReason: reason,
		   SettledN: settled, RejectedN: rejected, Actor: actor, UpdatedAt: now,
	   }
	  if ep, err := e.store.CurrentFlipEpoch(ctx, ""); err == nil {
		   cur.FlipEpoch = ep
	  }
	  if err := e.store.SaveCursor(ctx, cur); err != nil {
		  return err
	   }
	   _ = e.dropProof(ctx, DustQuoteDriftProof{
		   ProofID: uuid.NewString(), UserID: user, Kind: ProofScanHalt, Detail: reason,
		  Actor: actor, ScanBatch: batch, At: now, CreatedAt: now,
	   })
	  if e.metric != nil {
		   e.metric.IncScanHalt()
	  }
	   return ErrScanHalted
  }

// GetLease 只读。
   func (e *Engine) GetLease(ctx context.Context, id string) (DustQuoteLease, error) {
	   if err := e.ok(); err != nil {
		  return DustQuoteLease{}, err
	   }
	   return e.store.GetLease(ctx, id)
  }

// ExportProofs 证明导出（幂等读取）。
  func (e *Engine) ExportProofs(ctx context.Context, user string, limit int) ([]DustQuoteDriftProof, error) {
	   if err := e.ok(); err != nil {
		   return nil, err
	   }
	   return e.store.ListProofsByUser(ctx, user, limit)
  }

   func (e *Engine) dropProof(ctx context.Context, p DustQuoteDriftProof) error {
	  if p.ProofID == "" {
		  p.ProofID = uuid.NewString()
	  }
	   if p.At.IsZero() {
		  p.At = e.now()
	   }
	  if p.CreatedAt.IsZero() {
		  p.CreatedAt = p.At
	  }
	  return e.store.SaveProof(ctx, p)
   }

   func (e *Engine) proofFrom(cur DustQuoteLease, live PolicyRateFingerprint, err error, actor string) DustQuoteDriftProof {
	   return DustQuoteDriftProof{
		  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, UserID: cur.UserID, Asset: cur.Asset,
		  Kind:       ProofKindOf(err),
		   QuotedRate: cur.Rate, LiveRate: live.Rate,
		   QuotedGen: cur.PolicyGen, LiveGen: live.PolicyGen, QuotedBook: cur.BookFP, LiveBook: live.BookFP,
		   QuotedCap: cur.CapBPS, LiveCap: live.CapBPS, Detail: err.Error(), Actor: actor,
		  ScanBatch: cur.ScanBatch, At: e.now(), CreatedAt: e.now(),
	  }
  }

   func (e *Engine) proofKind(cur DustQuoteLease, kind ProofKind, actor, detail string) DustQuoteDriftProof {
	   return DustQuoteDriftProof{
		  ProofID: uuid.NewString(), LeaseID: cur.LeaseID, UserID: cur.UserID, Asset: cur.Asset,
		  Kind: kind, QuotedRate: cur.Rate, LiveRate: cur.Rate, QuotedGen: cur.PolicyGen, LiveGen: cur.PolicyGen,
		   QuotedBook: cur.BookFP, LiveBook: cur.BookFP, QuotedCap: cur.CapBPS, LiveCap: cur.CapBPS,
		   Detail: detail, Actor: actor, ScanBatch: cur.ScanBatch, At: e.now(), CreatedAt: e.now(),
	   }
   }

  func (e *Engine) workerActor() string { return "dust-quote-gate-worker" }

// Store 暴露仓（HTTP 查询）。
  func (e *Engine) Store() Store { return e.store }
